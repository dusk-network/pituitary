package index

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/ranking"
	"github.com/dusk-network/pituitary/internal/resultmeta"
	stindex "github.com/dusk-network/stroma/v2/index"
)

const (
	defaultSearchLimit = 10
	maxSearchLimit     = 50
)

const (
	// SearchSpecScoreKindHybridRelevance indicates scores from hybrid retrieval
	// that blends vector and lexical matches into a fused relevance rank.
	SearchSpecScoreKindHybridRelevance = "hybrid_relevance"

	// SearchSpecScoreKindSemanticSimilarity indicates scores from vector-only
	// retrieval where the score remains cosine-similarity-like.
	SearchSpecScoreKindSemanticSimilarity = "semantic_similarity"
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
	Matches          []SearchSpecMatch        `json:"matches"`
	ScoreKind        string                   `json:"score_kind,omitempty"`
	ScoreDescription string                   `json:"score_description,omitempty"`
	ContentTrust     *resultmeta.ContentTrust `json:"content_trust,omitempty"`
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

type historicalSectionReranker struct {
	preferHistorical bool
}

func (r historicalSectionReranker) Rerank(_ context.Context, _ string, candidates []stindex.SearchHit) ([]stindex.SearchHit, error) {
	reranked := append([]stindex.SearchHit(nil), candidates...)
	sort.SliceStable(reranked, func(i, j int) bool {
		leftScore := ranking.AdjustHistoricalSectionScore(reranked[i].Score, reranked[i].Heading, r.preferHistorical)
		rightScore := ranking.AdjustHistoricalSectionScore(reranked[j].Score, reranked[j].Heading, r.preferHistorical)
		switch {
		case leftScore != rightScore:
			return leftScore > rightScore
		case reranked[i].Ref != reranked[j].Ref:
			return reranked[i].Ref < reranked[j].Ref
		case reranked[i].Heading != reranked[j].Heading:
			return reranked[i].Heading < reranked[j].Heading
		default:
			return reranked[i].ChunkID < reranked[j].ChunkID
		}
	})
	return reranked, nil
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
	return searchSpecsContextWithScoreKind(ctx, cfg, query, SearchSpecScoreKindHybridRelevance, loadRankedCandidatesContext)
}

// SearchSpecsBySemanticSimilarityContext executes vector-only retrieval over
// the indexed spec corpus and returns cosine-similarity-like scores.
func SearchSpecsBySemanticSimilarityContext(ctx context.Context, cfg *config.Config, query SearchSpecQuery) (*SearchSpecResult, error) {
	return searchSpecsContextWithScoreKind(ctx, cfg, query, SearchSpecScoreKindSemanticSimilarity, loadSemanticSimilarityCandidatesContext)
}

// SearchSpecScoreColumnLabel returns the user-facing score column name for one
// search result score kind.
func SearchSpecScoreColumnLabel(kind string) string {
	switch kind {
	case SearchSpecScoreKindSemanticSimilarity:
		return "SIMILARITY"
	default:
		return "RELEVANCE"
	}
}

// SearchSpecScoreDescription returns a user-facing description of what the
// search score represents for one search result score kind.
func SearchSpecScoreDescription(kind string) string {
	switch kind {
	case SearchSpecScoreKindSemanticSimilarity:
		return "cosine-style semantic similarity from vector-only retrieval"
	default:
		return "hybrid relevance from fused vector and lexical retrieval; not a cosine similarity percentage"
	}
}

type candidateLoader func(context.Context, *sql.DB, *stindex.Snapshot, Embedder, SearchSpecQuery) ([]chunkCandidate, error)

func searchSpecsContextWithScoreKind(ctx context.Context, cfg *config.Config, query SearchSpecQuery, scoreKind string, loader candidateLoader) (*SearchSpecResult, error) {
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

	searchCtx, err := openSearchContext(ctx, cfg, embedder)
	if err != nil {
		return nil, err
	}
	defer searchCtx.Close()

	candidates, err := loader(ctx, searchCtx.db, searchCtx.snapshot, searchCtx.embedder, query)
	if err != nil {
		return nil, err
	}

	return buildSearchSpecResult(candidates, scoreKind), nil
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

type searchContext struct {
	db       *sql.DB
	snapshot *stindex.Snapshot
	embedder Embedder
}

func openSearchContext(ctx context.Context, cfg *config.Config, embedder Embedder) (*searchContext, error) {
	db, err := OpenReadOnlyContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return nil, fmt.Errorf("open index %s: %w", cfg.Workspace.ResolvedIndexPath, err)
	}

	snapshot, err := OpenStromaSnapshotContext(ctx, db, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	dimension, err := embedder.Dimension(ctx)
	if err != nil {
		_ = snapshot.Close()
		_ = db.Close()
		return nil, fmt.Errorf("resolve embedder dimension: %w", err)
	}
	if err := validateStoredEmbedderContext(ctx, db, embedder.Fingerprint(), dimension); err != nil {
		_ = snapshot.Close()
		_ = db.Close()
		return nil, err
	}
	return &searchContext{
		db:       db,
		snapshot: snapshot,
		embedder: embedder,
	}, nil
}

func (s *searchContext) Close() {
	if s == nil {
		return
	}
	if s.snapshot != nil {
		_ = s.snapshot.Close()
	}
	if s.db != nil {
		_ = s.db.Close()
	}
}

func loadRankedCandidatesContext(ctx context.Context, db *sql.DB, snapshot *stindex.Snapshot, embedder Embedder, query SearchSpecQuery) ([]chunkCandidate, error) {
	preferHistorical := ranking.SearchPrefersHistoricalContext(query.Query)

	hits, err := snapshot.Search(ctx, stindex.SnapshotSearchQuery{
		SearchParams: stindex.SearchParams{
			Text:     query.Query,
			Limit:    searchCandidateLimit(query.Limit),
			Kinds:    []string{query.Kind},
			Embedder: embedder,
			Reranker: historicalSectionReranker{preferHistorical: preferHistorical},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query search candidates: %w", err)
	}

	return buildRankedCandidatesContext(ctx, db, hits, query, preferHistorical)
}

func loadSemanticSimilarityCandidatesContext(ctx context.Context, db *sql.DB, snapshot *stindex.Snapshot, embedder Embedder, query SearchSpecQuery) ([]chunkCandidate, error) {
	preferHistorical := ranking.SearchPrefersHistoricalContext(query.Query)

	vectors, err := embedder.EmbedQueries(ctx, []string{query.Query})
	if err != nil {
		return nil, fmt.Errorf("embed search query: %w", err)
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("embedder returned %d query vectors, want 1", len(vectors))
	}

	hits, err := snapshot.SearchVector(ctx, stindex.VectorSearchQuery{
		Embedding: vectors[0],
		Limit:     searchCandidateLimit(query.Limit),
		Kinds:     []string{query.Kind},
	})
	if err != nil {
		return nil, fmt.Errorf("query semantic similarity candidates: %w", err)
	}

	return buildRankedCandidatesContext(ctx, db, hits, query, preferHistorical)
}

func buildRankedCandidatesContext(ctx context.Context, db *sql.DB, hits []stindex.SearchHit, query SearchSpecQuery, preferHistorical bool) ([]chunkCandidate, error) {
	states, err := loadSearchArtifactStateContext(ctx, db, refsForSearchHits(hits))
	if err != nil {
		return nil, err
	}

	candidates := make([]chunkCandidate, 0, query.Limit)
	for _, hit := range hits {
		state := states[hit.Ref]
		candidate := chunkCandidate{
			ChunkID:        hit.ChunkID,
			Ref:            hit.Ref,
			Title:          hit.Title,
			SectionHeading: hit.Heading,
			Content:        strings.TrimSpace(hit.Content),
			SourceRef:      hit.SourceRef,
			Kind:           hit.Kind,
			Status:         state.Status,
			Domain:         state.Domain,
		}
		if !candidateMatchesSearchQuery(candidate, query) {
			continue
		}
		if len(hit.Metadata) > 0 {
			candidate.Repo = strings.TrimSpace(hit.Metadata["repo_id"])
			candidate.Inference, err = model.DecodeInferenceConfidence(hit.Metadata)
			if err != nil {
				return nil, fmt.Errorf("decode search inference for %s: %w", candidate.Ref, err)
			}
		}
		candidate.Score = ranking.AdjustHistoricalSectionScore(hit.Score, candidate.SectionHeading, preferHistorical)
		if candidate.Score <= 0 {
			continue
		}
		candidates = append(candidates, candidate)
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

func buildSearchSpecResult(candidates []chunkCandidate, scoreKind string) *SearchSpecResult {
	result := &SearchSpecResult{
		Matches:          make([]SearchSpecMatch, 0, len(candidates)),
		ScoreKind:        scoreKind,
		ScoreDescription: SearchSpecScoreDescription(scoreKind),
		ContentTrust:     resultmeta.UntrustedWorkspaceText(),
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
	return result
}

type searchArtifactState struct {
	Status string
	Domain string
}

func refsForSearchHits(hits []stindex.SearchHit) []string {
	if len(hits) == 0 {
		return nil
	}
	refs := make([]string, 0, len(hits))
	seen := make(map[string]struct{}, len(hits))
	for _, hit := range hits {
		if _, ok := seen[hit.Ref]; ok {
			continue
		}
		seen[hit.Ref] = struct{}{}
		refs = append(refs, hit.Ref)
	}
	return refs
}

func loadSearchArtifactStateContext(ctx context.Context, db *sql.DB, refs []string) (map[string]searchArtifactState, error) {
	states := make(map[string]searchArtifactState, len(refs))
	if len(refs) == 0 {
		return states, nil
	}

	var builder strings.Builder
	args := make([]any, 0, len(refs))

	builder.WriteString(`
SELECT
  a.ref,
  COALESCE(a.status, ''),
  COALESCE(a.domain, '')
FROM artifacts a
WHERE a.ref IN (`)
	for i, ref := range refs {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString("?")
		args = append(args, ref)
	}
	builder.WriteString(`)`)

	rows, err := db.QueryContext(ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query search record state: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			ref    string
			status string
			domain string
		)
		if err := rows.Scan(&ref, &status, &domain); err != nil {
			return nil, fmt.Errorf("scan search record state: %w", err)
		}
		states[ref] = searchArtifactState{Status: status, Domain: domain}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search record state: %w", err)
	}
	return states, nil
}

func candidateMatchesSearchQuery(candidate chunkCandidate, query SearchSpecQuery) bool {
	if candidate.Kind != query.Kind {
		return false
	}
	if len(query.Statuses) > 0 {
		matched := false
		for _, status := range query.Statuses {
			if candidate.Status == status {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if query.Domain != "" && candidate.Domain != query.Domain {
		return false
	}
	return true
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
