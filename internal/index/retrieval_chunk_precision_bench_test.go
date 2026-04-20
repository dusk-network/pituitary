//go:build precision_bench

package index

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// ResolveStatus values for resolveChunkRelevance. Centralized as unexported
// consts so every write-site (implementation + tests + downstream callers in
// Task 4) goes through the same symbol rather than a typo-prone string literal.
const (
	resolveStatusOK         = "ok"
	resolveStatusNoSpans    = "no_spans"
	resolveStatusUnresolved = "unresolved"
)

// resolveChunkRelevance returns the set of chunk ids (across all spans) whose
// content contains the span's anchor substring. Matching is ASCII-case-
// insensitive and whitespace-collapsed; Unicode case folding is not
// applied — revisit if the corpus grows non-Latin content.
//
// Return semantics:
//   - status resolveStatusNoSpans → case has no source spans (caller skips chunk scoring)
//   - status resolveStatusUnresolved → ≥1 span anchor matched zero chunks. The
//     returned map still contains ids from spans that DID resolve, which is
//     useful for labeling-gap diagnosis.
//   - status resolveStatusOK → every span resolved to ≥1 chunk id
func resolveChunkRelevance(db *sql.DB, spans []sourceSpan) (map[int64]bool, string, error) {
	if len(spans) == 0 {
		return nil, resolveStatusNoSpans, nil
	}
	out := make(map[int64]bool)
	unresolved := false
	for _, span := range spans {
		needle := normalizeAnchor(span.Anchor)
		if needle == "" {
			unresolved = true
			continue
		}
		rows, err := db.Query(`SELECT id, content FROM chunks WHERE record_ref = ?`, span.DocRef)
		if err != nil {
			return nil, "", fmt.Errorf("query chunks for %s: %w", span.DocRef, err)
		}
		matched := 0
		for rows.Next() {
			var id int64
			var content string
			if err := rows.Scan(&id, &content); err != nil {
				rows.Close()
				return nil, "", fmt.Errorf("scan: %w", err)
			}
			if strings.Contains(normalizeAnchor(content), needle) {
				out[id] = true
				matched++
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, "", fmt.Errorf("rows iter for %s: %w", span.DocRef, err)
		}
		if err := rows.Close(); err != nil {
			return nil, "", err
		}
		if matched == 0 {
			unresolved = true
		}
	}
	if unresolved {
		return out, resolveStatusUnresolved, nil
	}
	return out, resolveStatusOK, nil
}

var whitespaceRun = regexp.MustCompile(`\s+`)

func normalizeAnchor(s string) string {
	return strings.ToLower(whitespaceRun.ReplaceAllString(strings.TrimSpace(s), " "))
}

// TestResolveChunkRelevance_AnchorMatch confirms that given a seeded chunks
// table, resolveChunkRelevance returns the chunk ids whose content contains
// the span anchor (case-insensitive, whitespace-normalized).
func TestResolveChunkRelevance_AnchorMatch(t *testing.T) {
	db := newTestChunksDB(t)
	// Seed: doc://a has two chunks; only chunk #2 contains the anchor.
	mustInsertChunk(t, db, 1, "doc://a", 0, "intro", "welcome to the guide")
	mustInsertChunk(t, db, 2, "doc://a", 1, "body", "the session boundary decides next steps")
	// doc://b has one chunk that does not contain the anchor.
	mustInsertChunk(t, db, 3, "doc://b", 0, "hdr", "unrelated content")

	spans := []sourceSpan{
		{DocRef: "doc://a", Anchor: "session boundary decides next steps"},
	}
	got, status, err := resolveChunkRelevance(db, spans)
	if err != nil {
		t.Fatalf("resolveChunkRelevance: %v", err)
	}
	if status != resolveStatusOK {
		t.Fatalf("status = %q, want %q", status, resolveStatusOK)
	}
	if len(got) != 1 || !got[2] {
		t.Fatalf("got = %v, want {2: true}", got)
	}
}

// TestResolveChunkRelevance_Unresolved confirms that a span whose anchor matches
// zero chunks surfaces "unresolved" — a labeling bug or SHA-pin mismatch.
func TestResolveChunkRelevance_Unresolved(t *testing.T) {
	db := newTestChunksDB(t)
	mustInsertChunk(t, db, 1, "doc://a", 0, "h", "nothing matches here")
	spans := []sourceSpan{
		{DocRef: "doc://a", Anchor: "this anchor is absent from every chunk"},
	}
	got, status, err := resolveChunkRelevance(db, spans)
	if err != nil {
		t.Fatalf("resolveChunkRelevance: %v", err)
	}
	if status != resolveStatusUnresolved {
		t.Fatalf("status = %q, want %q", status, resolveStatusUnresolved)
	}
	if len(got) != 0 {
		t.Fatalf("got = %v, want empty", got)
	}
}

// TestResolveChunkRelevance_NoSpans returns "no_spans" when the case has no spans.
func TestResolveChunkRelevance_NoSpans(t *testing.T) {
	db := newTestChunksDB(t)
	got, status, err := resolveChunkRelevance(db, nil)
	if err != nil {
		t.Fatalf("resolveChunkRelevance: %v", err)
	}
	if status != resolveStatusNoSpans {
		t.Fatalf("status = %q, want %q", status, resolveStatusNoSpans)
	}
	if len(got) != 0 {
		t.Fatalf("got = %v, want empty", got)
	}
}

func newTestChunksDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE chunks (
		id              INTEGER PRIMARY KEY,
		record_ref      TEXT NOT NULL,
		chunk_index     INTEGER NOT NULL,
		heading         TEXT NOT NULL,
		content         TEXT NOT NULL,
		context_prefix  TEXT NOT NULL DEFAULT '',
		parent_chunk_id INTEGER
	)`)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	return db
}

func mustInsertChunk(t *testing.T, db *sql.DB, id int64, ref string, idx int, heading, content string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO chunks(id, record_ref, chunk_index, heading, content) VALUES (?, ?, ?, ?, ?)`,
		id, ref, idx, heading, content)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}
