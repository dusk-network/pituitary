package index

import (
	"context"
	"testing"

	stindex "github.com/dusk-network/stroma/v2/index"
)

// TestArmAwareRerankerBoostsFTSOnlyTerminologyMatch documents the one
// governance-aware decision the reranker currently makes: when a
// candidate is sourced only from the FTS arm and its heading overlaps
// with the query terminology, bump it above an equally-scored dense-only
// candidate. Dense-only hits for the same query carry no heading-literal
// signal and should stay below the FTS-only terminology match.
func TestArmAwareRerankerBoostsFTSOnlyTerminologyMatch(t *testing.T) {
	t.Parallel()

	reranker := armAwareHistoricalReranker{}
	hits := []stindex.SearchHit{
		{
			ChunkID: 1,
			Ref:     "spec-a",
			Heading: "Conceptual overview",
			Score:   0.50,
			Provenance: &stindex.HitProvenance{Arms: map[string]stindex.ArmEvidence{
				stindex.ArmVector: {Rank: 0, Score: 0.50},
			}},
		},
		{
			ChunkID: 2,
			Ref:     "spec-b",
			Heading: "Fusion policy",
			Score:   0.50,
			Provenance: &stindex.HitProvenance{Arms: map[string]stindex.ArmEvidence{
				stindex.ArmFTS: {Rank: 1, Score: -2.7},
			}},
		},
	}

	reranked, err := reranker.Rerank(context.Background(), "fusion policy", hits)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if reranked[0].Ref != "spec-b" {
		t.Fatalf("FTS-only terminology hit should rank first; got order %q, %q",
			reranked[0].Ref, reranked[1].Ref)
	}
}

// TestArmAwareRerankerIgnoresBoostWithoutHeadingMatch confirms the boost
// is terminology-gated: an FTS-only hit whose heading does not overlap
// with the query stays in its score-order position relative to the
// dense-only peer.
func TestArmAwareRerankerIgnoresBoostWithoutHeadingMatch(t *testing.T) {
	t.Parallel()

	reranker := armAwareHistoricalReranker{}
	hits := []stindex.SearchHit{
		{
			ChunkID: 1,
			Ref:     "spec-a",
			Heading: "Overview",
			Score:   0.50,
			Provenance: &stindex.HitProvenance{Arms: map[string]stindex.ArmEvidence{
				stindex.ArmVector: {Rank: 0, Score: 0.50},
			}},
		},
		{
			ChunkID: 2,
			Ref:     "spec-b",
			Heading: "Related work",
			Score:   0.50,
			Provenance: &stindex.HitProvenance{Arms: map[string]stindex.ArmEvidence{
				stindex.ArmFTS: {Rank: 1, Score: -2.7},
			}},
		},
	}

	reranked, err := reranker.Rerank(context.Background(), "fusion policy", hits)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if reranked[0].Ref != "spec-a" {
		t.Fatalf("without heading match the deterministic ref tiebreak should win; got %q first", reranked[0].Ref)
	}
}

// TestArmAwareRerankerPreservesOrderingForMultiArmHits documents that
// hits contributed by both arms do not receive the FTS-only boost — the
// boost is a signal for "this hit would not have been found by dense
// retrieval alone", which only applies to arm-exclusive hits.
func TestArmAwareRerankerPreservesOrderingForMultiArmHits(t *testing.T) {
	t.Parallel()

	reranker := armAwareHistoricalReranker{}
	hits := []stindex.SearchHit{
		{
			ChunkID: 1,
			Ref:     "spec-a",
			Heading: "Conceptual overview",
			Score:   0.50,
			Provenance: &stindex.HitProvenance{Arms: map[string]stindex.ArmEvidence{
				stindex.ArmVector: {Rank: 0, Score: 0.50},
			}},
		},
		{
			ChunkID: 2,
			Ref:     "spec-b",
			Heading: "Fusion policy",
			Score:   0.50,
			Provenance: &stindex.HitProvenance{Arms: map[string]stindex.ArmEvidence{
				stindex.ArmFTS:    {Rank: 1, Score: -2.7},
				stindex.ArmVector: {Rank: 3, Score: 0.48},
			}},
		},
	}

	reranked, err := reranker.Rerank(context.Background(), "fusion policy", hits)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	// Ties break on ref; spec-a comes first alphabetically, no boost applies.
	if reranked[0].Ref != "spec-a" {
		t.Fatalf("multi-arm hits should not receive the FTS-only boost; got %q first", reranked[0].Ref)
	}
}

// TestArmAwareRerankerIgnoresCustomExtraArmsOnFTSHit guards the
// pluggable-fusion contract: custom FusionStrategy implementations may
// introduce additional arm names beyond ArmVector/ArmFTS, and a hit
// contributed by FTS + a custom arm must NOT be treated as FTS-only
// (and therefore must not receive the terminology boost). Otherwise
// the reranker silently over-ranks multi-arm hits once non-default
// strategies land.
func TestArmAwareRerankerIgnoresCustomExtraArmsOnFTSHit(t *testing.T) {
	t.Parallel()

	reranker := armAwareHistoricalReranker{}
	hits := []stindex.SearchHit{
		{
			ChunkID: 1,
			Ref:     "spec-a",
			Heading: "Fusion policy",
			Score:   0.50,
			Provenance: &stindex.HitProvenance{Arms: map[string]stindex.ArmEvidence{
				stindex.ArmFTS: {Rank: 0, Score: -2.7},
				"custom_arm":   {Rank: 1, Score: 0.42},
			}},
		},
		{
			ChunkID: 2,
			Ref:     "spec-b",
			Heading: "Fusion policy",
			Score:   0.50,
			Provenance: &stindex.HitProvenance{Arms: map[string]stindex.ArmEvidence{
				stindex.ArmFTS: {Rank: 1, Score: -2.7},
			}},
		},
	}

	reranked, err := reranker.Rerank(context.Background(), "fusion policy", hits)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	// spec-b is single-arm FTS → gets the boost.
	// spec-a is multi-arm (FTS+custom) → no boost.
	// spec-b therefore ranks first.
	if reranked[0].Ref != "spec-b" {
		t.Fatalf("single-arm FTS hit should outrank FTS+custom multi-arm hit; got %q first",
			reranked[0].Ref)
	}
}

// TestArmAwareRerankerDoesNotBoostOnSubstringHeadingMatch guards the
// false-positive failure mode where `strings.Contains` would treat any
// heading containing a query token as a substring as a terminology
// match (e.g. "rate" inside "migration"). Token equality is required,
// so unrelated headings that happen to share a substring must not
// trigger the boost.
func TestArmAwareRerankerDoesNotBoostOnSubstringHeadingMatch(t *testing.T) {
	t.Parallel()

	reranker := armAwareHistoricalReranker{}
	hits := []stindex.SearchHit{
		{
			ChunkID: 1,
			Ref:     "spec-a",
			Heading: "Conceptual overview",
			Score:   0.50,
			Provenance: &stindex.HitProvenance{Arms: map[string]stindex.ArmEvidence{
				stindex.ArmVector: {Rank: 0, Score: 0.50},
			}},
		},
		{
			ChunkID: 2,
			Ref:     "spec-b",
			// "rate" is only a substring inside "migration", never a
			// standalone token. Must not receive the boost.
			Heading: "Migration plan",
			Score:   0.50,
			Provenance: &stindex.HitProvenance{Arms: map[string]stindex.ArmEvidence{
				stindex.ArmFTS: {Rank: 1, Score: -2.7},
			}},
		},
	}

	reranked, err := reranker.Rerank(context.Background(), "rate limits", hits)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	// Without a real terminology match the deterministic ref tiebreak
	// should win — "spec-a" < "spec-b".
	if reranked[0].Ref != "spec-a" {
		t.Fatalf("substring-only heading match should not trigger the terminology boost; got %q first",
			reranked[0].Ref)
	}
}

// TestArmAwareRerankerFallsBackWithoutProvenance proves the reranker
// degrades gracefully on pre-stroma-v2 snapshots that carry no
// HitProvenance — it must behave identically to historicalSectionReranker
// on those inputs so legacy retrieval paths stay byte-identical.
func TestArmAwareRerankerFallsBackWithoutProvenance(t *testing.T) {
	t.Parallel()

	armAware := armAwareHistoricalReranker{}
	historical := historicalSectionReranker{}
	hits := []stindex.SearchHit{
		{ChunkID: 1, Ref: "spec-a", Heading: "Fusion policy", Score: 0.50},
		{ChunkID: 2, Ref: "spec-b", Heading: "Conceptual overview", Score: 0.60},
		{ChunkID: 3, Ref: "spec-c", Heading: "Fusion policy", Score: 0.40},
	}

	armResult, err := armAware.Rerank(context.Background(), "fusion policy", cloneHits(hits))
	if err != nil {
		t.Fatalf("Rerank(armAware): %v", err)
	}
	histResult, err := historical.Rerank(context.Background(), "fusion policy", cloneHits(hits))
	if err != nil {
		t.Fatalf("Rerank(historical): %v", err)
	}

	if len(armResult) != len(histResult) {
		t.Fatalf("result lengths differ: arm=%d, hist=%d", len(armResult), len(histResult))
	}
	for i := range armResult {
		if armResult[i].ChunkID != histResult[i].ChunkID {
			t.Fatalf("without provenance arm-aware reranker diverged at pos %d: arm=%d hist=%d",
				i, armResult[i].ChunkID, histResult[i].ChunkID)
		}
	}
}

func cloneHits(in []stindex.SearchHit) []stindex.SearchHit {
	out := make([]stindex.SearchHit, len(in))
	copy(out, in)
	return out
}

func BenchmarkArmAwareRerankerOnFTSOnlyCandidates(b *testing.B) {
	hits := make([]stindex.SearchHit, 0, 64)
	for i := 0; i < 32; i++ {
		hits = append(hits, stindex.SearchHit{
			ChunkID: int64(i),
			Ref:     "spec-a",
			Heading: "Fusion policy",
			Score:   0.5,
			Provenance: &stindex.HitProvenance{Arms: map[string]stindex.ArmEvidence{
				stindex.ArmFTS: {Rank: i, Score: -2.7},
			}},
		})
	}
	for i := 0; i < 32; i++ {
		hits = append(hits, stindex.SearchHit{
			ChunkID: int64(1000 + i),
			Ref:     "spec-b",
			Heading: "Conceptual overview",
			Score:   0.5,
			Provenance: &stindex.HitProvenance{Arms: map[string]stindex.ArmEvidence{
				stindex.ArmVector: {Rank: i, Score: 0.5},
			}},
		})
	}

	reranker := armAwareHistoricalReranker{}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := reranker.Rerank(ctx, "fusion policy", cloneHits(hits)); err != nil {
			b.Fatal(err)
		}
	}
}
