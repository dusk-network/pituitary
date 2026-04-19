package index

import (
	"context"
	"fmt"

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

	const query = `
SELECT COUNT(*),
       IFNULL(GROUP_CONCAT(c.record_ref || '#' || c.id, ','), '')
FROM   chunks c
JOIN   chunks p ON c.parent_chunk_id = p.id
JOIN   records r ON c.record_ref = r.ref
WHERE  r.kind = ?
       AND (c.record_ref <> p.record_ref
            OR p.parent_chunk_id IS NOT NULL)`

	var brokenCount int
	var brokenRefs string
	if err := db.QueryRowContext(ctx, query, sdk.ArtifactKindDoc).Scan(&brokenCount, &brokenRefs); err != nil {
		return fmt.Errorf("query doc parent-chain invariant: %w", err)
	}
	if brokenCount > 0 {
		return fmt.Errorf(
			"doc parent-chain validation failed: %d doc leaf chunk(s) point at a parent outside the same record or at a non-root parent; offenders: %s",
			brokenCount, brokenRefs,
		)
	}
	return nil
}
