//go:build precision_bench

package index

import (
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	stindex "github.com/dusk-network/stroma/v2/index"
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

// evaluateChunkPrecisionCase scores on chunk-id rank order without any
// doc-ref dedup. Two hits from the same doc both count if both are in
// the relevant set. The status field is propagated from resolveChunkRelevance.
func evaluateChunkPrecisionCase(
	c precisionCase,
	hits []stindex.SearchHit,
	relevantChunkIDs map[int64]bool,
	status string,
) chunkPrecisionCaseResult {
	retrieved := make([]int64, 0, len(hits))
	for _, h := range hits {
		retrieved = append(retrieved, h.ChunkID)
	}
	top5 := truncateInt64(retrieved, 5)
	top10 := truncateInt64(retrieved, 10)

	totalRelevant := len(relevantChunkIDs)
	ids := make([]int64, 0, totalRelevant)
	for id := range relevantChunkIDs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	return chunkPrecisionCaseResult{
		ID:                   c.ID,
		Query:                c.Query,
		RelevantChunkIDs:     ids,
		RetrievedTop10Chunks: top10,
		ChunkPrecisionAt5:    precisionAtInt64(top5, relevantChunkIDs, 5),
		ChunkPrecisionAt10:   precisionAtInt64(top10, relevantChunkIDs, 10),
		ChunkRecallAt10:      recallAtInt64(top10, relevantChunkIDs, totalRelevant),
		ChunkReciprocalRank:  reciprocalRankInt64(top10, relevantChunkIDs),
		ResolveStatus:        status,
	}
}

func precisionAtInt64(retrieved []int64, relevant map[int64]bool, k int) float64 {
	if k <= 0 {
		return 0
	}
	hits := 0
	for _, r := range retrieved {
		if relevant[r] {
			hits++
		}
	}
	return float64(hits) / float64(k)
}

func recallAtInt64(retrieved []int64, relevant map[int64]bool, totalRelevant int) float64 {
	if totalRelevant <= 0 {
		return 0
	}
	hits := 0
	for _, r := range retrieved {
		if relevant[r] {
			hits++
		}
	}
	return float64(hits) / float64(totalRelevant)
}

func reciprocalRankInt64(retrieved []int64, relevant map[int64]bool) float64 {
	for i, r := range retrieved {
		if relevant[r] {
			return 1.0 / float64(i+1)
		}
	}
	return 0
}

func truncateInt64(values []int64, n int) []int64 {
	if len(values) <= n {
		return append([]int64{}, values...)
	}
	return append([]int64{}, values[:n]...)
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

// TestEvaluateChunkPrecisionCase: chunk-level scoring does NOT dedup by doc ref.
// Two hits with the same Ref but different ChunkID both count at chunk level.
func TestEvaluateChunkPrecisionCase(t *testing.T) {
	c := precisionCase{
		ID:              "chunk_rank_test",
		Query:           "q",
		RelevantDocRefs: []string{"doc://a"},
		RelevantSourceSpans: []sourceSpan{
			{DocRef: "doc://a", Anchor: "x"},
		},
	}
	hits := []stindex.SearchHit{
		{ChunkID: 10, Ref: "doc://a"}, // relevant (in set)
		{ChunkID: 20, Ref: "doc://b"}, // irrelevant
		{ChunkID: 11, Ref: "doc://a"}, // relevant (in set)
	}
	relevant := map[int64]bool{10: true, 11: true}
	got := evaluateChunkPrecisionCase(c, hits, relevant, resolveStatusOK)
	if got.ChunkPrecisionAt5 != 2.0/5.0 {
		t.Errorf("p@5 = %v, want 0.4", got.ChunkPrecisionAt5)
	}
	if got.ChunkReciprocalRank != 1.0 {
		t.Errorf("RR = %v, want 1.0 (first hit is relevant)", got.ChunkReciprocalRank)
	}
	if len(got.RetrievedTop10Chunks) != 3 || got.RetrievedTop10Chunks[2] != 11 {
		t.Errorf("RetrievedTop10Chunks = %v, want [10 20 11]", got.RetrievedTop10Chunks)
	}
	if got.ResolveStatus != resolveStatusOK {
		t.Errorf("ResolveStatus = %q, want %q", got.ResolveStatus, resolveStatusOK)
	}
}

// TestEvaluateChunkPrecisionCase_NoSpans returns a zero'd result with
// status resolveStatusNoSpans so the aggregator knows to skip it.
func TestEvaluateChunkPrecisionCase_NoSpans(t *testing.T) {
	c := precisionCase{ID: "x", Query: "q", RelevantDocRefs: []string{"doc://a"}}
	got := evaluateChunkPrecisionCase(c, nil, nil, resolveStatusNoSpans)
	if got.ResolveStatus != resolveStatusNoSpans {
		t.Errorf("status = %q, want %q", got.ResolveStatus, resolveStatusNoSpans)
	}
	if got.ChunkPrecisionAt5 != 0 || got.ChunkRecallAt10 != 0 {
		t.Errorf("expected zeroed scores, got p@5=%v r@10=%v", got.ChunkPrecisionAt5, got.ChunkRecallAt10)
	}
}
