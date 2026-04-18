package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRuntimeChunkingDefaultsToZero(t *testing.T) {
	t.Parallel()

	cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"
`)

	if !cfg.Runtime.Chunking.IsZero() {
		t.Fatalf("runtime.chunking should be zero by default; got %+v", cfg.Runtime.Chunking)
	}
}

func TestLoadRuntimeChunkingParsesSpecAndDoc(t *testing.T) {
	t.Parallel()

	cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.chunking.spec]
policy = "markdown"
max_tokens = 512
overlap_tokens = 32
max_sections = 128

[runtime.chunking.doc]
policy = "late_chunk"
max_tokens = 2048
child_max_tokens = 512
child_overlap_tokens = 64
max_sections = 256
`)

	spec := cfg.Runtime.Chunking.Spec
	if spec.Policy != ChunkPolicyMarkdown || spec.MaxTokens != 512 || spec.OverlapTokens != 32 || spec.MaxSections != 128 {
		t.Fatalf("spec chunking = %+v", spec)
	}
	doc := cfg.Runtime.Chunking.Doc
	if doc.Policy != ChunkPolicyLateChunk || doc.MaxTokens != 2048 || doc.ChildMaxTokens != 512 || doc.ChildOverlapTokens != 64 || doc.MaxSections != 256 {
		t.Fatalf("doc chunking = %+v", doc)
	}
}

func TestLoadRuntimeChunkingRejectsLateChunkWithoutChildMaxTokens(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.chunking.doc]
policy = "late_chunk"
`)
	if err == nil {
		t.Fatal("expected error when late_chunk has no child_max_tokens")
	}
	if !strings.Contains(err.Error(), "child_max_tokens") {
		t.Fatalf("error should mention child_max_tokens; got %v", err)
	}
}

func TestLoadRuntimeChunkingRejectsUnknownPolicy(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.chunking.spec]
policy = "semantic"
`)
	if err == nil {
		t.Fatal("expected error for unknown policy")
	}
	if !strings.Contains(err.Error(), "unsupported policy") {
		t.Fatalf("error should mention unsupported policy; got %v", err)
	}
}

func TestLoadRuntimeChunkingRejectsUnknownKindWithSuggestion(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.chunking.spce]
policy = "markdown"
max_tokens = 512
`)
	if err == nil {
		t.Fatal("expected error for misspelled kind name")
	}
	if !strings.Contains(err.Error(), "spce") {
		t.Fatalf("error should name the misspelled kind; got %v", err)
	}
	if !strings.Contains(err.Error(), `"spec"`) {
		t.Fatalf("error should suggest spec/doc rather than field names; got %v", err)
	}
}

func TestLoadRuntimeChunkingRejectsUnknownField(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.chunking.spec]
policy = "markdown"
maks_tokens = 512
`)
	if err == nil {
		t.Fatal("expected error for misspelled field")
	}
	if !strings.Contains(err.Error(), "maks_tokens") {
		t.Fatalf("error should name the misspelled field; got %v", err)
	}
	if !strings.Contains(err.Error(), "max_tokens") {
		t.Fatalf("error should suggest the correct field; got %v", err)
	}
}

func TestLoadRuntimeChunkingParsesContextualizer(t *testing.T) {
	t.Parallel()

	cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.chunking.contextualizer]
format = "title_ancestry"
`)

	if got := cfg.Runtime.Chunking.Contextualizer.Format; got != ChunkContextualizerFormatTitleAncestry {
		t.Fatalf("contextualizer.format = %q, want %q", got, ChunkContextualizerFormatTitleAncestry)
	}
	if cfg.Runtime.Chunking.Contextualizer.IsZero() {
		t.Fatal("contextualizer should not be IsZero once format is set")
	}
}

func TestLoadRuntimeChunkingRejectsUnknownContextualizerFormat(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.chunking.contextualizer]
format = "semantic"
`)
	if err == nil {
		t.Fatal("expected error for unsupported contextualizer format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("error should mention unsupported format; got %v", err)
	}
	if !strings.Contains(err.Error(), `"title_ancestry"`) {
		t.Fatalf("error should list supported formats; got %v", err)
	}
}

func TestLoadRuntimeChunkingRejectsUnknownContextualizerField(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.chunking.contextualizer]
formst = "title_ancestry"
`)
	if err == nil {
		t.Fatal("expected error for misspelled contextualizer field")
	}
	if !strings.Contains(err.Error(), "formst") {
		t.Fatalf("error should name the misspelled field; got %v", err)
	}
	if !strings.Contains(err.Error(), "format") {
		t.Fatalf("error should suggest the correct field; got %v", err)
	}
}

func TestLoadRuntimeChunkingRejectsUnknownChunkingSubtree(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.chunking.gadget]
policy = "markdown"
`)
	if err == nil {
		t.Fatal("expected error for unknown subtree under runtime.chunking")
	}
	if !strings.Contains(err.Error(), `"contextualizer"`) {
		t.Fatalf("error should suggest contextualizer alongside spec/doc; got %v", err)
	}
}

func TestRenderRoundTripPreservesContextualizer(t *testing.T) {
	t.Parallel()

	// Regression: the first #343 drop shipped with ChunkingConfig
	// carrying Contextualizer but Render omitting the block, which
	// meant any render-based workflow silently downgraded the next
	// load back to the no-contextualizer path. This test guards that
	// an enabled contextualizer survives Render -> Load unchanged.
	cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.chunking.contextualizer]
format = "title_ancestry"
`)

	rendered, err := Render(cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(rendered, "[runtime.chunking.contextualizer]") {
		t.Fatalf("rendered output missing contextualizer block:\n%s", rendered)
	}
	if !strings.Contains(rendered, `format = "title_ancestry"`) {
		t.Fatalf("rendered output missing format value:\n%s", rendered)
	}

	round := loadRenderedConfig(t, rendered)
	if got := round.Runtime.Chunking.Contextualizer.Format; got != ChunkContextualizerFormatTitleAncestry {
		t.Fatalf("round-tripped contextualizer.format = %q, want %q", got, ChunkContextualizerFormatTitleAncestry)
	}
}

// loadRenderedConfig loads an already-complete rendered config body
// (one that already includes its own [[sources]] tables) without the
// fixture append loadChunkingFixture performs.
func loadRenderedConfig(t *testing.T, body string) *Config {
	t.Helper()
	repo := t.TempDir()
	workspace := filepath.Join(repo, "workspace")
	mustMkdirAll(t, filepath.Join(workspace, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, body)
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load round-trip: %v", err)
	}
	return cfg
}

func TestRenderRoundTripPreservesChunking(t *testing.T) {
	t.Parallel()

	cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.chunking.spec]
policy = "markdown"
max_tokens = 512

[runtime.chunking.doc]
policy = "late_chunk"
max_tokens = 2048
child_max_tokens = 512
`)

	rendered, err := Render(cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(rendered, "[runtime.chunking.spec]") {
		t.Fatalf("rendered output missing spec chunking block:\n%s", rendered)
	}
	if !strings.Contains(rendered, "[runtime.chunking.doc]") {
		t.Fatalf("rendered output missing doc chunking block:\n%s", rendered)
	}
	if !strings.Contains(rendered, `policy = "late_chunk"`) {
		t.Fatalf("rendered output missing late_chunk policy value:\n%s", rendered)
	}
	if !strings.Contains(rendered, "child_max_tokens = 512") {
		t.Fatalf("rendered output missing child_max_tokens:\n%s", rendered)
	}
}

func loadChunkingFixture(t *testing.T, body string) *Config {
	t.Helper()
	cfg, err := loadChunkingFixtureErr(t, body)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

func loadChunkingFixtureErr(t *testing.T, body string) (*Config, error) {
	t.Helper()
	repo := t.TempDir()
	workspace := filepath.Join(repo, "workspace")
	mustMkdirAll(t, filepath.Join(workspace, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, body+`
[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	return Load(configPath)
}
