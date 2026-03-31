package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/ranking"
	"github.com/dusk-network/pituitary/internal/resultmeta"
)

const (
	defaultSearchLimit = 10
	maxSearchLimit     = 50
)

// SearchSpecFilters captures the optional search filters exposed by transports.
type SearchSpecFilters struct {
	Domain   string   `json:"domain,omitempty"`
	Statuses []string `json:"statuses"`
}

// SearchSpecRequest is the shared transport-level search request.
type SearchSpecRequest struct {
	Query   string            `json:"query"`
	Filters SearchSpecFilters `json:"filters"`
	Limit   *int              `json:"limit,omitempty"`
}

// SearchSpecQuery is the normalized search request for the spec index.
type SearchSpecQuery struct {
	Query    string   `json:"query"`
	Kind     string   `json:"-"`
	Domain   string   `json:"domain,omitempty"`
	Statuses []string `json:"statuses"`
	Limit    int      `json:"limit,omitempty"`
}

// SearchSpecMatch is one ranked section match.
type SearchSpecMatch struct {
	Ref            string                     `json:"ref"`
	Title          string                     `json:"title"`
	SectionHeading string                     `json:"section_heading"`
	Excerpt        string                     `json:"excerpt"`
	Repo           string                     `json:"repo,omitempty"`
	SourceRef      string                     `json:"source_ref"`
	Kind           string                     `json:"kind,omitempty"`
	Status         string                     `json:"status,omitempty"`
	Domain         string                     `json:"domain,omitempty"`
	Score          float64                    `json:"score"`
	Inference      *model.InferenceConfidence `json:"inference,omitempty"`
}

// SearchSpecResult is the ranked retrieval output.
type SearchSpecResult struct {
	Matches      []SearchSpecMatch        `json:"matches"`
	ContentTrust *resultmeta.ContentTrust `json:"content_trust,omitempty"`
}

type chunkCandidate struct {
	ChunkID        int64
	Ref            string
	Title          string
	SectionHeading string
	Content        string
	Repo           string
	SourceRef      string
	Kind           string
	Status         string
	Domain         string
	Score          float64
	Inference      *model.InferenceConfidence
}

// ToQuery normalizes the transport request into an executable search query.
func (r SearchSpecRequest) ToQuery() (SearchSpecQuery, error) {
	statuses, err := NormalizeSearchStatuses(r.Filters.Statuses)
	if err != nil {
		return SearchSpecQuery{}, err
	}
	limit, err := normalizeRequestedSearchLimit(r.Limit)
	if err != nil {
		return SearchSpecQuery{}, err
	}
	return SearchSpecQuery{
		Query:    strings.TrimSpace(r.Query),
		Kind:     model.ArtifactKindSpec,
		Domain:   strings.TrimSpace(r.Filters.Domain),
		Statuses: statuses,
		Limit:    limit,
	}, nil
}

// NormalizeSearchStatuses applies the canonical status filter defaults and validation.
func NormalizeSearchStatuses(input []string) ([]string, error) {
	if len(input) == 0 {
		return []string{model.StatusDraft, model.StatusReview, model.StatusAccepted}, nil
	}

	seen := make(map[string]struct{}, len(input))
	statuses := make([]string, 0, len(input))
	for _, status := range input {
		status = strings.TrimSpace(status)
		if !isSupportedSearchStatus(status) {
			return nil, fmt.Errorf("unsupported status %q", status)
		}
		if _, ok := seen[status]; ok {
			continue
		}
		seen[status] = struct{}{}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// SearchSpecs executes semantic retrieval over the indexed spec corpus.
func SearchSpecs(cfg *config.Config, query SearchSpecQuery) (*SearchSpecResult, error) {
	return SearchSpecsContext(context.Background(), cfg, query)
}

// SearchSpecsContext executes semantic retrieval over the indexed spec corpus.
func SearchSpecsContext(ctx context.Context, cfg *config.Config, query SearchSpecQuery) (*SearchSpecResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	query, err := normalizeSearchSpecQuery(query)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(query.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}

	embedder, err := newEmbedder(cfg.Runtime.Embedder)
	if err != nil {
		return nil, err
	}

	db, err := OpenReadOnlyContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return nil, fmt.Errorf("open index %s: %w", cfg.Workspace.ResolvedIndexPath, err)
	}
	defer db.Close()

	vectors, err := embedder.EmbedQueries(ctx, []string{query.Query})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("embed query: returned %d vector(s) for 1 query", len(vectors))
	}
	if err := validateStoredEmbedderContext(ctx, db, embedder.Fingerprint(), len(vectors[0])); err != nil {
		return nil, err
	}

	candidates, err := loadRankedCandidatesContext(ctx, db, query, vectors[0])
	if err != nil {
		return nil, err
	}

	result := &SearchSpecResult{
		Matches:      make([]SearchSpecMatch, 0, len(candidates)),
		ContentTrust: resultmeta.UntrustedWorkspaceText(),
	}
	for _, candidate := range candidates {
		result.Matches = append(result.Matches, SearchSpecMatch{
			Ref:            candidate.Ref,
			Title:          candidate.Title,
			SectionHeading: candidate.SectionHeading,
			Excerpt:        excerpt(candidate.Content),
			Repo:           candidate.Repo,
			SourceRef:      candidate.SourceRef,
			Kind:           candidate.Kind,
			Status:         candidate.Status,
			Domain:         candidate.Domain,
			Score:          candidate.Score,
			Inference:      candidate.Inference,
		})
	}

	return result, nil
}

func normalizeSearchSpecQuery(query SearchSpecQuery) (SearchSpecQuery, error) {
	statuses, err := NormalizeSearchStatuses(query.Statuses)
	if err != nil {
		return SearchSpecQuery{}, err
	}
	limit, err := normalizeSearchQueryLimit(query.Limit)
	if err != nil {
		return SearchSpecQuery{}, err
	}
	query.Query = strings.TrimSpace(query.Query)
	query.Kind = defaultString(query.Kind, model.ArtifactKindSpec)
	query.Domain = strings.TrimSpace(query.Domain)
	query.Statuses = statuses
	query.Limit = limit
	return query, nil
}

func normalizeRequestedSearchLimit(limit *int) (int, error) {
	if limit == nil {
		return defaultSearchLimit, nil
	}
	return normalizePositiveSearchLimit(*limit)
}

func normalizeSearchQueryLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultSearchLimit, nil
	}
	return normalizePositiveSearchLimit(limit)
}

func normalizePositiveSearchLimit(limit int) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("limit must be greater than zero")
	}
	if limit > maxSearchLimit {
		return 0, fmt.Errorf("limit must be less than or equal to %d", maxSearchLimit)
	}
	return limit, nil
}

func isSupportedSearchStatus(status string) bool {
	switch status {
	case model.StatusDraft, model.StatusReview, model.StatusAccepted, model.StatusSuperseded, model.StatusDeprecated:
		return true
	default:
		return false
	}
}

func validateStoredEmbedderContext(ctx context.Context, db *sql.DB, fingerprint string, configured int) error {
	var raw string
	err := db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'embedder_dimension'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return fmt.Errorf("index is missing embedder metadata; run `pituitary index --rebuild`")
	}
	if err != nil {
		return fmt.Errorf("read index metadata: %w", err)
	}

	stored, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("parse embedder_dimension %q: %w", raw, err)
	}
	if stored != configured {
		return fmt.Errorf("index embedder dimension %d does not match configured embedder dimension %d; run `pituitary index --rebuild`", stored, configured)
	}

	if strings.TrimSpace(fingerprint) == "" {
		return nil
	}

	var storedFingerprint string
	err = db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'embedder_fingerprint'`).Scan(&storedFingerprint)
	if err == sql.ErrNoRows {
		return fmt.Errorf("index is missing embedder fingerprint metadata; run `pituitary index --rebuild`")
	}
	if err != nil {
		return fmt.Errorf("read index metadata: %w", err)
	}
	if storedFingerprint != fingerprint {
		return fmt.Errorf("index embedder fingerprint %q does not match configured embedder fingerprint %q; run `pituitary index --rebuild`", storedFingerprint, fingerprint)
	}
	return nil
}

func loadRankedCandidatesContext(ctx context.Context, db *sql.DB, query SearchSpecQuery, queryEmbedding []float64) ([]chunkCandidate, error) {
	queryBlob, err := encodeVectorBlob(queryEmbedding)
	if err != nil {
		return nil, fmt.Errorf("encode query embedding: %w", err)
	}
	preferHistorical := ranking.SearchPrefersHistoricalContext(query.Query)

	sqlText, args := buildCandidateQuery(query, queryBlob)
	rows, err := db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("query search candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]chunkCandidate, 0, query.Limit)
	for rows.Next() {
		var (
			candidate   chunkCandidate
			rawMetadata string
			distance    float64
		)
		if err := rows.Scan(
			&candidate.ChunkID,
			&candidate.Ref,
			&candidate.Title,
			&candidate.SectionHeading,
			&candidate.Content,
			&candidate.SourceRef,
			&candidate.Kind,
			&candidate.Status,
			&candidate.Domain,
			&rawMetadata,
			&distance,
		); err != nil {
			return nil, fmt.Errorf("scan search candidate: %w", err)
		}
		if strings.TrimSpace(rawMetadata) != "" {
			metadata := map[string]string{}
			if err := json.Unmarshal([]byte(rawMetadata), &metadata); err != nil {
				return nil, fmt.Errorf("parse search metadata for %s: %w", candidate.Ref, err)
			}
			candidate.Repo = strings.TrimSpace(metadata["repo_id"])
			candidate.Inference, err = model.DecodeInferenceConfidence(metadata)
			if err != nil {
				return nil, fmt.Errorf("decode search inference for %s: %w", candidate.Ref, err)
			}
		}

		candidate.Content = strings.TrimSpace(candidate.Content)
		candidate.Score = ranking.AdjustHistoricalSectionScore(
			cosineScoreFromDistance(distance),
			candidate.SectionHeading,
			preferHistorical,
		)
		if candidate.Score <= 0 {
			continue
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search candidates: %w", err)
	}
	sort.Slice(candidates, func(i, j int) bool {
		switch {
		case candidates[i].Score != candidates[j].Score:
			return candidates[i].Score > candidates[j].Score
		case candidates[i].Ref != candidates[j].Ref:
			return candidates[i].Ref < candidates[j].Ref
		case candidates[i].SectionHeading != candidates[j].SectionHeading:
			return candidates[i].SectionHeading < candidates[j].SectionHeading
		default:
			return candidates[i].ChunkID < candidates[j].ChunkID
		}
	})
	if len(candidates) > query.Limit {
		candidates = candidates[:query.Limit]
	}
	return candidates, nil
}

func buildCandidateQuery(query SearchSpecQuery, queryBlob []byte) (string, []any) {
	var builder strings.Builder
	args := make([]any, 0, 4+len(query.Statuses))

	builder.WriteString(`
WITH vector_hits AS (
  SELECT chunk_id, distance
  FROM chunks_vec
  WHERE embedding MATCH ? AND k = ?
  ORDER BY distance
)
SELECT
  vh.chunk_id,
  a.ref,
  a.title,
  c.section,
  c.content,
  a.source_ref,
  a.kind,
  COALESCE(a.status, ''),
  COALESCE(a.domain, ''),
  a.metadata_json,
  vh.distance
FROM vector_hits vh
JOIN chunks c ON c.id = vh.chunk_id
JOIN artifacts a ON a.ref = c.artifact_ref
WHERE a.kind = ?`)
	args = append(args, queryBlob, searchCandidateLimit(query.Limit), query.Kind)

	if len(query.Statuses) > 0 {
		builder.WriteString(" AND a.status IN (")
		for i, status := range query.Statuses {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString("?")
			args = append(args, status)
		}
		builder.WriteString(")")
	}

	if query.Domain != "" {
		builder.WriteString(" AND a.domain = ?")
		args = append(args, query.Domain)
	}

	builder.WriteString(`
ORDER BY vh.distance ASC, a.ref ASC, c.section ASC, vh.chunk_id ASC
LIMIT ?`)
	args = append(args, searchCandidateLimit(query.Limit))

	return builder.String(), args
}

func searchCandidateLimit(limit int) int {
	overfetch := limit * 5
	if overfetch < 50 {
		overfetch = 50
	}
	if overfetch > 250 {
		overfetch = 250
	}
	return overfetch
}

func excerpt(content string) string {
	content = strings.Join(strings.Fields(content), " ")
	if len(content) <= 180 {
		return content
	}
	return content[:177] + "..."
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
