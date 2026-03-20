package index

import (
	"context"
	"errors"
	"testing"

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
