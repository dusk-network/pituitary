package analysis

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestCheckOverlapDetectsKnownFixtureOverlap(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckOverlap(cfg, OverlapRequest{SpecRef: "SPEC-042"})
	if err != nil {
		t.Fatalf("CheckOverlap() error = %v", err)
	}
	if result.Candidate.Ref != "SPEC-042" {
		t.Fatalf("candidate ref = %q, want %q", result.Candidate.Ref, "SPEC-042")
	}
	if len(result.Overlaps) == 0 {
		t.Fatal("CheckOverlap() returned no overlaps")
	}
	if result.Overlaps[0].Ref != "SPEC-008" {
		t.Fatalf("top overlap = %+v, want SPEC-008 first", result.Overlaps[0])
	}
	if result.Overlaps[0].Relationship != "extends" {
		t.Fatalf("top overlap relationship = %q, want %q", result.Overlaps[0].Relationship, "extends")
	}
	if result.Recommendation != "proceed_with_supersedes" {
		t.Fatalf("recommendation = %q, want %q", result.Recommendation, "proceed_with_supersedes")
	}
}

func TestCheckOverlapSupportsDraftSpecRecord(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	var base model.SpecRecord
	for _, spec := range records.Specs {
		if spec.Ref == "SPEC-042" {
			base = spec
			break
		}
	}
	base.Ref = "SPEC-900"
	base.Title = "Draft Rate Limiting Update"
	base.Status = model.StatusDraft

	result, err := CheckOverlap(cfg, OverlapRequest{SpecRecord: &base})
	if err != nil {
		t.Fatalf("CheckOverlap() draft error = %v", err)
	}
	if len(result.Overlaps) == 0 {
		t.Fatal("CheckOverlap() draft returned no overlaps")
	}
	if result.Overlaps[0].Ref != "SPEC-042" {
		t.Fatalf("top draft overlap = %+v, want SPEC-042", result.Overlaps[0])
	}
}

func TestCheckOverlapContextHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = CheckOverlapContext(ctx, cfg, OverlapRequest{SpecRef: "SPEC-042"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CheckOverlapContext() error = %v, want context.Canceled", err)
	}
}

func loadFixtureConfig(tb testing.TB) *config.Config {
	tb.Helper()

	repoRoot := repoRoot(tb)
	indexPath := filepath.Join(tb.TempDir(), "pituitary.db")
	configPath := filepath.Join(tb.TempDir(), "pituitary.toml")
	mustWriteFile(tb, configPath, `
[workspace]
root = "`+filepath.ToSlash(repoRoot)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}

func repoRoot(tb testing.TB) string {
	tb.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		tb.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func mustWriteFile(tb testing.TB, path, content string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		tb.Fatalf("write %s: %v", path, err)
	}
}
