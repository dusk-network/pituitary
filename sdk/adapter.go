package sdk

import "context"

// Adapter loads canonical records from one configured source.
type Adapter interface {
	Load(ctx context.Context, cfg SourceConfig) (*AdapterResult, error)
}

// AdapterResult is the canonical record set returned by one adapter.
type AdapterResult struct {
	Specs []SpecRecord
	Docs  []DocRecord
}

// Previewer enumerates the items an adapter would index without rebuilding.
// Adapters that don't support source previews may omit this interface.
type Previewer interface {
	Preview(ctx context.Context, cfg SourceConfig) ([]PreviewItem, error)
}

// PreviewItem describes one source item that an adapter would index.
type PreviewItem struct {
	ArtifactKind string `json:"artifact_kind"`
	Path         string `json:"path"`
}

// AdapterFactory creates an adapter instance.
type AdapterFactory func() Adapter

// SourceConfig is the adapter-facing subset of one configured source.
type SourceConfig struct {
	Name    string         `json:"name"`
	Adapter string         `json:"adapter"`
	Kind    string         `json:"kind"`
	Repo    string         `json:"repo,omitempty"`
	Path    string         `json:"path,omitempty"`
	Files   []string       `json:"files,omitempty"`
	Include []string       `json:"include,omitempty"`
	Exclude []string       `json:"exclude,omitempty"`
	Options map[string]any `json:"options,omitempty"`

	// WorkspaceRoot is the absolute workspace root path.
	WorkspaceRoot string `json:"-"`
	PrimaryRepoID string `json:"-"`
}
