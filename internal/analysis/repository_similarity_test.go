package analysis

import (
	"path/filepath"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestShortlistScoresForEmbeddingDownranksHistoricalDocSections(t *testing.T) {
	t.Parallel()

	cfg := loadHistoricalRankingFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	repo, err := openAnalysisRepository(cfg)
	if err != nil {
		t.Fatalf("openAnalysisRepository() error = %v", err)
	}
	defer repo.Close()

	specs, err := repo.loadSelectedSpecs([]string{"SPEC-LOCALITY"})
	if err != nil {
		t.Fatalf("repo.loadSelectedSpecs() error = %v", err)
	}
	candidate := specs["SPEC-LOCALITY"]
	if len(candidate.Sections) == 0 {
		t.Fatal("candidate sections = empty, want indexed section")
	}

	scores, err := repo.shortlistScoresForEmbedding(candidate.Sections[0].Embedding, artifactShortlistQuery{
		Kind:  model.ArtifactKindDoc,
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("repo.shortlistScoresForEmbedding() error = %v", err)
	}
	currentScore, ok := scores["doc://guides/current-locality"]
	if !ok {
		t.Fatalf("doc scores = %+v, want active doc score present", scores)
	}
	historicalScore, ok := scores["doc://guides/locality-history"]
	if !ok {
		t.Fatalf("doc scores = %+v, want historical doc score present", scores)
	}
	if currentScore <= historicalScore {
		t.Fatalf("doc scores = %+v, want active doc ahead of historical provenance doc", scores)
	}
}

func TestShortlistScoresForEmbeddingDownranksHistoricalSpecSectionsForDocDrift(t *testing.T) {
	t.Parallel()

	cfg := loadHistoricalRankingFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	repo, err := openAnalysisRepository(cfg)
	if err != nil {
		t.Fatalf("openAnalysisRepository() error = %v", err)
	}
	defer repo.Close()

	docs, err := repo.loadSelectedDocs([]string{"doc://guides/current-locality"})
	if err != nil {
		t.Fatalf("repo.loadSelectedDocs() error = %v", err)
	}
	doc := docs["doc://guides/current-locality"]
	if len(doc.Sections) == 0 {
		t.Fatal("doc sections = empty, want indexed section")
	}

	scores, err := repo.shortlistScoresForEmbedding(doc.Sections[0].Embedding, artifactShortlistQuery{
		Kind:     model.ArtifactKindSpec,
		Statuses: []string{model.StatusAccepted},
		Limit:    2,
	})
	if err != nil {
		t.Fatalf("repo.shortlistScoresForEmbedding() error = %v", err)
	}
	currentScore, ok := scores["SPEC-LOCALITY"]
	if !ok {
		t.Fatalf("spec scores = %+v, want active spec score present", scores)
	}
	historicalScore, ok := scores["SPEC-LEGACY-CONTEXT"]
	if !ok {
		t.Fatalf("spec scores = %+v, want historical spec score present", scores)
	}
	if currentScore <= historicalScore {
		t.Fatalf("spec scores = %+v, want active spec ahead of historical provenance spec", scores)
	}
}

func loadHistoricalRankingFixtureConfig(tb testing.TB) *config.Config {
	tb.Helper()

	root := tb.TempDir()
	configPath := filepath.Join(root, "pituitary.toml")
	mustWriteFile(tb, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

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
include = ["guides/*.md"]
`)

	mustWriteFile(tb, filepath.Join(root, "specs", "locality-kernel", "spec.toml"), `
id = "SPEC-LOCALITY"
title = "Locality Kernel Contract"
status = "accepted"
domain = "runtime"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "locality-kernel", "body.md"), `
## Requirements

Locality continuity kernel semantics define the active runtime contract.
`)

	mustWriteFile(tb, filepath.Join(root, "specs", "legacy-context", "spec.toml"), `
id = "SPEC-LEGACY-CONTEXT"
title = "Locality Legacy Context"
status = "accepted"
domain = "runtime"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "legacy-context", "body.md"), `
## Historical provenance

Locality continuity kernel semantics defined earlier drafts and rollout history.
`)

	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "current-locality.md"), `
# Current Locality Guide

## Runtime Contract

Locality continuity kernel define the active runtime contract.
`)

	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "locality-history.md"), `
# Locality Guide

## Historical provenance

Locality continuity kernel semantics defined earlier drafts and rollout history.
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}
