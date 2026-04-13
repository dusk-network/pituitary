package analysis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/ranking"
)

const (
	overlapSpecShortlistLimit  = 48
	impactDocShortlistLimit    = 24
	docDriftSpecShortlistLimit = 24
)

type artifactShortlistQuery struct {
	Kind        string
	Statuses    []string
	ExcludeRefs []string
	Limit       int
}

type scoredArtifactRef struct {
	Ref   string
	Score float64
	Hits  int
}

func (r *analysisRepository) overlapTargetRefs(candidate specDocument) ([]string, error) {
	refs, err := r.shortlistSimilarArtifactRefs(candidate.Sections, artifactShortlistQuery{
		Kind: model.ArtifactKindSpec,
		Statuses: []string{
			model.StatusDraft,
			model.StatusReview,
			model.StatusAccepted,
			model.StatusSuperseded,
		},
		ExcludeRefs: []string{candidate.Record.Ref},
		Limit:       overlapSpecShortlistLimit,
	})
	if err != nil {
		return nil, err
	}

	sharedRefs, err := r.specRefsSharingAppliesTo(candidate.Record.AppliesTo, []string{candidate.Record.Ref})
	if err != nil {
		return nil, err
	}
	refs = append(refs, sharedRefs...)
	refs = append(refs, relationRefs(candidate.Record.Relations, model.RelationSupersedes)...)

	if candidate.Record.Ref != "" {
		incomingRefs, err := r.specRefsWithIncomingEdge(model.RelationSupersedes, candidate.Record.Ref, []string{candidate.Record.Ref})
		if err != nil {
			return nil, err
		}
		refs = append(refs, incomingRefs...)
	}

	return uniqueStrings(refs), nil
}

func (r *analysisRepository) impactedSpecRefs(candidate model.SpecRecord) ([]string, error) {
	refs := relationRefs(candidate.Relations, model.RelationSupersedes)
	if candidate.Ref == "" {
		return refs, nil
	}

	dependentRefs, err := r.specRefsWithIncomingEdge(model.RelationDependsOn, candidate.Ref, nil)
	if err != nil {
		return nil, err
	}
	refs = append(refs, dependentRefs...)

	// Traverse relates_to edges in both directions: outgoing from the
	// candidate and incoming from other specs.
	refs = append(refs, relationRefs(candidate.Relations, model.RelationRelatesTo)...)
	incomingRelatesTo, err := r.specRefsWithIncomingEdge(model.RelationRelatesTo, candidate.Ref, nil)
	if err != nil {
		return nil, err
	}
	refs = append(refs, incomingRelatesTo...)

	return uniqueStrings(refs), nil
}

func (r *analysisRepository) impactedDocRefs(candidate specDocument) ([]string, error) {
	return r.shortlistSimilarArtifactRefs(candidate.Sections, artifactShortlistQuery{
		Kind:  model.ArtifactKindDoc,
		Limit: impactDocShortlistLimit,
	})
}

func (r *analysisRepository) relevantDocDriftSpecRefs(docs map[string]docDocument) ([]string, error) {
	var refs []string
	for _, ref := range sortedDocRefs(docs) {
		similarRefs, err := r.shortlistSimilarArtifactRefs(docs[ref].Sections, artifactShortlistQuery{
			Kind:     model.ArtifactKindSpec,
			Statuses: []string{model.StatusAccepted},
			Limit:    docDriftSpecShortlistLimit,
		})
		if err != nil {
			return nil, err
		}
		refs = append(refs, similarRefs...)
	}
	return uniqueStrings(refs), nil
}

func (r *analysisRepository) shortlistSimilarArtifactRefs(sections []embeddedSection, query artifactShortlistQuery) ([]string, error) {
	if len(sections) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(query.Kind) == "" {
		return nil, fmt.Errorf("artifact shortlist kind is required")
	}
	limit := normalizeArtifactShortlistLimit(query.Limit)

	aggregate := make(map[string]scoredArtifactRef)
	for _, section := range sections {
		if len(section.Embedding) == 0 {
			continue
		}
		sectionScores, err := r.shortlistScoresForEmbedding(section.Embedding, artifactShortlistQuery{
			Kind:        query.Kind,
			Statuses:    uniqueStrings(query.Statuses),
			ExcludeRefs: uniqueStrings(query.ExcludeRefs),
			Limit:       limit,
		})
		if err != nil {
			return nil, err
		}
		for ref, score := range sectionScores {
			scored := aggregate[ref]
			scored.Ref = ref
			scored.Score += score
			scored.Hits++
			aggregate[ref] = scored
		}
	}

	scoredRefs := make([]scoredArtifactRef, 0, len(aggregate))
	for _, scored := range aggregate {
		scoredRefs = append(scoredRefs, scored)
	}
	sort.Slice(scoredRefs, func(i, j int) bool {
		switch {
		case scoredRefs[i].Score != scoredRefs[j].Score:
			return scoredRefs[i].Score > scoredRefs[j].Score
		case scoredRefs[i].Hits != scoredRefs[j].Hits:
			return scoredRefs[i].Hits > scoredRefs[j].Hits
		default:
			return scoredRefs[i].Ref < scoredRefs[j].Ref
		}
	})

	if len(scoredRefs) > limit {
		scoredRefs = scoredRefs[:limit]
	}

	refs := make([]string, 0, len(scoredRefs))
	for _, scored := range scoredRefs {
		refs = append(refs, scored.Ref)
	}
	return refs, nil
}

func (r *analysisRepository) shortlistScoresForEmbedding(embedding []float64, query artifactShortlistQuery) (map[string]float64, error) {
	queryBlob, err := index.EncodeVectorBlob(embedding)
	if err != nil {
		return nil, fmt.Errorf("encode shortlist embedding: %w", err)
	}

	sqlText, args := buildArtifactShortlistQuery(query, queryBlob)
	rows, err := r.db.QueryContext(r.ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("query artifact shortlist: %w", err)
	}
	defer rows.Close()

	scores := make(map[string]float64)
	for rows.Next() {
		var (
			ref      string
			heading  string
			distance float64
		)
		if err := rows.Scan(&ref, &heading, &distance); err != nil {
			return nil, fmt.Errorf("scan shortlisted artifact: %w", err)
		}
		score := ranking.AdjustHistoricalSectionScore(similarityScoreFromDistance(distance), heading, false)
		if score <= 0 {
			continue
		}
		if score > scores[ref] {
			scores[ref] = score
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate shortlisted artifacts: %w", err)
	}
	return scores, nil
}

func buildArtifactShortlistQuery(query artifactShortlistQuery, queryBlob []byte) (string, []any) {
	limit := normalizeArtifactShortlistLimit(query.Limit)

	var builder strings.Builder
	args := make([]any, 0, 4+len(query.Statuses)+len(query.ExcludeRefs))

	builder.WriteString(`
WITH vector_hits AS (
  SELECT chunk_id, distance
  FROM chunks_vec
  WHERE embedding MATCH ? AND k = ?
  ORDER BY distance
)
SELECT
  a.ref,
  c.heading,
  vh.distance
FROM vector_hits vh
JOIN chunks c ON c.id = vh.chunk_id
JOIN artifacts a ON a.ref = c.record_ref
WHERE a.kind = ?`)
	args = append(args, queryBlob, shortlistChunkProbeLimit(limit), query.Kind)

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

	if len(query.ExcludeRefs) > 0 {
		builder.WriteString(" AND a.ref NOT IN (")
		for i, ref := range query.ExcludeRefs {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString("?")
			args = append(args, ref)
		}
		builder.WriteString(")")
	}

	builder.WriteString(`
ORDER BY vh.distance ASC, a.ref ASC, c.heading ASC, vh.chunk_id ASC
LIMIT ?`)
	args = append(args, shortlistChunkProbeLimit(limit))

	return builder.String(), args
}

func (r *analysisRepository) specRefsSharingAppliesTo(appliesTo []string, excludeRefs []string) ([]string, error) {
	appliesTo = uniqueStrings(appliesTo)
	if len(appliesTo) == 0 {
		return nil, nil
	}

	var builder strings.Builder
	args := make([]any, 0, 2+len(appliesTo)+len(excludeRefs)+2)
	builder.WriteString(`
SELECT DISTINCT a.ref
FROM edges e
JOIN artifacts a ON a.ref = e.from_ref
WHERE a.kind = ?
  AND e.edge_type = ?
  AND e.to_ref IN (`)
	args = append(args, model.ArtifactKindSpec, "applies_to")
	for i, ref := range appliesTo {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString("?")
		args = append(args, ref)
	}
	builder.WriteString(")")
	appendTemporalEdgeClause(&builder, &args, r.atDate)
	appendMinConfidenceEdgeClause(&builder, r.minConfidence)
	appendExcludedRefsClause(&builder, &args, excludeRefs)
	builder.WriteString(" ORDER BY a.ref ASC")

	rows, err := r.db.QueryContext(r.ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query applies_to-related specs: %w", err)
	}
	defer rows.Close()

	return scanRefRows(rows)
}

func (r *analysisRepository) specRefsWithIncomingEdge(edgeType model.RelationType, toRef string, excludeRefs []string) ([]string, error) {
	if strings.TrimSpace(toRef) == "" {
		return nil, nil
	}

	var builder strings.Builder
	args := make([]any, 0, 3+len(excludeRefs)+2)
	builder.WriteString(`
SELECT DISTINCT a.ref
FROM edges e
JOIN artifacts a ON a.ref = e.from_ref
WHERE a.kind = ?
  AND e.edge_type = ?
  AND e.to_ref = ?`)
	args = append(args, model.ArtifactKindSpec, edgeType, toRef)
	appendTemporalEdgeClause(&builder, &args, r.atDate)
	appendMinConfidenceEdgeClause(&builder, r.minConfidence)
	appendExcludedRefsClause(&builder, &args, excludeRefs)
	builder.WriteString(" ORDER BY a.ref ASC")

	rows, err := r.db.QueryContext(r.ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query incoming %s specs: %w", edgeType, err)
	}
	defer rows.Close()

	return scanRefRows(rows)
}

// appendMinConfidenceEdgeClause adds a WHERE filter on edge confidence tier
// when minConfidence is non-empty.
func appendMinConfidenceEdgeClause(builder *strings.Builder, minConfidence string) {
	switch strings.TrimSpace(strings.ToLower(minConfidence)) {
	case "extracted":
		builder.WriteString(` AND e.confidence = 'extracted'`)
	case "inferred":
		builder.WriteString(` AND e.confidence IN ('extracted', 'inferred')`)
	}
}

// appendTemporalEdgeClause adds WHERE conditions to filter edges by temporal
// validity when atDate is non-empty.
func appendTemporalEdgeClause(builder *strings.Builder, args *[]any, atDate string) {
	if atDate == "" {
		return
	}
	builder.WriteString(` AND (e.valid_from IS NULL OR e.valid_from <= ?)`)
	*args = append(*args, atDate)
	builder.WriteString(` AND (e.valid_to IS NULL OR e.valid_to >= ?)`)
	*args = append(*args, atDate)
}

func appendExcludedRefsClause(builder *strings.Builder, args *[]any, excludeRefs []string) {
	excludeRefs = uniqueStrings(excludeRefs)
	if len(excludeRefs) == 0 {
		return
	}
	builder.WriteString(" AND a.ref NOT IN (")
	for i, ref := range excludeRefs {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString("?")
		*args = append(*args, ref)
	}
	builder.WriteString(")")
}

func scanRefRows(rows rowScanner) ([]string, error) {
	var refs []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, fmt.Errorf("scan ref row: %w", err)
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ref rows: %w", err)
	}
	return refs, nil
}

type rowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func relationRefs(relations []model.Relation, relationType model.RelationType) []string {
	refs := make([]string, 0, len(relations))
	for _, relation := range relations {
		if relation.Type == relationType {
			refs = append(refs, relation.Ref)
		}
	}
	return uniqueStrings(refs)
}

func similarityScoreFromDistance(distance float64) float64 {
	score := 1 - distance
	switch {
	case score < 0:
		return 0
	case score > 1:
		return 1
	default:
		return score
	}
}

func normalizeArtifactShortlistLimit(limit int) int {
	if limit <= 0 {
		return impactDocShortlistLimit
	}
	return limit
}

func shortlistRefProbeLimit(limit int) int {
	probeLimit := limit * 4
	if probeLimit < 24 {
		probeLimit = 24
	}
	if probeLimit > 128 {
		probeLimit = 128
	}
	return probeLimit
}

func shortlistChunkProbeLimit(limit int) int {
	probeLimit := shortlistRefProbeLimit(limit) * 4
	if probeLimit > 256 {
		probeLimit = 256
	}
	return probeLimit
}
