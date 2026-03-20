package analysis

import (
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
