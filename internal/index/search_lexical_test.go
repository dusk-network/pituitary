package index

import (
	"context"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/source"
)

func TestSearchSpecsLexicalModeBypassesEmbedder(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	result, err := SearchSpecsContext(context.Background(), cfg, SearchSpecQuery{
		Query: "rate limiting",
		Mode:  SearchSpecModeLexical,
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("SearchSpecsContext() error = %v", err)
	}
	if got, want := result.ScoreKind, SearchSpecScoreKindLexicalRelevance; got != want {
		t.Fatalf("result.ScoreKind = %q, want %q", got, want)
	}
	if !strings.Contains(result.ScoreDescription, "FTS-only") {
		t.Fatalf("result.ScoreDescription = %q, want FTS-only guidance", result.ScoreDescription)
	}
	if result.Provenance == nil {
		t.Fatal("result.Provenance = nil, want explicit lexical provenance")
	}
	if got, want := result.Provenance.Mode, "lexical"; got != want {
		t.Fatalf("provenance.Mode = %q, want %q", got, want)
	}
	if !result.Provenance.EmbedderBypassed {
		t.Fatal("provenance.EmbedderBypassed = false, want true for lexical mode")
	}
	if got := result.Provenance.FallbackReason; got != "" {
		t.Fatalf("provenance.FallbackReason = %q, want empty for explicit lexical", got)
	}
	if len(result.Matches) == 0 {
		t.Fatal("result.Matches = empty, want lexical hits for 'rate limiting'")
	}
}

func TestSearchSpecsLexicalModeOrdersDeterministically(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	first, err := SearchSpecsContext(context.Background(), cfg, SearchSpecQuery{
		Query: "rate limiting",
		Mode:  SearchSpecModeLexical,
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("SearchSpecsContext() #1 error = %v", err)
	}
	second, err := SearchSpecsContext(context.Background(), cfg, SearchSpecQuery{
		Query: "rate limiting",
		Mode:  SearchSpecModeLexical,
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("SearchSpecsContext() #2 error = %v", err)
	}
	if len(first.Matches) != len(second.Matches) {
		t.Fatalf("match counts differ: %d vs %d", len(first.Matches), len(second.Matches))
	}
	for i := range first.Matches {
		if first.Matches[i].Ref != second.Matches[i].Ref ||
			first.Matches[i].SectionHeading != second.Matches[i].SectionHeading ||
			first.Matches[i].Score != second.Matches[i].Score {
			t.Fatalf("match[%d] differs: %+v vs %+v", i, first.Matches[i], second.Matches[i])
		}
	}
}

func TestSearchSpecsRequestRejectsUnknownMode(t *testing.T) {
	t.Parallel()

	_, err := (SearchSpecRequest{Query: "anything", Mode: "vector_only"}).ToQuery()
	if err == nil {
		t.Fatal("ToQuery() error = nil, want unsupported-mode error")
	}
	if !strings.Contains(err.Error(), "unsupported search mode") {
		t.Fatalf("ToQuery() error = %q, want unsupported-mode message", err)
	}
}
