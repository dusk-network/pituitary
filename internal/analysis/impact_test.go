package analysis

import (
	"testing"

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
