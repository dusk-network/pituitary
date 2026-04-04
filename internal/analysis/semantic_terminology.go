package analysis

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
)

const (
	semanticTerminologyShortlistLimit      = 32
	semanticTerminologySimilarityThreshold = 0.40
	semanticTerminologyMaxTerms            = 16
)

// semanticTerminologyMatch reports one embedding-similarity match between a
// governed term concept and an indexed chunk that does not literally contain
// the governed term.
type semanticTerminologyMatch struct {
	Term        string
	Preferred   string
	ArtifactRef string
	Kind        string
	Title       string
	SourceRef   string
	Section     string
	Excerpt     string
	Similarity  float64
}

// semanticTerminologyNearMisses searches the index for chunks that are
// semantically similar to each governed term but do not contain the literal
// term string. This catches paraphrases, adjectival usage, and conceptual
// drift that deterministic matching cannot detect.
//
// It requires a real embedding provider (not fixture) and returns nil when
// embeddings are unavailable.
func semanticTerminologyNearMisses(ctx context.Context, cfg *config.Config, repo *analysisRepository, governed map[string]terminologyGovernedTerm, request TerminologyAuditRequest) ([]semanticTerminologyMatch, error) {
	if !isSemanticEmbedderConfigured(cfg) {
		return nil, nil
	}

	embedder, err := index.NewEmbedder(cfg.Runtime.Embedder)
	if err != nil {
		return nil, fmt.Errorf("resolve embedder for semantic terminology: %w", err)
	}

	// Build embedding queries from governed terms: "term: <preferred> (replaces: <displaced>)"
	// Sort keys for deterministic selection when truncating at the max limit.
	type queryEntry struct {
		term      string
		preferred string
	}
	sortedKeys := make([]string, 0, len(governed))
	for key := range governed {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)

	entries := make([]queryEntry, 0, len(governed))
	texts := make([]string, 0, len(governed))
	for _, key := range sortedKeys {
		if len(entries) >= semanticTerminologyMaxTerms {
			break
		}
		rule := governed[key]
		queryText := fmt.Sprintf("terminology: %s (replaces: %s)", rule.PreferredTerm, rule.Term)
		entries = append(entries, queryEntry{term: rule.Term, preferred: rule.PreferredTerm})
		texts = append(texts, queryText)
	}
	if len(texts) == 0 {
		return nil, nil
	}

	embeddings, err := embedder.EmbedQueries(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embed terminology queries: %w", err)
	}
	if len(embeddings) != len(entries) {
		return nil, fmt.Errorf("embedder returned %d vectors for %d terminology queries", len(embeddings), len(entries))
	}

	// Determine which artifact kinds to search based on scope.
	kinds := terminologyArtifactKinds(request.Scope)

	// Compile literal matchers to filter out chunks already caught by exact matching.
	literalMatchers := compileTerminologyMatchers(request.Terms)

	var allMatches []semanticTerminologyMatch
	for i, entry := range entries {
		if len(embeddings[i]) == 0 {
			continue
		}
		matches, err := searchTermChunks(ctx, repo, embeddings[i], entry.term, entry.preferred, kinds, literalMatchers)
		if err != nil {
			return nil, err
		}
		allMatches = append(allMatches, matches...)
	}

	// Deduplicate by (artifact_ref, section, term).
	allMatches = deduplicateSemanticMatches(allMatches)
	sort.Slice(allMatches, func(i, j int) bool {
		if allMatches[i].Similarity != allMatches[j].Similarity {
			return allMatches[i].Similarity > allMatches[j].Similarity
		}
		return allMatches[i].ArtifactRef < allMatches[j].ArtifactRef
	})
	return allMatches, nil
}

// searchTermChunks queries the index for chunks similar to a single term
// embedding, filtering out chunks that contain the literal term.
func searchTermChunks(ctx context.Context, repo *analysisRepository, embedding []float64, term, preferred string, kinds []string, literalMatchers []terminologyMatcher) ([]semanticTerminologyMatch, error) {
	queryBlob, err := index.EncodeVectorBlob(embedding)
	if err != nil {
		return nil, fmt.Errorf("encode term embedding: %w", err)
	}

	// Query all matching artifact kinds.
	var matches []semanticTerminologyMatch
	for _, kind := range kinds {
		kindMatches, err := searchTermChunksForKind(ctx, repo, queryBlob, term, preferred, kind, literalMatchers)
		if err != nil {
			return nil, err
		}
		matches = append(matches, kindMatches...)
	}
	return matches, nil
}

func searchTermChunksForKind(ctx context.Context, repo *analysisRepository, queryBlob []byte, term, preferred, kind string, literalMatchers []terminologyMatcher) ([]semanticTerminologyMatch, error) {
	sqlText, args := buildSemanticTermChunkQuery(kind, queryBlob)
	rows, err := repo.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("query semantic terminology chunks: %w", err)
	}
	defer rows.Close()

	termLower := strings.ToLower(term)
	var matches []semanticTerminologyMatch
	for rows.Next() {
		var (
			ref       string
			title     string
			sourceRef string
			section   string
			content   string
			distance  float64
		)
		if err := rows.Scan(&ref, &title, &sourceRef, &section, &content, &distance); err != nil {
			return nil, fmt.Errorf("scan semantic terminology chunk: %w", err)
		}

		similarity := similarityScoreFromDistance(distance)
		if similarity < semanticTerminologySimilarityThreshold {
			continue
		}

		// Skip chunks where the literal term already appears — those are
		// caught by deterministic matching.
		contentLower := strings.ToLower(content)
		if strings.Contains(contentLower, termLower) {
			continue
		}

		// Also skip if any of the governed literal matchers fire on this chunk.
		if terminologyChunkHasLiteralMatch(content, literalMatchers) {
			continue
		}

		excerpt := extractSemanticExcerpt(content)
		matches = append(matches, semanticTerminologyMatch{
			Term:        term,
			Preferred:   preferred,
			ArtifactRef: ref,
			Kind:        kind,
			Title:       title,
			SourceRef:   sourceRef,
			Section:     section,
			Excerpt:     excerpt,
			Similarity:  roundScore(similarity),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate semantic terminology chunks: %w", err)
	}
	return matches, nil
}

func buildSemanticTermChunkQuery(kind string, queryBlob []byte) (string, []any) {
	return `
WITH vector_hits AS (
  SELECT chunk_id, distance
  FROM chunks_vec
  WHERE embedding MATCH ? AND k = ?
  ORDER BY distance
)
SELECT
  a.ref,
  a.title,
  a.source_ref,
  c.section,
  c.content,
  vh.distance
FROM vector_hits vh
JOIN chunks c ON c.id = vh.chunk_id
JOIN artifacts a ON a.ref = c.artifact_ref
WHERE a.kind = ?
ORDER BY vh.distance ASC
LIMIT ?`, []any{queryBlob, semanticTerminologyShortlistLimit * 4, kind, semanticTerminologyShortlistLimit}
}

// terminologyChunkHasLiteralMatch returns true if any governed term matcher
// fires on the given content.
func terminologyChunkHasLiteralMatch(content string, matchers []terminologyMatcher) bool {
	for _, m := range matchers {
		if m.Pattern.MatchString(content) {
			return true
		}
	}
	return false
}

func extractSemanticExcerpt(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if trimmed != "" {
			if len(trimmed) > 200 {
				return trimmed[:197] + "..."
			}
			return trimmed
		}
	}
	return ""
}

func deduplicateSemanticMatches(matches []semanticTerminologyMatch) []semanticTerminologyMatch {
	type key struct {
		ref     string
		section string
		term    string
	}
	seen := make(map[key]int, len(matches))
	result := make([]semanticTerminologyMatch, 0, len(matches))
	for _, m := range matches {
		k := key{ref: m.ArtifactRef, section: m.Section, term: m.Term}
		if idx, ok := seen[k]; ok {
			// Keep the higher-similarity match.
			if m.Similarity > result[idx].Similarity {
				result[idx] = m
			}
			continue
		}
		seen[k] = len(result)
		result = append(result, m)
	}
	return result
}

// isSemanticEmbedderConfigured returns true when the configured embedder is a
// real semantic model (not the deterministic fixture).
func isSemanticEmbedderConfigured(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	provider := strings.TrimSpace(cfg.Runtime.Embedder.Provider)
	return provider != "" && provider != config.RuntimeProviderFixture
}

// sourceRefFromMetadata extracts the source_ref from artifact metadata.
func sourceRefFromMetadata(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	return strings.TrimSpace(metadata["source_ref"])
}

// convertSemanticMatchesToFindings converts semantic near-misses into
// TerminologyFinding entries that integrate with the standard results.
func convertSemanticMatchesToFindings(matches []semanticTerminologyMatch, governed map[string]terminologyGovernedTerm) []TerminologyFinding {
	// Group by artifact ref.
	type artifactGroup struct {
		ref       string
		kind      string
		title     string
		sourceRef string
		matches   []semanticTerminologyMatch
	}
	groups := make(map[string]*artifactGroup)
	for _, m := range matches {
		g, ok := groups[m.ArtifactRef]
		if !ok {
			g = &artifactGroup{
				ref:       m.ArtifactRef,
				kind:      m.Kind,
				title:     m.Title,
				sourceRef: m.SourceRef,
			}
			groups[m.ArtifactRef] = g
		}
		g.matches = append(g.matches, m)
	}

	findings := make([]TerminologyFinding, 0, len(groups))
	for _, g := range groups {
		sections := make([]TerminologySectionFinding, 0, len(g.matches))
		terms := make([]string, 0)
		bestScore := 0.0

		for _, m := range g.matches {
			rule, hasRule := governed[strings.ToLower(m.Term)]

			match := TerminologyTermMatch{
				Term:           m.Term,
				PreferredTerm:  m.Preferred,
				Classification: terminologyClassificationDisplacedTerm,
				Context:        terminologyContextCurrentState,
				Severity:       config.TerminologySeverityWarning,
				Replacement:    m.Preferred,
				Provenance:     ProvenanceEmbeddingSimilarity,
				Confidence:     m.Similarity,
			}
			if hasRule {
				match.Classification = rule.Classification
			}

			sections = append(sections, TerminologySectionFinding{
				Section:    defaultString(strings.TrimSpace(m.Section), "(body)"),
				Terms:      []string{m.Term},
				Matches:    []TerminologyTermMatch{match},
				Excerpt:    m.Excerpt,
				Assessment: fmt.Sprintf("semantic near-miss (%.0f%% similarity) — concept present without literal term", m.Similarity*100),
			})
			terms = append(terms, m.Term)
			if m.Similarity > bestScore {
				bestScore = m.Similarity
			}
		}

		findings = append(findings, TerminologyFinding{
			Ref:       g.ref,
			Kind:      g.kind,
			Title:     g.title,
			SourceRef: g.sourceRef,
			Terms:     uniqueStrings(terms),
			Score:     roundScore(bestScore),
			Sections:  sections,
		})
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Score != findings[j].Score {
			return findings[i].Score > findings[j].Score
		}
		return findings[i].Ref < findings[j].Ref
	})
	return findings
}
