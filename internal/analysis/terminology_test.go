package analysis

import (
	"os"
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
			if section.Assessment == "" {
				t.Fatalf("section = %+v, want assessment", section)
			}
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

func TestCheckTerminologyRequiresTermsOrGovernancePolicy(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "specs", "kernel-locality", "spec.toml"), `
id = "SPEC-LOCALITY"
title = "Kernel Locality Contract"
status = "accepted"
domain = "kernel"
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(root, "specs", "kernel-locality", "body.md"), `
# Kernel Locality Contract

Use locality language.
`)
	mustWriteFile(t, filepath.Join(root, "pituitary.toml"), `
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

	cfg, err := config.Load(filepath.Join(root, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	_, err = CheckTerminology(cfg, TerminologyAuditRequest{Scope: "docs"})
	if err == nil {
		t.Fatal("CheckTerminology() error = nil, want validation error")
	}
	if got, want := err.Error(), "at least one term or terminology policy is required"; got != want {
		t.Fatalf("CheckTerminology() error = %q, want %q", got, want)
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

func TestCheckTerminologyReportsCompatibilityOnlyMentionsAsTolerated(t *testing.T) {
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
		SpecRef:        "SPEC-LOCALITY",
		Scope:          "docs",
	})
	if err != nil {
		t.Fatalf("CheckTerminology() error = %v", err)
	}
	if len(result.Tolerated) != 1 || result.Tolerated[0].Ref != "doc://guides/repo-compatibility" {
		t.Fatalf("tolerated = %+v, want compatibility-only doc as tolerated historical use", result.Tolerated)
	}
	match := result.Tolerated[0].Sections[0].Matches[0]
	if !match.Tolerated || match.Context != terminologyContextHistorical {
		t.Fatalf("match = %+v, want tolerated historical context", match)
	}
	if match.Classification != terminologyClassificationHistoricalAlias {
		t.Fatalf("match classification = %q, want historical alias", match.Classification)
	}
}

func TestCheckTerminologyUsesConfigGovernanceWhenTermsOmitted(t *testing.T) {
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
		SpecRef: "SPEC-LOCALITY",
		Scope:   "docs",
	})
	if err != nil {
		t.Fatalf("CheckTerminology() error = %v", err)
	}
	if len(result.Findings) != 1 || result.Findings[0].Ref != "doc://guides/repo-kernel" {
		t.Fatalf("findings = %+v, want repo-kernel as the actionable finding", result.Findings)
	}
	if len(result.Tolerated) != 1 || result.Tolerated[0].Ref != "doc://guides/repo-compatibility" {
		t.Fatalf("tolerated = %+v, want compatibility doc as tolerated output", result.Tolerated)
	}
	if got := result.Findings[0].Sections[0].Matches[0].Replacement; got == "" {
		t.Fatalf("section matches = %+v, want replacement suggestion", result.Findings[0].Sections[0].Matches)
	}
}

func TestCheckTerminologyRebuildDropsRemovedLegacyTerms(t *testing.T) {
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
	if len(result.Findings) != 1 || result.Findings[0].Ref != "doc://guides/repo-kernel" {
		t.Fatalf("initial findings = %+v, want stale repo guide only", result.Findings)
	}

	if err := os.WriteFile(filepath.Join(cfg.Workspace.RootPath, "docs", "guides", "repo-kernel.md"), []byte(`
# Locality Kernel Guide

The kernel keeps continuity in each locality.
Locality storage is the default operator model.
`), 0o644); err != nil {
		t.Fatalf("rewrite stale doc: %v", err)
	}

	records, err = source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() after rewrite error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() after rewrite error = %v", err)
	}

	result, err = CheckTerminology(cfg, TerminologyAuditRequest{
		Terms:          []string{"repo"},
		CanonicalTerms: []string{"locality"},
		Scope:          "docs",
	})
	if err != nil {
		t.Fatalf("CheckTerminology() after rewrite error = %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("findings after rewrite = %+v, want none", result.Findings)
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
	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "repo-compatibility.md"), `
# Repo Compatibility Notes

Legacy repo references remain available only as a compatibility alias during migration to locality.
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

[[terminology.policies]]
preferred = "locality"
historical_aliases = ["repo"]
forbidden_current = ["repository"]
docs_severity = "error"
specs_severity = "warning"

[[terminology.policies]]
preferred = "continuity"
deprecated_terms = ["workflow"]
docs_severity = "error"
specs_severity = "warning"

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
