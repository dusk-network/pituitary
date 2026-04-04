package analysis

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/resultmeta"
)

const (
	freshnessDocShortlistLimit        = 24
	freshnessDecisionTrailScoreFloor  = 0.40
	freshnessContradictoryScoreFloor  = 0.35
	freshnessFoundationSeverityWeight = 0.6
	freshnessDecisionTrailWeight      = 0.8
	freshnessContradictoryWeight      = 0.5
)

// Freshness signal provenance types.
const (
	freshnessSignalDecisionTrail = "decision_trail"
	freshnessSignalFoundation    = "foundation"
	freshnessSignalContradictory = "contradictory"
)

// FreshnessRequest is the normalized input for spec-freshness detection.
type FreshnessRequest struct {
	SpecRef  string   `json:"spec_ref,omitempty"`
	Scope    string   `json:"scope,omitempty"`
	SpecRefs []string `json:"spec_refs,omitempty"`
}

// FreshnessSignal reports one staleness indicator for a spec.
type FreshnessSignal struct {
	Kind       string             `json:"kind"`
	Confidence string             `json:"confidence"`
	Score      float64            `json:"score"`
	Summary    string             `json:"summary"`
	Provenance string             `json:"provenance"`
	Evidence   *FreshnessEvidence `json:"evidence,omitempty"`
}

// FreshnessEvidence reports the concrete sections supporting a staleness signal.
type FreshnessEvidence struct {
	SpecRef        string `json:"spec_ref,omitempty"`
	SpecTitle      string `json:"spec_title,omitempty"`
	SpecSourceRef  string `json:"spec_source_ref,omitempty"`
	SpecSection    string `json:"spec_section,omitempty"`
	SpecExcerpt    string `json:"spec_excerpt,omitempty"`
	TrailRef       string `json:"trail_ref,omitempty"`
	TrailTitle     string `json:"trail_title,omitempty"`
	TrailSourceRef string `json:"trail_source_ref,omitempty"`
	TrailSection   string `json:"trail_section,omitempty"`
	TrailExcerpt   string `json:"trail_excerpt,omitempty"`
	LinkReason     string `json:"link_reason,omitempty"`
}

// FreshnessItem reports the freshness assessment for one spec.
type FreshnessItem struct {
	SpecRef    string                     `json:"spec_ref"`
	Title      string                     `json:"title"`
	Repo       string                     `json:"repo,omitempty"`
	SourceRef  string                     `json:"source_ref"`
	Status     string                     `json:"status"`
	Verdict    string                     `json:"verdict"`
	Confidence string                     `json:"confidence"`
	Score      float64                    `json:"score"`
	Signals    []FreshnessSignal          `json:"signals"`
	Inference  *model.InferenceConfidence `json:"inference,omitempty"`
}

// FreshnessResult is the structured spec-freshness response.
type FreshnessResult struct {
	Scope        string                   `json:"scope"`
	Items        []FreshnessItem          `json:"items"`
	Warnings     []Warning                `json:"warnings,omitempty"`
	ContentTrust *resultmeta.ContentTrust `json:"content_trust,omitempty"`
}

// CheckSpecFreshness runs the freshness detection flow.
func CheckSpecFreshness(cfg *config.Config, request FreshnessRequest) (*FreshnessResult, error) {
	return CheckSpecFreshnessContext(context.Background(), cfg, request)
}

// CheckSpecFreshnessContext runs the freshness detection flow with a parent context.
func CheckSpecFreshnessContext(ctx context.Context, cfg *config.Config, request FreshnessRequest) (*FreshnessResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	repo, err := openAnalysisRepositoryContext(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer repo.Close()

	scope, specRefs, err := normalizeFreshnessScope(request, repo)
	if err != nil {
		return nil, err
	}

	specs, err := repo.loadSelectedSpecs(specRefs)
	if err != nil {
		return nil, err
	}
	for _, ref := range specRefs {
		if _, ok := specs[ref]; !ok {
			return nil, newSpecRefNotFoundError(ref)
		}
	}

	allDocs, err := repo.loadAllDocs()
	if err != nil {
		return nil, err
	}

	allSpecs, err := repo.loadAllSpecs()
	if err != nil {
		return nil, err
	}

	// Precompute doc classification once outside the per-spec loop.
	decisionDocs := filterDecisionBearingDocs(allDocs)
	nonDecisionDocs := make(map[string]docDocument, len(allDocs)-len(decisionDocs))
	for ref, doc := range allDocs {
		if !isDecisionBearingDoc(doc) {
			nonDecisionDocs[ref] = doc
		}
	}

	items := make([]FreshnessItem, 0, len(specs))
	var warnings []Warning
	for _, ref := range sortedSpecRefs(specs) {
		spec := specs[ref]

		item := FreshnessItem{
			SpecRef:   spec.Record.Ref,
			Title:     spec.Record.Title,
			Repo:      spec.Record.Metadata["repo_id"],
			SourceRef: spec.Record.SourceRef,
			Status:    spec.Record.Status,
		}

		if inf, err := model.DecodeInferenceConfidence(spec.Record.Metadata); err == nil && inf != nil {
			item.Inference = inf
		}
		warnings = append(warnings, buildSpecInferenceWarnings("check-spec-freshness", spec)...)

		var signals []FreshnessSignal

		// Signal 1: decision-trail — search decision-bearing docs for contradictions.
		if len(decisionDocs) > 0 {
			trailSignals := decisionTrailSignals(spec, decisionDocs)
			signals = append(signals, trailSignals...)
		}

		// Signal 2: foundation — check if depends_on targets are superseded/deprecated.
		foundationSignals := foundationSignals(spec, allSpecs)
		signals = append(signals, foundationSignals...)

		// Signal 3: contradictory — reverse of doc-drift; docs disagree with spec.
		contradictorySignals := contradictoryDocSignals(spec, nonDecisionDocs)
		signals = append(signals, contradictorySignals...)

		item.Signals = signals
		item.Verdict, item.Confidence, item.Score = assessFreshness(signals)
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		return items[i].SpecRef < items[j].SpecRef
	})

	return &FreshnessResult{
		Scope:        scope,
		Items:        items,
		Warnings:     warnings,
		ContentTrust: resultmeta.UntrustedWorkspaceText(),
	}, nil
}

func normalizeFreshnessScope(request FreshnessRequest, repo *analysisRepository) (string, []string, error) {
	trimmedRef := strings.TrimSpace(request.SpecRef)
	trimmedScope := strings.TrimSpace(request.Scope)

	switch {
	case trimmedRef != "":
		return "single", []string{trimmedRef}, nil
	case len(request.SpecRefs) > 0:
		return "selected", request.SpecRefs, nil
	case trimmedScope == "all" || (trimmedRef == "" && trimmedScope == ""):
		allSpecs, err := repo.loadAllSpecs()
		if err != nil {
			return "", nil, err
		}
		refs := make([]string, 0, len(allSpecs))
		for ref, spec := range allSpecs {
			if spec.Record.Status == model.StatusAccepted || spec.Record.Status == model.StatusReview {
				refs = append(refs, ref)
			}
		}
		sort.Strings(refs)
		return "all", refs, nil
	default:
		return "", nil, fmt.Errorf("unsupported scope %q", trimmedScope)
	}
}

// isDecisionBearingDoc identifies docs that serve as decision-trail artifacts
// based on their source role metadata or file path conventions.
func isDecisionBearingDoc(doc docDocument) bool {
	role := config.NormalizeSourceRole(doc.Record.Metadata[sourceRoleMetadataKey])
	switch role {
	case config.SourceRoleDecisionLog, config.SourceRoleCurrentState:
		return true
	}

	src := strings.ToLower(filepath.ToSlash(doc.Record.SourceRef))
	base := strings.ToLower(filepath.Base(src))

	decisionPaths := []string{
		"decisions/", "decision-records/", "memory/", "adr/", "adrs/",
	}
	for _, prefix := range decisionPaths {
		if strings.Contains(src, prefix) {
			return true
		}
	}

	decisionBases := []string{
		"memory.md", "handoff.md", "decision-log.md", "decision_log.md",
		"decisions.md",
	}
	for _, db := range decisionBases {
		if base == db {
			return true
		}
	}

	if strings.HasPrefix(base, "adr-") || strings.HasPrefix(base, "decision-") {
		return true
	}

	return false
}

func filterDecisionBearingDocs(docs map[string]docDocument) map[string]docDocument {
	result := make(map[string]docDocument)
	for ref, doc := range docs {
		if isDecisionBearingDoc(doc) {
			result[ref] = doc
		}
	}
	return result
}

// decisionTrailSignals searches decision-bearing docs for sections that
// semantically overlap with the spec's sections, which suggests a deliberate
// decision was made that may contradict or supersede the spec.
func decisionTrailSignals(spec specDocument, decisionDocs map[string]docDocument) []FreshnessSignal {
	var signals []FreshnessSignal

	for _, docRef := range sortedDocRefs(decisionDocs) {
		doc := decisionDocs[docRef]
		best := bestSectionPair(spec.Sections, doc.Sections)
		if best == nil || best.score < freshnessDecisionTrailScoreFloor {
			continue
		}

		specExcerpt := truncateExcerpt(best.specSection.Content, 300)
		trailExcerpt := truncateExcerpt(best.docSection.Content, 300)

		signals = append(signals, FreshnessSignal{
			Kind:       freshnessSignalDecisionTrail,
			Confidence: scoreToConfidence(best.score),
			Score:      best.score,
			Summary:    fmt.Sprintf("decision-bearing artifact %q has content semantically related to this spec (score %.3f)", doc.Record.Title, best.score),
			Provenance: ProvenanceEmbeddingSimilarity,
			Evidence: &FreshnessEvidence{
				SpecRef:        spec.Record.Ref,
				SpecTitle:      spec.Record.Title,
				SpecSourceRef:  spec.Record.SourceRef,
				SpecSection:    best.specSection.Heading,
				SpecExcerpt:    specExcerpt,
				TrailRef:       doc.Record.Ref,
				TrailTitle:     doc.Record.Title,
				TrailSourceRef: doc.Record.SourceRef,
				TrailSection:   best.docSection.Heading,
				TrailExcerpt:   trailExcerpt,
				LinkReason:     fmt.Sprintf("highest section-level semantic overlap (score %.3f)", best.score),
			},
		})
	}

	sort.Slice(signals, func(i, j int) bool {
		return signals[i].Score > signals[j].Score
	})
	return signals
}

// foundationSignals checks whether the spec's depends_on targets have been
// superseded or deprecated, which makes the spec's foundations unreliable.
func foundationSignals(spec specDocument, allSpecs map[string]specDocument) []FreshnessSignal {
	var signals []FreshnessSignal

	for _, rel := range spec.Record.Relations {
		if rel.Type != model.RelationDependsOn {
			continue
		}
		target, ok := allSpecs[rel.Ref]
		if !ok {
			continue
		}
		switch target.Record.Status {
		case model.StatusSuperseded:
			signals = append(signals, FreshnessSignal{
				Kind:       freshnessSignalFoundation,
				Confidence: "high",
				Score:      freshnessFoundationSeverityWeight,
				Summary:    fmt.Sprintf("depends on %s (%s) which has been superseded", target.Record.Ref, target.Record.Title),
				Provenance: ProvenanceLiteral,
				Evidence: &FreshnessEvidence{
					SpecRef:        spec.Record.Ref,
					SpecTitle:      spec.Record.Title,
					SpecSourceRef:  spec.Record.SourceRef,
					TrailRef:       target.Record.Ref,
					TrailTitle:     target.Record.Title,
					TrailSourceRef: target.Record.SourceRef,
					LinkReason:     fmt.Sprintf("explicit depends_on relation to superseded spec %s", target.Record.Ref),
				},
			})
		case model.StatusDeprecated:
			signals = append(signals, FreshnessSignal{
				Kind:       freshnessSignalFoundation,
				Confidence: "high",
				Score:      freshnessFoundationSeverityWeight * 0.8,
				Summary:    fmt.Sprintf("depends on %s (%s) which has been deprecated", target.Record.Ref, target.Record.Title),
				Provenance: ProvenanceLiteral,
				Evidence: &FreshnessEvidence{
					SpecRef:        spec.Record.Ref,
					SpecTitle:      spec.Record.Title,
					SpecSourceRef:  spec.Record.SourceRef,
					TrailRef:       target.Record.Ref,
					TrailTitle:     target.Record.Title,
					TrailSourceRef: target.Record.SourceRef,
					LinkReason:     fmt.Sprintf("explicit depends_on relation to deprecated spec %s", target.Record.Ref),
				},
			})
		}
	}

	return signals
}

// contradictoryDocSignals identifies non-decision-trail docs whose content
// significantly overlaps with the spec. When multiple docs independently
// describe the same area differently from the spec, that's a weak staleness
// indicator (especially without a decision trail).
func contradictoryDocSignals(spec specDocument, allDocs map[string]docDocument) []FreshnessSignal {
	var signals []FreshnessSignal

	for _, docRef := range sortedDocRefs(allDocs) {
		doc := allDocs[docRef]
		if isDecisionBearingDoc(doc) {
			continue
		}
		best := bestSectionPair(spec.Sections, doc.Sections)
		if best == nil || best.score < freshnessContradictoryScoreFloor {
			continue
		}

		// Only report docs with very high semantic overlap as contradictory signals.
		// Lower overlap just means the doc covers similar ground, not contradiction.
		if best.score < 0.55 {
			continue
		}

		specExcerpt := truncateExcerpt(best.specSection.Content, 300)
		docExcerpt := truncateExcerpt(best.docSection.Content, 300)

		signals = append(signals, FreshnessSignal{
			Kind:       freshnessSignalContradictory,
			Confidence: scoreToConfidence(best.score * 0.8), // downweight: overlap isn't proof of contradiction
			Score:      best.score * freshnessContradictoryWeight,
			Summary:    fmt.Sprintf("doc %q has high semantic overlap with this spec (score %.3f); may describe the same area differently", doc.Record.Title, best.score),
			Provenance: ProvenanceEmbeddingSimilarity,
			Evidence: &FreshnessEvidence{
				SpecRef:        spec.Record.Ref,
				SpecTitle:      spec.Record.Title,
				SpecSourceRef:  spec.Record.SourceRef,
				SpecSection:    best.specSection.Heading,
				SpecExcerpt:    specExcerpt,
				TrailRef:       doc.Record.Ref,
				TrailTitle:     doc.Record.Title,
				TrailSourceRef: doc.Record.SourceRef,
				TrailSection:   best.docSection.Heading,
				TrailExcerpt:   docExcerpt,
				LinkReason:     fmt.Sprintf("highest section-level semantic overlap (score %.3f)", best.score),
			},
		})
	}

	sort.Slice(signals, func(i, j int) bool {
		return signals[i].Score > signals[j].Score
	})

	// Cap at a reasonable number to avoid noise.
	if len(signals) > 5 {
		signals = signals[:5]
	}
	return signals
}

type sectionPairResult struct {
	specSection embeddedSection
	docSection  embeddedSection
	score       float64
}

// bestSectionPair finds the highest-scoring pair of sections between spec and doc.
func bestSectionPair(specSections, docSections []embeddedSection) *sectionPairResult {
	var best *sectionPairResult

	for _, ss := range specSections {
		if len(ss.Embedding) == 0 {
			continue
		}
		for _, ds := range docSections {
			if len(ds.Embedding) == 0 {
				continue
			}
			score := cosineSimilarity(ss.Embedding, ds.Embedding)
			if best == nil || score > best.score {
				best = &sectionPairResult{
					specSection: ss,
					docSection:  ds,
					score:       score,
				}
			}
		}
	}

	return best
}

// assessFreshness combines signals into an overall verdict.
func assessFreshness(signals []FreshnessSignal) (verdict, confidence string, score float64) {
	if len(signals) == 0 {
		return "fresh", "high", 0
	}

	var hasDecisionTrail, hasFoundation bool
	var maxScore float64
	var totalWeightedScore float64
	var count int

	for _, s := range signals {
		if s.Score > maxScore {
			maxScore = s.Score
		}
		totalWeightedScore += s.Score
		count++
		switch s.Kind {
		case freshnessSignalDecisionTrail:
			hasDecisionTrail = true
		case freshnessSignalFoundation:
			hasFoundation = true
		}
	}

	avgScore := totalWeightedScore / float64(count)

	// Decision trail is the strongest signal per the issue design.
	if hasDecisionTrail && maxScore >= 0.50 {
		return "likely_stale", "high", maxScore
	}
	if hasDecisionTrail {
		return "likely_stale", "medium", maxScore
	}

	// Foundation issues are structural — the spec's dependencies are broken.
	if hasFoundation {
		return "stale_foundation", "high", maxScore
	}

	// Contradictory signals alone are weak — spec is still authoritative.
	if avgScore >= 0.40 {
		return "review_recommended", "low", avgScore
	}

	return "fresh", "high", 0
}

func scoreToConfidence(score float64) string {
	switch {
	case score >= 0.70:
		return "high"
	case score >= 0.45:
		return "medium"
	default:
		return "low"
	}
}

func truncateExcerpt(s string, maxLen int) string {
	if maxLen <= 0 {
		return "..."
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	// Find last space before limit to avoid mid-word truncation.
	prefix := string(runes[:maxLen])
	cut := maxLen
	if idx := strings.LastIndex(prefix, " "); idx > maxLen/2 {
		cut = idx
	}
	return string(runes[:cut]) + "..."
}
