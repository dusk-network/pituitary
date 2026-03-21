package source

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dusk-network/pituitary/internal/config"
)

// PreviewResult describes which items each configured source will contribute.
type PreviewResult struct {
	Sources []SourcePreview `json:"sources"`
}

// SourcePreview describes one configured source and the paths it contributes.
type SourcePreview struct {
	Name      string        `json:"name"`
	Adapter   string        `json:"adapter"`
	Kind      string        `json:"kind"`
	Path      string        `json:"path"`
	Include   []string      `json:"include,omitempty"`
	Exclude   []string      `json:"exclude,omitempty"`
	ItemCount int           `json:"item_count"`
	Items     []PreviewItem `json:"items"`
}

// PreviewItem describes one workspace-relative path that would be indexed.
type PreviewItem struct {
	ArtifactKind string `json:"artifact_kind"`
	Path         string `json:"path"`
}

// PreviewFromConfig enumerates source items without rebuilding the index.
func PreviewFromConfig(cfg *config.Config) (*PreviewResult, error) {
	result := &PreviewResult{
		Sources: make([]SourcePreview, 0, len(cfg.Sources)),
	}

	for _, source := range cfg.Sources {
		preview, err := previewSource(cfg.Workspace.RootPath, source)
		if err != nil {
			return nil, err
		}
		result.Sources = append(result.Sources, preview)
	}

	return result, nil
}

func previewSource(workspaceRoot string, source config.Source) (SourcePreview, error) {
	preview := SourcePreview{
		Name:    source.Name,
		Adapter: source.Adapter,
		Kind:    source.Kind,
		Path:    source.Path,
		Include: append([]string(nil), source.Include...),
		Exclude: append([]string(nil), source.Exclude...),
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

			preview.Items = append(preview.Items, PreviewItem{
				ArtifactKind: "doc",
				Path:         workspaceRelative(workspaceRoot, path),
			})
			return nil
		})
		if err != nil {
			return SourcePreview{}, err
		}
	default:
		return SourcePreview{}, fmt.Errorf("source %q: unsupported kind %q", source.Name, source.Kind)
	}

	preview.ItemCount = len(preview.Items)
	return preview, nil
}
