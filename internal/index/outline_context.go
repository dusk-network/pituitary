package index

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/fusion"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/ranking"
	"github.com/dusk-network/pituitary/internal/resultmeta"
	stchunk "github.com/dusk-network/stroma/v3/chunk"
	stindex "github.com/dusk-network/stroma/v3/index"
)

const (
	defaultOutlineContextLimit        = 5
	maxOutlineContextLimit            = 10
	defaultOutlineSelectionsPerRecord = 1
	maxOutlineSelectionsPerRecord     = 3
	defaultOutlineRowsPerRecord       = 80
	maxOutlineRowsPerRecord           = 200
	defaultOutlineContextSectionBytes = 2400
	maxOutlineContextSectionBytes     = 12000
	maxOutlineNeighborWindow          = 3
)

// OutlineContextSelector optionally replaces deterministic chunk selection.
// Implementations must return chunk ids from the provided outline. The caller
// caps accepted selections and falls back to deterministic selection on errors
// or unusable ids.
type OutlineContextSelector func(context.Context, OutlineContextSelectionInput) ([]int64, error)

// OutlineContextQuery controls outline-guided retrieval over the indexed corpus.
type OutlineContextQuery struct {
	Query                   string                 `json:"query"`
	Refs                    []string               `json:"refs,omitempty"`
	Kinds                   []string               `json:"kinds,omitempty"`
	Limit                   int                    `json:"limit,omitempty"`
	MaxSelectionsPerRecord  int                    `json:"max_selections_per_record,omitempty"`
	IncludeParent           bool                   `json:"include_parent,omitempty"`
	NeighborWindow          int                    `json:"neighbor_window,omitempty"`
	MaxOutlineRowsPerRecord int                    `json:"max_outline_rows_per_record,omitempty"`
	MaxSectionBytes         int                    `json:"max_section_bytes,omitempty"`
	Selector                OutlineContextSelector `json:"-"`
}

// OutlineContextSelectionInput is the bounded payload passed to an optional
// selector.
type OutlineContextSelectionInput struct {
	Query         string                       `json:"query"`
	Record        OutlineContextRecordSummary  `json:"record"`
	Outline       []OutlineContextOutlineRow   `json:"outline"`
	Contributions []OutlineContextContribution `json:"contributions"`
	Limit         int                          `json:"limit"`
}

// OutlineContextResult is the reusable PageIndex-inspired context payload.
type OutlineContextResult struct {
	Query               string                   `json:"query"`
	SnapshotFingerprint string                   `json:"snapshot_fingerprint,omitempty"`
	Records             []OutlineContextRecord   `json:"records"`
	Warnings            []string                 `json:"warnings,omitempty"`
	ContentTrust        *resultmeta.ContentTrust `json:"content_trust,omitempty"`
}

// OutlineContextRecord groups outline and expanded sections for one record.
type OutlineContextRecord struct {
	OutlineContextRecordSummary
	Score            float64                      `json:"score"`
	Outline          []OutlineContextOutlineRow   `json:"outline"`
	OutlineTruncated bool                         `json:"outline_truncated,omitempty"`
	Contributions    []OutlineContextContribution `json:"contributions,omitempty"`
	Selections       []OutlineContextSelection    `json:"selections"`
}

// OutlineContextRecordSummary identifies one retrieved record.
type OutlineContextRecordSummary struct {
	Ref       string `json:"ref"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	SourceRef string `json:"source_ref"`
}

// OutlineContextOutlineRow is one structural row from a record outline.
type OutlineContextOutlineRow struct {
	ChunkID       int64                     `json:"chunk_id"`
	Heading       string                    `json:"heading"`
	ParentChunkID *int64                    `json:"parent_chunk_id,omitempty"`
	Depth         int                       `json:"depth"`
	ContextPrefix string                    `json:"context_prefix,omitempty"`
	SourceSpan    *OutlineContextSourceSpan `json:"source_span,omitempty"`
}

// OutlineContextContribution records why a record entered the candidate set.
type OutlineContextContribution struct {
	ChunkID    int64                     `json:"chunk_id"`
	Heading    string                    `json:"heading"`
	Score      float64                   `json:"score"`
	SourceSpan *OutlineContextSourceSpan `json:"source_span,omitempty"`
}

// OutlineContextSelection is one selected chunk and its expanded context.
type OutlineContextSelection struct {
	ChunkID         int64                     `json:"chunk_id"`
	Heading         string                    `json:"heading"`
	Score           float64                   `json:"score,omitempty"`
	SelectionSource string                    `json:"selection_source"`
	SourceSpan      *OutlineContextSourceSpan `json:"source_span,omitempty"`
	Expanded        []OutlineContextSection   `json:"expanded"`
}

// OutlineContextSection is one bounded section returned by ExpandContext.
type OutlineContextSection struct {
	ChunkID          int64                     `json:"chunk_id"`
	Ref              string                    `json:"ref"`
	Kind             string                    `json:"kind"`
	Title            string                    `json:"title"`
	SourceRef        string                    `json:"source_ref"`
	Heading          string                    `json:"heading"`
	Role             string                    `json:"role"`
	Depth            *int                      `json:"depth,omitempty"`
	Content          string                    `json:"content"`
	ContentTruncated bool                      `json:"content_truncated,omitempty"`
	ContextPrefix    string                    `json:"context_prefix,omitempty"`
	SourceSpan       *OutlineContextSourceSpan `json:"source_span,omitempty"`
}

// OutlineContextSourceSpan is a JSON-stable source extent.
type OutlineContextSourceSpan struct {
	Unit     string            `json:"unit"`
	Start    int64             `json:"start"`
	End      int64             `json:"end"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type outlineContextSelectionPlan struct {
	chunkIDs []int64
	source   string
	warning  string
}

// RetrieveOutlineContext executes outline-guided context retrieval.
func RetrieveOutlineContext(cfg *config.Config, query OutlineContextQuery) (*OutlineContextResult, error) {
	return RetrieveOutlineContextContext(context.Background(), cfg, query)
}

// RetrieveOutlineContextContext executes outline-guided context retrieval.
func RetrieveOutlineContextContext(ctx context.Context, cfg *config.Config, query OutlineContextQuery) (*OutlineContextResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	query, err := normalizeOutlineContextQuery(query)
	if err != nil {
		return nil, err
	}

	// #341 preflight: reject an oversized matryoshka prefilter using
	// the index's stored embedder_dimension metadata, before any
	// embedder is constructed or invoked. See
	// preflightMatryoshkaPrefilterContext for rationale.
	if err := preflightMatryoshkaPrefilterContext(ctx, cfg); err != nil {
		return nil, err
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

	return RetrieveOutlineContextWithSnapshotContext(ctx, cfg, searchCtx.snapshot, searchCtx.embedder, query)
}

// RetrieveOutlineContextWithSnapshotContext executes outline-guided context
// retrieval against a caller-owned snapshot. The caller remains responsible for
// closing the snapshot.
func RetrieveOutlineContextWithSnapshotContext(ctx context.Context, cfg *config.Config, snapshot *stindex.Snapshot, embedder Embedder, query OutlineContextQuery) (*OutlineContextResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if snapshot == nil {
		return nil, fmt.Errorf("stroma snapshot is required")
	}
	if embedder == nil {
		return nil, fmt.Errorf("embedder is required")
	}
	query, err := normalizeOutlineContextQuery(query)
	if err != nil {
		return nil, err
	}
	// #341 preflight: also guard the with-snapshot entry point so
	// direct callers (e.g. analysis/review) get the same OpenAI-round-
	// trip protection as RetrieveOutlineContextContext above.
	if err := preflightMatryoshkaPrefilterContext(ctx, cfg); err != nil {
		return nil, err
	}
	strategy, err := fusionStrategyFromConfig(cfg.Runtime.Search)
	if err != nil {
		return nil, err
	}
	hits, err := snapshot.SearchRecords(ctx, stindex.SnapshotRecordSearchQuery{
		SearchParams: stindex.SearchParams{
			Text:            query.Query,
			Limit:           searchCandidateLimit(query.Limit),
			Kinds:           query.Kinds,
			Refs:            query.Refs,
			Embedder:        embedder,
			Fusion:          strategy,
			Reranker:        selectSearchReranker(cfg.Runtime.Search.Reranker, ranking.SearchPrefersHistoricalContext(query.Query)),
			SearchDimension: cfg.Runtime.Search.PrefilterDimension,
		},
		Aggregation: stindex.RecordAggregationOptions{Limit: query.Limit},
	})
	if err != nil {
		return nil, fmt.Errorf("search outline-context candidates: %w", err)
	}

	stats, err := snapshot.Stats(ctx)
	if err != nil {
		return nil, fmt.Errorf("read stroma snapshot stats: %w", err)
	}
	groupedOutline, err := loadOutlineContextRows(ctx, snapshot, refsForRecordHits(hits), query.Kinds)
	if err != nil {
		return nil, err
	}

	result := &OutlineContextResult{
		Query:               query.Query,
		SnapshotFingerprint: stats.ContentFingerprint,
		Records:             make([]OutlineContextRecord, 0, len(hits)),
		ContentTrust:        resultmeta.UntrustedWorkspaceText(),
	}
	for _, hit := range hits {
		record, warnings, err := buildOutlineContextRecord(ctx, snapshot, query, hit, groupedOutline[hit.Ref])
		if err != nil {
			return nil, err
		}
		result.Warnings = append(result.Warnings, warnings...)
		result.Records = append(result.Records, record)
	}
	return result, nil
}

func fusionStrategyFromConfig(cfg config.SearchConfig) (stindex.FusionStrategy, error) {
	strategy, err := fusion.Resolve(fusionConfigFromRuntime(cfg))
	if err != nil {
		return nil, fmt.Errorf("resolve search fusion: %w", err)
	}
	return strategy, nil
}

func normalizeOutlineContextQuery(query OutlineContextQuery) (OutlineContextQuery, error) {
	query.Query = strings.TrimSpace(query.Query)
	if query.Query == "" {
		return OutlineContextQuery{}, fmt.Errorf("query is required")
	}
	limit, err := normalizeOutlineBoundedLimit(query.Limit, defaultOutlineContextLimit, maxOutlineContextLimit, "limit")
	if err != nil {
		return OutlineContextQuery{}, err
	}
	selections, err := normalizeOutlineBoundedLimit(query.MaxSelectionsPerRecord, defaultOutlineSelectionsPerRecord, maxOutlineSelectionsPerRecord, "max_selections_per_record")
	if err != nil {
		return OutlineContextQuery{}, err
	}
	outlineRows, err := normalizeOutlineBoundedLimit(query.MaxOutlineRowsPerRecord, defaultOutlineRowsPerRecord, maxOutlineRowsPerRecord, "max_outline_rows_per_record")
	if err != nil {
		return OutlineContextQuery{}, err
	}
	sectionBytes, err := normalizeOutlineBoundedLimit(query.MaxSectionBytes, defaultOutlineContextSectionBytes, maxOutlineContextSectionBytes, "max_section_bytes")
	if err != nil {
		return OutlineContextQuery{}, err
	}
	if query.NeighborWindow < 0 {
		return OutlineContextQuery{}, fmt.Errorf("neighbor_window must be greater than or equal to zero")
	}
	if query.NeighborWindow > maxOutlineNeighborWindow {
		return OutlineContextQuery{}, fmt.Errorf("neighbor_window must be less than or equal to %d", maxOutlineNeighborWindow)
	}
	query.Limit = limit
	query.MaxSelectionsPerRecord = selections
	query.MaxOutlineRowsPerRecord = outlineRows
	query.MaxSectionBytes = sectionBytes
	query.Kinds = normalizeOutlineKinds(query.Kinds)
	query.Refs = uniqueTrimmedStrings(query.Refs)
	return query, nil
}

func normalizeOutlineBoundedLimit(value, fallback, max int, name string) (int, error) {
	if value == 0 {
		return fallback, nil
	}
	if value < 0 {
		return 0, fmt.Errorf("%s must be positive; use 0 to select the default", name)
	}
	if value > max {
		return 0, fmt.Errorf("%s must be less than or equal to %d", name, max)
	}
	return value, nil
}

func normalizeOutlineKinds(kinds []string) []string {
	kinds = uniqueTrimmedStrings(kinds)
	if len(kinds) == 0 {
		return []string{model.ArtifactKindSpec, model.ArtifactKindDoc}
	}
	return kinds
}

func uniqueTrimmedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func refsForRecordHits(hits []stindex.RecordSearchHit) []string {
	if len(hits) == 0 {
		return nil
	}
	refs := make([]string, 0, len(hits))
	for _, hit := range hits {
		refs = append(refs, hit.Ref)
	}
	return refs
}

func loadOutlineContextRows(ctx context.Context, snapshot *stindex.Snapshot, refs, kinds []string) (map[string][]OutlineContextOutlineRow, error) {
	grouped := make(map[string][]OutlineContextOutlineRow, len(refs))
	if len(refs) == 0 {
		return grouped, nil
	}
	rows, err := snapshot.Outline(ctx, stindex.OutlineQuery{
		Refs:  refs,
		Kinds: kinds,
	})
	if err != nil {
		return nil, fmt.Errorf("read candidate outlines: %w", err)
	}
	for _, row := range rows {
		grouped[row.Ref] = append(grouped[row.Ref], outlineRowFromStroma(row))
	}
	return grouped, nil
}

func buildOutlineContextRecord(ctx context.Context, snapshot *stindex.Snapshot, query OutlineContextQuery, hit stindex.RecordSearchHit, outline []OutlineContextOutlineRow) (OutlineContextRecord, []string, error) {
	record := OutlineContextRecord{
		OutlineContextRecordSummary: OutlineContextRecordSummary{
			Ref:       hit.Ref,
			Kind:      hit.Kind,
			Title:     hit.Title,
			SourceRef: hit.SourceRef,
		},
		Score:         hit.Score,
		Outline:       trimOutlineRows(outline, query.MaxOutlineRowsPerRecord),
		Contributions: contributionsFromRecordHit(hit),
		Selections:    []OutlineContextSelection{},
	}
	record.OutlineTruncated = len(record.Outline) < len(outline)

	selectionPlan := planOutlineContextSelection(ctx, query, record, outline)
	var warnings []string
	if selectionPlan.warning != "" {
		warnings = append(warnings, selectionPlan.warning)
	}

	outlineByChunkID := outlineRowsByChunkID(outline)
	contributionsByChunkID := contributionsByChunkID(record.Contributions)
	for _, chunkID := range selectionPlan.chunkIDs {
		expanded, err := snapshot.ExpandContext(ctx, chunkID, stindex.ContextOptions{
			IncludeParent:  query.IncludeParent,
			NeighborWindow: query.NeighborWindow,
		})
		if err != nil {
			return OutlineContextRecord{}, nil, fmt.Errorf("expand context for chunk %d: %w", chunkID, err)
		}
		record.Selections = append(record.Selections, buildOutlineContextSelection(
			chunkID,
			selectionPlan.source,
			outlineByChunkID,
			contributionsByChunkID,
			expanded,
			query.MaxSectionBytes,
		))
	}
	return record, warnings, nil
}

func trimOutlineRows(rows []OutlineContextOutlineRow, limit int) []OutlineContextOutlineRow {
	if len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

func planOutlineContextSelection(ctx context.Context, query OutlineContextQuery, record OutlineContextRecord, outline []OutlineContextOutlineRow) outlineContextSelectionPlan {
	if query.Selector != nil {
		input := OutlineContextSelectionInput{
			Query:         query.Query,
			Record:        record.OutlineContextRecordSummary,
			Outline:       trimOutlineRows(outline, query.MaxOutlineRowsPerRecord),
			Contributions: record.Contributions,
			Limit:         query.MaxSelectionsPerRecord,
		}
		chunkIDs, err := query.Selector(ctx, input)
		selected := normalizeSelectedChunkIDs(chunkIDs, outline, query.MaxSelectionsPerRecord)
		if err == nil && len(selected) > 0 {
			return outlineContextSelectionPlan{chunkIDs: selected, source: "selector"}
		}
		warning := fmt.Sprintf("selector returned no usable chunks for %s; deterministic selection used", record.Ref)
		if err != nil {
			warning = fmt.Sprintf("selector failed for %s: %v; deterministic selection used", record.Ref, err)
		} else if len(chunkIDs) > 0 {
			warning = fmt.Sprintf("selector returned chunks outside the outline for %s; deterministic selection used", record.Ref)
		}
		return outlineContextSelectionPlan{
			chunkIDs: deterministicOutlineChunkIDs(record.Contributions, outline, query.MaxSelectionsPerRecord),
			source:   "deterministic_fallback",
			warning:  warning,
		}
	}
	return outlineContextSelectionPlan{
		chunkIDs: deterministicOutlineChunkIDs(record.Contributions, outline, query.MaxSelectionsPerRecord),
		source:   "deterministic",
	}
}

func deterministicOutlineChunkIDs(contributions []OutlineContextContribution, outline []OutlineContextOutlineRow, limit int) []int64 {
	allowed := outlineChunkIDSet(outline)
	selected := make([]int64, 0, limit)
	seen := make(map[int64]struct{}, limit)
	for _, contribution := range sortedContributions(contributions) {
		if _, ok := allowed[contribution.ChunkID]; !ok {
			continue
		}
		if appendSelectedChunkID(&selected, seen, contribution.ChunkID, limit) {
			return selected
		}
	}
	for _, row := range outline {
		if appendSelectedChunkID(&selected, seen, row.ChunkID, limit) {
			return selected
		}
	}
	return selected
}

func normalizeSelectedChunkIDs(chunkIDs []int64, outline []OutlineContextOutlineRow, limit int) []int64 {
	allowed := outlineChunkIDSet(outline)
	selected := make([]int64, 0, limit)
	seen := make(map[int64]struct{}, limit)
	for _, chunkID := range chunkIDs {
		if _, ok := allowed[chunkID]; !ok {
			continue
		}
		if appendSelectedChunkID(&selected, seen, chunkID, limit) {
			return selected
		}
	}
	return selected
}

func appendSelectedChunkID(selected *[]int64, seen map[int64]struct{}, chunkID int64, limit int) bool {
	if chunkID == 0 {
		return false
	}
	if _, ok := seen[chunkID]; ok {
		return false
	}
	seen[chunkID] = struct{}{}
	*selected = append(*selected, chunkID)
	return len(*selected) >= limit
}

func sortedContributions(contributions []OutlineContextContribution) []OutlineContextContribution {
	sorted := append([]OutlineContextContribution(nil), contributions...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Score != sorted[j].Score {
			return sorted[i].Score > sorted[j].Score
		}
		return sorted[i].ChunkID < sorted[j].ChunkID
	})
	return sorted
}

func outlineChunkIDSet(outline []OutlineContextOutlineRow) map[int64]struct{} {
	set := make(map[int64]struct{}, len(outline))
	for _, row := range outline {
		set[row.ChunkID] = struct{}{}
	}
	return set
}

func outlineRowsByChunkID(outline []OutlineContextOutlineRow) map[int64]OutlineContextOutlineRow {
	byID := make(map[int64]OutlineContextOutlineRow, len(outline))
	for _, row := range outline {
		byID[row.ChunkID] = row
	}
	return byID
}

func contributionsByChunkID(contributions []OutlineContextContribution) map[int64]OutlineContextContribution {
	byID := make(map[int64]OutlineContextContribution, len(contributions))
	for _, contribution := range contributions {
		if _, ok := byID[contribution.ChunkID]; ok {
			continue
		}
		byID[contribution.ChunkID] = contribution
	}
	return byID
}

func buildOutlineContextSelection(chunkID int64, source string, outline map[int64]OutlineContextOutlineRow, contributions map[int64]OutlineContextContribution, expanded []stindex.Section, maxBytes int) OutlineContextSelection {
	row := outline[chunkID]
	contribution := contributions[chunkID]
	heading := defaultString(row.Heading, contribution.Heading)
	selection := OutlineContextSelection{
		ChunkID:         chunkID,
		Heading:         heading,
		Score:           contribution.Score,
		SelectionSource: source,
		SourceSpan:      firstSourceSpan(row.SourceSpan, contribution.SourceSpan),
		Expanded:        make([]OutlineContextSection, 0, len(expanded)),
	}
	for _, section := range expanded {
		selection.Expanded = append(selection.Expanded, sectionFromStroma(section, outline, row, chunkID, maxBytes))
	}
	return selection
}

func firstSourceSpan(spans ...*OutlineContextSourceSpan) *OutlineContextSourceSpan {
	for _, span := range spans {
		if span != nil {
			return span
		}
	}
	return nil
}

func sectionFromStroma(section stindex.Section, outline map[int64]OutlineContextOutlineRow, selectedRow OutlineContextOutlineRow, selectedID int64, maxBytes int) OutlineContextSection {
	content, truncated := truncateOutlineContent(section.Content, maxBytes)
	var depth *int
	if row, ok := outline[section.ChunkID]; ok {
		depth = &row.Depth
	}
	return OutlineContextSection{
		ChunkID:          section.ChunkID,
		Ref:              section.Ref,
		Kind:             section.Kind,
		Title:            section.Title,
		SourceRef:        section.SourceRef,
		Heading:          section.Heading,
		Role:             outlineContextSectionRole(section.ChunkID, selectedRow, selectedID),
		Depth:            depth,
		Content:          content,
		ContentTruncated: truncated,
		ContextPrefix:    section.ContextPrefix,
		SourceSpan:       sourceSpanFromStroma(section.SourceSpan),
	}
}

func outlineContextSectionRole(chunkID int64, selectedRow OutlineContextOutlineRow, selectedID int64) string {
	if chunkID == selectedID {
		return "selected"
	}
	if selectedRow.ParentChunkID != nil && chunkID == *selectedRow.ParentChunkID {
		return "parent"
	}
	return "neighbor"
}

func truncateOutlineContent(content string, maxBytes int) (string, bool) {
	content = strings.TrimSpace(content)
	if len(content) <= maxBytes {
		return content, false
	}
	cut := utf8SafeCut(content, maxBytes)
	return strings.TrimSpace(content[:cut]) + "...", true
}

func utf8SafeCut(content string, maxBytes int) int {
	if maxBytes <= 0 {
		return 0
	}
	for index, r := range content {
		next := index + utf8.RuneLen(r)
		if next > maxBytes {
			if index == 0 {
				return 0
			}
			return index
		}
	}
	return len(content)
}

func outlineRowFromStroma(row stindex.OutlineRow) OutlineContextOutlineRow {
	return OutlineContextOutlineRow{
		ChunkID:       row.ChunkID,
		Heading:       row.Heading,
		ParentChunkID: row.ParentChunkID,
		Depth:         row.Depth,
		ContextPrefix: row.ContextPrefix,
		SourceSpan:    sourceSpanFromStroma(row.SourceSpan),
	}
}

func contributionsFromRecordHit(hit stindex.RecordSearchHit) []OutlineContextContribution {
	if len(hit.Contributions) == 0 {
		return nil
	}
	contributions := make([]OutlineContextContribution, 0, len(hit.Contributions))
	for _, contribution := range hit.Contributions {
		contributions = append(contributions, OutlineContextContribution{
			ChunkID:    contribution.ChunkID,
			Heading:    contribution.Heading,
			Score:      contribution.Score,
			SourceSpan: sourceSpanFromStroma(contribution.SourceSpan),
		})
	}
	return contributions
}

func sourceSpanFromStroma(span *stchunk.SourceSpan) *OutlineContextSourceSpan {
	if span == nil {
		return nil
	}
	return &OutlineContextSourceSpan{
		Unit:     span.Unit,
		Start:    span.Start,
		End:      span.End,
		Metadata: cloneStringMap(span.Metadata),
	}
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
