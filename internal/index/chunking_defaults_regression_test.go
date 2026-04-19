package index

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	pchunk "github.com/dusk-network/pituitary/internal/chunk"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/source"
)

// TestValidateStoredChunkingConfigRejectsLegacyMissingFingerprint is
// the regression for the high-severity Codex finding on #344: a
// snapshot built before chunking_config_fingerprint was persisted
// still carried the pre-#344 (v1) resolver defaults. If the validator
// skipped the check on missing-fingerprint indexes AND the live
// resolver has advanced to v2 (doc default is now LateChunkPolicy),
// the first incremental update would re-chunk only the changed
// records under late-chunk and leave the rest on Markdown — the
// mixed-generation hazard the fingerprint exists to prevent.
//
// The validator must reject the missing-fingerprint branch whenever
// pchunk.ResolverDefaultsVersion != "1", forcing a full rebuild
// instead of a silently mixed update.
func TestValidateStoredChunkingConfigRejectsLegacyMissingFingerprint(t *testing.T) {
	t.Parallel()

	if pchunk.ResolverDefaultsVersion == "1" {
		t.Skip("regression only applies once resolver defaults have advanced past v1")
	}

	repoDir := t.TempDir()
	indexPath := filepath.Join(t.TempDir(), "pituitary.db")
	configPath := filepath.Join(t.TempDir(), "pituitary.toml")
	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "legacy.md"), "# Legacy\n\nbody.\n")
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
include = ["guides/*.md"]
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

	// Simulate a pre-fingerprint snapshot by deleting the metadata row.
	ctx := context.Background()
	db, err := sql.Open("sqlite3", indexPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM metadata WHERE key = 'chunking_config_fingerprint'`); err != nil {
		db.Close()
		t.Fatalf("clear chunking_config_fingerprint: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close index db: %v", err)
	}

	roDB, err := OpenReadOnlyContext(ctx, indexPath)
	if err != nil {
		t.Fatalf("OpenReadOnlyContext: %v", err)
	}
	defer roDB.Close()

	err = validateStoredChunkingConfigContext(ctx, roDB, cfg.Runtime.Chunking)
	if err == nil {
		t.Fatalf("validateStoredChunkingConfigContext on missing fingerprint = nil, want error (legacy snapshots must rebuild once defaults move past v1)")
	}
	if !strings.Contains(err.Error(), "resolver defaults have advanced") {
		t.Fatalf("error should mention advanced resolver defaults; got: %v", err)
	}
}

// TestRebuildChunkCountReflectsLateChunkLeaves is the regression for
// the medium-severity Codex finding on #344: summarizeRebuild's
// predictive ChunkCount used chunk.Markdown, which undercounts docs
// once LateChunkPolicy emits parent+leaf rows. The rebuild result
// must now reflect the actual snapshot chunk count so operators can
// see the real index-size / embed-cost footprint of the default flip.
func TestRebuildChunkCountReflectsLateChunkLeaves(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	indexPath := filepath.Join(t.TempDir(), "pituitary.db")
	configPath := filepath.Join(t.TempDir(), "pituitary.toml")

	// Build a doc body with one oversized section so LateChunkPolicy
	// must split it into parent + multiple leaves. "word " * 400 is
	// well above any reasonable ChildMaxTokens.
	longSection := "# Long section\n\n" + strings.Repeat("word ", 400) + "\n"
	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "long.md"), longSection)

	// Explicit LateChunkPolicy with a small ChildMaxTokens so the
	// split happens deterministically regardless of the resolver's
	// default tuning.
	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(repoDir)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[runtime.chunking.doc]
policy = "late_chunk"
max_tokens = 512
child_max_tokens = 64
child_overlap_tokens = 8

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig: %v", err)
	}
	result, err := Rebuild(cfg, records)
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	// chunk.Markdown would return one section for this body (single
	// heading with a long paragraph). LateChunkPolicy must emit a
	// parent plus at least two leaves, so the post-refresh ChunkCount
	// must exceed the Markdown baseline.
	if result.ChunkCount <= 1 {
		t.Fatalf("result.ChunkCount = %d, want > 1 (parent + leaves under LateChunkPolicy)", result.ChunkCount)
	}
}
