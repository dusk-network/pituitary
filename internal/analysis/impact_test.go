package analysis

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestAnalyzeImpactFindsDependentSpecsRefsAndDocs(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := AnalyzeImpact(cfg, AnalyzeImpactRequest{
		SpecRef:    "SPEC-042",
		ChangeType: "accepted",
	})
	if err != nil {
		t.Fatalf("AnalyzeImpact() error = %v", err)
	}
	if result.SpecRef != "SPEC-042" || result.ChangeType != "accepted" {
		t.Fatalf("result identity = %+v", result)
	}
	if len(result.AffectedSpecs) == 0 {
		t.Fatal("AnalyzeImpact() returned no affected specs")
	}
	if result.AffectedSpecs[0].Ref != "SPEC-055" {
		t.Fatalf("top affected spec = %+v, want SPEC-055 first", result.AffectedSpecs[0])
	}
	if len(result.AffectedRefs) == 0 {
		t.Fatal("AnalyzeImpact() returned no affected refs")
	}
	if len(result.AffectedDocs) == 0 {
		t.Fatal("AnalyzeImpact() returned no affected docs")
	}

	var foundGuide, foundRunbook bool
	for _, doc := range result.AffectedDocs {
		switch doc.Ref {
		case "doc://guides/api-rate-limits":
			foundGuide = true
		case "doc://runbooks/rate-limit-rollout":
			foundRunbook = true
		}
	}
	if !foundGuide || !foundRunbook {
		t.Fatalf("affected docs = %+v, want both fixture docs", result.AffectedDocs)
	}
}

func TestAnalyzeImpactSupportsDraftSpecRecord(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	var draft model.SpecRecord
	for _, spec := range records.Specs {
		if spec.Ref == "SPEC-042" {
			draft = spec
			break
		}
	}
	draft.Ref = "SPEC-900"
	draft.Title = "Draft Rate Limiting Update"
	draft.Status = model.StatusDraft

	result, err := AnalyzeImpact(cfg, AnalyzeImpactRequest{
		SpecRecord: &draft,
		ChangeType: "accepted",
	})
	if err != nil {
		t.Fatalf("AnalyzeImpact() draft error = %v", err)
	}
	if result.SpecRef != "SPEC-900" {
		t.Fatalf("result spec_ref = %q, want SPEC-900", result.SpecRef)
	}
	if len(result.AffectedRefs) == 0 || len(result.AffectedDocs) == 0 {
		t.Fatalf("draft impact = %+v, want refs and docs", result)
	}
}

func TestAnalyzeImpactIncludesCrossRepoArtifacts(t *testing.T) {
	t.Parallel()

	cfg := loadMultiRepoAnalysisConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := AnalyzeImpact(cfg, AnalyzeImpactRequest{
		SpecRef:    "SPEC-100",
		ChangeType: "accepted",
	})
	if err != nil {
		t.Fatalf("AnalyzeImpact() error = %v", err)
	}

	var foundSharedSpec, foundSharedDoc bool
	for _, spec := range result.AffectedSpecs {
		if spec.Ref == "SPEC-200" {
			foundSharedSpec = true
			if got, want := spec.Repo, "shared"; got != want {
				t.Fatalf("shared spec repo = %q, want %q", got, want)
			}
		}
	}
	for _, doc := range result.AffectedDocs {
		if doc.Ref == "doc://shared/guides/api-rate-limits" {
			foundSharedDoc = true
			if got, want := doc.Repo, "shared"; got != want {
				t.Fatalf("shared doc repo = %q, want %q", got, want)
			}
			if got, want := doc.SourceRef, "file://docs/guides/api-rate-limits.md"; got != want {
				t.Fatalf("shared doc source_ref = %q, want %q", got, want)
			}
		}
	}
	if !foundSharedSpec || !foundSharedDoc {
		t.Fatalf("impact = %+v, want shared repo spec and doc", result)
	}
}

func loadMultiRepoAnalysisConfig(tb testing.TB) *config.Config {
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
