package index

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestSearchSpecsReturnsRankedSections(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	result, err := SearchSpecs(cfg, SearchSpecQuery{Query: "rate limiting", Limit: 5})
	if err != nil {
		t.Fatalf("SearchSpecs() error = %v", err)
	}
	if len(result.Matches) == 0 {
		t.Fatal("SearchSpecs() returned no matches")
	}
	if result.Matches[0].Ref == "" || result.Matches[0].SectionHeading == "" {
		t.Fatalf("top match = %+v, want stable ref and section heading", result.Matches[0])
	}

	var found042 bool
	for _, match := range result.Matches {
		if match.Kind != model.ArtifactKindSpec {
			t.Fatalf("match kind = %q, want %q", match.Kind, model.ArtifactKindSpec)
		}
		if match.Status == model.StatusSuperseded || match.Status == model.StatusDeprecated {
			t.Fatalf("default search unexpectedly returned inactive status %q in %+v", match.Status, match)
		}
		if match.Ref == "SPEC-042" {
			found042 = true
		}
	}
	if !found042 {
		t.Fatalf("SearchSpecs() matches = %+v, want SPEC-042 among results", result.Matches)
	}
}

func TestSearchSpecsDefaultExcludesSupersededUnlessRequested(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	defaultResult, err := SearchSpecs(cfg, SearchSpecQuery{Query: "fixed window rate limiting", Limit: 5})
	if err != nil {
		t.Fatalf("SearchSpecs() default error = %v", err)
	}
	for _, match := range defaultResult.Matches {
		if match.Ref == "SPEC-008" {
			t.Fatalf("default search unexpectedly returned superseded spec: %+v", match)
		}
	}

	historicalResult, err := SearchSpecs(cfg, SearchSpecQuery{
		Query:    "fixed window rate limiting",
		Statuses: []string{model.StatusSuperseded},
		Limit:    5,
	})
	if err != nil {
		t.Fatalf("SearchSpecs() superseded error = %v", err)
	}
	if len(historicalResult.Matches) == 0 {
		t.Fatal("SearchSpecs() with superseded filter returned no matches")
	}
	if historicalResult.Matches[0].Ref != "SPEC-008" {
		t.Fatalf("top historical match = %+v, want SPEC-008", historicalResult.Matches[0])
	}
}

func TestSearchSpecsAppliesDomainFilter(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	result, err := SearchSpecs(cfg, SearchSpecQuery{
		Query:  "rate limiting",
		Domain: "payments",
	})
	if err != nil {
		t.Fatalf("SearchSpecs() error = %v", err)
	}
	if len(result.Matches) != 0 {
		t.Fatalf("SearchSpecs() matches = %+v, want no matches for unrelated domain", result.Matches)
	}
}

func TestSearchSpecsRejectsInvalidStatuses(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	_, err = SearchSpecs(cfg, SearchSpecQuery{
		Query:    "rate limiting",
		Statuses: []string{"invalid"},
	})
	if err == nil || err.Error() != `unsupported status "invalid"` {
		t.Fatalf("SearchSpecs() error = %v, want unsupported status", err)
	}
}

func TestSearchSpecsRejectsLimitAboveMaximum(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	_, err = SearchSpecs(cfg, SearchSpecQuery{
		Query: "rate limiting",
		Limit: maxSearchLimit + 1,
	})
	if err == nil || err.Error() != `limit must be less than or equal to 50` {
		t.Fatalf("SearchSpecs() error = %v, want maximum-limit validation", err)
	}
}

func TestSearchSpecsContextHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = SearchSpecsContext(ctx, cfg, SearchSpecQuery{Query: "rate limiting", Limit: 5})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SearchSpecsContext() error = %v, want context.Canceled", err)
	}
}

func TestSearchSpecsIndexesMarkdownContracts(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	configPath := filepath.Join(repo, "pituitary.toml")
	mustWriteFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "contracts"
adapter = "filesystem"
kind = "markdown_contract"
path = "contracts"
`)
	mustWriteFile(t, filepath.Join(repo, "contracts", "auth", "session-policy.md"), `
Ref: RFC-AUTH-001
Status: accepted
Domain: identity

# Session Policy

Interactive authentication sessions must use tenant-scoped policy checks and sliding-window enforcement.
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	result, err := SearchSpecs(cfg, SearchSpecQuery{
		Query:    "tenant-scoped authentication sessions",
		Statuses: []string{model.StatusAccepted},
		Limit:    5,
	})
	if err != nil {
		t.Fatalf("SearchSpecs() error = %v", err)
	}
	if len(result.Matches) == 0 {
		t.Fatal("SearchSpecs() returned no matches")
	}
	if got, want := result.Matches[0].Ref, "RFC-AUTH-001"; got != want {
		t.Fatalf("top match ref = %q, want %q", got, want)
	}
	if result.Matches[0].Inference == nil {
		t.Fatalf("top match inference = nil, want structured inference confidence")
	}
	if got, want := result.Matches[0].Inference.Kind, config.SourceKindMarkdownContract; got != want {
		t.Fatalf("top match inference kind = %q, want %q", got, want)
	}
	if got, want := result.Matches[0].Inference.Level, "medium"; got != want {
		t.Fatalf("top match inference level = %q, want %q", got, want)
	}
	if len(result.Matches[0].Inference.Reasons) == 0 || result.Matches[0].Inference.Reasons[0] != "applies_to missing" {
		t.Fatalf("top match inference reasons = %+v, want applies_to warning", result.Matches[0].Inference.Reasons)
	}
}

func TestSearchSpecsDownranksHistoricalSectionsByDefault(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	configPath := filepath.Join(repo, "pituitary.toml")
	mustWriteSearchFile(t, configPath, `
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
`)
	mustWriteSearchFile(t, filepath.Join(repo, "specs", "locality-kernel", "spec.toml"), `
id = "SPEC-LOCALITY"
title = "Locality Kernel Contract"
status = "accepted"
domain = "runtime"
body = "body.md"
`)
	mustWriteSearchFile(t, filepath.Join(repo, "specs", "locality-kernel", "body.md"), `
## Requirements

Locality continuity semantics define the active runtime contract.

## Historical provenance

Locality continuity kernel semantics governed earlier drafts.
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	activeResult, err := SearchSpecs(cfg, SearchSpecQuery{Query: "locality continuity kernel semantics", Limit: 5})
	if err != nil {
		t.Fatalf("SearchSpecs() default error = %v", err)
	}
	if len(activeResult.Matches) == 0 {
		t.Fatal("SearchSpecs() default returned no matches")
	}
	if strings.Contains(strings.ToLower(activeResult.Matches[0].SectionHeading), "historical provenance") {
		t.Fatalf("default top match = %+v, want active section ahead of historical provenance", activeResult.Matches[0])
	}

	historicalResult, err := SearchSpecs(cfg, SearchSpecQuery{Query: "historical provenance locality continuity kernel semantics", Limit: 5})
	if err != nil {
		t.Fatalf("SearchSpecs() historical error = %v", err)
	}
	if len(historicalResult.Matches) == 0 {
		t.Fatal("SearchSpecs() historical returned no matches")
	}
	if !strings.Contains(strings.ToLower(historicalResult.Matches[0].SectionHeading), "historical provenance") {
		t.Fatalf("historical top match = %+v, want historical provenance section when query asks for history", historicalResult.Matches[0])
	}
}

func TestSearchSpecsRejectsEmbedderFingerprintMismatch(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	db, err := sql.Open("sqlite3", cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := db.Exec(`UPDATE metadata SET value = ? WHERE key = 'embedder_fingerprint'`, "fixture|fixture-16d|plain_v1"); err != nil {
		t.Fatalf("update embedder_fingerprint: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close writable db: %v", err)
	}

	_, err = SearchSpecs(cfg, SearchSpecQuery{Query: "rate limiting", Limit: 5})
	if err == nil {
		t.Fatal("SearchSpecs() error = nil, want fingerprint mismatch")
	}
	if got := err.Error(); !strings.Contains(got, "embedder fingerprint") || !strings.Contains(got, "pituitary index --rebuild") {
		t.Fatalf("SearchSpecs() error = %q, want fingerprint rebuild guidance", got)
	}
}

func mustWriteSearchFile(tb testing.TB, path, content string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		tb.Fatalf("write %s: %v", path, err)
	}
}
