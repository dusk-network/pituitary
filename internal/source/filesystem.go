package source

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
)

// LoadResult contains canonical records emitted by configured source adapters.
type LoadResult struct {
	Specs []model.SpecRecord
	Docs  []model.DocRecord
}

// LoadFromConfig loads and normalizes all configured filesystem sources.
func LoadFromConfig(cfg *config.Config) (*LoadResult, error) {
	result := &LoadResult{}
	seenSpecs := make(map[string]artifactOrigin)
	seenDocs := make(map[string]artifactOrigin)

	for _, source := range cfg.Sources {
		switch {
		case source.Adapter != config.AdapterFilesystem:
			return nil, fmt.Errorf("source %q: unsupported adapter %q", source.Name, source.Adapter)
		case source.Kind == config.SourceKindSpecBundle:
			specs, err := loadSpecBundles(cfg.Workspace.RootPath, source)
			if err != nil {
				return nil, err
			}
			if err := appendUniqueSpecRecords(result, seenSpecs, source, specs); err != nil {
				return nil, err
			}
		case source.Kind == config.SourceKindMarkdownDocs:
			docs, err := loadMarkdownDocs(cfg.Workspace.RootPath, source)
			if err != nil {
				return nil, err
			}
			if err := appendUniqueDocRecords(result, seenDocs, source, docs); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("source %q: unsupported kind %q", source.Name, source.Kind)
		}
	}

	sort.Slice(result.Specs, func(i, j int) bool {
		return result.Specs[i].Ref < result.Specs[j].Ref
	})
	sort.Slice(result.Docs, func(i, j int) bool {
		return result.Docs[i].Ref < result.Docs[j].Ref
	})

	return result, nil
}

type artifactOrigin struct {
	sourceName string
	sourcePath string
	itemPath   string
}

func appendUniqueSpecRecords(result *LoadResult, seen map[string]artifactOrigin, source config.Source, records []model.SpecRecord) error {
	for _, record := range records {
		origin := specOrigin(source, record)
		if prior, exists := seen[record.Ref]; exists {
			return fmt.Errorf("duplicate spec ref %q: %s conflicts with %s", record.Ref, describeOrigin("bundle", origin), describeOrigin("bundle", prior))
		}
		seen[record.Ref] = origin
		result.Specs = append(result.Specs, record)
	}
	return nil
}

func appendUniqueDocRecords(result *LoadResult, seen map[string]artifactOrigin, source config.Source, records []model.DocRecord) error {
	for _, record := range records {
		origin := docOrigin(source, record)
		if prior, exists := seen[record.Ref]; exists {
			return fmt.Errorf("duplicate doc ref %q: %s conflicts with %s", record.Ref, describeOrigin("doc", origin), describeOrigin("doc", prior))
		}
		seen[record.Ref] = origin
		result.Docs = append(result.Docs, record)
	}
	return nil
}

func specOrigin(source config.Source, record model.SpecRecord) artifactOrigin {
	return artifactOrigin{
		sourceName: source.Name,
		sourcePath: source.Path,
		itemPath:   record.Metadata["bundle_path"],
	}
}

func docOrigin(source config.Source, record model.DocRecord) artifactOrigin {
	return artifactOrigin{
		sourceName: source.Name,
		sourcePath: source.Path,
		itemPath:   record.Metadata["path"],
	}
}

func describeOrigin(itemLabel string, origin artifactOrigin) string {
	return fmt.Sprintf("source %q path %q %s %q", origin.sourceName, origin.sourcePath, itemLabel, origin.itemPath)
}

func loadSpecBundles(workspaceRoot string, source config.Source) ([]model.SpecRecord, error) {
	bundleDirs, err := discoverSpecBundles(source)
	if err != nil {
		return nil, fmt.Errorf("source %q: %w", source.Name, err)
	}

	var records []model.SpecRecord
	for _, bundleDir := range bundleDirs {
		record, err := loadSpecBundle(workspaceRoot, source, bundleDir)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func discoverSpecBundles(source config.Source) ([]string, error) {
	bundleDirs, err := discoverSpecBundleDirs(source.ResolvedPath)
	if err != nil {
		return nil, err
	}

	filtered := bundleDirs[:0]
	for _, bundleDir := range bundleDirs {
		relBundleSpec := filepath.ToSlash(filepath.Join(workspaceRelative(source.ResolvedPath, bundleDir), "spec.toml"))
		allowed, err := sourcePathAllowed(source, relBundleSpec)
		if err != nil {
			return nil, fmt.Errorf("spec %q: %w", relBundleSpec, err)
		}
		if allowed {
			filtered = append(filtered, bundleDir)
		}
	}

	for i := range filtered {
		for j := 0; j < i; j++ {
			parent := filtered[j]
			if isNestedBundle(parent, filtered[i]) {
				return nil, fmt.Errorf("nested spec bundle %q inside %q", filepath.ToSlash(filtered[i]), filepath.ToSlash(parent))
			}
		}
	}

	return filtered, nil
}

func discoverSpecBundleDirs(root string) ([]string, error) {
	var bundleDirs []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		specPath := filepath.Join(path, "spec.toml")
		info, err := os.Stat(specPath)
		if err == nil && !info.IsDir() {
			bundleDirs = append(bundleDirs, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(bundleDirs)
	return bundleDirs, nil
}

func isNestedBundle(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil || rel == "." {
		return false
	}
	return !strings.HasPrefix(rel, "..")
}

type rawSpecBundle struct {
	ID         string
	Title      string
	Status     string
	Domain     string
	Authors    []string
	Tags       []string
	Body       string
	DependsOn  []string
	Supersedes []string
	AppliesTo  []string
}

func loadSpecBundle(workspaceRoot string, source config.Source, bundleDir string) (model.SpecRecord, error) {
	specPath := filepath.Join(bundleDir, "spec.toml")
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		return model.SpecRecord{}, fmt.Errorf("source %q bundle %q: read spec.toml: %w", source.Name, workspaceRelative(workspaceRoot, bundleDir), err)
	}

	raw, err := parseSpecBundle(specBytes)
	if err != nil {
		return model.SpecRecord{}, fmt.Errorf("source %q bundle %q: %w", source.Name, workspaceRelative(workspaceRoot, specPath), err)
	}
	if err := validateRawSpec(workspaceRoot, source.Name, bundleDir, raw); err != nil {
		return model.SpecRecord{}, err
	}

	bodyPath := filepath.Clean(filepath.Join(bundleDir, raw.Body))
	if !pathWithinRoot(bundleDir, bodyPath) {
		return model.SpecRecord{}, fmt.Errorf("source %q bundle %q body %q escapes the bundle directory", source.Name, workspaceRelative(workspaceRoot, bundleDir), raw.Body)
	}
	bodyInfo, err := os.Stat(bodyPath)
	switch {
	case err == nil && bodyInfo.IsDir():
		return model.SpecRecord{}, fmt.Errorf("source %q bundle %q body %q is a directory", source.Name, workspaceRelative(workspaceRoot, bundleDir), workspaceRelative(workspaceRoot, bodyPath))
	case err != nil:
		return model.SpecRecord{}, fmt.Errorf("source %q bundle %q body %q does not exist", source.Name, workspaceRelative(workspaceRoot, bundleDir), workspaceRelative(workspaceRoot, bodyPath))
	}

	bodyBytes, err := os.ReadFile(bodyPath)
	if err != nil {
		return model.SpecRecord{}, fmt.Errorf("source %q bundle %q: read body %q: %w", source.Name, workspaceRelative(workspaceRoot, bundleDir), workspaceRelative(workspaceRoot, bodyPath), err)
	}

	return model.SpecRecord{
		Ref:         raw.ID,
		Kind:        model.ArtifactKindSpec,
		Title:       raw.Title,
		Status:      raw.Status,
		Domain:      raw.Domain,
		Authors:     append([]string(nil), raw.Authors...),
		Tags:        append([]string(nil), raw.Tags...),
		Relations:   buildRelations(raw.DependsOn, raw.Supersedes),
		AppliesTo:   append([]string(nil), raw.AppliesTo...),
		SourceRef:   fileSourceRef(workspaceRoot, specPath),
		BodyFormat:  model.BodyFormatMarkdown,
		BodyText:    string(bodyBytes),
		ContentHash: joinedContentHash(specBytes, bodyBytes),
		Metadata: map[string]string{
			"source_name": source.Name,
			"bundle_path": workspaceRelative(workspaceRoot, bundleDir),
			"body_path":   workspaceRelative(workspaceRoot, bodyPath),
		},
	}, nil
}

func validateRawSpec(workspaceRoot, sourceName, bundleDir string, raw rawSpecBundle) error {
	var missing []string
	if raw.ID == "" {
		missing = append(missing, "id")
	}
	if raw.Title == "" {
		missing = append(missing, "title")
	}
	if raw.Status == "" {
		missing = append(missing, "status")
	} else if !isValidSpecStatus(raw.Status) {
		return fmt.Errorf("source %q bundle %q status %q is invalid", sourceName, workspaceRelative(workspaceRoot, bundleDir), raw.Status)
	}
	if raw.Domain == "" {
		missing = append(missing, "domain")
	}
	if raw.Body == "" {
		missing = append(missing, "body")
	}
	if len(missing) > 0 {
		return fmt.Errorf("source %q bundle %q missing required field(s): %s", sourceName, workspaceRelative(workspaceRoot, bundleDir), strings.Join(missing, ", "))
	}
	return nil
}

func buildRelations(dependsOn, supersedes []string) []model.Relation {
	relations := make([]model.Relation, 0, len(dependsOn)+len(supersedes))
	for _, ref := range dependsOn {
		relations = append(relations, model.Relation{Type: model.RelationDependsOn, Ref: ref})
	}
	for _, ref := range supersedes {
		relations = append(relations, model.Relation{Type: model.RelationSupersedes, Ref: ref})
	}
	return relations
}

func isValidSpecStatus(status string) bool {
	switch status {
	case model.StatusDraft, model.StatusReview, model.StatusAccepted, model.StatusSuperseded, model.StatusDeprecated:
		return true
	default:
		return false
	}
}

func loadMarkdownDocs(workspaceRoot string, source config.Source) ([]model.DocRecord, error) {
	var records []model.DocRecord
	err := filepath.WalkDir(source.ResolvedPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		relPath, err := filepath.Rel(source.ResolvedPath, path)
		if err != nil {
			return fmt.Errorf("source %q doc %q: resolve relative path: %w", source.Name, workspaceRelative(workspaceRoot, path), err)
		}
		allowed, err := sourcePathAllowed(source, relPath)
		if err != nil {
			return fmt.Errorf("source %q doc %q: %w", source.Name, workspaceRelative(workspaceRoot, path), err)
		}
		if !allowed {
			return nil
		}

		bodyBytes, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("source %q doc %q: read markdown: %w", source.Name, workspaceRelative(workspaceRoot, path), err)
		}
		docRef, err := docRefForPath(source.ResolvedPath, path)
		if err != nil {
			return fmt.Errorf("source %q doc %q: %w", source.Name, workspaceRelative(workspaceRoot, path), err)
		}
		records = append(records, model.DocRecord{
			Ref:         docRef,
			Kind:        model.ArtifactKindDoc,
			Title:       docTitle(path, bodyBytes),
			SourceRef:   fileSourceRef(workspaceRoot, path),
			BodyFormat:  model.BodyFormatMarkdown,
			BodyText:    string(bodyBytes),
			ContentHash: contentHash(bodyBytes),
			Metadata: map[string]string{
				"source_name": source.Name,
				"path":        workspaceRelative(workspaceRoot, path),
			},
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

func sourcePathAllowed(source config.Source, relPath string) (bool, error) {
	selection, err := evaluateSourcePathSelection(source, relPath)
	if err != nil {
		return false, err
	}
	return selection.Selected, nil
}

func docRefForPath(sourceRoot, path string) (string, error) {
	rel, err := filepath.Rel(sourceRoot, path)
	if err != nil {
		return "", err
	}
	if filepath.Ext(rel) != ".md" {
		return "", fmt.Errorf("doc path %q is not markdown", rel)
	}
	return "doc://" + strings.TrimSuffix(filepath.ToSlash(rel), ".md"), nil
}

func docTitle(path string, body []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), maxScannerTokenSize(len(body)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# ") {
			title := strings.TrimSpace(strings.TrimPrefix(line, "# "))
			if title != "" {
				return title
			}
		}
	}
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func parseSpecBundle(contents []byte) (rawSpecBundle, error) {
	var spec rawSpecBundle
	var activeArrayKey string

	scanner := bufio.NewScanner(bytes.NewReader(contents))
	scanner.Buffer(make([]byte, 0, 64*1024), maxScannerTokenSize(len(contents)))
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}

		if activeArrayKey != "" {
			if line == "]" {
				activeArrayKey = ""
				continue
			}
			values, err := parseQuotedValues(line)
			if err != nil {
				return rawSpecBundle{}, fmt.Errorf("line %d: %s: %w", lineNo, activeArrayKey, err)
			}
			if err := assignSpecArrayField(&spec, activeArrayKey, values); err != nil {
				return rawSpecBundle{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return rawSpecBundle{}, fmt.Errorf("line %d: expected key = value", lineNo)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if value == "[" {
			if !isSpecArrayField(key) {
				return rawSpecBundle{}, fmt.Errorf("line %d: unsupported array field %q", lineNo, key)
			}
			activeArrayKey = key
			if err := assignSpecArrayField(&spec, key, nil); err != nil {
				return rawSpecBundle{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
			continue
		}
		if strings.HasPrefix(value, "[") {
			if !isSpecArrayField(key) {
				return rawSpecBundle{}, fmt.Errorf("line %d: unsupported array field %q", lineNo, key)
			}
			values, err := parseQuotedValues(value)
			if err != nil {
				return rawSpecBundle{}, fmt.Errorf("line %d: %s: %w", lineNo, key, err)
			}
			if err := assignSpecArrayField(&spec, key, values); err != nil {
				return rawSpecBundle{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
			continue
		}

		parsed, err := parseQuotedString(value)
		if err != nil {
			return rawSpecBundle{}, fmt.Errorf("line %d: %s: %w", lineNo, key, err)
		}
		if err := assignSpecScalarField(&spec, key, parsed); err != nil {
			return rawSpecBundle{}, fmt.Errorf("line %d: %w", lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return rawSpecBundle{}, err
	}
	if activeArrayKey != "" {
		return rawSpecBundle{}, fmt.Errorf("unterminated array for %q", activeArrayKey)
	}

	return spec, nil
}

func assignSpecScalarField(spec *rawSpecBundle, key, value string) error {
	switch key {
	case "id":
		spec.ID = value
	case "title":
		spec.Title = value
	case "status":
		spec.Status = value
	case "domain":
		spec.Domain = value
	case "body":
		spec.Body = value
	default:
		return fmt.Errorf("unsupported field %q", key)
	}
	return nil
}

func isSpecArrayField(key string) bool {
	switch key {
	case "authors", "tags", "depends_on", "supersedes", "applies_to":
		return true
	default:
		return false
	}
}

func assignSpecArrayField(spec *rawSpecBundle, key string, values []string) error {
	switch key {
	case "authors":
		spec.Authors = append(spec.Authors, values...)
	case "tags":
		spec.Tags = append(spec.Tags, values...)
	case "depends_on":
		spec.DependsOn = append(spec.DependsOn, values...)
	case "supersedes":
		spec.Supersedes = append(spec.Supersedes, values...)
	case "applies_to":
		spec.AppliesTo = append(spec.AppliesTo, values...)
	default:
		return fmt.Errorf("unsupported array field %q", key)
	}
	return nil
}

func parseQuotedValues(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("expected quoted string")
	}
	if strings.HasPrefix(value, "[") {
		if !strings.HasSuffix(value, "]") {
			return nil, fmt.Errorf("unterminated array")
		}
		value = strings.TrimSpace(value[1 : len(value)-1])
	}

	var values []string
	for {
		value = strings.TrimSpace(value)
		switch {
		case value == "":
			return values, nil
		case strings.HasPrefix(value, ","):
			value = value[1:]
			continue
		case strings.HasPrefix(value, "]"):
			value = strings.TrimSpace(value[1:])
			if value != "" {
				return nil, fmt.Errorf("unexpected trailing content %q", value)
			}
			return values, nil
		case !strings.HasPrefix(value, "\""):
			return nil, fmt.Errorf("expected quoted string")
		}

		quoted := nextQuotedString(value)
		parsed, err := strconv.Unquote(quoted)
		if err != nil {
			return nil, err
		}
		values = append(values, parsed)
		value = value[len(quoted):]
	}
}

func nextQuotedString(value string) string {
	escaped := false
	for i := 1; i < len(value); i++ {
		switch {
		case escaped:
			escaped = false
		case value[i] == '\\':
			escaped = true
		case value[i] == '"':
			return value[:i+1]
		}
	}
	return value
}

func parseQuotedString(value string) (string, error) {
	return strconv.Unquote(value)
}

func fileSourceRef(workspaceRoot, path string) string {
	return "file://" + workspaceRelative(workspaceRoot, path)
}

func workspaceRelative(workspaceRoot, path string) string {
	rel, err := filepath.Rel(workspaceRoot, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func joinedContentHash(parts ...[]byte) string {
	hasher := sha256.New()
	for i, part := range parts {
		if i > 0 {
			_, _ = hasher.Write([]byte{0})
		}
		_, _ = hasher.Write(part)
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func contentHash(body []byte) string {
	return joinedContentHash(body)
}

func stripComment(line string) string {
	var builder strings.Builder
	inString := false
	escaped := false
	for _, r := range line {
		switch {
		case escaped:
			builder.WriteRune(r)
			escaped = false
		case r == '\\':
			builder.WriteRune(r)
			escaped = true
		case r == '"':
			builder.WriteRune(r)
			inString = !inString
		case r == '#' && !inString:
			return builder.String()
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func pathWithinRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

func maxScannerTokenSize(size int) int {
	if size < 64*1024 {
		return 64 * 1024
	}
	return size + 1
}
