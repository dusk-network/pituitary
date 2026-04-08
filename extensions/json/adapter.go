package jsonadapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	stdjson "encoding/json"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/sdk"
)

const (
	adapterName = "json"
	kindSpec    = "json_spec"
	kindDoc     = "json_doc"
)

func init() {
	sdk.Register(adapterName, func() sdk.Adapter {
		return &adapter{}
	})
}

type adapter struct{}

func (a *adapter) Load(ctx context.Context, cfg sdk.SourceConfig) (*sdk.AdapterResult, error) {
	files, kind, err := enumerateSelectedJSONFiles(cfg)
	if err != nil {
		return nil, err
	}
	options, err := parseSourceOptions(kind, cfg.Options)
	if err != nil {
		return nil, err
	}

	result := &sdk.AdapterResult{}
	for _, file := range files {
		record, err := loadJSONArtifact(ctx, cfg, options, kind, file)
		if err != nil {
			return nil, err
		}
		switch kind {
		case kindSpec:
			result.Specs = append(result.Specs, record.spec)
		case kindDoc:
			result.Docs = append(result.Docs, record.doc)
		default:
			return nil, fmt.Errorf("unsupported kind %q", kind)
		}
	}
	return result, nil
}

func (a *adapter) Preview(ctx context.Context, cfg sdk.SourceConfig) ([]sdk.PreviewItem, error) {
	_ = ctx

	files, kind, err := enumerateSelectedJSONFiles(cfg)
	if err != nil {
		return nil, err
	}

	artifactKind := sdk.ArtifactKindDoc
	if kind == kindSpec {
		artifactKind = sdk.ArtifactKindSpec
	}

	items := make([]sdk.PreviewItem, 0, len(files))
	for _, file := range files {
		items = append(items, sdk.PreviewItem{
			ArtifactKind: artifactKind,
			Path:         workspaceRelative(cfg.WorkspaceRoot, file.AbsolutePath),
		})
	}
	return items, nil
}

type sourceOptions struct {
	refPointer        string
	titlePointer      string
	bodyPointer       string
	statusPointer     string
	domainPointer     string
	authorsPointer    string
	tagsPointer       string
	dependsOnPointer  string
	supersedesPointer string
	relatesToPointer  string
	appliesToPointer  string
}

func parseSourceOptions(kind string, options map[string]any) (sourceOptions, error) {
	parsed := sourceOptions{}
	if len(options) == 0 {
		return parsed, nil
	}

	allowed := map[string]*string{
		"applies_to_pointer": &parsed.appliesToPointer,
		"authors_pointer":    &parsed.authorsPointer,
		"body_pointer":       &parsed.bodyPointer,
		"depends_on_pointer": &parsed.dependsOnPointer,
		"domain_pointer":     &parsed.domainPointer,
		"ref_pointer":        &parsed.refPointer,
		"relates_to_pointer": &parsed.relatesToPointer,
		"status_pointer":     &parsed.statusPointer,
		"supersedes_pointer": &parsed.supersedesPointer,
		"tags_pointer":       &parsed.tagsPointer,
		"title_pointer":      &parsed.titlePointer,
	}
	for key, value := range options {
		target, ok := allowed[key]
		if !ok {
			return sourceOptions{}, fmt.Errorf("unsupported option %q", key)
		}
		pointer, err := optionPointer(value)
		if err != nil {
			return sourceOptions{}, fmt.Errorf("options.%s: %w", key, err)
		}
		*target = pointer
	}

	if kind == kindDoc {
		specOnly := map[string]string{
			"applies_to_pointer": parsed.appliesToPointer,
			"authors_pointer":    parsed.authorsPointer,
			"depends_on_pointer": parsed.dependsOnPointer,
			"domain_pointer":     parsed.domainPointer,
			"relates_to_pointer": parsed.relatesToPointer,
			"status_pointer":     parsed.statusPointer,
			"supersedes_pointer": parsed.supersedesPointer,
			"tags_pointer":       parsed.tagsPointer,
		}
		for key, pointer := range specOnly {
			if pointer != "" {
				return sourceOptions{}, fmt.Errorf("options.%s is only supported for kind %q", key, kindSpec)
			}
		}
	}

	return parsed, nil
}

func optionPointer(value any) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("expected string, got %T", value)
	}
	if err := validateJSONPointer(text); err != nil {
		return "", err
	}
	return text, nil
}

type selectedJSONFile struct {
	AbsolutePath string
	RelativePath string
}

func enumerateSelectedJSONFiles(cfg sdk.SourceConfig) ([]selectedJSONFile, string, error) {
	kind := normalizeKind(cfg.Kind)
	if kind == "" {
		return nil, "", fmt.Errorf("unsupported kind %q", cfg.Kind)
	}

	resolvedPath, err := resolveSourceRoot(cfg.WorkspaceRoot, cfg.Path)
	if err != nil {
		return nil, "", err
	}
	if err := validateExplicitJSONFiles(resolvedPath, cfg.Files); err != nil {
		return nil, "", err
	}

	matches := make([]selectedJSONFile, 0)
	err = filepath.WalkDir(resolvedPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		relPath, err := filepath.Rel(resolvedPath, path)
		if err != nil {
			return fmt.Errorf("json %q: resolve relative path: %w", workspaceRelative(cfg.WorkspaceRoot, path), err)
		}
		relPath = filepath.ToSlash(relPath)
		allowed, err := sourcePathAllowed(cfg, relPath)
		if err != nil {
			return fmt.Errorf("json %q: %w", workspaceRelative(cfg.WorkspaceRoot, path), err)
		}
		if !allowed {
			return nil
		}
		matches = append(matches, selectedJSONFile{
			AbsolutePath: path,
			RelativePath: relPath,
		})
		return nil
	})
	if err != nil {
		return nil, "", err
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].RelativePath < matches[j].RelativePath
	})
	return matches, kind, nil
}

func resolveSourceRoot(workspaceRoot, sourcePath string) (string, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return "", fmt.Errorf("path is required for adapter %q", adapterName)
	}

	resolvedPath := sourcePath
	if !filepath.IsAbs(resolvedPath) {
		resolvedPath = filepath.Join(workspaceRoot, sourcePath)
	}
	resolvedPath = filepath.Clean(resolvedPath)

	info, err := os.Stat(resolvedPath)
	switch {
	case err == nil && !info.IsDir():
		return "", fmt.Errorf("path %q is not a directory", sourcePath)
	case err != nil:
		return "", fmt.Errorf("path %q does not exist", sourcePath)
	}
	return resolvedPath, nil
}

func validateExplicitJSONFiles(sourceRoot string, files []string) error {
	for i, relFile := range files {
		normalized := filepath.ToSlash(strings.TrimSpace(relFile))
		if pathpkg.Ext(normalized) != ".json" {
			return fmt.Errorf("files[%d]: %q must point to a JSON file", i, relFile)
		}
		resolvedFile := filepath.Join(sourceRoot, filepath.FromSlash(normalized))
		info, err := os.Stat(resolvedFile)
		switch {
		case err == nil && info.IsDir():
			return fmt.Errorf("files[%d]: %q is a directory", i, relFile)
		case err != nil:
			return fmt.Errorf("files[%d]: %q does not exist", i, relFile)
		}
	}
	return nil
}

func normalizeKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case kindSpec, sdk.ArtifactKindSpec:
		return kindSpec
	case kindDoc, sdk.ArtifactKindDoc:
		return kindDoc
	default:
		return ""
	}
}

func sourcePathAllowed(cfg sdk.SourceConfig, relPath string) (bool, error) {
	relPath = filepath.ToSlash(relPath)

	if len(cfg.Files) > 0 {
		matched := false
		for _, file := range cfg.Files {
			if filepath.ToSlash(strings.TrimSpace(file)) == relPath {
				matched = true
				break
			}
		}
		if !matched {
			return false, nil
		}
	}

	if len(cfg.Include) > 0 {
		matched := false
		for _, pattern := range cfg.Include {
			ok, err := pathpkg.Match(pattern, relPath)
			if err != nil {
				return false, fmt.Errorf("include pattern %q is invalid: %w", pattern, err)
			}
			if ok {
				matched = true
			}
		}
		if !matched {
			return false, nil
		}
	}

	for _, pattern := range cfg.Exclude {
		ok, err := pathpkg.Match(pattern, relPath)
		if err != nil {
			return false, fmt.Errorf("exclude pattern %q is invalid: %w", pattern, err)
		}
		if ok {
			return false, nil
		}
	}

	return true, nil
}

type loadedArtifact struct {
	spec sdk.SpecRecord
	doc  sdk.DocRecord
}

func loadJSONArtifact(ctx context.Context, cfg sdk.SourceConfig, options sourceOptions, kind string, file selectedJSONFile) (loadedArtifact, error) {
	_ = ctx

	raw, err := os.ReadFile(file.AbsolutePath)
	if err != nil {
		return loadedArtifact{}, fmt.Errorf("source %q json %q: read file: %w", cfg.Name, workspaceRelative(cfg.WorkspaceRoot, file.AbsolutePath), err)
	}

	decoder := stdjson.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return loadedArtifact{}, fmt.Errorf("source %q json %q: parse JSON: %w", cfg.Name, workspaceRelative(cfg.WorkspaceRoot, file.AbsolutePath), err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return loadedArtifact{}, fmt.Errorf("source %q json %q: parse JSON: unexpected trailing content", cfg.Name, workspaceRelative(cfg.WorkspaceRoot, file.AbsolutePath))
		}
		return loadedArtifact{}, fmt.Errorf("source %q json %q: parse JSON: %w", cfg.Name, workspaceRelative(cfg.WorkspaceRoot, file.AbsolutePath), err)
	}

	switch kind {
	case kindSpec:
		spec, err := buildSpecRecord(cfg, options, file, raw, document)
		if err != nil {
			return loadedArtifact{}, err
		}
		return loadedArtifact{spec: spec}, nil
	case kindDoc:
		doc, err := buildDocRecord(cfg, options, file, raw, document)
		if err != nil {
			return loadedArtifact{}, err
		}
		return loadedArtifact{doc: doc}, nil
	default:
		return loadedArtifact{}, fmt.Errorf("unsupported kind %q", kind)
	}
}

func buildSpecRecord(cfg sdk.SourceConfig, options sourceOptions, file selectedJSONFile, raw []byte, document any) (sdk.SpecRecord, error) {
	title, titleSource, err := selectStringField(document, options.titlePointer, strings.TrimSuffix(filepath.Base(file.AbsolutePath), filepath.Ext(file.AbsolutePath)), "filename")
	if err != nil {
		return sdk.SpecRecord{}, fieldError(cfg.Name, file.AbsolutePath, "title_pointer", err)
	}
	refFallback := pathBasedRef("json-spec://", file.RelativePath, cfg.Repo, cfg.PrimaryRepoID)
	ref, refSource, err := selectStringField(document, options.refPointer, refFallback, "path")
	if err != nil {
		return sdk.SpecRecord{}, fieldError(cfg.Name, file.AbsolutePath, "ref_pointer", err)
	}
	status, statusSource, err := selectStatus(document, options.statusPointer)
	if err != nil {
		return sdk.SpecRecord{}, fieldError(cfg.Name, file.AbsolutePath, "status_pointer", err)
	}
	domain, domainSource, err := selectDomain(document, options.domainPointer, cfg.Name)
	if err != nil {
		return sdk.SpecRecord{}, fieldError(cfg.Name, file.AbsolutePath, "domain_pointer", err)
	}
	authors, err := selectStringListField(document, options.authorsPointer)
	if err != nil {
		return sdk.SpecRecord{}, fieldError(cfg.Name, file.AbsolutePath, "authors_pointer", err)
	}
	tags, err := selectStringListField(document, options.tagsPointer)
	if err != nil {
		return sdk.SpecRecord{}, fieldError(cfg.Name, file.AbsolutePath, "tags_pointer", err)
	}
	dependsOn, err := selectStringListField(document, options.dependsOnPointer)
	if err != nil {
		return sdk.SpecRecord{}, fieldError(cfg.Name, file.AbsolutePath, "depends_on_pointer", err)
	}
	supersedes, err := selectStringListField(document, options.supersedesPointer)
	if err != nil {
		return sdk.SpecRecord{}, fieldError(cfg.Name, file.AbsolutePath, "supersedes_pointer", err)
	}
	relatesTo, err := selectStringListField(document, options.relatesToPointer)
	if err != nil {
		return sdk.SpecRecord{}, fieldError(cfg.Name, file.AbsolutePath, "relates_to_pointer", err)
	}
	appliesTo, err := selectStringListField(document, options.appliesToPointer)
	if err != nil {
		return sdk.SpecRecord{}, fieldError(cfg.Name, file.AbsolutePath, "applies_to_pointer", err)
	}
	bodyValue, err := selectBodyValue(document, options.bodyPointer)
	if err != nil {
		return sdk.SpecRecord{}, fieldError(cfg.Name, file.AbsolutePath, "body_pointer", err)
	}
	bodyText, err := renderSpecBody(title, ref, status, domain, authors, tags, dependsOn, supersedes, relatesTo, appliesTo, bodyValue)
	if err != nil {
		return sdk.SpecRecord{}, fmt.Errorf("source %q json %q: render body: %w", cfg.Name, workspaceRelative(cfg.WorkspaceRoot, file.AbsolutePath), err)
	}

	return sdk.SpecRecord{
		Ref:         ref,
		Kind:        sdk.ArtifactKindSpec,
		Title:       title,
		Status:      status,
		Domain:      domain,
		Authors:     authors,
		Tags:        tags,
		Relations:   buildRelations(dependsOn, supersedes, relatesTo),
		AppliesTo:   appliesTo,
		SourceRef:   fileSourceRef(cfg.WorkspaceRoot, file.AbsolutePath),
		BodyFormat:  sdk.BodyFormatMarkdown,
		BodyText:    bodyText,
		ContentHash: contentHash(raw),
		Metadata: map[string]string{
			"path":          workspaceRelative(cfg.WorkspaceRoot, file.AbsolutePath),
			"source_name":   cfg.Name,
			"source_kind":   kindSpec,
			"path_ref":      refFallback,
			"ref_source":    refSource,
			"title_source":  titleSource,
			"status_source": statusSource,
			"domain_source": domainSource,
		},
	}, nil
}

func buildDocRecord(cfg sdk.SourceConfig, options sourceOptions, file selectedJSONFile, raw []byte, document any) (sdk.DocRecord, error) {
	title, titleSource, err := selectStringField(document, options.titlePointer, strings.TrimSuffix(filepath.Base(file.AbsolutePath), filepath.Ext(file.AbsolutePath)), "filename")
	if err != nil {
		return sdk.DocRecord{}, fieldError(cfg.Name, file.AbsolutePath, "title_pointer", err)
	}
	refFallback := pathBasedRef("json-doc://", file.RelativePath, cfg.Repo, cfg.PrimaryRepoID)
	ref, refSource, err := selectStringField(document, options.refPointer, refFallback, "path")
	if err != nil {
		return sdk.DocRecord{}, fieldError(cfg.Name, file.AbsolutePath, "ref_pointer", err)
	}
	bodyValue, err := selectBodyValue(document, options.bodyPointer)
	if err != nil {
		return sdk.DocRecord{}, fieldError(cfg.Name, file.AbsolutePath, "body_pointer", err)
	}
	bodyText, err := renderDocBody(title, bodyValue)
	if err != nil {
		return sdk.DocRecord{}, fmt.Errorf("source %q json %q: render body: %w", cfg.Name, workspaceRelative(cfg.WorkspaceRoot, file.AbsolutePath), err)
	}

	return sdk.DocRecord{
		Ref:         ref,
		Kind:        sdk.ArtifactKindDoc,
		Title:       title,
		SourceRef:   fileSourceRef(cfg.WorkspaceRoot, file.AbsolutePath),
		BodyFormat:  sdk.BodyFormatMarkdown,
		BodyText:    bodyText,
		ContentHash: contentHash(raw),
		Metadata: map[string]string{
			"path":         workspaceRelative(cfg.WorkspaceRoot, file.AbsolutePath),
			"source_name":  cfg.Name,
			"source_kind":  kindDoc,
			"path_ref":     refFallback,
			"ref_source":   refSource,
			"title_source": titleSource,
		},
	}, nil
}

func fieldError(sourceName, path, option string, err error) error {
	return fmt.Errorf("source %q json %q %s: %w", sourceName, filepath.ToSlash(path), option, err)
}

func selectStringField(document any, pointer, fallback, fallbackSource string) (string, string, error) {
	if pointer == "" {
		return fallback, fallbackSource, nil
	}
	value, err := lookupPointer(document, pointer)
	if err != nil {
		return "", "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", "", fmt.Errorf("expected string, got %T", value)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", "", fmt.Errorf("value must not be empty")
	}
	return text, "json_pointer", nil
}

func selectStatus(document any, pointer string) (string, string, error) {
	if pointer == "" {
		return sdk.StatusDraft, "default", nil
	}
	value, err := lookupPointer(document, pointer)
	if err != nil {
		return "", "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", "", fmt.Errorf("expected string, got %T", value)
	}
	status := strings.ToLower(strings.TrimSpace(text))
	switch status {
	case sdk.StatusDraft, sdk.StatusReview, sdk.StatusAccepted, sdk.StatusSuperseded, sdk.StatusDeprecated:
		return status, "json_pointer", nil
	default:
		return "", "", fmt.Errorf("unsupported status %q", text)
	}
}

func selectDomain(document any, pointer, sourceName string) (string, string, error) {
	if pointer == "" {
		return defaultDomain(sourceName), "source_name", nil
	}
	value, err := lookupPointer(document, pointer)
	if err != nil {
		return "", "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", "", fmt.Errorf("expected string, got %T", value)
	}
	text = normalizeDomain(text)
	if text == "" {
		return "", "", fmt.Errorf("value must not be empty")
	}
	return text, "json_pointer", nil
}

func selectStringListField(document any, pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	value, err := lookupPointer(document, pointer)
	if err != nil {
		return nil, err
	}
	return toStringSlice(value)
}

func selectBodyValue(document any, pointer string) (any, error) {
	if pointer == "" {
		return document, nil
	}
	return lookupPointer(document, pointer)
}

func renderSpecBody(title, ref, status, domain string, authors, tags, dependsOn, supersedes, relatesTo, appliesTo []string, bodyValue any) (string, error) {
	var builder strings.Builder
	builder.WriteString("# " + title + "\n\n")
	builder.WriteString("Ref: " + ref + "\n")
	builder.WriteString("Status: " + status + "\n")
	if domain != "" {
		builder.WriteString("Domain: " + domain + "\n")
	}
	if len(authors) > 0 {
		builder.WriteString("Authors: " + strings.Join(authors, ", ") + "\n")
	}
	if len(tags) > 0 {
		builder.WriteString("Tags: " + strings.Join(tags, ", ") + "\n")
	}
	if len(dependsOn) > 0 {
		builder.WriteString("Depends On: " + strings.Join(dependsOn, ", ") + "\n")
	}
	if len(supersedes) > 0 {
		builder.WriteString("Supersedes: " + strings.Join(supersedes, ", ") + "\n")
	}
	if len(relatesTo) > 0 {
		builder.WriteString("Relates To: " + strings.Join(relatesTo, ", ") + "\n")
	}
	if len(appliesTo) > 0 {
		builder.WriteString("Applies To: " + strings.Join(appliesTo, ", ") + "\n")
	}
	builder.WriteString("\n")

	body, err := renderBodyValue(bodyValue)
	if err != nil {
		return "", err
	}
	builder.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		builder.WriteString("\n")
	}
	return builder.String(), nil
}

func renderDocBody(title string, bodyValue any) (string, error) {
	var builder strings.Builder
	builder.WriteString("# " + title + "\n\n")
	body, err := renderBodyValue(bodyValue)
	if err != nil {
		return "", err
	}
	builder.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		builder.WriteString("\n")
	}
	return builder.String(), nil
}

func renderBodyValue(bodyValue any) (string, error) {
	switch typed := bodyValue.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return "```json\n\"\"\n```\n", nil
		}
		return trimmed + "\n", nil
	default:
		rendered, err := stdjson.MarshalIndent(bodyValue, "", "  ")
		if err != nil {
			return "", err
		}
		return "```json\n" + string(rendered) + "\n```\n", nil
	}
}

func buildRelations(dependsOn, supersedes, relatesTo []string) []sdk.Relation {
	relations := make([]sdk.Relation, 0, len(dependsOn)+len(supersedes)+len(relatesTo))
	for _, ref := range dependsOn {
		relations = append(relations, sdk.Relation{Type: sdk.RelationDependsOn, Ref: ref})
	}
	for _, ref := range supersedes {
		relations = append(relations, sdk.Relation{Type: sdk.RelationSupersedes, Ref: ref})
	}
	for _, ref := range relatesTo {
		relations = append(relations, sdk.Relation{Type: sdk.RelationRelatesTo, Ref: ref})
	}
	return relations
}

func toStringSlice(value any) ([]string, error) {
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, fmt.Errorf("value must not be empty")
		}
		return []string{text}, nil
	case []any:
		result := make([]string, 0, len(typed))
		for i, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("item %d: expected string, got %T", i, item)
			}
			text = strings.TrimSpace(text)
			if text == "" {
				return nil, fmt.Errorf("item %d: value must not be empty", i)
			}
			result = append(result, text)
		}
		return uniqueStrings(result), nil
	default:
		return nil, fmt.Errorf("expected string or string array, got %T", value)
	}
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func lookupPointer(document any, pointer string) (any, error) {
	if pointer == "" {
		return document, nil
	}
	parts, err := parseJSONPointer(pointer)
	if err != nil {
		return nil, err
	}

	current := document
	for _, part := range parts {
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[part]
			if !ok {
				return nil, fmt.Errorf("pointer %q does not exist", pointer)
			}
			current = next
		case []any:
			index, err := parsePointerIndex(part, len(typed))
			if err != nil {
				return nil, fmt.Errorf("pointer %q: %w", pointer, err)
			}
			current = typed[index]
		default:
			return nil, fmt.Errorf("pointer %q cannot descend into %T", pointer, current)
		}
	}
	return current, nil
}

func validateJSONPointer(pointer string) error {
	_, err := parseJSONPointer(pointer)
	return err
}

func parseJSONPointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("must be empty or start with '/'")
	}

	rawParts := strings.Split(pointer[1:], "/")
	parts := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		decoded, err := decodePointerToken(part)
		if err != nil {
			return nil, err
		}
		parts = append(parts, decoded)
	}
	return parts, nil
}

func decodePointerToken(token string) (string, error) {
	var builder strings.Builder
	for i := 0; i < len(token); i++ {
		if token[i] != '~' {
			builder.WriteByte(token[i])
			continue
		}
		if i+1 >= len(token) {
			return "", fmt.Errorf("invalid escape in JSON pointer token %q", token)
		}
		i++
		switch token[i] {
		case '0':
			builder.WriteByte('~')
		case '1':
			builder.WriteByte('/')
		default:
			return "", fmt.Errorf("invalid escape in JSON pointer token %q", token)
		}
	}
	return builder.String(), nil
}

func parsePointerIndex(raw string, length int) (int, error) {
	if raw == "" {
		return 0, fmt.Errorf("array index must not be empty")
	}
	index := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			return 0, fmt.Errorf("array index %q is not numeric", raw)
		}
		index = index*10 + int(raw[i]-'0')
	}
	if index < 0 || index >= length {
		return 0, fmt.Errorf("array index %q is out of range", raw)
	}
	return index, nil
}

func pathBasedRef(prefix, relativePath, repoID, primaryRepoID string) string {
	relativePath = strings.TrimSuffix(filepath.ToSlash(relativePath), ".json")
	relativePath = strings.TrimPrefix(strings.TrimSpace(relativePath), "/")
	repoID = strings.TrimSpace(repoID)
	primaryRepoID = strings.TrimSpace(primaryRepoID)
	if repoID == "" || repoID == primaryRepoID {
		return prefix + relativePath
	}
	return prefix + repoID + "/" + relativePath
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

func defaultDomain(sourceName string) string {
	normalized := normalizeDomain(sourceName)
	if normalized == "" {
		return "json"
	}
	return normalized
}

func normalizeDomain(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			builder.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '/':
			if !lastDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func contentHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
