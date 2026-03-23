package analysis

import (
	"encoding/json"
	"testing"

	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestReviewSpecComposesIndexedWorkflow(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := ReviewSpec(cfg, ReviewRequest{SpecRef: "SPEC-042"})
	if err != nil {
		t.Fatalf("ReviewSpec() error = %v", err)
	}
	if result.SpecRef != "SPEC-042" {
		t.Fatalf("spec_ref = %q, want SPEC-042", result.SpecRef)
	}
	if result.Overlap == nil || len(result.Overlap.Overlaps) == 0 || result.Overlap.Overlaps[0].Ref != "SPEC-008" {
		t.Fatalf("overlap = %+v, want SPEC-008 first", result.Overlap)
	}
	if result.Comparison == nil || len(result.Comparison.SpecRefs) != 2 {
		t.Fatalf("comparison = %+v, want composed comparison", result.Comparison)
	}
	if result.Comparison.SpecRefs[0] != "SPEC-042" || result.Comparison.SpecRefs[1] != "SPEC-008" {
		t.Fatalf("comparison spec_refs = %v, want [SPEC-042 SPEC-008]", result.Comparison.SpecRefs)
	}
	if result.Impact == nil || len(result.Impact.AffectedSpecs) == 0 || result.Impact.AffectedSpecs[0].Ref != "SPEC-055" {
		t.Fatalf("impact = %+v, want SPEC-055 impacted", result.Impact)
	}
	if result.DocDrift == nil || result.DocDrift.Scope.Mode != "doc_refs" {
		t.Fatalf("doc_drift = %+v, want targeted doc_refs scope", result.DocDrift)
	}
	if len(result.DocDrift.DriftItems) != 1 || result.DocDrift.DriftItems[0].DocRef != "doc://guides/api-rate-limits" {
		t.Fatalf("doc_drift items = %+v, want guide drift only", result.DocDrift.DriftItems)
	}
}

func TestReviewSpecSupportsDraftSpecRecord(t *testing.T) {
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

	result, err := ReviewSpec(cfg, ReviewRequest{SpecRecord: &draft})
	if err != nil {
		t.Fatalf("ReviewSpec() draft error = %v", err)
	}
	if result.SpecRef != "SPEC-900" {
		t.Fatalf("spec_ref = %q, want SPEC-900", result.SpecRef)
	}
	if result.Overlap == nil || len(result.Overlap.Overlaps) == 0 || result.Overlap.Overlaps[0].Ref != "SPEC-042" {
		t.Fatalf("draft overlap = %+v, want SPEC-042 first", result.Overlap)
	}
	if result.Comparison == nil || len(result.Comparison.SpecRefs) != 2 || result.Comparison.SpecRefs[0] != "SPEC-900" {
		t.Fatalf("draft comparison = %+v, want draft candidate first", result.Comparison)
	}
	if result.Comparison.SpecRefs[1] != "SPEC-042" {
		t.Fatalf("draft comparison spec_refs = %v, want [SPEC-900 SPEC-042]", result.Comparison.SpecRefs)
	}
	if result.Impact == nil || len(result.Impact.AffectedDocs) == 0 {
		t.Fatalf("draft impact = %+v, want affected docs", result.Impact)
	}
	if result.DocDrift == nil || result.DocDrift.Scope.Mode != "doc_refs" {
		t.Fatalf("draft doc_drift = %+v, want targeted doc_refs scope", result.DocDrift)
	}
}

func TestReviewSpecSurfacesStaleNamedArtifacts(t *testing.T) {
	t.Parallel()

	cfg := writeArtifactContractWorkspace(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := ReviewSpec(cfg, ReviewRequest{SpecRef: "SPEC-200"})
	if err != nil {
		t.Fatalf("ReviewSpec() error = %v", err)
	}
	if result.DocDrift == nil || result.DocDrift.Scope.Mode != "doc_refs" {
		t.Fatalf("doc_drift = %+v, want targeted doc_refs scope", result.DocDrift)
	}
	if len(result.DocDrift.DriftItems) != 1 || result.DocDrift.DriftItems[0].DocRef != "doc://guides/runtime-cache" {
		t.Fatalf("doc_drift items = %+v, want stale runtime-cache doc", result.DocDrift.DriftItems)
	}

	var found bool
	for _, finding := range result.DocDrift.DriftItems[0].Findings {
		if finding.Artifact == "work_queue.json" && finding.Code == "artifact_runtime_input_mismatch" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("findings = %+v, want work_queue.json runtime-input drift", result.DocDrift.DriftItems[0].Findings)
	}

	if result.DocRemediation == nil || len(result.DocRemediation.Items) != 1 || result.DocRemediation.Items[0].DocRef != "doc://guides/runtime-cache" {
		t.Fatalf("doc_remediation = %+v, want runtime-cache remediation", result.DocRemediation)
	}
}

func TestReviewSpecUsesAnalysisProviderForComparisonAndDocDrift(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	configureOpenAIAnalysisProvider(t, cfg, func(t *testing.T, request openAICompatibleChatRequest) string {
		t.Helper()
		switch request.Messages[0].Content {
		case openAICompatibleCompareSystemPrompt:
			return `{
				"tradeoffs": [{"topic": "migration", "summary": "SPEC-042 modernizes the accepted path while preserving the same governed scope as SPEC-008."}],
				"compatibility": {"level": "superseding", "summary": "SPEC-042 replaces SPEC-008 for the same API control surface."},
				"recommendation": "prefer SPEC-042 after provider-backed review because it is the accepted successor"
			}`
		case openAICompatibleDocDriftSystemPrompt:
			var prompt docDriftAnalysisPrompt
			if err := json.Unmarshal([]byte(request.Messages[1].Content), &prompt); err != nil {
				t.Fatalf("unmarshal prompt: %v", err)
			}
			if len(prompt.DeterministicFindings) == 0 {
				t.Fatal("deterministic_findings is empty")
			}
			first := prompt.DeterministicFindings[0]
			return `{
				"findings": [
					{
						"spec_ref": "` + first.SpecRef + `",
						"artifact": "` + first.Artifact + `",
						"code": "` + first.Code + `",
						"message": "provider-backed review confirmed the guide is stale against the accepted rate-limit spec"
					}
				],
				"suggestions": [
					{
						"spec_ref": "` + first.SpecRef + `",
						"code": "` + first.Code + `",
						"summary": "Refresh the guide so it matches the accepted per-tenant rate-limit behavior.",
						"suggested_edit": {
							"action": "replace_statement",
							"note": "Rewrite the stale sentence instead of layering caveats around it."
						}
					}
				]
			}`
		default:
			t.Fatalf("unexpected system prompt %q", request.Messages[0].Content)
			return ""
		}
	})

	result, err := ReviewSpec(cfg, ReviewRequest{SpecRef: "SPEC-042"})
	if err != nil {
		t.Fatalf("ReviewSpec() error = %v", err)
	}
	if result.Comparison == nil {
		t.Fatalf("comparison = %+v, want composed comparison", result.Comparison)
	}
	if got, want := result.Comparison.Comparison.Recommendation, "prefer SPEC-042 after provider-backed review because it is the accepted successor"; got != want {
		t.Fatalf("recommendation = %q, want %q", got, want)
	}
	if result.DocDrift == nil || len(result.DocDrift.DriftItems) != 1 {
		t.Fatalf("doc_drift = %+v, want one drift item", result.DocDrift)
	}
	if got, want := result.DocDrift.DriftItems[0].Findings[0].Message, "provider-backed review confirmed the guide is stale against the accepted rate-limit spec"; got != want {
		t.Fatalf("finding message = %q, want %q", got, want)
	}
	if result.DocRemediation == nil || len(result.DocRemediation.Items) != 1 {
		t.Fatalf("doc_remediation = %+v, want one remediation item", result.DocRemediation)
	}
	if got, want := result.DocRemediation.Items[0].Suggestions[0].Summary, "Refresh the guide so it matches the accepted per-tenant rate-limit behavior."; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
}
