package analysis

import (
	"path/filepath"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestCheckTerminologyAnchorsFindingsToAcceptedSpec(t *testing.T) {
	t.Parallel()

	cfg := loadTerminologyFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckTerminology(cfg, TerminologyAuditRequest{
		Terms:          []string{"repo", "workflow"},
		CanonicalTerms: []string{"locality", "continuity"},
		SpecRef:        "SPEC-LOCALITY",
		Scope:          "all",
	})
	if err != nil {
		t.Fatalf("CheckTerminology() error = %v", err)
	}
	if result.Scope.Mode != "spec_ref" || result.Scope.SpecRef != "SPEC-LOCALITY" {
		t.Fatalf("scope = %+v, want spec_ref anchored to SPEC-LOCALITY", result.Scope)
	}
	if len(result.AnchorSpecs) != 1 || result.AnchorSpecs[0].Ref != "SPEC-LOCALITY" {
		t.Fatalf("anchor_specs = %+v, want SPEC-LOCALITY", result.AnchorSpecs)
	}

	var foundDoc, foundSpec bool
	for _, finding := range result.Findings {
		switch finding.Ref {
		case "doc://guides/repo-kernel":
			foundDoc = true
		case "SPEC-REPO-ADAPTER":
			foundSpec = true
		case "doc://guides/locality-kernel":
			t.Fatalf("findings = %+v, did not expect aligned doc", result.Findings)
		}
		if len(finding.Sections) == 0 {
			t.Fatalf("finding = %+v, want section evidence", finding)
		}
		for _, section := range finding.Sections {
			if section.Evidence == nil {
				t.Fatalf("section = %+v, want canonical evidence", section)
			}
			if section.Evidence.SpecRef != "SPEC-LOCALITY" {
				t.Fatalf("section evidence = %+v, want SPEC-LOCALITY", section.Evidence)
			}
		}
	}
	if !foundDoc || !foundSpec {
		t.Fatalf("findings = %+v, want stale doc and stale spec", result.Findings)
	}
}

func TestCheckTerminologyWorkspaceScopeUsesCanonicalTerms(t *testing.T) {
	t.Parallel()

	cfg := loadTerminologyFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckTerminology(cfg, TerminologyAuditRequest{
		Terms:          []string{"repo"},
		CanonicalTerms: []string{"locality"},
		Scope:          "docs",
	})
	if err != nil {
		t.Fatalf("CheckTerminology() error = %v", err)
	}
	if result.Scope.Mode != "workspace" {
		t.Fatalf("scope = %+v, want workspace mode", result.Scope)
	}
	if len(result.AnchorSpecs) != 1 || result.AnchorSpecs[0].Ref != "SPEC-LOCALITY" {
		t.Fatalf("anchor_specs = %+v, want accepted locality spec", result.AnchorSpecs)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("findings = %+v, want one stale doc", result.Findings)
	}
	if got := result.Findings[0].Ref; got != "doc://guides/repo-kernel" {
		t.Fatalf("findings[0].ref = %q, want stale repo guide", got)
	}
	if got := result.Findings[0].Kind; got != "doc" {
		t.Fatalf("findings[0].kind = %q, want doc", got)
	}
}

func loadTerminologyFixtureConfig(tb testing.TB) *config.Config {
	tb.Helper()

	root := tb.TempDir()
	mustWriteFile(tb, filepath.Join(root, "specs", "kernel-locality", "spec.toml"), `
id = "SPEC-LOCALITY"
title = "Kernel Locality Contract"
status = "accepted"
domain = "kernel"
body = "body.md"

applies_to = ["config://state/locality"]
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "kernel-locality", "body.md"), `
# Kernel Locality Contract

## Core Model

The kernel keeps continuity in clone-local state.
The runtime is locality-centric and treats repository adapters as optional extensions.

## Guidance

Use locality and continuity language in operator docs and guides.
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "repo-adapter", "spec.toml"), `
id = "SPEC-REPO-ADAPTER"
title = "Repository Adapter Notes"
status = "review"
domain = "kernel"
body = "body.md"

depends_on = ["SPEC-LOCALITY"]
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "repo-adapter", "body.md"), `
# Repository Adapter Notes

## Legacy Language

The kernel keeps workflow continuity in each repo.
Repository metadata remains the default storage boundary for operators.
`)
	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "repo-kernel.md"), `
# Repo Kernel Guide

The kernel keeps workflow continuity in each repo.
Repository storage is the default operator model.
`)
	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "locality-kernel.md"), `
# Locality Kernel Guide

The kernel keeps continuity in each locality.
Locality storage is the default operator model.
`)
	mustWriteFile(tb, filepath.Join(root, "pituitary.toml"), `
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

	cfg, err := config.Load(filepath.Join(root, "pituitary.toml"))
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}
