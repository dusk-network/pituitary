//go:build precision_bench

package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	stindex "github.com/dusk-network/stroma/v2/index"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/source"
	"github.com/dusk-network/pituitary/sdk"
)

// TestRetrievalPrecisionBench measures precision@k / recall@10 / MRR for the
// doc arm of the index against a labeled cases file. It is opt-in via the
// precision_bench build tag because it needs a live embedder.
//
// Environment:
//
//	PITUITARY_PRECISION_CONFIG        path to workspace config toml (required)
//	PITUITARY_PRECISION_CASES         path to cases JSON (required)
//	PITUITARY_PRECISION_REPORT        path to write JSON report (optional)
//	PITUITARY_PRECISION_LABEL         string tag for the run (optional)
//	PITUITARY_PRECISION_SKIP_REBUILD  if "1", skip rebuild and reuse snapshot
func TestRetrievalPrecisionBench(t *testing.T) {
	configPath := strings.TrimSpace(os.Getenv("PITUITARY_PRECISION_CONFIG"))
	casesPath := strings.TrimSpace(os.Getenv("PITUITARY_PRECISION_CASES"))
	if configPath == "" || casesPath == "" {
		t.Skip("set PITUITARY_PRECISION_CONFIG and PITUITARY_PRECISION_CASES to run")
	}

	ctx := context.Background()

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config %s: %v", configPath, err)
	}

	cases, err := loadPrecisionCases(casesPath)
	if err != nil {
		t.Fatalf("load cases %s: %v", casesPath, err)
	}
	if len(cases) == 0 {
		t.Fatalf("cases file %s has no cases", casesPath)
	}

	if os.Getenv("PITUITARY_PRECISION_SKIP_REBUILD") != "1" {
		records, err := source.LoadFromConfig(cfg)
		if err != nil {
			t.Fatalf("load sources: %v", err)
		}
		if _, err := RebuildContext(ctx, cfg, records); err != nil {
			t.Fatalf("rebuild: %v", err)
		}
	}

	db, err := OpenReadOnlyContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("open index db %s: %v", cfg.Workspace.ResolvedIndexPath, err)
	}
	defer db.Close()

	snapshot, err := OpenStromaSnapshotContext(ctx, db, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("open stroma snapshot: %v", err)
	}
	defer snapshot.Close()

	// resolveChunkRelevance needs a *sql.DB against the stroma chunks
	// database — which is a separate file from the registry DB opened
	// by OpenReadOnlyContext. snapshot.Path() returns the stroma snapshot's
	// on-disk path; open a read-only sibling handle against it.
	sqlDB, err := sql.Open("sqlite3", snapshot.Path()+"?mode=ro")
	if err != nil {
		t.Fatalf("open stroma sql handle: %v", err)
	}
	defer sqlDB.Close()

	embedder, err := newEmbedder(cfg.Runtime.Embedder)
	if err != nil {
		t.Fatalf("build embedder: %v", err)
	}

	rep := &precisionReport{
		Label:       strings.TrimSpace(os.Getenv("PITUITARY_PRECISION_LABEL")),
		ConfigPath:  configPath,
		CasesPath:   casesPath,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		CaseCount:   len(cases),
	}

	chunkEligible := 0
	for _, c := range cases {
		hits, err := snapshot.Search(ctx, stindex.SnapshotSearchQuery{
			SearchParams: stindex.SearchParams{
				Text:     c.Query,
				Limit:    10,
				Kinds:    []string{sdk.ArtifactKindDoc},
				Embedder: embedder,
			},
		})
		if err != nil {
			t.Fatalf("case %s: search: %v", c.ID, err)
		}
		cr := evaluatePrecisionCase(c, hits)
		rep.Cases = append(rep.Cases, cr)
		rep.MeanPrecisionAt5 += cr.PrecisionAt5
		rep.MeanPrecisionAt10 += cr.PrecisionAt10
		rep.MeanRecallAt10 += cr.RecallAt10
		rep.MeanReciprocalRank += cr.ReciprocalRank

		relevant, status, err := resolveChunkRelevance(sqlDB, c.RelevantSourceSpans)
		if err != nil {
			t.Fatalf("case %s: resolve chunk relevance: %v", c.ID, err)
		}
		ccr := evaluateChunkPrecisionCase(c, hits, relevant, status)
		rep.ChunkCases = append(rep.ChunkCases, ccr)
		if status == resolveStatusOK {
			chunkEligible++
			rep.MeanChunkPrecisionAt5 += ccr.ChunkPrecisionAt5
			rep.MeanChunkPrecisionAt10 += ccr.ChunkPrecisionAt10
			rep.MeanChunkRecallAt10 += ccr.ChunkRecallAt10
			rep.MeanChunkReciprocalRank += ccr.ChunkReciprocalRank
		}
	}
	n := float64(len(cases))
	rep.MeanPrecisionAt5 /= n
	rep.MeanPrecisionAt10 /= n
	rep.MeanRecallAt10 /= n
	rep.MeanReciprocalRank /= n
	rep.ChunkCaseCount = chunkEligible
	if chunkEligible > 0 {
		cn := float64(chunkEligible)
		rep.MeanChunkPrecisionAt5 /= cn
		rep.MeanChunkPrecisionAt10 /= cn
		rep.MeanChunkRecallAt10 /= cn
		rep.MeanChunkReciprocalRank /= cn
	}

	if reportPath := strings.TrimSpace(os.Getenv("PITUITARY_PRECISION_REPORT")); reportPath != "" {
		if err := writePrecisionReport(reportPath, rep); err != nil {
			t.Fatalf("write report %s: %v", reportPath, err)
		}
		t.Logf("wrote precision report: %s", reportPath)
	}

	t.Logf(
		"label=%q cases=%d p@5=%.3f p@10=%.3f r@10=%.3f mrr=%.3f",
		rep.Label, rep.CaseCount,
		rep.MeanPrecisionAt5, rep.MeanPrecisionAt10,
		rep.MeanRecallAt10, rep.MeanReciprocalRank,
	)
}

type precisionCase struct {
	ID                  string       `json:"id"`
	Query               string       `json:"query"`
	RelevantDocRefs     []string     `json:"relevant_doc_refs"`
	RelevantSourceSpans []sourceSpan `json:"relevant_source_spans,omitempty"`
	Tags                []string     `json:"tags,omitempty"`
}

// sourceSpan pins the answering text for a case via a distinctive
// anchor substring that must appear in a chunk's content for that
// chunk to be counted relevant under the chunk-level metric.
// start_line/end_line are optional provenance only — scoring does
// not consume them.
type sourceSpan struct {
	DocRef    string `json:"doc_ref"`
	Anchor    string `json:"anchor"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

type precisionCaseResult struct {
	ID             string   `json:"id"`
	Query          string   `json:"query"`
	Relevant       []string `json:"relevant"`
	RetrievedTop10 []string `json:"retrieved_top_10"`
	PrecisionAt5   float64  `json:"precision_at_5"`
	PrecisionAt10  float64  `json:"precision_at_10"`
	RecallAt10     float64  `json:"recall_at_10"`
	ReciprocalRank float64  `json:"reciprocal_rank"`
}

type chunkPrecisionCaseResult struct {
	ID                   string  `json:"id"`
	Query                string  `json:"query"`
	RelevantChunkIDs     []int64 `json:"relevant_chunk_ids"`
	RetrievedTop10Chunks []int64 `json:"retrieved_top_10_chunk_ids"`
	ChunkPrecisionAt5    float64 `json:"chunk_precision_at_5"`
	ChunkPrecisionAt10   float64 `json:"chunk_precision_at_10"`
	ChunkRecallAt10      float64 `json:"chunk_recall_at_10"`
	ChunkReciprocalRank  float64 `json:"chunk_reciprocal_rank"`
	// ResolveStatus is one of: "ok", "no_spans" (case has no source spans),
	// "unresolved" (≥1 span anchor matched zero chunks — labeling or pin bug).
	ResolveStatus string `json:"resolve_status"`
}

type precisionReport struct {
	Label              string                `json:"label,omitempty"`
	GeneratedAt        string                `json:"generated_at"`
	ConfigPath         string                `json:"config_path"`
	CasesPath          string                `json:"cases_path"`
	CaseCount          int                   `json:"case_count"`
	MeanPrecisionAt5   float64               `json:"mean_precision_at_5"`
	MeanPrecisionAt10  float64               `json:"mean_precision_at_10"`
	MeanRecallAt10     float64               `json:"mean_recall_at_10"`
	MeanReciprocalRank float64               `json:"mean_reciprocal_rank"`
	Cases              []precisionCaseResult `json:"cases"`
	// Chunk-level aggregates — populated only when ≥1 case has spans.
	ChunkCaseCount          int                        `json:"chunk_case_count"`
	MeanChunkPrecisionAt5   float64                    `json:"mean_chunk_precision_at_5,omitempty"`
	MeanChunkPrecisionAt10  float64                    `json:"mean_chunk_precision_at_10,omitempty"`
	MeanChunkRecallAt10     float64                    `json:"mean_chunk_recall_at_10,omitempty"`
	MeanChunkReciprocalRank float64                    `json:"mean_chunk_reciprocal_rank,omitempty"`
	ChunkCases              []chunkPrecisionCaseResult `json:"chunk_cases,omitempty"`
	// SnapshotSizeBytes is folded in by the runner (not this test).
	SnapshotSizeBytes int64 `json:"snapshot_size_bytes,omitempty"`
}

func loadPrecisionCases(path string) ([]precisionCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []precisionCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, fmt.Errorf("decode cases: %w", err)
	}
	for i := range cases {
		cases[i].ID = strings.TrimSpace(cases[i].ID)
		cases[i].Query = strings.TrimSpace(cases[i].Query)
		if cases[i].ID == "" {
			return nil, fmt.Errorf("case %d: id is required", i)
		}
		if cases[i].Query == "" {
			return nil, fmt.Errorf("case %s: query is required", cases[i].ID)
		}
		if len(cases[i].RelevantDocRefs) == 0 {
			return nil, fmt.Errorf("case %s: relevant_doc_refs is required", cases[i].ID)
		}
		for j, span := range cases[i].RelevantSourceSpans {
			span.DocRef = strings.TrimSpace(span.DocRef)
			span.Anchor = strings.TrimSpace(span.Anchor)
			if span.DocRef == "" {
				return nil, fmt.Errorf("case %s span %d: doc_ref is required", cases[i].ID, j)
			}
			if span.Anchor == "" {
				return nil, fmt.Errorf("case %s span %d: anchor is required (≥5 words, distinctive)", cases[i].ID, j)
			}
			cases[i].RelevantSourceSpans[j] = span
		}
	}
	return cases, nil
}

func evaluatePrecisionCase(c precisionCase, hits []stindex.SearchHit) precisionCaseResult {
	// Dedupe to unique doc refs in rank order — a doc appearing in
	// multiple leaf chunks should still only count once at the doc level.
	uniqueRefs := make([]string, 0, len(hits))
	seen := make(map[string]bool, len(hits))
	for _, h := range hits {
		ref := strings.TrimSpace(h.Ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		uniqueRefs = append(uniqueRefs, ref)
	}

	relevant := make(map[string]bool, len(c.RelevantDocRefs))
	for _, r := range c.RelevantDocRefs {
		relevant[strings.TrimSpace(r)] = true
	}

	top5 := truncate(uniqueRefs, 5)
	top10 := truncate(uniqueRefs, 10)

	cr := precisionCaseResult{
		ID:             c.ID,
		Query:          c.Query,
		Relevant:       append([]string{}, c.RelevantDocRefs...),
		RetrievedTop10: top10,
		PrecisionAt5:   precisionAt(top5, relevant, 5),
		PrecisionAt10:  precisionAt(top10, relevant, 10),
		RecallAt10:     recallAt(top10, relevant, len(c.RelevantDocRefs)),
		ReciprocalRank: reciprocalRank(top10, relevant),
	}
	sort.Strings(cr.Relevant)
	return cr
}

func precisionAt(retrieved []string, relevant map[string]bool, k int) float64 {
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

func recallAt(retrieved []string, relevant map[string]bool, totalRelevant int) float64 {
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

func reciprocalRank(retrieved []string, relevant map[string]bool) float64 {
	for i, r := range retrieved {
		if relevant[r] {
			return 1.0 / float64(i+1)
		}
	}
	return 0
}

func truncate(values []string, n int) []string {
	if len(values) <= n {
		return append([]string{}, values...)
	}
	return append([]string{}, values[:n]...)
}

func writePrecisionReport(path string, rep *precisionReport) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
