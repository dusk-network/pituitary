package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/diag"
	"github.com/dusk-network/pituitary/sdk"
)

// PreviewResult describes which items each configured source will contribute.
type PreviewResult struct {
	Sources []SourcePreview `json:"sources"`
}

// SourcePreview describes one configured source and the paths it contributes.
type SourcePreview struct {
	Name          string                `json:"name"`
	Adapter       string                `json:"adapter"`
	Kind          string                `json:"kind"`
	Path          string                `json:"path"`
	ResolvedPath  string                `json:"resolved_path,omitempty"`
	Files         []string              `json:"files,omitempty"`
	Include       []string              `json:"include,omitempty"`
	Exclude       []string              `json:"exclude,omitempty"`
	CandidateCount int                  `json:"candidate_count,omitempty"`
	ItemCount     int                   `json:"item_count"`
	Items         []PreviewItem         `json:"items"`
	RejectedItems []PreviewRejectedItem `json:"rejected_items,omitempty"`
}

// PreviewItem describes one workspace-relative path that would be indexed.
type PreviewItem struct {
	ArtifactKind   string   `json:"artifact_kind"`
	Path           string   `json:"path"`
	FilesMatched   []string `json:"files_matched,omitempty"`
	IncludeMatches []string `json:"include_matches,omitempty"`
}

// PreviewRejectedItem describes one candidate path that was skipped by source selectors.
type PreviewRejectedItem struct {
	Path           string   `json:"path"`
	Reason         string   `json:"reason"`
	FilesMatched   []string `json:"files_matched,omitempty"`
	IncludeMatches []string `json:"include_matches,omitempty"`
	ExcludeMatches []string `json:"exclude_matches,omitempty"`
}

// PreviewOptions controls diagnostic behavior during source previews.
type PreviewOptions struct {
	Logger  *diag.Logger
	Verbose bool
}

// PreviewFromConfig enumerates source items without rebuilding the index.
func PreviewFromConfig(cfg *config.Config) (*PreviewResult, error) {
	return PreviewFromConfigWithOptions(cfg, PreviewOptions{})
}

// PreviewFromConfigWithOptions enumerates source items without rebuilding the index.
func PreviewFromConfigWithOptions(cfg *config.Config, options PreviewOptions) (*PreviewResult, error) {
	logger := options.Logger
	result := &PreviewResult{
		Sources: make([]SourcePreview, 0, len(cfg.Sources)),
	}

	for _, source := range cfg.Sources {
		preview, err := previewSource(cfg.Workspace.RootPath, source, options.Verbose)
		if err != nil {
			return nil, err
		}
		if preview.ItemCount == 0 {
			logger.Warnf("preview", "source %q (%s %s) would index 0 item(s)", source.Name, source.Kind, filepath.ToSlash(source.Path))
		} else {
			logger.Infof("preview", "source %q (%s %s) would index %d item(s)", source.Name, source.Kind, filepath.ToSlash(source.Path), preview.ItemCount)
		}
		result.Sources = append(result.Sources, preview)
	}

	return result, nil
}

func previewSource(workspaceRoot string, source config.Source, verbose bool) (SourcePreview, error) {
	preview := SourcePreview{
		Name:         source.Name,
		Adapter:      source.Adapter,
		Kind:         source.Kind,
		Path:         source.Path,
		ResolvedPath: filepath.ToSlash(source.ResolvedPath),
		Files:        append([]string(nil), source.Files...),
		Include:      append([]string(nil), source.Include...),
		Exclude:      append([]string(nil), source.Exclude...),
	}

	if source.Adapter != config.AdapterFilesystem {
		return previewViaAdapter(preview, source)
	}

	switch source.Kind {
	case config.SourceKindSpecBundle:
		bundleDirs, err := discoverSpecBundles(source)
		if err != nil {
			return SourcePreview{}, fmt.Errorf("source %q: %w", source.Name, err)
		}
		for _, bundleDir := range bundleDirs {
			specPath := filepath.Join(bundleDir, "spec.toml")
			preview.Items = append(preview.Items, PreviewItem{
				ArtifactKind: "spec",
				Path:         workspaceRelative(workspaceRoot, specPath),
			})
		}
	case config.SourceKindMarkdownDocs:
		matches, candidateCount, rejected, err := previewMarkdownPaths(workspaceRoot, source, "doc", verbose)
		if err != nil {
			return SourcePreview{}, err
		}
		preview.CandidateCount = candidateCount
		for _, match := range matches {
			preview.Items = append(preview.Items, PreviewItem{
				ArtifactKind:   "doc",
				Path:           workspaceRelative(workspaceRoot, match.AbsolutePath),
				FilesMatched:   append([]string(nil), match.Selection.FilesMatched...),
				IncludeMatches: append([]string(nil), match.Selection.IncludeMatches...),
			})
		}
		preview.RejectedItems = rejected
	case config.SourceKindMarkdownContract:
		matches, candidateCount, rejected, err := previewMarkdownPaths(workspaceRoot, source, "contract", verbose)
		if err != nil {
			return SourcePreview{}, err
		}
		preview.CandidateCount = candidateCount
		for _, match := range matches {
			preview.Items = append(preview.Items, PreviewItem{
				ArtifactKind:   "spec",
				Path:           workspaceRelative(workspaceRoot, match.AbsolutePath),
				FilesMatched:   append([]string(nil), match.Selection.FilesMatched...),
				IncludeMatches: append([]string(nil), match.Selection.IncludeMatches...),
			})
		}
		preview.RejectedItems = rejected
	default:
		return SourcePreview{}, fmt.Errorf("source %q: unsupported kind %q", source.Name, source.Kind)
	}

	preview.ItemCount = len(preview.Items)
	return preview, nil
}

type previewMarkdownPath struct {
	selectedMarkdownPath
	Selection sourcePathSelection
}

func previewMarkdownPaths(workspaceRoot string, source config.Source, label string, verbose bool) ([]previewMarkdownPath, int, []PreviewRejectedItem, error) {
	matches := make([]previewMarkdownPath, 0)
	rejected := make([]PreviewRejectedItem, 0)
	candidateCount := 0
	err := filepath.WalkDir(source.ResolvedPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		candidateCount++
		relPath, err := filepath.Rel(source.ResolvedPath, path)
		if err != nil {
			return fmt.Errorf("source %q %s %q: resolve relative path: %w", source.Name, label, workspaceRelative(workspaceRoot, path), err)
		}
		relPath = filepath.ToSlash(relPath)
		selection, err := evaluateSourcePathSelection(source, relPath)
		if err != nil {
			return fmt.Errorf("source %q %s %q: %w", source.Name, label, workspaceRelative(workspaceRoot, path), err)
		}
		if !selection.Selected {
			if verbose {
				rejected = append(rejected, PreviewRejectedItem{
					Path:           workspaceRelative(workspaceRoot, path),
					Reason:         selection.Reason,
					FilesMatched:   append([]string(nil), selection.FilesMatched...),
					IncludeMatches: append([]string(nil), selection.IncludeMatches...),
					ExcludeMatches: append([]string(nil), selection.ExcludeMatches...),
				})
			}
			return nil
		}
		matches = append(matches, previewMarkdownPath{
			selectedMarkdownPath: selectedMarkdownPath{
				AbsolutePath: path,
				RelativePath: relPath,
			},
			Selection: selection,
		})
		return nil
	})
	if err != nil {
		return nil, 0, nil, err
	}
	return matches, candidateCount, rejected, nil
}

func previewViaAdapter(preview SourcePreview, source config.Source) (SourcePreview, error) {
	factory := LookupAdapter(source.Adapter)
	if factory == nil {
		return SourcePreview{}, unknownAdapterError(source.Name, source.Adapter)
	}

	adapter := factory()
	previewer, ok := adapter.(sdk.Previewer)
	if !ok {
		return SourcePreview{}, fmt.Errorf("source %q: adapter %q does not support preview", source.Name, source.Adapter)
	}

	items, err := previewer.Preview(context.Background(), sdk.SourceConfig{
		Name:          source.Name,
		Adapter:       source.Adapter,
		Kind:          source.Kind,
		Repo:          source.ResolvedRepo,
		Path:          source.Path,
		Files:         append([]string(nil), source.Files...),
		Include:       append([]string(nil), source.Include...),
		Exclude:       append([]string(nil), source.Exclude...),
		Options:       config.CloneSourceOptions(source.Options),
		WorkspaceRoot: source.RepoRootPath,
		PrimaryRepoID: source.PrimaryRepo,
	})
	if err != nil {
		return SourcePreview{}, fmt.Errorf("source %q: %w", source.Name, err)
	}

	for _, item := range items {
		preview.Items = append(preview.Items, PreviewItem{
			ArtifactKind: item.ArtifactKind,
			Path:         item.Path,
		})
	}
	preview.ItemCount = len(preview.Items)
	return preview, nil
}
