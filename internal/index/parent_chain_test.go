package index

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	stchunk "github.com/dusk-network/stroma/v2/chunk"

	pchunk "github.com/dusk-network/pituitary/internal/chunk"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/source"
	"github.com/dusk-network/pituitary/sdk"
)

func TestDocLateChunkActive(t *testing.T) {
	t.Parallel()

	t.Run("nil policy is inactive", func(t *testing.T) {
		t.Parallel()
		if docLateChunkActive(nil) {
			t.Fatalf("docLateChunkActive(nil) = true, want false")
		}
	})
	t.Run("markdown-only router is inactive", func(t *testing.T) {
		t.Parallel()
		router := stchunk.KindRouterPolicy{
			Default: stchunk.MarkdownPolicy{},
			ByKind: map[string]stchunk.Policy{
				sdk.ArtifactKindDoc:  stchunk.MarkdownPolicy{},
				sdk.ArtifactKindSpec: stchunk.MarkdownPolicy{},
			},
		}
		if docLateChunkActive(router) {
			t.Fatalf("docLateChunkActive(markdown router) = true, want false")
		}
	})
	t.Run("router with doc late-chunk is active", func(t *testing.T) {
		t.Parallel()
		router := stchunk.KindRouterPolicy{
			Default: stchunk.MarkdownPolicy{},
			ByKind: map[string]stchunk.Policy{
				sdk.ArtifactKindDoc: stchunk.LateChunkPolicy{ChildMaxTokens: 256},
			},
		}
		if !docLateChunkActive(router) {
			t.Fatalf("docLateChunkActive(late-chunk router) = false, want true")
		}
	})
	t.Run("resolver #344 default is active", func(t *testing.T) {
		t.Parallel()
		// The zero-config resolver is what production takes when the
		// operator has no [runtime.chunking] block; the validator must
		// treat that as late-chunk active so publishes are guarded.
		policy, err := pchunk.Resolve(pchunk.Config{})
		if err != nil {
			t.Fatalf("Resolve(zero): %v", err)
		}
		if !docLateChunkActive(policy) {
			t.Fatalf("docLateChunkActive(Resolve(zero)) = false, want true (#344 default)")
		}
	})
}

// TestValidateDocParentChainContextRejectsMultiLevelChain constructs
// a synthetic snapshot where a doc chunk has parent_chunk_id pointing
// at another non-root chunk — a shape LateChunkPolicy never emits
// (it is one level deep by design). Stroma enforces same-record
// linkage via a trigger but does NOT forbid multi-level chains, so
// this is the exact gap the validator is here to cover.
func TestValidateDocParentChainContextRejectsMultiLevelChain(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repoDir := t.TempDir()
	indexPath := filepath.Join(t.TempDir(), "pituitary.db")
	configPath := filepath.Join(t.TempDir(), "pituitary.toml")

	// A doc body with multiple heading-aware sections so the snapshot
	// has several chunk rows we can stitch into an illegal chain.
	docBody := "# Part 1\n\nalpha\n\n# Part 2\n\nbravo\n\n# Part 3\n\ncharlie\n"
	mustWriteFile(t, filepath.Join(repoDir, "docs", "guide", "multi.md"), docBody)
	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(repoDir)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guide/*.md"]
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig: %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("initial Rebuild: %v", err)
	}

	snapshotPath := currentSnapshotPathForTest(t, indexPath)
	snapshotDB, err := sql.Open("sqlite3", snapshotPath)
	if err != nil {
		t.Fatalf("open snapshot db: %v", err)
	}
	defer snapshotDB.Close()

	// Build an illegal A→B→C chain inside the single doc record.
	rows, err := snapshotDB.QueryContext(ctx, `
SELECT id FROM chunks
 WHERE record_ref = 'doc://guide/multi'
 ORDER BY chunk_index
 LIMIT 3`)
	if err != nil {
		t.Fatalf("query chunk ids: %v", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatalf("scan id: %v", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) < 3 {
		t.Fatalf("need at least 3 chunks for the test; got %d", len(ids))
	}

	// Make id[1]'s parent = id[0] (legitimate one-level), then make
	// id[2]'s parent = id[1] (illegitimate two-level chain).
	if _, err := snapshotDB.ExecContext(ctx,
		`UPDATE chunks SET parent_chunk_id = ? WHERE id = ?`, ids[0], ids[1]); err != nil {
		t.Fatalf("set one-level parent: %v", err)
	}
	if _, err := snapshotDB.ExecContext(ctx,
		`UPDATE chunks SET parent_chunk_id = ? WHERE id = ?`, ids[1], ids[2]); err != nil {
		t.Fatalf("stage two-level chain: %v", err)
	}

	if err := validateDocParentChainContext(ctx, snapshotPath); err == nil {
		t.Fatalf("expected validation error for multi-level chain; got nil")
	} else if !containsAll(err.Error(), "parent-chain validation failed", "doc://guide/multi") {
		t.Fatalf("error should name the offender; got: %v", err)
	}
}

func currentSnapshotPathForTest(tb testing.TB, indexPath string) string {
	tb.Helper()
	ctx := context.Background()
	db, err := OpenReadOnlyContext(ctx, indexPath)
	if err != nil {
		tb.Fatalf("open index db: %v", err)
	}
	defer db.Close()
	snapshot, err := stromaSnapshotPathFromDBContext(ctx, db, indexPath)
	if err != nil {
		tb.Fatalf("stroma snapshot path: %v", err)
	}
	return snapshot
}

func containsAll(haystack string, needles ...string) bool {
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			return false
		}
	}
	return true
}
