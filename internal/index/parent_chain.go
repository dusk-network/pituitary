package index

import (
	"context"
	"fmt"
	"strings"

	stchunk "github.com/dusk-network/stroma/v2/chunk"

	"github.com/dusk-network/pituitary/sdk"
)

// docLateChunkActive reports whether the resolved chunk policy routes
// doc records through LateChunkPolicy — either because an explicit
// [runtime.chunking.doc] set it, or because the #344 zero-config
// default applied. Unknown shapes (nil policy, raw MarkdownPolicy,
// unfamiliar wrappers) return false so the validator short-circuits
// without raising spurious errors.
func docLateChunkActive(policy stchunk.Policy) bool {
	router, ok := policy.(stchunk.KindRouterPolicy)
	if !ok {
		return false
	}
	_, ok = router.ByKind[sdk.ArtifactKindDoc].(stchunk.LateChunkPolicy)
	return ok
}

// validateDocParentChainContext asserts the structural invariants of
// LateChunkPolicy's parent/leaf output on doc records (#344 AC):
//
//  1. Any doc chunk with parent_chunk_id set must point at a chunk
//     belonging to the same record — the parent/leaf span stays
//     anchored inside one doc.
//  2. The referenced parent must itself be a real parent (parent_chunk_id
//     IS NULL). Stroma's LateChunkPolicy is one level deep; a leaf
//     pointing at another leaf would mean the shape regressed.
//
// Note that LateChunkPolicy legitimately emits doc records as purely
// flat parents (all parent_chunk_id NULL) when every heading-aware
// section fits within ChildMaxTokens — stroma skips leaf emission in
// that case to avoid duplicating the parent's body in a single-child
// split. So we cannot require that leaves exist; we only validate the
// shape of the leaves that do exist.
func validateDocParentChainContext(ctx context.Context, snapshotPath string) error {
	db, err := OpenReadOnlyContext(ctx, snapshotPath)
	if err != nil {
		return fmt.Errorf("open stroma snapshot %s for doc parent-chain validation: %w", snapshotPath, err)
	}
	defer db.Close()

	const countQuery = `
SELECT COUNT(*)
FROM   chunks c
JOIN   chunks p ON c.parent_chunk_id = p.id
JOIN   records r ON c.record_ref = r.ref
WHERE  r.kind = ?
       AND (c.record_ref <> p.record_ref
            OR p.parent_chunk_id IS NOT NULL)`

	var brokenCount int
	if err := db.QueryRowContext(ctx, countQuery, sdk.ArtifactKindDoc).Scan(&brokenCount); err != nil {
		return fmt.Errorf("query doc parent-chain invariant: %w", err)
	}
	if brokenCount == 0 {
		return nil
	}

	// Only when validation fails do we pay for a bounded sample of
	// offenders for the error message. GROUP_CONCAT over every row on
	// a healthy large snapshot would be wasted work (and can bump into
	// SQLite's GROUP_CONCAT length limits on very large corpora).
	const brokenSampleLimit = 10
	const sampleQuery = `
SELECT c.record_ref || '#' || c.id
FROM   chunks c
JOIN   chunks p ON c.parent_chunk_id = p.id
JOIN   records r ON c.record_ref = r.ref
WHERE  r.kind = ?
       AND (c.record_ref <> p.record_ref
            OR p.parent_chunk_id IS NOT NULL)
LIMIT  ?`

	rows, err := db.QueryContext(ctx, sampleQuery, sdk.ArtifactKindDoc, brokenSampleLimit)
	if err != nil {
		return fmt.Errorf("sample doc parent-chain offenders: %w", err)
	}
	defer rows.Close()
	var offenders []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return fmt.Errorf("scan doc parent-chain offender: %w", err)
		}
		offenders = append(offenders, ref)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate doc parent-chain offenders: %w", err)
	}

	sampleNote := ""
	if brokenCount > len(offenders) {
		sampleNote = fmt.Sprintf(" (sampled %d of %d)", len(offenders), brokenCount)
	}
	return fmt.Errorf(
		"doc parent-chain validation failed: %d doc leaf chunk(s) point at a parent outside the same record or at a non-root parent; offenders%s: %s",
		brokenCount, sampleNote, strings.Join(offenders, ","),
	)
}
