package analysis

import (
	"context"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

type benchmarkFixture struct {
	cfg   *config.Config
	draft model.SpecRecord
}

func BenchmarkCheckOverlap(b *testing.B) {
	fixture := prepareBenchmarkFixture(b)

	b.Run("indexed", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			result, err := CheckOverlap(fixture.cfg, OverlapRequest{SpecRef: "SPEC-042"})
			if err != nil {
				b.Fatalf("CheckOverlap() error = %v", err)
			}
			if len(result.Overlaps) == 0 {
				b.Fatal("CheckOverlap() returned no overlaps")
			}
		}
	})

	b.Run("draft", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			result, err := CheckOverlap(fixture.cfg, OverlapRequest{SpecRecord: &fixture.draft})
			if err != nil {
				b.Fatalf("CheckOverlap() draft error = %v", err)
			}
			if len(result.Overlaps) == 0 {
				b.Fatal("CheckOverlap() draft returned no overlaps")
			}
		}
	})
}

func BenchmarkCompareSpecs(b *testing.B) {
	fixture := prepareBenchmarkFixture(b)

	b.Run("indexed", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			result, err := CompareSpecs(fixture.cfg, CompareRequest{SpecRefs: []string{"SPEC-008", "SPEC-042"}})
			if err != nil {
				b.Fatalf("CompareSpecs() error = %v", err)
			}
			if len(result.SpecRefs) != 2 {
				b.Fatalf("CompareSpecs() spec_refs = %v, want 2 refs", result.SpecRefs)
			}
		}
	})

	b.Run("draft", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			result, err := CompareSpecs(fixture.cfg, CompareRequest{
				SpecRecord: &fixture.draft,
				SpecRefs:   []string{"SPEC-042"},
			})
			if err != nil {
				b.Fatalf("CompareSpecs() draft error = %v", err)
			}
			if len(result.SpecRefs) != 2 {
				b.Fatalf("CompareSpecs() draft spec_refs = %v, want 2 refs", result.SpecRefs)
			}
		}
	})
}

func BenchmarkAnalyzeImpact(b *testing.B) {
	fixture := prepareBenchmarkFixture(b)

	b.Run("indexed", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			result, err := AnalyzeImpact(fixture.cfg, AnalyzeImpactRequest{
				SpecRef:    "SPEC-042",
				ChangeType: "accepted",
			})
			if err != nil {
				b.Fatalf("AnalyzeImpact() error = %v", err)
			}
			if len(result.AffectedDocs) == 0 {
				b.Fatal("AnalyzeImpact() returned no affected docs")
			}
		}
	})

	b.Run("draft", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			result, err := AnalyzeImpact(fixture.cfg, AnalyzeImpactRequest{
				SpecRecord: &fixture.draft,
				ChangeType: "accepted",
			})
			if err != nil {
				b.Fatalf("AnalyzeImpact() draft error = %v", err)
			}
			if len(result.AffectedDocs) == 0 {
				b.Fatal("AnalyzeImpact() draft returned no affected docs")
			}
		}
	})
}

func BenchmarkCheckDocDrift(b *testing.B) {
	fixture := prepareBenchmarkFixture(b)

	b.Run("all", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			result, err := CheckDocDrift(fixture.cfg, DocDriftRequest{Scope: "all"})
			if err != nil {
				b.Fatalf("CheckDocDrift() error = %v", err)
			}
			if len(result.DriftItems) == 0 {
				b.Fatal("CheckDocDrift() returned no drift items")
			}
		}
	})

	b.Run("targeted", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			result, err := CheckDocDrift(fixture.cfg, DocDriftRequest{
				DocRefs: []string{"doc://guides/api-rate-limits", "doc://runbooks/rate-limit-rollout"},
			})
			if err != nil {
				b.Fatalf("CheckDocDrift() targeted error = %v", err)
			}
			if len(result.DriftItems) == 0 {
				b.Fatal("CheckDocDrift() targeted returned no drift items")
			}
		}
	})
}

func BenchmarkReviewSpec(b *testing.B) {
	fixture := prepareBenchmarkFixture(b)

	b.Run("indexed", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			result, err := ReviewSpec(fixture.cfg, ReviewRequest{SpecRef: "SPEC-042"})
			if err != nil {
				b.Fatalf("ReviewSpec() error = %v", err)
			}
			if result.Overlap == nil || result.Impact == nil {
				b.Fatalf("ReviewSpec() result = %+v, want composed output", result)
			}
		}
	})

	b.Run("draft", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			result, err := ReviewSpec(fixture.cfg, ReviewRequest{SpecRecord: &fixture.draft})
			if err != nil {
				b.Fatalf("ReviewSpec() draft error = %v", err)
			}
			if result.Overlap == nil || result.Impact == nil {
				b.Fatalf("ReviewSpec() draft result = %+v, want composed output", result)
			}
		}
	})
}

func BenchmarkLoadIndexedTargets(b *testing.B) {
	fixture := prepareBenchmarkFixture(b)

	db, err := index.OpenReadOnlyContext(context.Background(), fixture.cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		b.Fatalf("index.OpenReadOnlyContext() error = %v", err)
	}
	defer db.Close()

	b.Run("specs_selected", func(b *testing.B) {
		refs := []string{"SPEC-042", "SPEC-055"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			specs, err := loadIndexedSpecsContext(context.Background(), db, refs)
			if err != nil {
				b.Fatalf("loadIndexedSpecsContext() error = %v", err)
			}
			if len(specs) != 2 {
				b.Fatalf("loadIndexedSpecsContext() returned %d specs, want 2", len(specs))
			}
		}
	})

	b.Run("docs_selected", func(b *testing.B) {
		refs := []string{"doc://guides/api-rate-limits", "doc://runbooks/rate-limit-rollout"}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			docs, err := loadIndexedDocsContext(context.Background(), db, refs)
			if err != nil {
				b.Fatalf("loadIndexedDocsContext() error = %v", err)
			}
			if len(docs) != 2 {
				b.Fatalf("loadIndexedDocsContext() returned %d docs, want 2", len(docs))
			}
		}
	})
}

func prepareBenchmarkFixture(b *testing.B) benchmarkFixture {
	b.Helper()

	cfg := loadFixtureConfig(b)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		b.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		b.Fatalf("index.Rebuild() error = %v", err)
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

	return benchmarkFixture{
		cfg:   cfg,
		draft: draft,
	}
}
