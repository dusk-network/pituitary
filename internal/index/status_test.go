package index

import (
	"context"
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

func TestReadStatusReportsInferAppliesToEnabled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		tomlFlag string
		want     bool
	}{
		{name: "enabled", tomlFlag: "true", want: true},
		{name: "disabled", tomlFlag: "false", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			indexPath := filepath.Join(dir, ".pituitary", "pituitary.db")

			configContent := `
[workspace]
root = "` + filepath.ToSlash(dir) + `"
index_path = "` + filepath.ToSlash(indexPath) + `"
infer_applies_to = ` + tc.tomlFlag + `

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`
			mustWriteFile(t, filepath.Join(dir, "pituitary.toml"), configContent)
			mustWriteFile(t, filepath.Join(dir, "specs", "rate-limit", "spec.toml"), `id = "SPEC-042"
title = "Rate Limiting"
status = "accepted"
domain = "api"
authors = ["test"]
body = "body.md"
`)
			mustWriteFile(t, filepath.Join(dir, "specs", "rate-limit", "body.md"), "body text\n")

			cfg, err := config.Load(filepath.Join(dir, "pituitary.toml"))
			if err != nil {
				t.Fatalf("config.Load: %v", err)
			}
			records, err := source.LoadFromConfig(cfg)
			if err != nil {
				t.Fatalf("LoadFromConfig: %v", err)
			}
			if _, err := Rebuild(cfg, records); err != nil {
				t.Fatalf("Rebuild: %v", err)
			}

			status, err := ReadStatus(cfg.Workspace.ResolvedIndexPath)
			if err != nil {
				t.Fatalf("ReadStatus: %v", err)
			}
			if status.InferAppliesToEnabled == nil {
				t.Fatalf("status.InferAppliesToEnabled = nil, want %v", tc.want)
			}
			if *status.InferAppliesToEnabled != tc.want {
				t.Errorf("status.InferAppliesToEnabled = %v, want %v", *status.InferAppliesToEnabled, tc.want)
			}
		})
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

func TestReadStatusReportsGovernanceHotspots(t *testing.T) {
	t.Parallel()

	cfg := loadGovernanceHotspotFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	insertStatusEdge(t, cfg.Workspace.ResolvedIndexPath, "SPEC-100", "code://src/service/weak.go", "inferred", "inferred", 0.7)
	insertStatusEdge(t, cfg.Workspace.ResolvedIndexPath, "SPEC-200", "code://src/service/weak.go", "inferred", "ambiguous", 0.5)

	status, err := ReadStatus(cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("ReadStatus() error = %v", err)
	}
	if status.GovernanceHotspots == nil {
		t.Fatal("status.GovernanceHotspots = nil, want hotspot summary")
	}

	fanOut, ok := findGovernanceSpecHotspot(status.GovernanceHotspots.HighFanOutSpecs, "SPEC-300")
	if !ok {
		t.Fatalf("high fan-out specs = %+v, want SPEC-300", status.GovernanceHotspots.HighFanOutSpecs)
	}
	if got, want := fanOut.AppliesToCount, 4; got != want {
		t.Fatalf("SPEC-300 applies_to_count = %d, want %d", got, want)
	}

	weakLink, ok := findGovernanceArtifactHotspot(status.GovernanceHotspots.WeakLinkArtifacts, "code://src/service/weak.go")
	if !ok {
		t.Fatalf("weak link artifacts = %+v, want weak.go", status.GovernanceHotspots.WeakLinkArtifacts)
	}
	if weakLink.ExtractedEdgeCount != 0 || weakLink.InferredEdgeCount != 1 || weakLink.AmbiguousEdgeCount != 1 {
		t.Fatalf("weak link hotspot = %+v, want 0 extracted / 1 inferred / 1 ambiguous", weakLink)
	}

	multiGoverned, ok := findGovernanceArtifactHotspot(status.GovernanceHotspots.MultiGovernedArtifacts, "code://src/service/handler.go")
	if !ok {
		t.Fatalf("multi-governed artifacts = %+v, want handler.go", status.GovernanceHotspots.MultiGovernedArtifacts)
	}
	if got, want := multiGoverned.GoverningSpecCount, 2; got != want {
		t.Fatalf("handler.go governing_spec_count = %d, want %d", got, want)
	}
	if got, want := multiGoverned.GoverningSpecs, []string{"SPEC-100", "SPEC-200"}; !equalStrings(got, want) {
		t.Fatalf("handler.go governing_specs = %#v, want %#v", got, want)
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
# Multi-repo coverage test does not exercise inference; pin off.
infer_applies_to = false

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

func loadGovernanceHotspotFixtureConfig(tb testing.TB) *config.Config {
	tb.Helper()

	root := tb.TempDir()
	indexPath := filepath.Join(root, "pituitary.db")
	configPath := filepath.Join(root, "pituitary.toml")
	mustWriteFile(tb, configPath, fmt.Sprintf(`
[workspace]
root = "%s"
index_path = "%s"
# Hotspot test does not exercise inference; pin off.
infer_applies_to = false

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
`, filepath.ToSlash(root), filepath.ToSlash(indexPath)))
	mustWriteFile(tb, filepath.Join(root, "specs", "spec-100", "spec.toml"), `
id = "SPEC-100"
title = "Handler Governance"
status = "accepted"
domain = "api"
body = "body.md"
applies_to = [
  "code://src/service/handler.go",
  "code://src/service/shared.go",
]
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "spec-100", "body.md"), `
# Handler Governance

Shared handler behavior is explicitly governed.
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "spec-200", "spec.toml"), `
id = "SPEC-200"
title = "Worker Governance"
status = "accepted"
domain = "api"
body = "body.md"
applies_to = [
  "code://src/service/handler.go",
  "code://src/service/worker.go",
]
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "spec-200", "body.md"), `
# Worker Governance

Worker behavior overlaps on the shared handler path.
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "spec-300", "spec.toml"), `
id = "SPEC-300"
title = "Fanout Governance"
status = "accepted"
domain = "api"
body = "body.md"
applies_to = [
  "code://src/service/fanout-a.go",
  "code://src/service/fanout-b.go",
  "code://src/service/fanout-c.go",
  "code://src/service/fanout-d.go",
]
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "spec-300", "body.md"), `
# Fanout Governance

This spec governs a broader surface.
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}

func insertStatusEdge(tb testing.TB, indexPath, fromRef, toRef, edgeSource, confidence string, confidenceScore float64) {
	tb.Helper()

	db, err := openReadWriteContext(context.Background(), indexPath)
	if err != nil {
		tb.Fatalf("openReadWriteContext() error = %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(
		context.Background(),
		`INSERT OR REPLACE INTO edges (from_ref, to_ref, edge_type, edge_source, confidence, confidence_score) VALUES (?, ?, 'applies_to', ?, ?, ?)`,
		fromRef,
		toRef,
		edgeSource,
		confidence,
		confidenceScore,
	); err != nil {
		tb.Fatalf("insert hotspot edge %s -> %s: %v", fromRef, toRef, err)
	}
}

func findGovernanceSpecHotspot(items []GovernanceSpecHotspot, ref string) (GovernanceSpecHotspot, bool) {
	for _, item := range items {
		if item.Ref == ref {
			return item, true
		}
	}
	return GovernanceSpecHotspot{}, false
}

func findGovernanceArtifactHotspot(items []GovernanceArtifactHotspot, ref string) (GovernanceArtifactHotspot, bool) {
	for _, item := range items {
		if item.Ref == ref {
			return item, true
		}
	}
	return GovernanceArtifactHotspot{}, false
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
