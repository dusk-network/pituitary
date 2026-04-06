package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
)

// CanonicalizeOptions controls promotion of one inferred markdown contract into
// an explicit spec bundle.
type CanonicalizeOptions struct {
	Path              string
	BundleDir         string
	Write             bool
	MetadataInference *CanonicalizeInference
}

// CanonicalizeInference holds pre-computed model-inferred metadata to be
// merged into the canonicalized spec record. This is populated by the caller
// (cmd layer) to avoid import cycles between source and analysis.
type CanonicalizeInference struct {
	Domain     *InferredValue  `json:"domain,omitempty"`
	Tags       *InferredValues `json:"tags,omitempty"`
	AppliesTo  *InferredValues `json:"applies_to,omitempty"`
	Status     *InferredValue  `json:"status,omitempty"`
	DependsOn  []InferredRef   `json:"depends_on,omitempty"`
	Supersedes []InferredRef   `json:"supersedes,omitempty"`
}

// InferredValue is a single model-inferred string with confidence.
type InferredValue struct {
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
}

// InferredValues is a model-inferred string list with confidence.
type InferredValues struct {
	Values     []string `json:"values"`
	Confidence float64  `json:"confidence"`
}

// InferredRef is a model-inferred spec relation with confidence.
type InferredRef struct {
	Ref        string  `json:"ref"`
	Confidence float64 `json:"confidence"`
}

// CanonicalizeResult previews or writes one generated spec bundle.
type CanonicalizeResult struct {
	WorkspaceRoot string                    `json:"workspace_root"`
	SourcePath    string                    `json:"source_path"`
	BundleDir     string                    `json:"bundle_dir"`
	WroteBundle   bool                      `json:"wrote_bundle,omitempty"`
	Spec          model.SpecRecord          `json:"spec"`
	Provenance    CanonicalizeProvenance    `json:"provenance"`
	Files         []CanonicalizedBundleFile `json:"files"`
}

// CanonicalizeProvenance preserves where the generated bundle came from.
type CanonicalizeProvenance struct {
	SourceRef string `json:"source_ref"`
	PathRef   string `json:"path_ref,omitempty"`
}

// CanonicalizedBundleFile previews one generated file.
type CanonicalizedBundleFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// CanonicalizeMarkdownContract generates or writes a suggested explicit spec
// bundle from one inferred markdown contract file.
func CanonicalizeMarkdownContract(options CanonicalizeOptions) (*CanonicalizeResult, error) {
	workspaceRoot, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	workspaceRoot, err = filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}

	sourcePath, err := resolveCanonicalizePath(workspaceRoot, options.Path)
	if err != nil {
		return nil, err
	}
	if filepath.Ext(sourcePath) != ".md" {
		return nil, fmt.Errorf("canonicalize source %q is not markdown", workspaceRelative(workspaceRoot, sourcePath))
	}

	// #nosec G304 -- sourcePath is resolved from the workspace and validated before reading.
	body, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read markdown contract %q: %w", workspaceRelative(workspaceRoot, sourcePath), err)
	}

	spec, err := inferMarkdownContract(workspaceRoot, config.Source{
		Name: "canonicalize",
		Kind: config.SourceKindMarkdownContract,
	}, sourcePath, body)
	if err != nil {
		return nil, err
	}

	// Semantic metadata enrichment: merge pre-computed model inference into
	// the heuristic result when the caller provides it.
	if options.MetadataInference != nil {
		enrichCanonicalizeWithInference(&spec, options.MetadataInference)
	}

	bundleDir, err := canonicalizeBundleDir(workspaceRoot, sourcePath, options.BundleDir)
	if err != nil {
		return nil, err
	}

	specToml, err := renderCanonicalizedSpecToml(workspaceRoot, sourcePath, spec)
	if err != nil {
		return nil, err
	}
	bodyMarkdown := normalizeCanonicalizedBody(sourcePath, body, spec.Title)

	result := &CanonicalizeResult{
		WorkspaceRoot: workspaceRoot,
		SourcePath:    workspaceRelative(workspaceRoot, sourcePath),
		BundleDir:     workspaceRelative(workspaceRoot, bundleDir),
		Spec:          spec,
		Provenance: CanonicalizeProvenance{
			SourceRef: spec.SourceRef,
			PathRef:   spec.Metadata["path_ref"],
		},
		Files: []CanonicalizedBundleFile{
			{
				Path:    filepath.ToSlash(filepath.Join(workspaceRelative(workspaceRoot, bundleDir), "spec.toml")),
				Content: specToml,
			},
			{
				Path:    filepath.ToSlash(filepath.Join(workspaceRelative(workspaceRoot, bundleDir), "body.md")),
				Content: bodyMarkdown,
			},
		},
	}

	if options.Write {
		if err := writeCanonicalizedBundle(bundleDir, specToml, bodyMarkdown); err != nil {
			return nil, err
		}
		result.WroteBundle = true
	}

	return result, nil
}

func resolveCanonicalizePath(workspaceRoot, rawPath string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", fmt.Errorf("--path is required")
	}

	absPath := rawPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(workspaceRoot, rawPath)
	}
	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", rawPath, err)
	}
	if !pathWithinRoot(workspaceRoot, absPath) {
		return "", fmt.Errorf("--path %q resolves outside workspace root %q", rawPath, filepath.ToSlash(workspaceRoot))
	}
	info, err := os.Stat(absPath)
	switch {
	case err == nil && info.IsDir():
		return "", fmt.Errorf("canonicalize source %q is a directory", filepath.ToSlash(rawPath))
	case err != nil:
		return "", fmt.Errorf("canonicalize source %q: %w", filepath.ToSlash(rawPath), err)
	}
	return absPath, nil
}

func canonicalizeBundleDir(workspaceRoot, sourcePath, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		stem := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
		requested = filepath.Join(".pituitary", "canonicalized", sanitizeDiscoverySourceName(stem))
	}

	bundleDir := requested
	if !filepath.IsAbs(bundleDir) {
		bundleDir = filepath.Join(workspaceRoot, bundleDir)
	}
	bundleDir, err := filepath.Abs(bundleDir)
	if err != nil {
		return "", fmt.Errorf("resolve bundle directory %q: %w", requested, err)
	}
	if !pathWithinRoot(workspaceRoot, bundleDir) {
		return "", fmt.Errorf("--bundle-dir %q resolves outside workspace root %q", requested, filepath.ToSlash(workspaceRoot))
	}
	return bundleDir, nil
}

func renderCanonicalizedSpecToml(workspaceRoot, sourcePath string, spec model.SpecRecord) (string, error) {
	domain := strings.TrimSpace(spec.Domain)
	if domain == "" {
		domain = "unknown"
	}

	var builder strings.Builder
	fmt.Fprintf(&builder, "# Generated by pituitary canonicalize\n")
	fmt.Fprintf(&builder, "# Source markdown contract: %s\n", workspaceRelative(workspaceRoot, sourcePath))
	fmt.Fprintf(&builder, "# Source ref: %s\n", spec.SourceRef)
	if pathRef := strings.TrimSpace(spec.Metadata["path_ref"]); pathRef != "" {
		fmt.Fprintf(&builder, "# Stable inferred ref: %s\n", pathRef)
	}
	if spec.Inference != nil && len(spec.Inference.Reasons) > 0 {
		fmt.Fprintf(&builder, "# Inference notes: %s\n", strings.Join(spec.Inference.Reasons, "; "))
	}
	if strings.TrimSpace(spec.Domain) == "" {
		fmt.Fprintf(&builder, "# TODO: replace placeholder domain \"unknown\" if this contract belongs to a real domain\n")
	}
	fmt.Fprintf(&builder, "id = %s\n", strconv.Quote(spec.Ref))
	fmt.Fprintf(&builder, "title = %s\n", strconv.Quote(spec.Title))
	fmt.Fprintf(&builder, "status = %s\n", strconv.Quote(spec.Status))
	fmt.Fprintf(&builder, "domain = %s\n", strconv.Quote(domain))
	fmt.Fprintf(&builder, "body = %s\n", strconv.Quote("body.md"))
	writeSpecBundleArray(&builder, "authors", spec.Authors)
	writeSpecBundleArray(&builder, "tags", spec.Tags)
	writeSpecBundleArray(&builder, "depends_on", collectRelationRefs(spec.Relations, model.RelationDependsOn))
	writeSpecBundleArray(&builder, "supersedes", collectRelationRefs(spec.Relations, model.RelationSupersedes))
	writeSpecBundleArray(&builder, "relates_to", collectRelationRefs(spec.Relations, model.RelationRelatesTo))
	writeSpecBundleArray(&builder, "applies_to", spec.AppliesTo)
	return builder.String(), nil
}

func normalizeCanonicalizedBody(sourcePath string, body []byte, title string) string {
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	var (
		normalized []string
		activeList string
		inHeader   = true
		hasTitle   bool
	)

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if inHeader {
			switch {
			case line == "", line == "---":
				continue
			case strings.HasPrefix(line, "# "):
				normalized = append(normalized, "# "+strings.TrimSpace(strings.TrimPrefix(line, "# ")))
				hasTitle = true
				continue
			case activeList != "":
				if strings.HasPrefix(line, "- ") {
					continue
				}
				activeList = ""
			}

			key, _, ok := strings.Cut(line, ":")
			if ok {
				key = normalizeMarkdownContractKey(key)
				if isMarkdownContractField(key) {
					if isMarkdownContractListField(key) {
						activeList = key
					}
					continue
				}
			}

			inHeader = false
		}
		normalized = append(normalized, raw)
	}

	bodyText := strings.TrimSpace(strings.Join(normalized, "\n"))
	if !hasTitle {
		if bodyText == "" {
			bodyText = "# " + title + "\n"
		} else {
			bodyText = "# " + title + "\n\n" + bodyText
		}
	}
	if bodyText == "" {
		bodyText = "# " + title + "\n"
	}
	return strings.TrimRight(bodyText, "\n") + "\n"
}

func writeCanonicalizedBundle(bundleDir, specToml, bodyMarkdown string) error {
	for _, path := range []string{
		filepath.Join(bundleDir, "spec.toml"),
		filepath.Join(bundleDir, "body.md"),
	} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return fmt.Errorf("refusing to overwrite existing file %s", filepath.ToSlash(path))
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("stat output path %s: %w", filepath.ToSlash(path), err)
		}
	}
	// #nosec G301 -- generated bundle directories use normal checkout permissions for repo files.
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		return fmt.Errorf("create bundle directory: %w", err)
	}
	// #nosec G306 -- generated spec bundles are non-secret repo files intended to be readable by standard tooling.
	if err := os.WriteFile(filepath.Join(bundleDir, "spec.toml"), []byte(specToml), 0o644); err != nil {
		return fmt.Errorf("write generated spec.toml: %w", err)
	}
	// #nosec G306 -- generated spec bundles are non-secret repo files intended to be readable by standard tooling.
	if err := os.WriteFile(filepath.Join(bundleDir, "body.md"), []byte(bodyMarkdown), 0o644); err != nil {
		return fmt.Errorf("write generated body.md: %w", err)
	}
	return nil
}

func writeSpecBundleArray(builder *strings.Builder, key string, values []string) {
	if len(values) == 0 {
		return
	}
	builder.WriteString(key)
	builder.WriteString(" = [\n")
	for _, value := range values {
		builder.WriteString("  ")
		builder.WriteString(strconv.Quote(value))
		builder.WriteString(",\n")
	}
	builder.WriteString("]\n")
}

func collectRelationRefs(relations []model.Relation, relationType model.RelationType) []string {
	var refs []string
	for _, relation := range relations {
		if relation.Type == relationType {
			refs = append(refs, relation.Ref)
		}
	}
	return refs
}

// enrichCanonicalizeWithInference merges pre-computed model inference into the
// heuristic spec record, updating the confidence scoring system.
func enrichCanonicalizeWithInference(spec *model.SpecRecord, inference *CanonicalizeInference) {
	if inference == nil {
		return
	}

	inferenceSource := "model"

	// Domain: use model inference when heuristic left it empty.
	if strings.TrimSpace(spec.Domain) == "" && inference.Domain != nil && inference.Domain.Value != "" {
		spec.Domain = inference.Domain.Value
		updateInferenceField(spec.Inference, "domain", inferenceSource, inference.Domain.Confidence)
	}

	// Tags: use model inference when heuristic produced none.
	if len(spec.Tags) == 0 && inference.Tags != nil && len(inference.Tags.Values) > 0 {
		spec.Tags = inference.Tags.Values
		addInferenceField(spec.Inference, "tags", inferenceSource, inference.Tags.Confidence)
	}

	// AppliesTo: use model inference when heuristic produced none.
	if len(spec.AppliesTo) == 0 && inference.AppliesTo != nil && len(inference.AppliesTo.Values) > 0 {
		spec.AppliesTo = inference.AppliesTo.Values
		addInferenceField(spec.Inference, "applies_to", inferenceSource, inference.AppliesTo.Confidence)
	}

	// Status: use model inference when heuristic defaulted to "draft".
	if spec.Metadata != nil && spec.Metadata["status_source"] == "default" && inference.Status != nil {
		spec.Status = inference.Status.Value
		spec.Metadata["status_source"] = inferenceSource
		updateInferenceField(spec.Inference, "status", inferenceSource, inference.Status.Confidence)
	}

	// DependsOn: add inferred relations with sufficient confidence, deduplicating
	// against existing relations.
	existingRelations := make(map[string]struct{}, len(spec.Relations))
	for _, rel := range spec.Relations {
		existingRelations[string(rel.Type)+"\x00"+rel.Ref] = struct{}{}
	}

	for _, dep := range inference.DependsOn {
		ref := strings.TrimSpace(dep.Ref)
		if ref == "" || dep.Confidence < 0.5 {
			continue
		}
		key := string(model.RelationDependsOn) + "\x00" + ref
		if _, exists := existingRelations[key]; exists {
			continue
		}
		existingRelations[key] = struct{}{}
		spec.Relations = append(spec.Relations, model.Relation{
			Type: model.RelationDependsOn,
			Ref:  ref,
		})
	}

	// Supersedes: add inferred relations with sufficient confidence.
	for _, sup := range inference.Supersedes {
		ref := strings.TrimSpace(sup.Ref)
		if ref == "" || sup.Confidence < 0.5 {
			continue
		}
		key := string(model.RelationSupersedes) + "\x00" + ref
		if _, exists := existingRelations[key]; exists {
			continue
		}
		existingRelations[key] = struct{}{}
		spec.Relations = append(spec.Relations, model.Relation{
			Type: model.RelationSupersedes,
			Ref:  ref,
		})
	}

	// Recalculate overall confidence score.
	if spec.Inference != nil && len(spec.Inference.Fields) > 0 {
		total := 0.0
		for _, f := range spec.Inference.Fields {
			total += f.Score
		}
		spec.Inference.Score = total / float64(len(spec.Inference.Fields))
		spec.Inference.Level = model.ConfidenceLevelFromScore(spec.Inference.Score)

		hasModelField := false
		for _, f := range spec.Inference.Fields {
			if f.Source == inferenceSource {
				hasModelField = true
				break
			}
		}
		if hasModelField {
			spec.Inference.Reasons = append(spec.Inference.Reasons, "some fields inferred by analysis model")
		}
	}
}

// updateInferenceField updates an existing field in the inference confidence.
func updateInferenceField(confidence *model.InferenceConfidence, name, source string, score float64) {
	if confidence == nil {
		return
	}
	for i := range confidence.Fields {
		if confidence.Fields[i].Name == name {
			confidence.Fields[i].Source = source
			confidence.Fields[i].Score = score
			confidence.Fields[i].Level = model.ConfidenceLevelFromScore(score)
			return
		}
	}
	addInferenceField(confidence, name, source, score)
}

// addInferenceField appends a new field to the inference confidence.
func addInferenceField(confidence *model.InferenceConfidence, name, source string, score float64) {
	if confidence == nil {
		return
	}
	confidence.Fields = append(confidence.Fields, model.InferenceFieldConfidence{
		Name:   name,
		Source: source,
		Score:  score,
		Level:  model.ConfidenceLevelFromScore(score),
	})
}
