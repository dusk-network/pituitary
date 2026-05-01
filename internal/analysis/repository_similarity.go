package analysis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/ranking"
	stindex "github.com/dusk-network/stroma/v2/index"
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
	hits, err := r.snapshot.SearchVector(r.ctx, stindex.VectorSearchQuery{
		Embedding: embedding,
		Limit:     shortlistChunkProbeLimit(normalizeArtifactShortlistLimit(query.Limit)),
		Kinds:     []string{query.Kind},
	})
	if err != nil {
		return nil, fmt.Errorf("query artifact shortlist: %w", err)
	}

	states, err := r.loadArtifactShortlistState(refsFromSearchHits(hits))
	if err != nil {
		return nil, err
	}

	scores := make(map[string]float64)
	excluded := make(map[string]struct{}, len(query.ExcludeRefs))
	for _, ref := range uniqueStrings(query.ExcludeRefs) {
		excluded[ref] = struct{}{}
	}

	for _, hit := range hits {
		if _, ok := excluded[hit.Ref]; ok {
			continue
		}
		state := states[hit.Ref]
		if len(query.Statuses) > 0 && !containsString(query.Statuses, state) {
			continue
		}
		score := ranking.AdjustHistoricalSectionScore(hit.Score, hit.Heading, false)
		if score <= 0 {
			continue
		}
		if score > scores[hit.Ref] {
			scores[hit.Ref] = score
		}
	}
	return scores, nil
}

func (r *analysisRepository) loadArtifactShortlistState(refs []string) (map[string]string, error) {
	states := make(map[string]string, len(refs))
	if len(refs) == 0 {
		return states, nil
	}

	var builder strings.Builder
	args := make([]any, 0, len(refs))
	builder.WriteString(`
SELECT ref, COALESCE(status, '')
FROM artifacts
WHERE ref IN (`)
	for i, ref := range refs {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString("?")
		args = append(args, ref)
	}
	builder.WriteString(`)`)

	rows, err := r.db.QueryContext(r.ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query artifact shortlist state: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ref, status string
		if err := rows.Scan(&ref, &status); err != nil {
			return nil, fmt.Errorf("scan artifact shortlist state: %w", err)
		}
		states[ref] = status
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifact shortlist state: %w", err)
	}
	return states, nil
}

func refsFromSearchHits(hits []stindex.SearchHit) []string {
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
// validity when the normalized YYYY-MM-DD atDate is non-empty.
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
