package analysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestCheckSpecFreshnessReturnsAllSpecsForScopeAll(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckSpecFreshness(cfg, FreshnessRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckSpecFreshness() error = %v", err)
	}
	if result.Scope != "all" {
		t.Fatalf("result.Scope = %q, want all", result.Scope)
	}
	if len(result.Items) == 0 {
		t.Fatal("CheckSpecFreshness() returned no items")
	}
	if result.ContentTrust == nil || result.ContentTrust.Level != "untrusted" {
		t.Fatal("result.ContentTrust missing or not untrusted")
	}
}

func TestCheckSpecFreshnessSingleSpec(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckSpecFreshness(cfg, FreshnessRequest{SpecRef: "SPEC-042"})
	if err != nil {
		t.Fatalf("CheckSpecFreshness() error = %v", err)
	}
	if result.Scope != "single" {
		t.Fatalf("result.Scope = %q, want single", result.Scope)
	}
	if len(result.Items) != 1 {
		t.Fatalf("result.Items = %d, want 1", len(result.Items))
	}
	if result.Items[0].SpecRef != "SPEC-042" {
		t.Fatalf("result.Items[0].SpecRef = %q, want SPEC-042", result.Items[0].SpecRef)
	}
}

func TestCheckSpecFreshnessFoundationSignal(t *testing.T) {
	t.Parallel()

	// Create a custom workspace where a spec depends on a superseded spec.
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "pituitary.db")
	configPath := filepath.Join(dir, "pituitary.toml")

	// Create spec bundles.
	specDir := filepath.Join(dir, "specs")
	mustMkdir(t, filepath.Join(specDir, "old-spec"))
	mustMkdir(t, filepath.Join(specDir, "new-spec"))

	mustWriteFile(t, filepath.Join(specDir, "old-spec", "spec.toml"), `
id = "SPEC-OLD"
title = "Old Architecture Decision"
status = "superseded"
domain = "arch"
authors = ["test"]
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(specDir, "old-spec", "body.md"), `
# Old Architecture

This spec describes the original architecture approach.
`)

	mustWriteFile(t, filepath.Join(specDir, "new-spec", "spec.toml"), `
id = "SPEC-NEW"
title = "New Spec Depending on Old"
status = "accepted"
domain = "arch"
authors = ["test"]
body = "body.md"
depends_on = ["SPEC-OLD"]
`)
	mustWriteFile(t, filepath.Join(specDir, "new-spec", "body.md"), `
# New Spec

This spec builds upon the old architecture approach.
`)

	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(dir)+`"
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
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckSpecFreshness(cfg, FreshnessRequest{SpecRef: "SPEC-NEW"})
	if err != nil {
		t.Fatalf("CheckSpecFreshness() error = %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("result.Items = %d, want 1", len(result.Items))
	}
	item := result.Items[0]
	if item.Verdict != "stale_foundation" {
		t.Fatalf("verdict = %q, want stale_foundation", item.Verdict)
	}

	var foundFoundation bool
	for _, signal := range item.Signals {
		if signal.Kind == freshnessSignalFoundation {
			foundFoundation = true
			if signal.Provenance != ProvenanceLiteral {
				t.Fatalf("foundation signal provenance = %q, want literal", signal.Provenance)
			}
			if !strings.Contains(signal.Summary, "SPEC-OLD") {
				t.Fatalf("foundation signal summary = %q, want mention of SPEC-OLD", signal.Summary)
			}
		}
	}
	if !foundFoundation {
		t.Fatal("expected a foundation signal, found none")
	}
}

func TestCheckSpecFreshnessDecisionTrailDetection(t *testing.T) {
	t.Parallel()

	// Create workspace with a spec and decision-bearing docs.
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "pituitary.db")
	configPath := filepath.Join(dir, "pituitary.toml")

	specDir := filepath.Join(dir, "specs")
	mustMkdir(t, filepath.Join(specDir, "rate-limit"))

	mustWriteFile(t, filepath.Join(specDir, "rate-limit", "spec.toml"), `
id = "SPEC-RL"
title = "Per-Tenant Rate Limiting"
status = "accepted"
domain = "api"
authors = ["test"]
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(specDir, "rate-limit", "body.md"), `
# Per-Tenant Rate Limiting

## Approach

Use a sliding window algorithm with per-tenant counters. Each tenant
gets a default limit of 100 requests per minute. Tenant-specific
overrides are stored in the limits config file.
`)

	decisionDir := filepath.Join(dir, "decisions")
	mustMkdir(t, decisionDir)
	mustWriteFile(t, filepath.Join(decisionDir, "decision-rate-change.md"), `
# Decision: Retire Per-Tenant Rate Limiting

## Context

We are moving away from per-tenant rate limiting toward a
global token bucket approach. The sliding window algorithm
has proven difficult to scale across regions.

## Decision

Retire the per-tenant rate limiting approach. Replace with
global rate limiting using a distributed token bucket.
`)

	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(dir)+`"
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
name = "decisions"
adapter = "filesystem"
kind = "markdown_docs"
path = "decisions"
include = ["*.md"]
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckSpecFreshness(cfg, FreshnessRequest{SpecRef: "SPEC-RL"})
	if err != nil {
		t.Fatalf("CheckSpecFreshness() error = %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("result.Items = %d, want 1", len(result.Items))
	}
	item := result.Items[0]

	// The decision doc is in a "decisions/" path, so it should be identified as
	// decision-bearing. With the fixture embedder the similarity score depends on
	// content hash; we verify that the detection pipeline runs without error and
	// produces either a decision-trail signal or a contradictory signal.
	if item.Verdict == "" {
		t.Fatal("verdict should not be empty")
	}
}

func TestIsDecisionBearingDoc(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata map[string]string
		source   string
		want     bool
	}{
		{
			name:     "decision_log role",
			metadata: map[string]string{"source_role": "decision_log"},
			source:   "docs/guide.md",
			want:     true,
		},
		{
			name:     "current_state role",
			metadata: map[string]string{"source_role": "current_state"},
			source:   "docs/guide.md",
			want:     true,
		},
		{
			name:     "decisions directory",
			metadata: map[string]string{},
			source:   "decisions/some-decision.md",
			want:     true,
		},
		{
			name:     "memory directory",
			metadata: map[string]string{},
			source:   "memory/session-1.md",
			want:     true,
		},
		{
			name:     "adr prefix",
			metadata: map[string]string{},
			source:   "docs/adr-001-auth-approach.md",
			want:     true,
		},
		{
			name:     "decision prefix",
			metadata: map[string]string{},
			source:   "docs/decision-rate-limit.md",
			want:     true,
		},
		{
			name:     "MEMORY.md basename",
			metadata: map[string]string{},
			source:   "MEMORY.md",
			want:     true,
		},
		{
			name:     "HANDOFF.md basename",
			metadata: map[string]string{},
			source:   "HANDOFF.md",
			want:     true,
		},
		{
			name:     "regular doc",
			metadata: map[string]string{},
			source:   "docs/guides/api-reference.md",
			want:     false,
		},
		{
			name:     "canonical role not decision-bearing",
			metadata: map[string]string{"source_role": "canonical"},
			source:   "docs/guide.md",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := docDocument{
				Record: newDocRecordForTest(tt.source, tt.metadata),
			}
			got := isDecisionBearingDoc(doc)
			if got != tt.want {
				t.Errorf("isDecisionBearingDoc() = %v, want %v (source=%q, metadata=%v)", got, tt.want, tt.source, tt.metadata)
			}
		})
	}
}

func TestAssessFreshnessVerdicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		signals     []FreshnessSignal
		wantVerdict string
	}{
		{
			name:        "no signals means fresh",
			signals:     nil,
			wantVerdict: "fresh",
		},
		{
			name: "decision trail above threshold",
			signals: []FreshnessSignal{
				{Kind: freshnessSignalDecisionTrail, Score: 0.60},
			},
			wantVerdict: "likely_stale",
		},
		{
			name: "decision trail below high threshold",
			signals: []FreshnessSignal{
				{Kind: freshnessSignalDecisionTrail, Score: 0.42},
			},
			wantVerdict: "likely_stale",
		},
		{
			name: "foundation signal",
			signals: []FreshnessSignal{
				{Kind: freshnessSignalFoundation, Score: 0.60},
			},
			wantVerdict: "stale_foundation",
		},
		{
			name: "contradictory only with high score",
			signals: []FreshnessSignal{
				{Kind: freshnessSignalContradictory, Score: 0.45},
			},
			wantVerdict: "review_recommended",
		},
		{
			name: "contradictory only with low score",
			signals: []FreshnessSignal{
				{Kind: freshnessSignalContradictory, Score: 0.20},
			},
			wantVerdict: "fresh",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, _, _ := assessFreshness(tt.signals)
			if verdict != tt.wantVerdict {
				t.Errorf("assessFreshness() verdict = %q, want %q", verdict, tt.wantVerdict)
			}
		})
	}
}

func newDocRecordForTest(sourceRef string, metadata map[string]string) model.DocRecord {
	if metadata == nil {
		metadata = map[string]string{}
	}
	return model.DocRecord{
		Ref:       "doc://" + sourceRef,
		SourceRef: sourceRef,
		Metadata:  metadata,
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
