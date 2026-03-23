package analysis

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestCheckDocDriftFlagsGuideButNotRunbook(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckDocDrift(cfg, DocDriftRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}
	if result.Scope.Mode != "all" {
		t.Fatalf("scope = %+v, want mode all", result.Scope)
	}

	var foundGuide, foundRunbook bool
	for _, item := range result.DriftItems {
		switch item.DocRef {
		case "doc://guides/api-rate-limits":
			foundGuide = true
			if len(item.Findings) == 0 {
				t.Fatalf("guide drift item = %+v, want findings", item)
			}
			top := item.Findings[0]
			if top.Rationale == "" || top.Evidence == nil || top.Evidence.SpecSection == "" || top.Evidence.DocSection == "" || top.Confidence == nil || top.Confidence.Level == "" {
				t.Fatalf("top finding = %+v, want rationale, evidence, and confidence", top)
			}
		case "doc://runbooks/rate-limit-rollout":
			foundRunbook = true
		}
	}
	if !foundGuide {
		t.Fatalf("drift_items = %+v, want guide drift", result.DriftItems)
	}
	if foundRunbook {
		t.Fatalf("drift_items = %+v, did not expect aligned runbook", result.DriftItems)
	}
	if result.Remediation == nil || len(result.Remediation.Items) != 1 {
		t.Fatalf("remediation = %+v, want one remediation item", result.Remediation)
	}
	if result.Remediation.Items[0].DocRef != "doc://guides/api-rate-limits" {
		t.Fatalf("remediation item = %+v, want guide remediation", result.Remediation.Items[0])
	}
	if len(result.Remediation.Items[0].Suggestions) < 3 {
		t.Fatalf("remediation suggestions = %+v, want multiple actionable suggestions", result.Remediation.Items[0].Suggestions)
	}
	top := result.Remediation.Items[0].Suggestions[0]
	if top.SpecRef == "" || top.Evidence.SpecSection == "" || top.SuggestedEdit.Action == "" {
		t.Fatalf("top remediation suggestion = %+v, want evidence and suggested edit", top)
	}

	var foundGuideAssessment bool
	for _, assessment := range result.Assessments {
		switch assessment.DocRef {
		case "doc://guides/api-rate-limits":
			foundGuideAssessment = true
			if assessment.Status != "drift" {
				t.Fatalf("guide assessment = %+v, want drift status", assessment)
			}
			if assessment.Rationale == "" || assessment.Evidence == nil || assessment.Confidence == nil || assessment.Confidence.Level == "" {
				t.Fatalf("guide assessment = %+v, want rationale, evidence, and confidence", assessment)
			}
		}
	}
	if !foundGuideAssessment {
		t.Fatalf("assessments = %+v, want guide assessment", result.Assessments)
	}
}

func TestCheckDocDriftSupportsTargetedDocRefs(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckDocDrift(cfg, DocDriftRequest{
		DocRefs: []string{"doc://guides/api-rate-limits", "doc://runbooks/rate-limit-rollout"},
	})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}
	if result.Scope.Mode != "doc_refs" {
		t.Fatalf("scope = %+v, want mode doc_refs", result.Scope)
	}
	if len(result.Scope.DocRefs) != 2 {
		t.Fatalf("scope.doc_refs = %v, want 2 refs", result.Scope.DocRefs)
	}
	if len(result.DriftItems) != 1 || result.DriftItems[0].DocRef != "doc://guides/api-rate-limits" {
		t.Fatalf("drift_items = %+v, want only guide drift", result.DriftItems)
	}
}

func TestCheckDocDriftFlagsStaleNamedArtifacts(t *testing.T) {
	t.Parallel()

	cfg := writeArtifactContractWorkspace(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckDocDrift(cfg, DocDriftRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}

	var stale, aligned, native *DriftItem
	for i := range result.DriftItems {
		item := &result.DriftItems[i]
		switch item.DocRef {
		case "doc://guides/runtime-cache":
			stale = item
		case "doc://guides/runtime-derived":
			aligned = item
		case "doc://guides/runtime-native":
			native = item
		}
	}
	if stale == nil {
		t.Fatalf("drift_items = %+v, want stale runtime-cache doc", result.DriftItems)
	}
	if aligned != nil {
		t.Fatalf("drift_items = %+v, did not expect aligned derived doc", result.DriftItems)
	}
	if native != nil {
		t.Fatalf("drift_items = %+v, did not expect canonical state.db doc", result.DriftItems)
	}

	var foundWorkQueue, foundCompiledState bool
	for _, finding := range stale.Findings {
		switch {
		case finding.Artifact == "work_queue.json" && finding.Code == "artifact_runtime_input_mismatch":
			foundWorkQueue = true
		case finding.Artifact == "compiled_state.json" && finding.Code == "artifact_contract_mismatch":
			foundCompiledState = true
		}
	}
	if !foundWorkQueue || !foundCompiledState {
		t.Fatalf("findings = %+v, want work_queue.json runtime-input drift and compiled_state.json contract drift", stale.Findings)
	}

	if result.Remediation == nil || len(result.Remediation.Items) != 1 || result.Remediation.Items[0].DocRef != "doc://guides/runtime-cache" {
		t.Fatalf("remediation = %+v, want runtime-cache remediation only", result.Remediation)
	}
}

func TestCheckDocDriftUsesAnalysisProviderWhenEnabled(t *testing.T) {
	t.Parallel()

	cfg := writeArtifactContractWorkspace(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	configureOpenAIAnalysisProvider(t, cfg, func(t *testing.T, request openAICompatibleChatRequest) string {
		t.Helper()
		var prompt docDriftAnalysisPrompt
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &prompt); err != nil {
			t.Fatalf("unmarshal prompt: %v", err)
		}
		if got, want := prompt.Command, "check-doc-drift"; got != want {
			t.Fatalf("command = %q, want %q", got, want)
		}
		if got, want := prompt.Doc.Ref, "doc://guides/runtime-cache"; got != want {
			t.Fatalf("doc.ref = %q, want %q", got, want)
		}

		return `{
			"findings": [
				{
					"spec_ref": "SPEC-200",
					"artifact": "work_queue.json",
					"code": "artifact_runtime_input_mismatch",
					"message": "runtime-cache guide still presents work_queue.json as the canonical startup input",
					"expected": "not a canonical runtime input",
					"observed": "documented as the canonical startup input"
				}
			],
			"suggestions": [
				{
					"spec_ref": "SPEC-200",
					"code": "artifact_runtime_input_mismatch",
					"summary": "Rewrite the runtime-cache guide so work_queue.json is clearly described as a derived cache, not a required runtime input.",
					"evidence": {
						"expected": "not a canonical runtime input",
						"observed": "documented as the canonical startup input"
					},
					"suggested_edit": {
						"action": "replace_statement",
						"note": "Point readers at state.db as the canonical store and downgrade work_queue.json to optional cache status."
					}
				}
			]
		}`
	})

	result, err := CheckDocDrift(cfg, DocDriftRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}

	var stale *DriftItem
	for i := range result.DriftItems {
		if result.DriftItems[i].DocRef == "doc://guides/runtime-cache" {
			stale = &result.DriftItems[i]
			break
		}
	}
	if stale == nil {
		t.Fatalf("drift_items = %+v, want runtime-cache drift", result.DriftItems)
	}

	var refined bool
	for _, finding := range stale.Findings {
		if finding.Artifact == "work_queue.json" && finding.Code == "artifact_runtime_input_mismatch" && finding.Message == "runtime-cache guide still presents work_queue.json as the canonical startup input" {
			refined = true
			break
		}
	}
	if !refined {
		t.Fatalf("findings = %+v, want provider-refined work_queue.json message", stale.Findings)
	}

	if result.Remediation == nil || len(result.Remediation.Items) != 1 {
		t.Fatalf("remediation = %+v, want one remediation item", result.Remediation)
	}
	suggestion := result.Remediation.Items[0].Suggestions[0]
	if got, want := suggestion.Summary, "Rewrite the runtime-cache guide so work_queue.json is clearly described as a derived cache, not a required runtime input."; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if got, want := suggestion.SuggestedEdit.Note, "Point readers at state.db as the canonical store and downgrade work_queue.json to optional cache status."; got != want {
		t.Fatalf("suggested_edit.note = %q, want %q", got, want)
	}
}

func TestCheckDocDriftSurfacesPossibleDriftForConceptualNearMatch(t *testing.T) {
	t.Parallel()

	cfg := writePossibleDocDriftWorkspace(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckDocDrift(cfg, DocDriftRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}
	if len(result.DriftItems) != 0 {
		t.Fatalf("drift_items = %+v, want no deterministic drift items", result.DriftItems)
	}
	if len(result.Assessments) != 1 {
		t.Fatalf("assessments = %+v, want one assessment", result.Assessments)
	}
	assessment := result.Assessments[0]
	if got, want := assessment.Status, "possible_drift"; got != want {
		t.Fatalf("assessment.status = %q, want %q", got, want)
	}
	if assessment.Confidence == nil || assessment.Confidence.Level != "low" {
		t.Fatalf("assessment.confidence = %+v, want low confidence", assessment.Confidence)
	}
	if assessment.Evidence == nil || assessment.Evidence.SpecSection == "" || assessment.Evidence.DocSection == "" {
		t.Fatalf("assessment = %+v, want evidence sections", assessment)
	}
}

func TestClassifyArtifactConstraintScopesRuntimeInputToLocalArtifact(t *testing.T) {
	t.Parallel()

	line := "Prefer `state.db` for canonical runtime state, and the kernel must not read `work_queue.json` as canonical runtime input."

	if kind, _, ok := classifyArtifactConstraint(line, "state.db"); ok || kind != "" {
		t.Fatalf("classifyArtifactConstraint(state.db) = %q, %t, want no constraint", kind, ok)
	}
	if kind, expected, ok := classifyArtifactConstraint(line, "work_queue.json"); !ok || kind != "runtime_input" || expected != "not a canonical runtime input" {
		t.Fatalf("classifyArtifactConstraint(work_queue.json) = kind=%q expected=%q ok=%t, want runtime_input/not a canonical runtime input/true", kind, expected, ok)
	}
}

func writeArtifactContractWorkspace(tb testing.TB) *config.Config {
	tb.Helper()

	root := tb.TempDir()
	indexPath := filepath.Join(root, ".pituitary", "pituitary.db")
	configPath := filepath.Join(root, "pituitary.toml")

	mustWriteFile(tb, filepath.Join(root, "specs", "runtime-contract", "spec.toml"), `
id = "SPEC-200"
title = "Runtime Contract"
status = "accepted"
domain = "runtime"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "runtime-contract", "body.md"), "# Runtime Contract\n\n"+
		"- Legacy derived files such as `handoff.md`, `compiled_state.json`, and `work_queue.json` are not part of the accepted runtime contract.\n"+
		"- The kernel must not read `work_queue.json` as canonical runtime input.\n"+
		"- `compiled_state.json` is not a required artifact in the accepted runtime contract.\n")

	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "runtime-cache.md"), "# Runtime Cache Guide\n\n"+
		"`ccd start` writes `work_queue.json` for the active clone and reads it on the next startup.\n\n"+
		"The clone-local runtime layout also keeps `compiled_state.json` alongside that cache.\n")

	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "runtime-derived.md"), "# Runtime Derived Exports\n\n"+
		"- `handoff.md` remains an optional derived export rendered from canonical state.\n")

	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "runtime-native.md"), "# Runtime Native State\n\n"+
		"- `state.db` is the canonical clone-local runtime store.\n")

	mustWriteFile(tb, configPath, `
[workspace]
root = "`+filepath.ToSlash(root)+`"
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
include = ["guides/*.md"]
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}

func writePossibleDocDriftWorkspace(tb testing.TB) *config.Config {
	tb.Helper()

	root := tb.TempDir()
	indexPath := filepath.Join(root, ".pituitary", "pituitary.db")
	configPath := filepath.Join(root, "pituitary.toml")

	mustWriteFile(tb, filepath.Join(root, "specs", "kernel-locality", "spec.toml"), `
id = "SPEC-LOCALITY"
title = "Kernel Locality Contract"
status = "accepted"
domain = "kernel"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "kernel-locality", "body.md"), `
# Kernel Locality Contract

## Core Model

The kernel keeps continuity in clone-local state and treats locality as the primary runtime boundary.

## Operator Guidance

Use locality and continuity language in operator guidance.
`)

	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "kernel-migration.md"), `
# Kernel Migration Guide

## Working Notes

The kernel keeps continuity in local state during migration.
Operators should map old repository language to the new locality model while updating guides.
`)

	mustWriteFile(tb, configPath, fmt.Sprintf(`
[workspace]
root = %q
index_path = %q

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = %q

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = %q
`, root, indexPath, filepath.Join(root, "specs"), filepath.Join(root, "docs")))

	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}
