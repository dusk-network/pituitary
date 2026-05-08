package index

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/ranking"
	stindex "github.com/dusk-network/stroma/v3/index"
)

// searchSpecsLexical executes FTS-only retrieval over the indexed spec
// corpus. It does not call the embedder and does not validate the
// stored embedder dimension or fingerprint, so it remains usable when
// the configured embedder runtime is unavailable.
//
// fallbackReason, when non-empty, is recorded on the result provenance
// so callers can tell hybrid retrieval was downgraded to lexical (vs.
// the caller asking for lexical explicitly).
func searchSpecsLexical(ctx context.Context, cfg *config.Config, query SearchSpecQuery, fallbackReason string) (*SearchSpecResult, error) {
	db, err := OpenReadOnlyContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return nil, fmt.Errorf("open index %s: %w", cfg.Workspace.ResolvedIndexPath, err)
	}
	defer db.Close()

	snapshot, err := OpenStromaSnapshotContext(ctx, db, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return nil, err
	}
	defer snapshot.Close()

	candidates, err := loadLexicalCandidatesContext(ctx, db, snapshot, query)
	if err != nil {
		return nil, err
	}

	result := buildSearchSpecResult(candidates, SearchSpecScoreKindLexicalRelevance)
	result.Provenance = &SearchSpecProvenance{
		Mode:             "lexical",
		EmbedderBypassed: true,
		FallbackReason:   fallbackReason,
	}
	return result, nil
}

func loadLexicalCandidatesContext(ctx context.Context, db *sql.DB, snapshot *stindex.Snapshot, query SearchSpecQuery) ([]chunkCandidate, error) {
	preferHistorical := ranking.SearchPrefersHistoricalContext(query.Query)

	hits, err := snapshot.SearchLexical(ctx, stindex.SnapshotLexicalSearchQuery{
		LexicalSearchParams: stindex.LexicalSearchParams{
			Text:  query.Query,
			Limit: searchCandidateLimit(query.Limit),
			Kinds: []string{query.Kind},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query lexical candidates: %w", err)
	}
	// Lexical retrieval has no fusion provenance and no vector arm, so
	// the historical-section ordering applies as it would in the
	// non-arm-aware default. Pass scorePreAdjusted=false so
	// buildRankedCandidatesContext computes the historical adjustment
	// once on the raw FTS score, using the same query-aware
	// preferHistorical signal as the hybrid and vector paths.
	return buildRankedCandidatesContext(ctx, db, hits, query, preferHistorical, false)
}
