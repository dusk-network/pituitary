package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/resultmeta"
	stcorpus "github.com/dusk-network/stroma/v3/corpus"
	stindex "github.com/dusk-network/stroma/v3/index"
)

const (
	defaultIntentContextRows = defaultOutlineRowsPerRecord
	maxIntentContextRows     = maxOutlineRowsPerRecord
)

// IntentOutlineRequest identifies one indexed governance artifact to inspect.
type IntentOutlineRequest struct {
	Ref            string            `json:"ref" jsonschema_description:"Indexed record ref returned by search_specs or another Pituitary tool"`
	Kind           string            `json:"kind,omitempty" jsonschema_description:"Optional artifact kind: spec or doc"`
	Filters        SearchSpecFilters `json:"filters,omitempty" jsonschema_description:"Optional spec domain/status filters; default statuses are draft, review, and accepted"`
	MaxOutlineRows int               `json:"max_outline_rows,omitempty" jsonschema_description:"Maximum outline rows to return; zero selects the bounded default"`
}

// IntentOutlineResult is a bounded structural view of one indexed record.
type IntentOutlineResult struct {
	Record              OutlineContextRecordSummary `json:"record"`
	Status              string                      `json:"status,omitempty"`
	Domain              string                      `json:"domain,omitempty"`
	SnapshotFingerprint string                      `json:"snapshot_fingerprint,omitempty"`
	Outline             []OutlineContextOutlineRow  `json:"outline"`
	OutlineTruncated    bool                        `json:"outline_truncated,omitempty"`
	ContentTrust        *resultmeta.ContentTrust    `json:"content_trust,omitempty"`
}

// ExpandIntentContextRequest expands one known chunk handle into bounded local
// context.
type ExpandIntentContextRequest struct {
	ChunkID             int64             `json:"chunk_id" jsonschema_description:"Chunk ID from get_intent_outline or review_spec outline_context"`
	SnapshotFingerprint string            `json:"snapshot_fingerprint" jsonschema_description:"Snapshot fingerprint from the result that supplied chunk_id; required to reject stale handles"`
	Filters             SearchSpecFilters `json:"filters,omitempty" jsonschema_description:"Optional spec domain/status filters; default statuses are draft, review, and accepted"`
	IncludeParent       bool              `json:"include_parent,omitempty" jsonschema_description:"Include the parent section when lineage is available"`
	NeighborWindow      int               `json:"neighbor_window,omitempty" jsonschema_description:"Sibling sections on each side of the selected chunk"`
	MaxSectionBytes     int               `json:"max_section_bytes,omitempty" jsonschema_description:"Maximum bytes per returned section; zero selects the bounded default"`
}

// ExpandIntentContextResult is a bounded content payload for one known chunk.
type ExpandIntentContextResult struct {
	ChunkID             int64                    `json:"chunk_id"`
	SnapshotFingerprint string                   `json:"snapshot_fingerprint,omitempty"`
	Sections            []OutlineContextSection  `json:"sections"`
	ContentTrust        *resultmeta.ContentTrust `json:"content_trust,omitempty"`
}

// MissingRecordError reports that a record ref does not exist in the current
// snapshot.
type MissingRecordError struct {
	Ref  string
	Kind string
}

func (e *MissingRecordError) Error() string {
	if e == nil {
		return "record not found"
	}
	if strings.TrimSpace(e.Kind) != "" {
		return fmt.Sprintf("record %q of kind %q was not found in the current snapshot", e.Ref, e.Kind)
	}
	return fmt.Sprintf("record %q was not found in the current snapshot", e.Ref)
}

// RecordFilteredError reports that a found record is excluded by active filters.
type RecordFilteredError struct {
	Ref          string
	Status       string
	Domain       string
	Statuses     []string
	FilterDomain string
}

func (e *RecordFilteredError) Error() string {
	if e == nil {
		return "record excluded by filters"
	}
	detail := fmt.Sprintf("record %q is excluded by the requested filters", e.Ref)
	if len(e.Statuses) > 0 {
		detail = fmt.Sprintf("%s; status %q is not in %v", detail, e.Status, e.Statuses)
	}
	if strings.TrimSpace(e.FilterDomain) != "" {
		detail = fmt.Sprintf("%s; domain %q does not match %q", detail, e.Domain, e.FilterDomain)
	}
	return detail
}

// MissingChunkError reports that a chunk handle does not exist in the current
// snapshot. In normal agent workflows this usually means the handle came from a
// stale snapshot.
type MissingChunkError struct {
	ChunkID             int64
	SnapshotFingerprint string
}

func (e *MissingChunkError) Error() string {
	if e == nil {
		return "chunk handle not found"
	}
	if strings.TrimSpace(e.SnapshotFingerprint) != "" {
		return fmt.Sprintf("chunk handle %d was not found in snapshot %q; the handle may be stale", e.ChunkID, e.SnapshotFingerprint)
	}
	return fmt.Sprintf("chunk handle %d was not found in the current snapshot; refresh chunk IDs with get_intent_outline", e.ChunkID)
}

// StaleSnapshotError reports that a caller supplied a snapshot fingerprint that
// is no longer current.
type StaleSnapshotError struct {
	Expected string
	Current  string
}

func (e *StaleSnapshotError) Error() string {
	if e == nil {
		return "stale snapshot fingerprint"
	}
	return fmt.Sprintf("stale snapshot fingerprint %q; current snapshot fingerprint is %q", e.Expected, e.Current)
}

// IntentExpansionInvariantError reports an unexpected expansion shape from the
// underlying snapshot.
type IntentExpansionInvariantError struct {
	ChunkID int64
	Message string
}

func (e *IntentExpansionInvariantError) Error() string {
	if e == nil {
		return "intent context expansion invariant failed"
	}
	return fmt.Sprintf("intent context expansion invariant failed for chunk %d: %s", e.ChunkID, e.Message)
}

// IsMissingRecord reports whether err wraps MissingRecordError.
func IsMissingRecord(err error) bool {
	var target *MissingRecordError
	return errors.As(err, &target)
}

// IsMissingChunk reports whether err wraps MissingChunkError.
func IsMissingChunk(err error) bool {
	var target *MissingChunkError
	return errors.As(err, &target)
}

// IsStaleSnapshot reports whether err wraps StaleSnapshotError.
func IsStaleSnapshot(err error) bool {
	var target *StaleSnapshotError
	return errors.As(err, &target)
}

// IsRecordFiltered reports whether err wraps RecordFilteredError.
func IsRecordFiltered(err error) bool {
	var target *RecordFilteredError
	return errors.As(err, &target)
}

// IsIntentExpansionInvariant reports whether err wraps IntentExpansionInvariantError.
func IsIntentExpansionInvariant(err error) bool {
	var target *IntentExpansionInvariantError
	return errors.As(err, &target)
}

// IsIntentContextNotFound reports user-visible missing-handle failures.
func IsIntentContextNotFound(err error) bool {
	return IsMissingRecord(err) || IsMissingChunk(err)
}

// GetIntentOutline returns a bounded outline for one indexed record.
func GetIntentOutline(cfg *config.Config, request IntentOutlineRequest) (*IntentOutlineResult, error) {
	return GetIntentOutlineContext(context.Background(), cfg, request)
}

// GetIntentOutlineContext returns a bounded outline for one indexed record.
func GetIntentOutlineContext(ctx context.Context, cfg *config.Config, request IntentOutlineRequest) (*IntentOutlineResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	request, err := normalizeIntentOutlineRequest(request)
	if err != nil {
		return nil, err
	}
	return withIntentSnapshot(ctx, cfg, func(db *sql.DB, snapshot *stindex.Snapshot, stats *stindex.Stats) (*IntentOutlineResult, error) {
		record, err := loadIntentRecord(ctx, snapshot, request.Ref, request.Kind)
		if err != nil {
			return nil, err
		}
		state, err := loadIntentRecordStateForSpec(ctx, db, record, request.Filters)
		if err != nil {
			return nil, err
		}
		outline, err := loadIntentOutlineRows(ctx, snapshot, record.Ref, record.Kind, request.MaxOutlineRows+1)
		if err != nil {
			return nil, err
		}
		return buildIntentOutlineResult(record, state, stats.ContentFingerprint, outline, request.MaxOutlineRows), nil
	})
}

// ExpandIntentContext expands one known chunk into bounded local context.
func ExpandIntentContext(cfg *config.Config, request ExpandIntentContextRequest) (*ExpandIntentContextResult, error) {
	return ExpandIntentContextContext(context.Background(), cfg, request)
}

// ExpandIntentContextContext expands one known chunk into bounded local context.
func ExpandIntentContextContext(ctx context.Context, cfg *config.Config, request ExpandIntentContextRequest) (*ExpandIntentContextResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	request, err := normalizeExpandIntentContextRequest(request)
	if err != nil {
		return nil, err
	}
	return withIntentSnapshot(ctx, cfg, func(db *sql.DB, snapshot *stindex.Snapshot, stats *stindex.Stats) (*ExpandIntentContextResult, error) {
		if err := validateIntentSnapshot(stats.ContentFingerprint, request.SnapshotFingerprint); err != nil {
			return nil, err
		}
		expanded, err := snapshot.ExpandContext(ctx, request.ChunkID, stindex.ContextOptions{
			IncludeParent:  request.IncludeParent,
			NeighborWindow: request.NeighborWindow,
		})
		if err != nil {
			return nil, fmt.Errorf("expand intent context for chunk %d: %w", request.ChunkID, err)
		}
		if len(expanded) == 0 {
			// Stroma's ExpandContext contract is empty result + nil error for
			// absent chunk IDs; found chunks always include the selected chunk.
			return nil, &MissingChunkError{
				ChunkID:             request.ChunkID,
				SnapshotFingerprint: stats.ContentFingerprint,
			}
		}
		selected, err := validateExpandedIntentShape(expanded, request.ChunkID)
		if err != nil {
			return nil, err
		}
		if err := validateExpandedIntentFilters(ctx, db, *selected, request.Filters); err != nil {
			return nil, err
		}
		outline, err := loadExpandedIntentOutline(ctx, snapshot, expanded)
		if err != nil {
			return nil, err
		}
		return buildExpandIntentContextResult(request, stats.ContentFingerprint, expanded, outline), nil
	})
}

func normalizeIntentOutlineRequest(request IntentOutlineRequest) (IntentOutlineRequest, error) {
	request.Ref = strings.TrimSpace(request.Ref)
	if request.Ref == "" {
		return IntentOutlineRequest{}, fmt.Errorf("ref is required")
	}
	kind, err := normalizeIntentKind(request.Kind)
	if err != nil {
		return IntentOutlineRequest{}, err
	}
	rows, err := normalizeOutlineBoundedLimit(request.MaxOutlineRows, defaultIntentContextRows, maxIntentContextRows, "max_outline_rows")
	if err != nil {
		return IntentOutlineRequest{}, err
	}
	filters, err := normalizeIntentFilters(request.Filters)
	if err != nil {
		return IntentOutlineRequest{}, err
	}
	request.Kind = kind
	request.Filters = filters
	request.MaxOutlineRows = rows
	return request, nil
}

func normalizeExpandIntentContextRequest(request ExpandIntentContextRequest) (ExpandIntentContextRequest, error) {
	if request.ChunkID <= 0 {
		return ExpandIntentContextRequest{}, fmt.Errorf("chunk_id must be positive")
	}
	request.SnapshotFingerprint = strings.TrimSpace(request.SnapshotFingerprint)
	if request.SnapshotFingerprint == "" {
		return ExpandIntentContextRequest{}, fmt.Errorf("snapshot_fingerprint is required; call get_intent_outline before expand_intent_context")
	}
	if request.NeighborWindow < 0 {
		return ExpandIntentContextRequest{}, fmt.Errorf("neighbor_window must be greater than or equal to zero")
	}
	if request.NeighborWindow > maxOutlineNeighborWindow {
		return ExpandIntentContextRequest{}, fmt.Errorf("neighbor_window must be less than or equal to %d", maxOutlineNeighborWindow)
	}
	sectionBytes, err := normalizeOutlineBoundedLimit(request.MaxSectionBytes, defaultOutlineContextSectionBytes, maxOutlineContextSectionBytes, "max_section_bytes")
	if err != nil {
		return ExpandIntentContextRequest{}, err
	}
	filters, err := normalizeIntentFilters(request.Filters)
	if err != nil {
		return ExpandIntentContextRequest{}, err
	}
	request.MaxSectionBytes = sectionBytes
	request.Filters = filters
	return request, nil
}

func normalizeIntentKind(kind string) (string, error) {
	kind = strings.TrimSpace(kind)
	switch kind {
	case "", model.ArtifactKindSpec, model.ArtifactKindDoc:
		return kind, nil
	default:
		return "", fmt.Errorf("unsupported kind %q", kind)
	}
}

func normalizeIntentFilters(filters SearchSpecFilters) (SearchSpecFilters, error) {
	statuses, err := NormalizeSearchStatuses(filters.Statuses)
	if err != nil {
		return SearchSpecFilters{}, err
	}
	return SearchSpecFilters{
		Domain:   strings.TrimSpace(filters.Domain),
		Statuses: statuses,
	}, nil
}

func withIntentSnapshot[T any](ctx context.Context, cfg *config.Config, fn func(*sql.DB, *stindex.Snapshot, *stindex.Stats) (*T, error)) (*T, error) {
	db, err := OpenReadOnlyContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return nil, fmt.Errorf("open index %s: %w", cfg.Workspace.ResolvedIndexPath, err)
	}
	defer db.Close()

	snapshot, err := OpenStromaSnapshotContext(ctx, db, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return nil, err
	}
	defer snapshot.Close()

	stats, err := snapshot.Stats(ctx)
	if err != nil {
		return nil, fmt.Errorf("read stroma snapshot stats: %w", err)
	}
	return fn(db, snapshot, stats)
}

func validateIntentSnapshot(current, expected string) error {
	expected = strings.TrimSpace(expected)
	if expected == "" || expected == current {
		return nil
	}
	return &StaleSnapshotError{
		Expected: expected,
		Current:  current,
	}
}

func loadIntentRecord(ctx context.Context, snapshot *stindex.Snapshot, ref, kind string) (stcorpus.Record, error) {
	query := stindex.RecordQuery{
		Refs:         []string{ref},
		OmitMetadata: true,
	}
	if kind != "" {
		query.Kinds = []string{kind}
	}
	records, err := snapshot.Records(ctx, query)
	if err != nil {
		return stcorpus.Record{}, fmt.Errorf("read intent record %s: %w", ref, err)
	}
	if len(records) == 0 {
		return stcorpus.Record{}, &MissingRecordError{Ref: ref, Kind: kind}
	}
	return records[0], nil
}

func loadIntentRecordState(ctx context.Context, db *sql.DB, ref string) (searchArtifactState, error) {
	states, err := loadSearchArtifactStateContext(ctx, db, []string{ref})
	if err != nil {
		return searchArtifactState{}, err
	}
	return states[ref], nil
}

func loadIntentRecordStateForSpec(ctx context.Context, db *sql.DB, record stcorpus.Record, filters SearchSpecFilters) (searchArtifactState, error) {
	if record.Kind != model.ArtifactKindSpec {
		return searchArtifactState{}, nil
	}
	state, err := loadIntentRecordState(ctx, db, record.Ref)
	if err != nil {
		return searchArtifactState{}, err
	}
	return state, validateIntentRecordFilters(record, state, filters)
}

func validateIntentRecordFilters(record stcorpus.Record, state searchArtifactState, filters SearchSpecFilters) error {
	if record.Kind != model.ArtifactKindSpec {
		return nil
	}
	if !intentStatusAllowed(state.Status, filters.Statuses) || !intentDomainAllowed(state.Domain, filters.Domain) {
		return &RecordFilteredError{
			Ref:          record.Ref,
			Status:       state.Status,
			Domain:       state.Domain,
			Statuses:     filters.Statuses,
			FilterDomain: filters.Domain,
		}
	}
	return nil
}

func intentStatusAllowed(status string, allowed []string) bool {
	for _, candidate := range allowed {
		if status == candidate {
			return true
		}
	}
	return false
}

func intentDomainAllowed(domain, filter string) bool {
	filter = strings.TrimSpace(filter)
	return filter == "" || domain == filter
}

func loadIntentOutlineRows(ctx context.Context, snapshot *stindex.Snapshot, ref, kind string, limit int) ([]OutlineContextOutlineRow, error) {
	query := stindex.OutlineQuery{Refs: []string{ref}}
	if kind != "" {
		query.Kinds = []string{kind}
	}
	outline := make([]OutlineContextOutlineRow, 0, limit)
	err := snapshot.WalkOutline(ctx, query, func(row stindex.OutlineRow) error {
		outline = append(outline, outlineRowFromStroma(row))
		if len(outline) >= limit {
			return stindex.ErrStopWalk
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read intent outline for %s: %w", ref, err)
	}
	return outline, nil
}

func buildIntentOutlineResult(record stcorpus.Record, state searchArtifactState, fingerprint string, outline []OutlineContextOutlineRow, maxRows int) *IntentOutlineResult {
	trimmed := trimOutlineRows(outline, maxRows)
	return &IntentOutlineResult{
		Record: OutlineContextRecordSummary{
			Ref:       record.Ref,
			Kind:      record.Kind,
			Title:     record.Title,
			SourceRef: record.SourceRef,
		},
		Status:              state.Status,
		Domain:              state.Domain,
		SnapshotFingerprint: fingerprint,
		Outline:             trimmed,
		OutlineTruncated:    len(trimmed) < len(outline),
		ContentTrust:        resultmeta.UntrustedWorkspaceText(),
	}
}

func validateExpandedIntentShape(expanded []stindex.Section, selectedID int64) (*stindex.Section, error) {
	selected := selectedIntentSection(expanded, selectedID)
	if selected == nil {
		return nil, &IntentExpansionInvariantError{
			ChunkID: selectedID,
			Message: "expanded context did not include the selected chunk",
		}
	}
	for _, section := range expanded {
		if section.Ref != selected.Ref || section.Kind != selected.Kind {
			return nil, &IntentExpansionInvariantError{
				ChunkID: selectedID,
				Message: fmt.Sprintf("expanded section %d belongs to %s %q, not selected %s %q", section.ChunkID, section.Kind, section.Ref, selected.Kind, selected.Ref),
			}
		}
	}
	return selected, nil
}

func validateExpandedIntentFilters(ctx context.Context, db *sql.DB, selected stindex.Section, filters SearchSpecFilters) error {
	if selected.Kind != model.ArtifactKindSpec {
		return nil
	}
	state, err := loadIntentRecordState(ctx, db, selected.Ref)
	if err != nil {
		return err
	}
	return validateIntentRecordFilters(stcorpus.Record{
		Ref:  selected.Ref,
		Kind: selected.Kind,
	}, state, filters)
}

func selectedIntentSection(expanded []stindex.Section, selectedID int64) *stindex.Section {
	for i := range expanded {
		if expanded[i].ChunkID == selectedID {
			return &expanded[i]
		}
	}
	return nil
}

func loadExpandedIntentOutline(ctx context.Context, snapshot *stindex.Snapshot, expanded []stindex.Section) (map[int64]OutlineContextOutlineRow, error) {
	refs := refsForIntentSections(expanded)
	kinds := kindsForIntentSections(expanded)
	rows, err := loadOutlineContextRows(ctx, snapshot, refs, kinds)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]OutlineContextOutlineRow, len(rows))
	for _, rowSet := range rows {
		for _, row := range rowSet {
			byID[row.ChunkID] = row
		}
	}
	return byID, nil
}

func refsForIntentSections(sections []stindex.Section) []string {
	refs := make([]string, 0, len(sections))
	seen := make(map[string]struct{}, len(sections))
	for _, section := range sections {
		if section.Ref == "" {
			continue
		}
		if _, ok := seen[section.Ref]; ok {
			continue
		}
		seen[section.Ref] = struct{}{}
		refs = append(refs, section.Ref)
	}
	return refs
}

func kindsForIntentSections(sections []stindex.Section) []string {
	kinds := make([]string, 0, len(sections))
	seen := make(map[string]struct{}, len(sections))
	for _, section := range sections {
		if section.Kind == "" {
			continue
		}
		if _, ok := seen[section.Kind]; ok {
			continue
		}
		seen[section.Kind] = struct{}{}
		kinds = append(kinds, section.Kind)
	}
	return kinds
}

func buildExpandIntentContextResult(request ExpandIntentContextRequest, fingerprint string, expanded []stindex.Section, outline map[int64]OutlineContextOutlineRow) *ExpandIntentContextResult {
	selectedRow := outline[request.ChunkID]
	sections := make([]OutlineContextSection, 0, len(expanded))
	for _, section := range expanded {
		sections = append(sections, sectionFromStroma(section, outline, selectedRow, request.ChunkID, request.MaxSectionBytes))
	}
	return &ExpandIntentContextResult{
		ChunkID:             request.ChunkID,
		SnapshotFingerprint: fingerprint,
		Sections:            sections,
		ContentTrust:        resultmeta.UntrustedWorkspaceText(),
	}
}
