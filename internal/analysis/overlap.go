package analysis

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/chunk"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
	stindex "github.com/dusk-network/stroma/v2/index"
)

const (
	overlapThreshold           = 0.45
	overlapDraftMergeThreshold = 0.8
	overlapDuplicateThreshold  = 0.82
)

// OverlapRequest is the normalized input for overlap detection.
type OverlapRequest struct {
	SpecRef    string            `json:"spec_ref,omitempty"`
	SpecRecord *model.SpecRecord `json:"spec_record,omitempty"`
}

// OverlapCandidate identifies the spec being evaluated.
type OverlapCandidate struct {
	Ref       string                     `json:"ref"`
	Title     string                     `json:"title"`
	Status    string                     `json:"status,omitempty"`
	Domain    string                     `json:"domain,omitempty"`
	Inference *model.InferenceConfidence `json:"inference,omitempty"`
}

// OverlapItem describes one overlapping indexed spec.
type OverlapItem struct {
	Ref             string                     `json:"ref"`
	Title           string                     `json:"title"`
	Status          string                     `json:"status,omitempty"`
	Domain          string                     `json:"domain,omitempty"`
	Score           float64                    `json:"score"`
	OverlapDegree   string                     `json:"overlap_degree"`
	Relationship    string                     `json:"relationship"`
	Guidance        string                     `json:"guidance"`
	SharedAppliesTo []string                   `json:"shared_applies_to,omitempty"`
	Inference       *model.InferenceConfidence `json:"inference,omitempty"`
}

// OverlapResult is the structured overlap output.
type OverlapResult struct {
	Candidate      OverlapCandidate `json:"candidate"`
	Overlaps       []OverlapItem    `json:"overlaps"`
	Recommendation string           `json:"recommendation"`
}

type specDocument struct {
	Record   model.SpecRecord
	Sections []embeddedSection
}

type embeddedSection struct {
	Heading   string
	Content   string
	Embedding []float64
}

type indexedArtifactRow struct {
	Ref       string
	Title     string
	Status    string
	Domain    string
	SourceRef string
	Metadata  map[string]string
}

// CheckOverlap compares an indexed or draft spec against indexed specs.
func CheckOverlap(cfg *config.Config, request OverlapRequest) (*OverlapResult, error) {
	return CheckOverlapContext(context.Background(), cfg, request)
}

// CheckOverlapContext compares an indexed or draft spec against indexed specs.
func CheckOverlapContext(ctx context.Context, cfg *config.Config, request OverlapRequest) (*OverlapResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if err := validateOverlapRequest(request); err != nil {
		return nil, err
	}

	repo, err := openAnalysisRepositoryContext(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer repo.Close()

	candidate, err := loadCandidate(repo, request, nil)
	if err != nil {
		return nil, err
	}

	targetRefs, err := repo.overlapTargetRefs(*candidate)
	if err != nil {
		return nil, err
	}
	targets, err := repo.loadSelectedSpecs(targetRefs)
	if err != nil {
		return nil, err
	}

	return buildOverlapResult(candidate, targets), nil
}

func buildOverlapResult(candidate *specDocument, targets map[string]specDocument) *OverlapResult {
	var overlaps []OverlapItem
	for _, target := range targets {
		if target.Record.Status == model.StatusDeprecated {
			continue
		}
		if target.Record.Ref == candidate.Record.Ref {
			continue
		}

		score := overlapScore(*candidate, target)
		if score < overlapThreshold {
			continue
		}

		sharedAppliesTo := sharedStrings(candidate.Record.AppliesTo, target.Record.AppliesTo)
		relationship := overlapRelationship(candidate.Record, target.Record, score)
		overlaps = append(overlaps, OverlapItem{
			Ref:             target.Record.Ref,
			Title:           target.Record.Title,
			Status:          target.Record.Status,
			Domain:          target.Record.Domain,
			Score:           roundScore(score),
			OverlapDegree:   overlapDegree(score),
			Relationship:    relationship,
			Guidance:        overlapGuidance(candidate.Record, score, relationship),
			SharedAppliesTo: sharedAppliesTo,
			Inference:       target.Record.Inference,
		})
	}

	sort.Slice(overlaps, func(i, j int) bool {
		switch {
		case overlaps[i].Score != overlaps[j].Score:
			return overlaps[i].Score > overlaps[j].Score
		case overlaps[i].Guidance != overlaps[j].Guidance:
			return overlapGuidancePriority(overlaps[i].Guidance) < overlapGuidancePriority(overlaps[j].Guidance)
		default:
			return overlaps[i].Ref < overlaps[j].Ref
		}
	})

	return &OverlapResult{
		Candidate: OverlapCandidate{
			Ref:       candidate.Record.Ref,
			Title:     candidate.Record.Title,
			Status:    candidate.Record.Status,
			Domain:    candidate.Record.Domain,
			Inference: candidate.Record.Inference,
		},
		Overlaps:       overlaps,
		Recommendation: overlapRecommendation(candidate.Record, overlaps),
	}
}

func validateOverlapRequest(request OverlapRequest) error {
	hasRef := strings.TrimSpace(request.SpecRef) != ""
	hasRecord := request.SpecRecord != nil
	switch {
	case hasRef && hasRecord:
		return fmt.Errorf("exactly one of spec_ref or spec_record is allowed")
	case !hasRef && !hasRecord:
		return fmt.Errorf("one of spec_ref or spec_record is required")
	default:
		return nil
	}
}

func loadCandidate(repo *analysisRepository, request OverlapRequest, specs map[string]specDocument) (*specDocument, error) {
	ref := strings.TrimSpace(request.SpecRef)
	if ref != "" {
		var indexed map[string]specDocument
		if specs != nil {
			indexed = selectSpecs(specs, []string{ref})
		} else {
			loaded, err := repo.loadSpecs([]string{ref})
			if err != nil {
				return nil, err
			}
			indexed = loaded
		}
		spec, ok := indexed[ref]
		if !ok {
			availableRefs, err := repo.knownSpecRefs()
			if err != nil {
				return nil, err
			}
			return nil, newSpecRefNotFoundError(ref, availableRefs)
		}
		return &spec, nil
	}

	record := normalizeDraftSpec(*request.SpecRecord)
	if err := validateDraftSpec(record); err != nil {
		return nil, err
	}

	embedder, err := index.NewEmbedder(repo.cfg.Runtime.Embedder)
	if err != nil {
		return nil, err
	}

	sections := chunk.Markdown(record.Title, record.BodyText)
	texts := make([]string, 0, len(sections))
	for _, section := range sections {
		texts = append(texts, textForEmbedding(record.Title, section.Heading, section.Body))
	}
	vectors, err := embedder.EmbedDocuments(repo.ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embed draft spec %s: %w", record.Ref, err)
	}

	document := &specDocument{Record: record}
	for i, section := range sections {
		document.Sections = append(document.Sections, embeddedSection{
			Heading:   section.Heading,
			Content:   section.Body,
			Embedding: vectors[i],
		})
	}
	return document, nil
}

func normalizeDraftSpec(record model.SpecRecord) model.SpecRecord {
	record.Ref = strings.TrimSpace(record.Ref)
	record.Kind = defaultString(record.Kind, model.ArtifactKindSpec)
	record.Title = strings.TrimSpace(record.Title)
	record.Status = strings.TrimSpace(record.Status)
	record.Domain = strings.TrimSpace(record.Domain)
	record.SourceRef = strings.TrimSpace(record.SourceRef)
	record.BodyFormat = defaultString(strings.TrimSpace(record.BodyFormat), model.BodyFormatMarkdown)
	record.BodyText = strings.TrimSpace(record.BodyText)
	record.AppliesTo = uniqueStrings(record.AppliesTo)
	record.Relations = uniqueRelations(record.Relations)
	if record.Metadata == nil {
		record.Metadata = map[string]string{}
	}
	if record.Inference == nil {
		inference, err := model.DecodeInferenceConfidence(record.Metadata)
		if err == nil {
			record.Inference = inference
		}
	}
	return record
}

func validateDraftSpec(record model.SpecRecord) error {
	var missing []string
	if record.Ref == "" {
		missing = append(missing, "ref")
	}
	if record.Title == "" {
		missing = append(missing, "title")
	}
	if record.Status == "" {
		missing = append(missing, "status")
	}
	if record.Domain == "" {
		missing = append(missing, "domain")
	}
	if record.BodyText == "" {
		missing = append(missing, "body_text")
	}
	if len(missing) > 0 {
		return fmt.Errorf("spec_record missing required field(s): %s", strings.Join(missing, ", "))
	}
	if record.Kind != model.ArtifactKindSpec {
		return fmt.Errorf("spec_record.kind %q is invalid", record.Kind)
	}
	if record.BodyFormat != model.BodyFormatMarkdown {
		return fmt.Errorf("spec_record.body_format %q is invalid", record.BodyFormat)
	}
	if !isValidStatus(record.Status) {
		return fmt.Errorf("spec_record.status %q is invalid", record.Status)
	}
	return nil
}

func loadIndexedSpecsContext(ctx context.Context, db *sql.DB, snapshot *stindex.Snapshot, refs []string) (map[string]specDocument, error) {
	artifacts, err := loadIndexedArtifactRowsContext(ctx, db, refs)
	if err != nil {
		return nil, err
	}

	specs := make(map[string]specDocument, len(artifacts))
	for _, row := range artifacts {
		specs[row.Ref] = specDocument{
			Record: model.SpecRecord{
				Ref:        row.Ref,
				Kind:       model.ArtifactKindSpec,
				Title:      row.Title,
				Status:     row.Status,
				Domain:     row.Domain,
				SourceRef:  row.SourceRef,
				BodyFormat: model.BodyFormatMarkdown,
				Metadata:   row.Metadata,
			},
		}
		spec := specs[row.Ref]
		spec.Record.Inference, err = model.DecodeInferenceConfidence(row.Metadata)
		if err != nil {
			return nil, fmt.Errorf("decode spec inference for %s: %w", row.Ref, err)
		}
		specs[row.Ref] = spec
	}

	if err := loadSpecEdgesContext(ctx, db, specs); err != nil {
		return nil, err
	}
	if err := loadSpecSectionsContext(ctx, snapshot, specs); err != nil {
		return nil, err
	}

	return specs, nil
}

func loadIndexedArtifactRowsContext(ctx context.Context, db *sql.DB, refs []string) ([]indexedArtifactRow, error) {
	var builder strings.Builder
	args := make([]any, 0, len(refs))
	builder.WriteString(`
SELECT ref, title, COALESCE(status, ''), COALESCE(domain, ''), source_ref
     , metadata_json
FROM artifacts
WHERE kind = ?`)
	args = append(args, model.ArtifactKindSpec)
	if len(refs) > 0 {
		builder.WriteString(" AND ref IN (")
		for i, ref := range refs {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString("?")
			args = append(args, ref)
		}
		builder.WriteString(")")
	}

	rows, err := db.QueryContext(ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query indexed specs: %w", err)
	}
	defer rows.Close()

	var result []indexedArtifactRow
	for rows.Next() {
		var row indexedArtifactRow
		var rawMetadata string
		if err := rows.Scan(&row.Ref, &row.Title, &row.Status, &row.Domain, &row.SourceRef, &rawMetadata); err != nil {
			return nil, fmt.Errorf("scan indexed spec: %w", err)
		}
		if strings.TrimSpace(rawMetadata) != "" {
			if err := json.Unmarshal([]byte(rawMetadata), &row.Metadata); err != nil {
				return nil, fmt.Errorf("parse indexed metadata for %s: %w", row.Ref, err)
			}
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexed specs: %w", err)
	}
	return result, nil
}

func loadSpecEdgesContext(ctx context.Context, db *sql.DB, specs map[string]specDocument) error {
	refs := sortedSpecRefs(specs)
	if len(refs) == 0 {
		return nil
	}

	var builder strings.Builder
	args := make([]any, 0, len(refs))
	builder.WriteString(`
SELECT from_ref, to_ref, edge_type
FROM edges
WHERE edge_type IN ('depends_on', 'supersedes', 'relates_to', 'applies_to')`)
	appendRefFilterClause(&builder, &args, "from_ref", refs)
	builder.WriteString(`
ORDER BY from_ref ASC, edge_type ASC, to_ref ASC`)

	rows, err := db.QueryContext(ctx, builder.String(), args...)
	if err != nil {
		return fmt.Errorf("query spec edges: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			fromRef  string
			toRef    string
			edgeType string
		)
		if err := rows.Scan(&fromRef, &toRef, &edgeType); err != nil {
			return fmt.Errorf("scan spec edge: %w", err)
		}
		document, ok := specs[fromRef]
		if !ok {
			continue
		}
		switch edgeType {
		case "applies_to":
			document.Record.AppliesTo = append(document.Record.AppliesTo, toRef)
		case string(model.RelationDependsOn), string(model.RelationSupersedes), string(model.RelationRelatesTo):
			document.Record.Relations = append(document.Record.Relations, model.Relation{
				Type: model.RelationType(edgeType),
				Ref:  toRef,
			})
		}
		document.Record.AppliesTo = uniqueStrings(document.Record.AppliesTo)
		document.Record.Relations = uniqueRelations(document.Record.Relations)
		specs[fromRef] = document
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate spec edges: %w", err)
	}
	return nil
}

func loadSpecSectionsContext(ctx context.Context, snapshot *stindex.Snapshot, specs map[string]specDocument) error {
	refs := sortedSpecRefs(specs)
	if len(refs) == 0 {
		return nil
	}

	sections, err := snapshot.Sections(ctx, stindex.SectionQuery{
		Refs:              refs,
		Kinds:             []string{model.ArtifactKindSpec},
		IncludeEmbeddings: true,
	})
	if err != nil {
		return fmt.Errorf("query spec sections: %w", err)
	}

	for _, section := range sections {
		document, ok := specs[section.Ref]
		if !ok {
			continue
		}
		document.Sections = append(document.Sections, embeddedSection{
			Heading:   section.Heading,
			Content:   section.Content,
			Embedding: append([]float64(nil), section.Embedding...),
		})
		specs[section.Ref] = document
	}
	return nil
}

func overlapScore(candidate, target specDocument) float64 {
	sectionScore := bestSectionOverlap(candidate.Sections, target.Sections)
	sharedAppliesTo := overlapRatio(candidate.Record.AppliesTo, target.Record.AppliesTo)
	score := 0.7*sectionScore + 0.2*sharedAppliesTo
	if candidate.Record.Domain != "" && candidate.Record.Domain == target.Record.Domain {
		score += 0.03
	}
	if relationExists(candidate.Record.Relations, model.RelationSupersedes, target.Record.Ref) ||
		relationExists(target.Record.Relations, model.RelationSupersedes, candidate.Record.Ref) {
		score += 0.08
	}
	switch target.Record.Status {
	case model.StatusAccepted:
		score += 0.03
	case model.StatusSuperseded:
		score -= 0.03
	}
	if score > 1 {
		score = 1
	}
	if score < 0 {
		score = 0
	}
	return score
}

func sortedSpecRefs(specs map[string]specDocument) []string {
	refs := make([]string, 0, len(specs))
	for ref := range specs {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

func appendRefFilterClause(builder *strings.Builder, args *[]any, column string, refs []string) {
	refs = uniqueStrings(refs)
	if len(refs) == 0 {
		return
	}
	builder.WriteString(" AND ")
	builder.WriteString(column)
	builder.WriteString(" IN (")
	for i, ref := range refs {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString("?")
		*args = append(*args, ref)
	}
	builder.WriteString(")")
}

func bestSectionOverlap(left, right []embeddedSection) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}

	var total float64
	for _, leftSection := range left {
		best := 0.0
		for _, rightSection := range right {
			score := cosineSimilarity(leftSection.Embedding, rightSection.Embedding)
			if score > best {
				best = score
			}
		}
		total += best
	}
	return total / float64(len(left))
}

func cosineSimilarity(left, right []float64) float64 {
	if len(left) == 0 || len(right) == 0 || len(left) != len(right) {
		return 0
	}

	var (
		dot       float64
		leftNorm  float64
		rightNorm float64
	)
	for i := range left {
		dot += left[i] * right[i]
		leftNorm += left[i] * left[i]
		rightNorm += right[i] * right[i]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func overlapRatio(left, right []string) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	shared := sharedStrings(left, right)
	union := uniqueStrings(append(append([]string{}, left...), right...))
	if len(union) == 0 {
		return 0
	}
	return float64(len(shared)) / float64(len(union))
}

func overlapDegree(score float64) string {
	switch {
	case score >= 0.8:
		return "high"
	case score >= 0.62:
		return "medium"
	default:
		return "low"
	}
}

func overlapRelationship(candidate, target model.SpecRecord, score float64) string {
	switch {
	case relationExists(candidate.Relations, model.RelationSupersedes, target.Ref),
		relationExists(target.Relations, model.RelationSupersedes, candidate.Ref):
		return "extends"
	case relationExists(candidate.Relations, model.RelationDependsOn, target.Ref),
		relationExists(target.Relations, model.RelationDependsOn, candidate.Ref):
		return "adjacent"
	case sharedSet(candidate.AppliesTo, target.AppliesTo) && score >= overlapDuplicateThreshold:
		return "duplicates"
	default:
		return "adjacent"
	}
}

func overlapGuidance(candidate model.SpecRecord, score float64, relationship string) string {
	switch {
	case relationship == "duplicates" && score >= overlapDuplicateThreshold:
		return "merge_candidate"
	case overlapCandidateStillMutable(candidate) && relationship == "extends" && score >= overlapDraftMergeThreshold:
		return "merge_candidate"
	default:
		return "boundary_review"
	}
}

func overlapCandidateStillMutable(candidate model.SpecRecord) bool {
	switch candidate.Status {
	case model.StatusDraft, model.StatusReview:
		return true
	default:
		return false
	}
}

func overlapGuidancePriority(guidance string) int {
	switch guidance {
	case "merge_candidate":
		return 0
	case "boundary_review":
		return 1
	default:
		return 2
	}
}

func overlapRecommendation(candidate model.SpecRecord, overlaps []OverlapItem) string {
	if len(overlaps) == 0 {
		return "no_overlap"
	}
	for _, item := range overlaps {
		if relationExists(candidate.Relations, model.RelationSupersedes, item.Ref) {
			return "proceed_with_supersedes"
		}
	}
	if overlaps[0].Guidance == "merge_candidate" {
		return "merge_into_existing"
	}
	return "review_boundaries"
}

func roundScore(score float64) float64 {
	return math.Round(score*1000) / 1000
}

func relationExists(relations []model.Relation, relationType model.RelationType, ref string) bool {
	for _, relation := range relations {
		if relation.Type == relationType && relation.Ref == ref {
			return true
		}
	}
	return false
}

func sharedStrings(left, right []string) []string {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	rightSet := make(map[string]struct{}, len(right))
	for _, item := range right {
		rightSet[item] = struct{}{}
	}
	var shared []string
	seen := map[string]struct{}{}
	for _, item := range left {
		if _, ok := rightSet[item]; !ok {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		shared = append(shared, item)
	}
	sort.Strings(shared)
	return shared
}

func uniqueStrings(values []string) []string {
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
	sort.Strings(result)
	return result
}

func uniqueRelations(relations []model.Relation) []model.Relation {
	if len(relations) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(relations))
	result := make([]model.Relation, 0, len(relations))
	for _, relation := range relations {
		key := string(relation.Type) + "\x00" + relation.Ref
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, model.Relation{Type: relation.Type, Ref: strings.TrimSpace(relation.Ref)})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Type != result[j].Type {
			return result[i].Type < result[j].Type
		}
		return result[i].Ref < result[j].Ref
	})
	return result
}

func sharedSet(left, right []string) bool {
	return len(sharedStrings(left, right)) > 0
}

func isValidStatus(status string) bool {
	switch status {
	case model.StatusDraft, model.StatusReview, model.StatusAccepted, model.StatusSuperseded, model.StatusDeprecated:
		return true
	default:
		return false
	}
}

func textForEmbedding(title, heading, body string) string {
	parts := make([]string, 0, 3)
	if trimmed := strings.TrimSpace(title); trimmed != "" {
		parts = append(parts, trimmed)
	}
	if trimmed := strings.TrimSpace(heading); trimmed != "" && trimmed != strings.TrimSpace(title) {
		parts = append(parts, trimmed)
	}
	if trimmed := strings.TrimSpace(body); trimmed != "" {
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, "\n\n")
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
