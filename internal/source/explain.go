package source

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"

	"github.com/dusk-network/pituitary/internal/config"
)

const (
	explainReasonOutsideSourceRoot      = "outside_source_root"
	explainReasonExcludedBySelector     = "excluded_by_selector"
	explainReasonNotMatchedByInclude    = "not_matched_by_include"
	explainReasonNotMarkdownDoc         = "not_markdown_doc"
	explainReasonIndexedMarkdownDoc     = "indexed_markdown_doc"
	explainReasonIndexedSpecBundle      = "indexed_spec_bundle"
	explainReasonBundleMemberNotIndexed = "bundle_member_not_indexed_directly"
	explainReasonNotInSpecBundle        = "not_in_spec_bundle"
	explainReasonNestedBundleConflict   = "nested_bundle_conflict"
)

// ExplainFileResult describes how a single file is treated across configured sources.
type ExplainFileResult struct {
	AbsolutePath  string                  `json:"absolute_path"`
	WorkspacePath string                  `json:"workspace_path,omitempty"`
	Summary       ExplainFileSummary      `json:"summary"`
	Sources       []SourceFileExplanation `json:"sources"`
}

// ExplainFileSummary provides a high-level classification outcome for the file.
type ExplainFileSummary struct {
	Status    string   `json:"status"`
	IndexedBy []string `json:"indexed_by,omitempty"`
}

// SourceFileExplanation describes how one source evaluates the target file.
type SourceFileExplanation struct {
	Name            string   `json:"name"`
	Kind            string   `json:"kind"`
	Path            string   `json:"path"`
	RelativePath    string   `json:"relative_path,omitempty"`
	UnderSourceRoot bool     `json:"under_source_root"`
	Selected        bool     `json:"selected"`
	ArtifactKind    string   `json:"artifact_kind,omitempty"`
	Reason          string   `json:"reason"`
	IncludeMatches  []string `json:"include_matches,omitempty"`
	ExcludeMatches  []string `json:"exclude_matches,omitempty"`
	BundlePath      string   `json:"bundle_path,omitempty"`
	ConflictsWith   string   `json:"conflicts_with,omitempty"`
}

type sourcePathSelection struct {
	Selected       bool
	Reason         string
	IncludeMatches []string
	ExcludeMatches []string
}

// ExplainFile reports how the configured sources classify one concrete file path.
func ExplainFile(cfg *config.Config, path string) (*ExplainFileResult, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	info, err := os.Stat(absolutePath)
	switch {
	case err != nil:
		return nil, fmt.Errorf("stat %s: %w", path, err)
	case info.IsDir():
		return nil, fmt.Errorf("%s is a directory", path)
	}

	result := &ExplainFileResult{
		AbsolutePath: absolutePath,
		Sources:      make([]SourceFileExplanation, 0, len(cfg.Sources)),
	}
	if pathWithinRoot(cfg.Workspace.RootPath, absolutePath) {
		result.WorkspacePath = workspaceRelative(cfg.Workspace.RootPath, absolutePath)
	}

	indexedBy := make([]string, 0, len(cfg.Sources))
	hasExcludedReason := false
	hasSourceCoverage := false
	for _, source := range cfg.Sources {
		explanation, err := explainFileInSource(cfg.Workspace.RootPath, source, absolutePath)
		if err != nil {
			return nil, err
		}
		if explanation.Selected {
			indexedBy = append(indexedBy, source.Name)
		}
		if explanation.UnderSourceRoot {
			hasSourceCoverage = true
		}
		if explanation.Reason == explainReasonExcludedBySelector || explanation.Reason == explainReasonNotMatchedByInclude {
			hasExcludedReason = true
		}
		result.Sources = append(result.Sources, explanation)
	}

	result.Summary.IndexedBy = indexedBy
	switch {
	case len(indexedBy) > 0:
		result.Summary.Status = "indexed"
	case hasExcludedReason:
		result.Summary.Status = "excluded"
	case hasSourceCoverage:
		result.Summary.Status = "not_indexed"
	default:
		result.Summary.Status = "outside_sources"
	}

	return result, nil
}

func explainFileInSource(workspaceRoot string, source config.Source, absolutePath string) (SourceFileExplanation, error) {
	explanation := SourceFileExplanation{
		Name: source.Name,
		Kind: source.Kind,
		Path: source.Path,
	}
	if !pathWithinRoot(source.ResolvedPath, absolutePath) {
		explanation.Reason = explainReasonOutsideSourceRoot
		return explanation, nil
	}

	relPath, err := filepath.Rel(source.ResolvedPath, absolutePath)
	if err != nil {
		return SourceFileExplanation{}, fmt.Errorf("source %q file %q: resolve relative path: %w", source.Name, workspaceRelative(workspaceRoot, absolutePath), err)
	}
	relPath = filepath.ToSlash(relPath)
	explanation.UnderSourceRoot = true
	explanation.RelativePath = relPath

	switch source.Kind {
	case config.SourceKindMarkdownDocs:
		return explainMarkdownDocSource(explanation, source, relPath)
	case config.SourceKindSpecBundle:
		return explainSpecBundleSource(workspaceRoot, explanation, source, absolutePath)
	default:
		return SourceFileExplanation{}, fmt.Errorf("source %q: unsupported kind %q", source.Name, source.Kind)
	}
}

func explainMarkdownDocSource(explanation SourceFileExplanation, source config.Source, relPath string) (SourceFileExplanation, error) {
	selection, err := evaluateSourcePathSelection(source, relPath)
	if err != nil {
		return SourceFileExplanation{}, err
	}
	explanation.IncludeMatches = selection.IncludeMatches
	explanation.ExcludeMatches = selection.ExcludeMatches
	if filepath.Ext(relPath) != ".md" {
		explanation.Reason = explainReasonNotMarkdownDoc
		return explanation, nil
	}
	if !selection.Selected {
		explanation.Reason = selection.Reason
		return explanation, nil
	}

	explanation.Selected = true
	explanation.ArtifactKind = "doc"
	explanation.Reason = explainReasonIndexedMarkdownDoc
	return explanation, nil
}

func explainSpecBundleSource(workspaceRoot string, explanation SourceFileExplanation, source config.Source, absolutePath string) (SourceFileExplanation, error) {
	candidateDirs, err := discoverSpecBundleDirs(source.ResolvedPath)
	if err != nil {
		return SourceFileExplanation{}, err
	}

	targetBundleDir := findTargetSpecBundleDir(candidateDirs, absolutePath)
	if targetBundleDir == "" {
		explanation.Reason = explainReasonNotInSpecBundle
		return explanation, nil
	}

	targetBundleSpecPath := filepath.Join(targetBundleDir, "spec.toml")
	bundleSpecRel := filepath.ToSlash(filepath.Join(workspaceRelative(source.ResolvedPath, targetBundleDir), "spec.toml"))
	explanation.BundlePath = workspaceRelative(workspaceRoot, targetBundleSpecPath)

	selection, err := evaluateSourcePathSelection(source, bundleSpecRel)
	if err != nil {
		return SourceFileExplanation{}, err
	}
	explanation.IncludeMatches = selection.IncludeMatches
	explanation.ExcludeMatches = selection.ExcludeMatches
	if !selection.Selected {
		explanation.Reason = selection.Reason
		return explanation, nil
	}

	selectedDirs := make([]string, 0, len(candidateDirs))
	for _, bundleDir := range candidateDirs {
		relBundleSpec := filepath.ToSlash(filepath.Join(workspaceRelative(source.ResolvedPath, bundleDir), "spec.toml"))
		bundleSelection, err := evaluateSourcePathSelection(source, relBundleSpec)
		if err != nil {
			return SourceFileExplanation{}, err
		}
		if bundleSelection.Selected {
			selectedDirs = append(selectedDirs, bundleDir)
		}
	}

	if conflictDir := findBundleConflict(targetBundleDir, selectedDirs); conflictDir != "" {
		explanation.Reason = explainReasonNestedBundleConflict
		explanation.ConflictsWith = workspaceRelative(workspaceRoot, filepath.Join(conflictDir, "spec.toml"))
		return explanation, nil
	}

	if filepath.Base(absolutePath) != "spec.toml" {
		explanation.Reason = explainReasonBundleMemberNotIndexed
		return explanation, nil
	}

	explanation.Selected = true
	explanation.ArtifactKind = "spec"
	explanation.Reason = explainReasonIndexedSpecBundle
	return explanation, nil
}

func evaluateSourcePathSelection(source config.Source, relPath string) (sourcePathSelection, error) {
	selection := sourcePathSelection{
		Selected: true,
		Reason:   explainReasonIndexedMarkdownDoc,
	}
	relPath = filepath.ToSlash(relPath)

	if len(source.Include) > 0 {
		selection.Selected = false
		selection.Reason = explainReasonNotMatchedByInclude
		for _, pattern := range source.Include {
			ok, err := matchSourceSelector(pattern, relPath)
			if err != nil {
				return sourcePathSelection{}, err
			}
			if ok {
				selection.IncludeMatches = append(selection.IncludeMatches, pattern)
				selection.Selected = true
			}
		}
	}

	for _, pattern := range source.Exclude {
		ok, err := matchSourceSelector(pattern, relPath)
		if err != nil {
			return sourcePathSelection{}, err
		}
		if ok {
			selection.ExcludeMatches = append(selection.ExcludeMatches, pattern)
			selection.Selected = false
			selection.Reason = explainReasonExcludedBySelector
		}
	}

	if selection.Selected {
		selection.Reason = ""
	}
	return selection, nil
}

func matchSourceSelector(pattern, relPath string) (bool, error) {
	ok, err := pathpkg.Match(pattern, relPath)
	if err != nil {
		return false, fmt.Errorf("selector pattern %q is invalid: %w", pattern, err)
	}
	return ok, nil
}

func findTargetSpecBundleDir(candidateDirs []string, absolutePath string) string {
	if filepath.Base(absolutePath) == "spec.toml" {
		targetDir := filepath.Dir(absolutePath)
		for _, bundleDir := range candidateDirs {
			if bundleDir == targetDir {
				return targetDir
			}
		}
		return ""
	}

	var target string
	for _, bundleDir := range candidateDirs {
		if pathWithinRoot(bundleDir, absolutePath) {
			target = bundleDir
		}
	}
	return target
}

func findBundleConflict(targetBundleDir string, selectedDirs []string) string {
	for _, bundleDir := range selectedDirs {
		if bundleDir == targetBundleDir {
			continue
		}
		if isNestedBundle(bundleDir, targetBundleDir) || isNestedBundle(targetBundleDir, bundleDir) {
			return bundleDir
		}
	}
	return ""
}
