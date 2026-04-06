package source

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
)

const (
	explainReasonOutsideSourceRoot       = "outside_source_root"
	explainReasonNotListedInFiles        = "not_listed_in_files"
	explainReasonNotMatchedByInclude     = "not_matched_by_include"
	explainReasonExcludedBySelector      = "excluded_by_selector"
	explainReasonNotMarkdownFile         = "not_markdown_file"
	explainReasonIndexedMarkdownDoc      = "indexed_markdown_doc"
	explainReasonIndexedMarkdownContract = "indexed_markdown_contract"
	explainReasonIndexedSpecBundle       = "indexed_spec_bundle"
	explainReasonBundleMemberNotIndexed  = "bundle_member_not_indexed_directly"
	explainReasonNotInSpecBundle         = "not_in_spec_bundle"
	explainReasonNestedBundleConflict    = "nested_bundle_conflict"
)

// ExplainFileResult describes how one file is evaluated by the configured sources.
type ExplainFileResult struct {
	AbsolutePath  string                  `json:"absolute_path"`
	WorkspacePath string                  `json:"workspace_path,omitempty"`
	RepoID        string                  `json:"repo_id,omitempty"`
	Summary       ExplainFileSummary      `json:"summary"`
	Sources       []SourceFileExplanation `json:"sources"`
}

// ExplainFileSummary provides a top-level classification for the target file.
type ExplainFileSummary struct {
	Status    string   `json:"status"`
	IndexedBy []string `json:"indexed_by,omitempty"`
}

// SourceFileExplanation reports how one source treats the target file.
type SourceFileExplanation struct {
	Name            string                 `json:"name"`
	Kind            string                 `json:"kind"`
	Path            string                 `json:"path"`
	RelativePath    string                 `json:"relative_path,omitempty"`
	UnderSourceRoot bool                   `json:"under_source_root"`
	Selected        bool                   `json:"selected"`
	ArtifactKind    string                 `json:"artifact_kind,omitempty"`
	Reason          string                 `json:"reason"`
	Files           []string               `json:"files,omitempty"`
	Include         []string               `json:"include,omitempty"`
	Exclude         []string               `json:"exclude,omitempty"`
	FilesMatched    []string               `json:"files_matched,omitempty"`
	IncludeMatches  []string               `json:"include_matches,omitempty"`
	ExcludeMatches  []string               `json:"exclude_matches,omitempty"`
	BundlePath      string                 `json:"bundle_path,omitempty"`
	ConflictsWith   string                 `json:"conflicts_with,omitempty"`
	InferredSpec    *ExplainedInferredSpec `json:"inferred_spec,omitempty"`
}

// ExplainedInferredSpec surfaces metadata Pituitary would infer from a markdown contract.
type ExplainedInferredSpec struct {
	Ref        string                     `json:"ref"`
	Title      string                     `json:"title"`
	Status     string                     `json:"status"`
	Domain     string                     `json:"domain,omitempty"`
	DependsOn  []string                   `json:"depends_on,omitempty"`
	Supersedes []string                   `json:"supersedes,omitempty"`
	RelatesTo  []string                   `json:"relates_to,omitempty"`
	AppliesTo  []string                   `json:"applies_to,omitempty"`
	Inference  *model.InferenceConfidence `json:"inference,omitempty"`
}

type sourcePathSelection struct {
	Selected       bool
	Reason         string
	FilesMatched   []string
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
		result.RepoID = config.PrimaryRepoID(cfg)
	} else {
		for _, repo := range cfg.Workspace.Repos {
			if pathWithinRoot(repo.RootPath, absolutePath) {
				result.RepoID = repo.ID
				result.WorkspacePath = repo.ID + ":" + workspaceRelative(repo.RootPath, absolutePath)
				break
			}
		}
	}

	indexedBy := make([]string, 0, len(cfg.Sources))
	hasExcludedReason := false
	hasSourceCoverage := false

	for _, source := range cfg.Sources {
		explanation, err := explainFileInSource(source.RepoRootPath, source, absolutePath)
		if err != nil {
			return nil, err
		}
		if explanation.Selected {
			indexedBy = append(indexedBy, source.Name)
		}
		if explanation.UnderSourceRoot {
			hasSourceCoverage = true
		}
		if explanation.Reason == explainReasonNotListedInFiles ||
			explanation.Reason == explainReasonNotMatchedByInclude ||
			explanation.Reason == explainReasonExcludedBySelector {
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
		Name:    source.Name,
		Kind:    source.Kind,
		Path:    source.Path,
		Files:   append([]string(nil), source.Files...),
		Include: append([]string(nil), source.Include...),
		Exclude: append([]string(nil), source.Exclude...),
	}
	if !pathWithinRoot(source.ResolvedPath, absolutePath) {
		explanation.Reason = explainReasonOutsideSourceRoot
		return explanation, nil
	}

	relPath, err := filepath.Rel(source.ResolvedPath, absolutePath)
	if err != nil {
		return SourceFileExplanation{}, fmt.Errorf(
			"source %q file %q: resolve relative path: %w",
			source.Name,
			workspaceRelative(workspaceRoot, absolutePath),
			err,
		)
	}
	relPath = filepath.ToSlash(relPath)
	explanation.UnderSourceRoot = true
	explanation.RelativePath = relPath

	switch source.Kind {
	case config.SourceKindMarkdownDocs:
		return explainMarkdownDocSource(explanation, source)
	case config.SourceKindMarkdownContract:
		return explainMarkdownContractSource(workspaceRoot, explanation, source, absolutePath)
	case config.SourceKindSpecBundle:
		return explainSpecBundleSource(workspaceRoot, explanation, source, absolutePath)
	default:
		return SourceFileExplanation{}, fmt.Errorf("source %q: unsupported kind %q", source.Name, source.Kind)
	}
}

func explainMarkdownDocSource(explanation SourceFileExplanation, source config.Source) (SourceFileExplanation, error) {
	selection, err := evaluateSourcePathSelection(source, explanation.RelativePath)
	if err != nil {
		return SourceFileExplanation{}, err
	}
	populateSelectionMatches(&explanation, selection)

	if filepath.Ext(explanation.RelativePath) != ".md" {
		explanation.Reason = explainReasonNotMarkdownFile
		return explanation, nil
	}
	if !selection.Selected {
		explanation.Reason = selection.Reason
		return explanation, nil
	}

	explanation.Selected = true
	explanation.ArtifactKind = model.ArtifactKindDoc
	explanation.Reason = explainReasonIndexedMarkdownDoc
	return explanation, nil
}

func explainMarkdownContractSource(workspaceRoot string, explanation SourceFileExplanation, source config.Source, absolutePath string) (SourceFileExplanation, error) {
	selection, err := evaluateSourcePathSelection(source, explanation.RelativePath)
	if err != nil {
		return SourceFileExplanation{}, err
	}
	populateSelectionMatches(&explanation, selection)

	if filepath.Ext(explanation.RelativePath) != ".md" {
		explanation.Reason = explainReasonNotMarkdownFile
		return explanation, nil
	}

	// #nosec G304 -- absolutePath is derived from the selected workspace source and explanation target.
	body, err := os.ReadFile(absolutePath)
	if err != nil {
		return SourceFileExplanation{}, fmt.Errorf(
			"source %q contract %q: read markdown: %w",
			source.Name,
			workspaceRelative(workspaceRoot, absolutePath),
			err,
		)
	}
	record, err := inferMarkdownContract(workspaceRoot, source, absolutePath, body)
	if err != nil {
		return SourceFileExplanation{}, fmt.Errorf(
			"source %q contract %q: %w",
			source.Name,
			workspaceRelative(workspaceRoot, absolutePath),
			err,
		)
	}
	explanation.InferredSpec = &ExplainedInferredSpec{
		Ref:        record.Ref,
		Title:      record.Title,
		Status:     record.Status,
		Domain:     record.Domain,
		DependsOn:  relationRefs(record.Relations, model.RelationDependsOn),
		Supersedes: relationRefs(record.Relations, model.RelationSupersedes),
		RelatesTo:  relationRefs(record.Relations, model.RelationRelatesTo),
		AppliesTo:  append([]string(nil), record.AppliesTo...),
		Inference:  record.Inference,
	}

	if !selection.Selected {
		explanation.Reason = selection.Reason
		return explanation, nil
	}

	explanation.Selected = true
	explanation.ArtifactKind = model.ArtifactKindSpec
	explanation.Reason = explainReasonIndexedMarkdownContract
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
	explanation.BundlePath = workspaceRelative(workspaceRoot, targetBundleSpecPath)

	relBundleSpec := filepath.ToSlash(filepath.Join(workspaceRelative(source.ResolvedPath, targetBundleDir), "spec.toml"))
	selection, err := evaluateSourcePathSelection(source, relBundleSpec)
	if err != nil {
		return SourceFileExplanation{}, err
	}
	populateSelectionMatches(&explanation, selection)
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
	explanation.ArtifactKind = model.ArtifactKindSpec
	explanation.Reason = explainReasonIndexedSpecBundle
	return explanation, nil
}

func evaluateSourcePathSelection(source config.Source, relPath string) (sourcePathSelection, error) {
	selection := sourcePathSelection{Selected: true}
	relPath = filepath.ToSlash(relPath)

	if len(source.Files) > 0 {
		selection.Selected = false
		selection.Reason = explainReasonNotListedInFiles
		for _, file := range source.Files {
			if filepath.ToSlash(strings.TrimSpace(file)) == relPath {
				selection.Selected = true
				selection.Reason = ""
				selection.FilesMatched = append(selection.FilesMatched, file)
				break
			}
		}
		if !selection.Selected {
			return selection, nil
		}
	}

	if len(source.Include) > 0 {
		selection.Selected = false
		selection.Reason = explainReasonNotMatchedByInclude
		for _, pattern := range source.Include {
			ok, err := matchSourceSelector("include", pattern, relPath)
			if err != nil {
				return sourcePathSelection{}, err
			}
			if ok {
				selection.IncludeMatches = append(selection.IncludeMatches, pattern)
				selection.Selected = true
				selection.Reason = ""
			}
		}
		if !selection.Selected {
			return selection, nil
		}
	}

	for _, pattern := range source.Exclude {
		ok, err := matchSourceSelector("exclude", pattern, relPath)
		if err != nil {
			return sourcePathSelection{}, err
		}
		if ok {
			selection.ExcludeMatches = append(selection.ExcludeMatches, pattern)
			selection.Selected = false
			selection.Reason = explainReasonExcludedBySelector
		}
	}

	return selection, nil
}

func matchSourceSelector(kind, pattern, relPath string) (bool, error) {
	ok, err := pathpkg.Match(pattern, relPath)
	if err != nil {
		return false, fmt.Errorf("%s pattern %q is invalid: %w", kind, pattern, err)
	}
	return ok, nil
}

func populateSelectionMatches(explanation *SourceFileExplanation, selection sourcePathSelection) {
	explanation.FilesMatched = append([]string(nil), selection.FilesMatched...)
	explanation.IncludeMatches = append([]string(nil), selection.IncludeMatches...)
	explanation.ExcludeMatches = append([]string(nil), selection.ExcludeMatches...)
}

func relationRefs(relations []model.Relation, typ model.RelationType) []string {
	refs := make([]string, 0, len(relations))
	for _, relation := range relations {
		if relation.Type == typ {
			refs = append(refs, relation.Ref)
		}
	}
	return refs
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
		if !pathWithinRoot(bundleDir, absolutePath) {
			continue
		}
		if len(bundleDir) > len(target) {
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
