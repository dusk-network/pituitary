package index

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestReadStatusReportsMissingIndex(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/pituitary.db"
	status, err := ReadStatus(path)
	if err != nil {
		t.Fatalf("ReadStatus() error = %v", err)
	}
	if status.IndexPath != path {
		t.Fatalf("index path = %q, want %q", status.IndexPath, path)
	}
	if status.Exists {
		t.Fatal("status.Exists = true, want false")
	}
	if status.SpecCount != 0 || status.DocCount != 0 || status.ChunkCount != 0 {
		t.Fatalf("counts = %+v, want zero counts for missing index", status)
	}
}

func TestReadStatusReadsFixtureCounts(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	status, err := ReadStatus(cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("ReadStatus() error = %v", err)
	}
	if !status.Exists {
		t.Fatal("status.Exists = false, want true")
	}
	if status.SpecCount != 3 || status.DocCount != 2 || status.ChunkCount != 17 {
		t.Fatalf("status = %+v, want 3 specs, 2 docs, 17 chunks", status)
	}
}

func TestReadStatusReportsRepoCoverage(t *testing.T) {
	t.Parallel()

	cfg := loadMultiRepoFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	status, err := ReadStatus(cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("ReadStatus() error = %v", err)
	}
	if got, want := len(status.Repos), 2; got != want {
		t.Fatalf("repo coverage = %+v, want %d repos", status.Repos, want)
	}

	repos := map[string]RepoCoverage{}
	for _, repo := range status.Repos {
		repos[repo.Repo] = repo
	}
	for _, repoID := range []string{"primary", "shared"} {
		repo, ok := repos[repoID]
		if !ok {
			t.Fatalf("repo coverage = %+v, want repo %q", status.Repos, repoID)
		}
		if got, want := repo.ItemCount, 2; got != want {
			t.Fatalf("repo %q item_count = %d, want %d", repoID, got, want)
		}
		if got, want := repo.SpecCount, 1; got != want {
			t.Fatalf("repo %q spec_count = %d, want %d", repoID, got, want)
		}
		if got, want := repo.DocCount, 1; got != want {
			t.Fatalf("repo %q doc_count = %d, want %d", repoID, got, want)
		}
	}
}

func loadMultiRepoFixtureConfig(tb testing.TB) *config.Config {
	tb.Helper()

	root := tb.TempDir()
	primary := filepath.Join(root, "primary")
	shared := filepath.Join(root, "shared")
	indexPath := filepath.Join(root, "pituitary.db")
	configPath := filepath.Join(root, "pituitary.toml")
	mustWriteFile(tb, configPath, fmt.Sprintf(`
[workspace]
root = "%s"
repo_id = "primary"
index_path = "%s"

[[workspace.repos]]
id = "shared"
root = "%s"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "primary-specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "primary-docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]

[[sources]]
name = "shared-specs"
adapter = "filesystem"
kind = "spec_bundle"
repo = "shared"
path = "specs"

[[sources]]
name = "shared-docs"
adapter = "filesystem"
kind = "markdown_docs"
repo = "shared"
path = "docs"
include = ["guides/*.md"]
`, filepath.ToSlash(primary), filepath.ToSlash(indexPath), filepath.ToSlash(shared)))
	mustWriteFile(tb, filepath.Join(primary, "specs", "tenant-rate-limits", "spec.toml"), `
id = "SPEC-100"
title = "Tenant Rate Limits"
status = "accepted"
domain = "api"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(primary, "specs", "tenant-rate-limits", "body.md"), `
# Tenant Rate Limits

## Defaults

The default rate limit is 200 requests per minute.

## Rollout

All consumers must keep tenant-scoped defaults aligned.
`)
	mustWriteFile(tb, filepath.Join(primary, "docs", "guides", "api-rate-limits.md"), `
# API Rate Limits

The default rate limit is 200 requests per minute.
`)
	mustWriteFile(tb, filepath.Join(shared, "specs", "shared-rollout", "spec.toml"), `
id = "SPEC-200"
title = "Shared Repo Rollout"
status = "accepted"
domain = "api"
body = "body.md"
depends_on = ["SPEC-100"]
`)
	mustWriteFile(tb, filepath.Join(shared, "specs", "shared-rollout", "body.md"), `
# Shared Repo Rollout

## Dependencies

This rollout depends on SPEC-100.

## Tasks

Update shared consumers to respect the 200 requests per minute tenant default.
`)
	mustWriteFile(tb, filepath.Join(shared, "docs", "guides", "api-rate-limits.md"), `
# API Rate Limits

The default rate limit is 100 requests per minute.
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}
