package index

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/fusion"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/ranking"
	"github.com/dusk-network/pituitary/internal/resultmeta"
	stindex "github.com/dusk-network/stroma/v3/index"
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

	// SearchSpecScoreKindLexicalRelevance indicates scores from FTS-only
	// retrieval that bypassed the embedder.
	SearchSpecScoreKindLexicalRelevance = "lexical_relevance"
)

const (
	// SearchSpecModeHybrid is the default retrieval mode (vector + FTS fusion).
	SearchSpecModeHybrid = ""

	// SearchSpecModeLexical selects FTS-only retrieval that does not call
	// the embedder. Useful as an explicit fallback when the embedder is
	// unavailable or undesired.
	SearchSpecModeLexical = "lexical"
)

// SearchSpecFallbackReasonEmbedderUnavailable is recorded on the result
// provenance when the requested hybrid retrieval automatically degraded
// to lexical because the embedder runtime was unavailable.
const SearchSpecFallbackReasonEmbedderUnavailable = "embedder_unavailable"

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
	// Mode selects the retrieval mode. Empty means hybrid (default).
	// "lexical" forces FTS-only retrieval that bypasses the embedder.
	Mode string `json:"mode,omitempty"`
}

// SearchSpecQuery is the normalized search request for the spec index.
type SearchSpecQuery struct {
	Query    string   `json:"query"`
	Kind     string   `json:"-"`
	Domain   string   `json:"domain,omitempty"`
	Statuses []string `json:"statuses"`
	Limit    int      `json:"limit,omitempty"`
	Mode     string   `json:"mode,omitempty"`
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
	Provenance       *SearchSpecProvenance    `json:"provenance,omitempty"`
}

// SearchSpecProvenance reports how a search request was actually
// executed. It distinguishes the requested mode from the executed mode
// so callers can detect when hybrid retrieval automatically fell back
// to lexical because the embedder was unavailable.
type SearchSpecProvenance struct {
	// Mode is the retrieval mode actually used: "hybrid" or "lexical".
	Mode string `json:"mode"`
	// EmbedderBypassed is true when the request was answered without
	// calling the embedder (explicit lexical or fallback).
	EmbedderBypassed bool `json:"embedder_bypassed"`
	// FallbackReason names why hybrid was downgraded to lexical, when
	// applicable. Empty for explicit lexical or successful hybrid.
	FallbackReason string `json:"fallback_reason,omitempty"`
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

// armAwareHistoricalReranker extends historicalSectionReranker with
// governance-aware use of stroma's HitProvenance.Arms. When a query
// token literally matches a hit's section heading and the hit is
// sourced exclusively from the FTS arm, the reranker multiplies its
// adjusted score by a small boost so exact-terminology matches float
// above conceptually similar but off-terminology candidates.
//
// Hits with nil Provenance (e.g. legacy snapshots that pre-date the
// stroma v2 fusion pipeline) fall through to the same ordering as
// historicalSectionReranker — no boost, deterministic tiebreak on
// ref / heading / chunk.
type armAwareHistoricalReranker struct {
	preferHistorical bool
}

const armAwareTerminologyBoost = 1.05

func (r armAwareHistoricalReranker) Rerank(_ context.Context, query string, candidates []stindex.SearchHit) ([]stindex.SearchHit, error) {
	tokens := headingTerminologyTokens(query)
	reranked := append([]stindex.SearchHit(nil), candidates...)
	// Write the adjusted + boosted score back to each copy's Score so
	// downstream consumers (buildRankedCandidatesContext) see the
	// authoritative post-rerank value and do not silently re-sort by
	// the pre-rerank raw score — which would otherwise undo the
	// arm-aware boost entirely.
	for i := range reranked {
		reranked[i].Score = armAwareAdjustedScore(reranked[i], tokens, r.preferHistorical)
	}
	sort.SliceStable(reranked, func(i, j int) bool {
		switch {
		case reranked[i].Score != reranked[j].Score:
			return reranked[i].Score > reranked[j].Score
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

func armAwareAdjustedScore(hit stindex.SearchHit, queryTokens map[string]struct{}, preferHistorical bool) float64 {
	adjusted := ranking.AdjustHistoricalSectionScore(hit.Score, hit.Heading, preferHistorical)
	if !hitHasFTSOnlyProvenance(hit) {
		return adjusted
	}
	if !headingMatchesAnyToken(hit.Heading, queryTokens) {
		return adjusted
	}
	return adjusted * armAwareTerminologyBoost
}

// hitHasFTSOnlyProvenance reports whether the hit's provenance shows it
// was contributed by the FTS arm and no other arm. Requires exact
// single-arm provenance: the pluggable-fusion contract (stroma v2)
// allows custom FusionStrategy implementations to introduce additional
// arm names beyond ArmVector/ArmFTS, so a "has FTS, no Vector" check
// would misclassify multi-arm hits once non-default strategies land.
func hitHasFTSOnlyProvenance(hit stindex.SearchHit) bool {
	if hit.Provenance == nil || len(hit.Provenance.Arms) != 1 {
		return false
	}
	_, ok := hit.Provenance.Arms[stindex.ArmFTS]
	return ok
}

// normalizeTerminologyTokens lower-cases and strips punctuation from the
// input then returns the set of tokens at least 3 chars long. The
// heading match below does exact token equality, not substring
// containment, so unrelated words that happen to share a substring
// (e.g. "rate" vs "migration") don't falsely trigger the boost.
func normalizeTerminologyTokens(input string) map[string]struct{} {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	tokens := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(input, isTerminologyTokenSeparator) {
		token = strings.ToLower(token)
		if len(token) < 3 {
			continue
		}
		tokens[token] = struct{}{}
	}
	if len(tokens) == 0 {
		return nil
	}
	return tokens
}

func headingTerminologyTokens(query string) map[string]struct{} {
	return normalizeTerminologyTokens(query)
}

func headingMatchesAnyToken(heading string, queryTokens map[string]struct{}) bool {
	if len(queryTokens) == 0 {
		return false
	}
	headingTokens := normalizeTerminologyTokens(heading)
	for token := range headingTokens {
		if _, ok := queryTokens[token]; ok {
			return true
		}
	}
	return false
}

func isTerminologyTokenSeparator(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '.', ',', ';', ':', '!', '?', '"', '\'', '`',
		'(', ')', '[', ']', '{', '}', '/', '\\', '-', '_':
		return true
	}
	return false
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
	mode, err := normalizeSearchMode(r.Mode)
	if err != nil {
		return SearchSpecQuery{}, err
	}
	return SearchSpecQuery{
		Query:    strings.TrimSpace(r.Query),
		Kind:     model.ArtifactKindSpec,
		Domain:   strings.TrimSpace(r.Filters.Domain),
		Statuses: statuses,
		Limit:    limit,
		Mode:     mode,
	}, nil
}

// normalizeSearchMode validates the requested retrieval mode. Unknown
// values reject; the empty string and SearchSpecModeLexical are the
// only accepted inputs today.
func normalizeSearchMode(mode string) (string, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	switch mode {
	case SearchSpecModeHybrid, SearchSpecModeLexical:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported search mode %q", mode)
	}
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
//
// When query.Mode is SearchSpecModeLexical the embedder is bypassed and
// the request is answered from the snapshot's FTS index. When Mode is
// the default hybrid mode, retrieval automatically falls back to lexical
// if the embedder runtime is unavailable; the result's Provenance
// records whether the fallback occurred.
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

	if query.Mode == SearchSpecModeLexical {
		return searchSpecsLexical(ctx, cfg, query, "")
	}

	result, err := searchSpecsHybrid(ctx, cfg, query)
	if err == nil {
		return result, nil
	}
	// Only fall back for transient/recoverable embedder failures.
	// Auth and schema-mismatch failures indicate operator
	// misconfiguration; masking them with degraded lexical results
	// would let callers (CI, operators) trust output that never used
	// the configured embedder, so the original diagnostic surfaces
	// instead.
	if !IsRecoverableEmbedderFailure(err) {
		return nil, err
	}
	return searchSpecsLexical(ctx, cfg, query, SearchSpecFallbackReasonEmbedderUnavailable)
}

func searchSpecsHybrid(ctx context.Context, cfg *config.Config, query SearchSpecQuery) (*SearchSpecResult, error) {
	strategy, err := fusion.Resolve(fusionConfigFromRuntime(cfg.Runtime.Search))
	if err != nil {
		return nil, fmt.Errorf("resolve search fusion: %w", err)
	}
	loader := func(ctx context.Context, db *sql.DB, snapshot *stindex.Snapshot, embedder Embedder, query SearchSpecQuery) ([]chunkCandidate, error) {
		return loadRankedCandidatesContextWithFusion(ctx, db, snapshot, embedder, query, strategy, cfg.Runtime.Search.Reranker, cfg.Runtime.Search.PrefilterDimension)
	}
	// #341 preflight: reject an oversized matryoshka prefilter using
	// the index's stored embedder_dimension metadata, before any
	// embedder method is called. Threaded through openSearchContext so
	// the preflight reuses the same *sql.DB the search will, instead
	// of opening (and closing) a second handle on every hybrid query.
	result, err := searchSpecsContextWithScoreKindAndPrefilter(ctx, cfg, query, SearchSpecScoreKindHybridRelevance, loader, cfg.Runtime.Search.PrefilterDimension)
	if err != nil {
		return nil, err
	}
	result.Provenance = &SearchSpecProvenance{Mode: "hybrid"}
	return result, nil
}

// preflightMatryoshkaPrefilter rejects an oversized
// runtime.search.matryoshka_prefilter_dimension by comparing it against
// the index's stored embedder_dimension metadata. Returns nil when the
// prefilter is disabled (zero) or when the stored dimension is at least
// as large as the configured prefilter. Reads SQLite metadata through
// the supplied *sql.DB only — it must not call any embedder method, so
// a typo never burns an OpenAI-compatible round trip.
//
// This reads the business index's embedder_dimension. Pituitary keeps
// the business DB and the referenced stroma snapshot's metadata in
// lock-step at rebuild time, so reading either is equivalent for the
// dimension check. The quantization-vs-prefilter combination is rejected
// at config-load time (validateRuntimeQuantization) so a non-float32
// snapshot can never coexist with a configured matryoshka prefilter; if
// a stale snapshot somehow does, stroma's snapshot search rejects the
// pair with "SearchDimension is only supported for float32 indexes"
// before a real embedder call.
// preflightMatryoshkaPrefilterFromSnapshot is the snapshot-handle
// variant for callers that already hold a stroma snapshot but no
// SQLite handle (e.g. RetrieveOutlineContextWithSnapshotContext).
// EmbedderDimension on snapshot.Stats() mirrors the metadata
// embedder_dimension the SQLite preflight reads.
func preflightMatryoshkaPrefilterFromSnapshot(ctx context.Context, snapshot *stindex.Snapshot, prefilter int) error {
	if prefilter <= 0 {
		return nil
	}
	stats, err := snapshot.Stats(ctx)
	if err != nil {
		return fmt.Errorf("read stroma snapshot stats for prefilter preflight: %w", err)
	}
	if prefilter > stats.EmbedderDimension {
		return fmt.Errorf("runtime.search.matryoshka_prefilter_dimension %d exceeds stored embedder_dimension %d; lower the value or rebuild the index with a higher-dimension embedder", prefilter, stats.EmbedderDimension)
	}
	return nil
}

func preflightMatryoshkaPrefilter(ctx context.Context, db *sql.DB, prefilter int) error {
	if prefilter <= 0 {
		return nil
	}
	var raw string
	err := db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'embedder_dimension'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return fmt.Errorf("index is missing embedder metadata; run `pituitary index --rebuild`")
	}
	if err != nil {
		return fmt.Errorf("read embedder_dimension for prefilter preflight: %w", err)
	}
	stored, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("parse embedder_dimension %q: %w", raw, err)
	}
	if prefilter > stored {
		return fmt.Errorf("runtime.search.matryoshka_prefilter_dimension %d exceeds stored embedder_dimension %d; lower the value or rebuild the index with a higher-dimension embedder", prefilter, stored)
	}
	return nil
}

func fusionConfigFromRuntime(cfg config.SearchConfig) fusion.Config {
	return fusion.Config{
		Strategy: cfg.Fusion.Strategy,
		K:        cfg.Fusion.K,
	}
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
	case SearchSpecScoreKindLexicalRelevance:
		return "LEXICAL"
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
	case SearchSpecScoreKindLexicalRelevance:
		return "lexical relevance from FTS-only retrieval; embedder was bypassed"
	default:
		return "hybrid relevance from fused vector and lexical retrieval; not a cosine similarity percentage"
	}
}

type candidateLoader func(context.Context, *sql.DB, *stindex.Snapshot, Embedder, SearchSpecQuery) ([]chunkCandidate, error)

func searchSpecsContextWithScoreKind(ctx context.Context, cfg *config.Config, query SearchSpecQuery, scoreKind string, loader candidateLoader) (*SearchSpecResult, error) {
	return searchSpecsContextWithScoreKindAndPrefilter(ctx, cfg, query, scoreKind, loader, 0)
}

// searchSpecsContextWithScoreKindAndPrefilter is the hybrid-aware
// variant: prefilterDimension > 0 runs the matryoshka preflight against
// the same SQLite handle the search will use, before any embedder
// method is invoked. Non-hybrid callers pass zero to skip the check.
func searchSpecsContextWithScoreKindAndPrefilter(ctx context.Context, cfg *config.Config, query SearchSpecQuery, scoreKind string, loader candidateLoader, prefilterDimension int) (*SearchSpecResult, error) {
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

	searchCtx, err := openSearchContext(ctx, cfg, embedder, prefilterDimension)
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
	mode, err := normalizeSearchMode(query.Mode)
	if err != nil {
		return SearchSpecQuery{}, err
	}
	query.Query = strings.TrimSpace(query.Query)
	query.Kind = defaultString(query.Kind, model.ArtifactKindSpec)
	query.Domain = strings.TrimSpace(query.Domain)
	query.Statuses = statuses
	query.Limit = limit
	query.Mode = mode
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

func openSearchContext(ctx context.Context, cfg *config.Config, embedder Embedder, prefilterDimension int) (*searchContext, error) {
	db, err := OpenReadOnlyContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return nil, fmt.Errorf("open index %s: %w", cfg.Workspace.ResolvedIndexPath, err)
	}

	snapshot, err := OpenStromaSnapshotContext(ctx, db, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	// #341 preflight runs against the already-open db, before any
	// embedder method is called. A typo in
	// runtime.search.matryoshka_prefilter_dimension therefore rejects
	// without burning an OpenAI-compatible round trip from
	// embedder.Dimension below.
	if err := preflightMatryoshkaPrefilter(ctx, db, prefilterDimension); err != nil {
		_ = snapshot.Close()
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

func loadRankedCandidatesContextWithFusion(ctx context.Context, db *sql.DB, snapshot *stindex.Snapshot, embedder Embedder, query SearchSpecQuery, strategy stindex.FusionStrategy, rerankerPolicy string, prefilterDimension int) ([]chunkCandidate, error) {
	preferHistorical := ranking.SearchPrefersHistoricalContext(query.Query)

	hits, err := snapshot.Search(ctx, stindex.SnapshotSearchQuery{
		SearchParams: stindex.SearchParams{
			Text:            query.Query,
			Limit:           searchCandidateLimit(query.Limit),
			Kinds:           []string{query.Kind},
			Embedder:        embedder,
			Fusion:          strategy,
			Reranker:        selectSearchReranker(rerankerPolicy, preferHistorical),
			SearchDimension: prefilterDimension,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query search candidates: %w", err)
	}

	scorePreAdjusted := rerankerPolicy == config.SearchRerankerArmAwareHistorical
	return buildRankedCandidatesContext(ctx, db, hits, query, preferHistorical, scorePreAdjusted)
}

func selectSearchReranker(policy string, preferHistorical bool) stindex.Reranker {
	switch policy {
	case config.SearchRerankerArmAwareHistorical:
		return armAwareHistoricalReranker{preferHistorical: preferHistorical}
	default:
		return historicalSectionReranker{preferHistorical: preferHistorical}
	}
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

	// NOTE(#341): runtime.search.matryoshka_prefilter_dimension is
	// honoured only on the hybrid path
	// (loadRankedCandidatesContextWithFusion). Stroma v3's
	// VectorSearchQuery does not expose SearchDimension, so the
	// vector-only similarity path always runs at full stored
	// dimension. If/when stroma surfaces SearchDimension on
	// VectorSearchQuery, this function will need an additional
	// prefilterDimension parameter (cfg is not in scope here today)
	// threaded through from SearchSpecsBySemanticSimilarityContext.
	hits, err := snapshot.SearchVector(ctx, stindex.VectorSearchQuery{
		Embedding: vectors[0],
		Limit:     searchCandidateLimit(query.Limit),
		Kinds:     []string{query.Kind},
	})
	if err != nil {
		return nil, fmt.Errorf("query semantic similarity candidates: %w", err)
	}

	return buildRankedCandidatesContext(ctx, db, hits, query, preferHistorical, false)
}

func buildRankedCandidatesContext(ctx context.Context, db *sql.DB, hits []stindex.SearchHit, query SearchSpecQuery, preferHistorical, scorePreAdjusted bool) ([]chunkCandidate, error) {
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
		if scorePreAdjusted {
			candidate.Score = hit.Score
		} else {
			candidate.Score = ranking.AdjustHistoricalSectionScore(hit.Score, candidate.SectionHeading, preferHistorical)
		}
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
