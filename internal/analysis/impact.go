package analysis

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
)

const (
	impactDocClassificationSemanticNeighbor = "semantic_neighbor"
	impactDocClassificationGovernedSurface  = "governed_surface_neighbor"
)

// AnalyzeImpactRequest is the normalized input for impact analysis.
type AnalyzeImpactRequest struct {
	SpecRef    string            `json:"spec_ref,omitempty"`
	SpecRecord *model.SpecRecord `json:"spec_record,omitempty"`
	ChangeType string            `json:"change_type"`
	Summary    bool              `json:"summary,omitempty"`
}

// ImpactedSpec reports one affected spec.
type ImpactedSpec struct {
	Ref          string                     `json:"ref"`
	Title        string                     `json:"title"`
	Repo         string                     `json:"repo,omitempty"`
	Status       string                     `json:"status,omitempty"`
	Relationship string                     `json:"relationship"`
	Historical   bool                       `json:"historical"`
	Inference    *model.InferenceConfidence `json:"inference,omitempty"`
}

// ImpactedRef reports one affected code or config reference.
type ImpactedRef struct {
	Ref  string `json:"ref"`
	Kind string `json:"kind"`
}

// ImpactEvidence reports the concrete spec/doc sections that explain why one
// document was shortlisted for follow-up during impact analysis.
type ImpactEvidence struct {
	SpecRef       string `json:"spec_ref"`
	SpecTitle     string `json:"spec_title,omitempty"`
	SpecSourceRef string `json:"spec_source_ref,omitempty"`
	SpecSection   string `json:"spec_section,omitempty"`
	SpecExcerpt   string `json:"spec_excerpt,omitempty"`
	DocSourceRef  string `json:"doc_source_ref,omitempty"`
	DocSection    string `json:"doc_section,omitempty"`
	DocExcerpt    string `json:"doc_excerpt,omitempty"`
	LinkReason    string `json:"link_reason,omitempty"`
}

// ImpactEditTarget reports the most likely doc section to inspect or revise
// next when a spec change implies downstream edits.
type ImpactEditTarget struct {
	SourceRef        string   `json:"source_ref,omitempty"`
	Section          string   `json:"section,omitempty"`
	Excerpt          string   `json:"excerpt,omitempty"`
	Reason           string   `json:"reason,omitempty"`
	SuggestedBullets []string `json:"suggested_bullets,omitempty"`
}

// ImpactedDoc reports one semantically related document.
type ImpactedDoc struct {
	Ref              string             `json:"ref"`
	Title            string             `json:"title"`
	Repo             string             `json:"repo,omitempty"`
	SourceRef        string             `json:"source_ref"`
	Score            float64            `json:"score"`
	Classification   string             `json:"classification,omitempty"`
	Reasons          []string           `json:"reasons,omitempty"`
	Evidence         *ImpactEvidence    `json:"evidence,omitempty"`
	SuggestedTargets []ImpactEditTarget `json:"suggested_targets,omitempty"`
}

// ImpactSummaryItem captures one high-priority follow-up target.
type ImpactSummaryItem struct {
	Rank        int     `json:"rank"`
	Kind        string  `json:"kind"`
	Ref         string  `json:"ref"`
	Title       string  `json:"title,omitempty"`
	Repo        string  `json:"repo,omitempty"`
	SourceRef   string  `json:"source_ref,omitempty"`
	Score       float64 `json:"score,omitempty"`
	Why         string  `json:"why,omitempty"`
	ReviewFirst string  `json:"review_first,omitempty"`
}

// AnalyzeImpactResult is the structured impact-analysis response.
type AnalyzeImpactResult struct {
	SpecRef       string                     `json:"spec_ref"`
	SpecInference *model.InferenceConfidence `json:"spec_inference,omitempty"`
	ChangeType    string                     `json:"change_type"`
	SummaryOnly   bool                       `json:"summary_only,omitempty"`
	RankedSummary []ImpactSummaryItem        `json:"ranked_summary,omitempty"`
	AffectedSpecs []ImpactedSpec             `json:"affected_specs"`
	AffectedRefs  []ImpactedRef              `json:"affected_refs"`
	AffectedDocs  []ImpactedDoc              `json:"affected_docs"`
	Warnings      []Warning                  `json:"warnings,omitempty"`
}

// AnalyzeImpact determines dependent specs, governed refs, and related docs.
func AnalyzeImpact(cfg *config.Config, request AnalyzeImpactRequest) (*AnalyzeImpactResult, error) {
	return AnalyzeImpactContext(context.Background(), cfg, request)
}

// AnalyzeImpactContext determines dependent specs, governed refs, and related docs.
func AnalyzeImpactContext(ctx context.Context, cfg *config.Config, request AnalyzeImpactRequest) (*AnalyzeImpactResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	request.SpecRef = stringsTrimSpace(request.SpecRef)
	request.ChangeType = defaultString(stringsTrimSpace(request.ChangeType), "accepted")
	switch {
	case request.SpecRef != "" && request.SpecRecord != nil:
		return nil, fmt.Errorf("exactly one of spec_ref or spec_record is allowed")
	case request.SpecRef == "" && request.SpecRecord == nil:
		return nil, fmt.Errorf("one of spec_ref or spec_record is required")
	}
	if !isValidChangeType(request.ChangeType) {
		return nil, fmt.Errorf("change_type %q is invalid", request.ChangeType)
	}

	repo, err := openAnalysisRepositoryContext(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer repo.Close()

	candidate, err := loadCandidate(repo, OverlapRequest{
		SpecRef:    request.SpecRef,
		SpecRecord: request.SpecRecord,
	}, nil)
	if err != nil {
		return nil, err
	}

	specRefs, err := repo.impactedSpecRefs(candidate.Record)
	if err != nil {
		return nil, err
	}
	specs, err := repo.loadSelectedSpecs(specRefs)
	if err != nil {
		return nil, err
	}

	docRefs, err := repo.impactedDocRefs(*candidate)
	if err != nil {
		return nil, err
	}
	docs, err := repo.loadSelectedDocs(docRefs)
	if err != nil {
		return nil, err
	}

	return buildAnalyzeImpactResult(candidate, request.ChangeType, request.Summary, specs, docs), nil
}

func buildAnalyzeImpactResult(candidate *specDocument, changeType string, summaryOnly bool, specs map[string]specDocument, docs map[string]docDocument) *AnalyzeImpactResult {
	relevantSpecs := make([]specDocument, 0, len(specs)+1)
	relevantSpecs = append(relevantSpecs, *candidate)
	for _, spec := range specs {
		relevantSpecs = append(relevantSpecs, spec)
	}
	affectedSpecs := impactedSpecs(candidate.Record, specs)
	affectedDocs := impactedDocs(*candidate, docs)
	return &AnalyzeImpactResult{
		SpecRef:       candidate.Record.Ref,
		SpecInference: candidate.Record.Inference,
		ChangeType:    changeType,
		SummaryOnly:   summaryOnly,
		RankedSummary: buildImpactSummary(affectedSpecs, affectedDocs, 5),
		AffectedSpecs: affectedSpecs,
		AffectedRefs:  impactedRefs(candidate.Record, specs),
		AffectedDocs:  affectedDocs,
		Warnings:      buildSpecInferenceWarnings("impact analysis", relevantSpecs...),
	}
}

func impactedSpecs(candidate model.SpecRecord, specs map[string]specDocument) []ImpactedSpec {
	var result []ImpactedSpec
	for _, spec := range specs {
		if spec.Record.Ref == candidate.Ref {
			continue
		}

		switch {
		case relationExists(spec.Record.Relations, model.RelationDependsOn, candidate.Ref):
			result = append(result, ImpactedSpec{
				Ref:          spec.Record.Ref,
				Title:        spec.Record.Title,
				Repo:         artifactRepoID(spec.Record.Metadata),
				Status:       spec.Record.Status,
				Relationship: string(model.RelationDependsOn),
				Historical:   false,
				Inference:    spec.Record.Inference,
			})
		case relationExists(candidate.Relations, model.RelationSupersedes, spec.Record.Ref):
			result = append(result, ImpactedSpec{
				Ref:          spec.Record.Ref,
				Title:        spec.Record.Title,
				Repo:         artifactRepoID(spec.Record.Metadata),
				Status:       spec.Record.Status,
				Relationship: string(model.RelationSupersedes),
				Historical:   true,
				Inference:    spec.Record.Inference,
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		switch {
		case result[i].Historical != result[j].Historical:
			return !result[i].Historical
		case result[i].Relationship != result[j].Relationship:
			return result[i].Relationship < result[j].Relationship
		default:
			return result[i].Ref < result[j].Ref
		}
	})
	return result
}

func impactedRefs(candidate model.SpecRecord, specs map[string]specDocument) []ImpactedRef {
	refKinds := make(map[string]string)
	for _, ref := range candidate.AppliesTo {
		refKinds[ref] = refKind(ref)
	}
	for _, spec := range specs {
		if !relationExists(spec.Record.Relations, model.RelationDependsOn, candidate.Ref) {
			continue
		}
		for _, ref := range spec.Record.AppliesTo {
			refKinds[ref] = refKind(ref)
		}
	}

	refs := make([]string, 0, len(refKinds))
	for ref := range refKinds {
		refs = append(refs, ref)
	}
	sort.Strings(refs)

	result := make([]ImpactedRef, 0, len(refs))
	for _, ref := range refs {
		result = append(result, ImpactedRef{
			Ref:  ref,
			Kind: refKinds[ref],
		})
	}
	return result
}

func impactedDocs(candidate specDocument, docs map[string]docDocument) []ImpactedDoc {
	var result []ImpactedDoc
	for _, doc := range docs {
		score := documentSimilarity(candidate.Sections, doc.Sections)
		if score < 0.35 {
			continue
		}
		classification, reasons := classifyImpactedDoc(candidate, doc)
		evidence := impactDocEvidence(candidate, doc)
		result = append(result, ImpactedDoc{
			Ref:              doc.Record.Ref,
			Title:            doc.Record.Title,
			Repo:             artifactRepoID(doc.Record.Metadata),
			SourceRef:        doc.Record.SourceRef,
			Score:            roundScore(score),
			Classification:   classification,
			Reasons:          reasons,
			Evidence:         evidence,
			SuggestedTargets: impactEditTargets(doc, evidence, reasons),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		switch {
		case result[i].Score != result[j].Score:
			return result[i].Score > result[j].Score
		default:
			return result[i].Ref < result[j].Ref
		}
	})
	return result
}

func buildImpactSummary(specs []ImpactedSpec, docs []ImpactedDoc, limit int) []ImpactSummaryItem {
	if limit <= 0 {
		return nil
	}

	type scoredSummary struct {
		item     ImpactSummaryItem
		priority int
		score    float64
	}

	scored := make([]scoredSummary, 0, len(specs)+len(docs))
	for _, item := range specs {
		why := "depends on this spec"
		priority := 0
		if item.Historical {
			why = "historical spec superseded by this spec"
			priority = 3
		}
		scored = append(scored, scoredSummary{
			item: ImpactSummaryItem{
				Kind:        "spec",
				Ref:         item.Ref,
				Title:       item.Title,
				Repo:        item.Repo,
				Why:         why,
				ReviewFirst: impactSummarySpecTarget(item),
			},
			priority: priority,
		})
	}
	for _, item := range docs {
		priority := 2
		if item.Classification == impactDocClassificationGovernedSurface {
			priority = 1
		}
		scored = append(scored, scoredSummary{
			item: ImpactSummaryItem{
				Kind:        "doc",
				Ref:         item.Ref,
				Title:       item.Title,
				Repo:        item.Repo,
				SourceRef:   item.SourceRef,
				Score:       item.Score,
				Why:         impactSummaryDocReason(item),
				ReviewFirst: impactSummaryDocTarget(item),
			},
			priority: priority,
			score:    item.Score,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		switch {
		case scored[i].priority != scored[j].priority:
			return scored[i].priority < scored[j].priority
		case scored[i].score != scored[j].score:
			return scored[i].score > scored[j].score
		case scored[i].item.Repo != scored[j].item.Repo:
			return scored[i].item.Repo < scored[j].item.Repo
		default:
			return scored[i].item.Ref < scored[j].item.Ref
		}
	})

	if len(scored) > limit {
		scored = scored[:limit]
	}
	result := make([]ImpactSummaryItem, 0, len(scored))
	for i, item := range scored {
		item.item.Rank = i + 1
		result = append(result, item.item)
	}
	return result
}

func impactSummarySpecTarget(item ImpactedSpec) string {
	if stringsTrimSpace(item.Title) == "" {
		return item.Ref
	}
	return fmt.Sprintf("%s %s", item.Ref, item.Title)
}

func impactSummaryDocReason(item ImpactedDoc) string {
	for _, reason := range item.Reasons {
		if trimmed := stringsTrimSpace(reason); trimmed != "" {
			return trimmed
		}
	}
	if item.Evidence != nil {
		if trimmed := stringsTrimSpace(item.Evidence.LinkReason); trimmed != "" {
			return trimmed
		}
	}
	return humanizeImpactSummaryLabel(item.Classification)
}

func impactSummaryDocTarget(item ImpactedDoc) string {
	if len(item.SuggestedTargets) > 0 {
		target := item.SuggestedTargets[0]
		if trimmed := stringsTrimSpace(target.SourceRef); trimmed != "" {
			if section := stringsTrimSpace(target.Section); section != "" {
				return fmt.Sprintf("%s / %s", trimmed, section)
			}
			return trimmed
		}
		if section := stringsTrimSpace(target.Section); section != "" {
			return section
		}
	}
	if trimmed := stringsTrimSpace(item.SourceRef); trimmed != "" {
		return trimmed
	}
	return item.Ref
}

func humanizeImpactSummaryLabel(value string) string {
	value = stringsTrimSpace(value)
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	return value
}

func documentSimilarity(left, right []embeddedSection) float64 {
	return bestSectionOverlap(left, right)
}

func classifyImpactedDoc(candidate specDocument, doc docDocument) (string, []string) {
	if hasArtifactConstraintOverlap(artifactMentionSet(doc.Sections), candidate) {
		return impactDocClassificationGovernedSurface, []string{
			fmt.Sprintf("doc mentions a governed artifact or runtime surface constrained by %s", candidate.Record.Ref),
		}
	}
	return impactDocClassificationSemanticNeighbor, []string{
		fmt.Sprintf("doc contains the strongest semantic neighbor sections to %s and should be reviewed after this change", candidate.Record.Ref),
	}
}

func impactDocEvidence(candidate specDocument, doc docDocument) *ImpactEvidence {
	specSection, docSection, score := bestImpactSectionPair(candidate.Sections, doc.Sections)
	if specSection == nil && docSection == nil {
		return nil
	}

	evidence := &ImpactEvidence{
		SpecRef:       candidate.Record.Ref,
		SpecTitle:     candidate.Record.Title,
		SpecSourceRef: candidate.Record.SourceRef,
		DocSourceRef:  doc.Record.SourceRef,
		LinkReason:    fmt.Sprintf("highest section-level semantic overlap between %s and %s (score %.3f)", candidate.Record.Ref, doc.Record.Ref, roundScore(score)),
	}
	if specSection != nil {
		evidence.SpecSection = defaultString(stringsTrimSpace(specSection.Heading), "(body)")
		evidence.SpecExcerpt = defaultString(sectionExcerpt(*specSection), stringsTrimSpace(candidate.Record.Title))
	}
	if docSection != nil {
		evidence.DocSection = defaultString(stringsTrimSpace(docSection.Heading), "(body)")
		evidence.DocExcerpt = sectionExcerpt(*docSection)
	}
	return evidence
}

func bestImpactSectionPair(left, right []embeddedSection) (*embeddedSection, *embeddedSection, float64) {
	var (
		bestLeft  *embeddedSection
		bestRight *embeddedSection
		bestScore float64
	)
	for i := range left {
		for j := range right {
			score := cosineSimilarity(left[i].Embedding, right[j].Embedding)
			if score <= bestScore {
				continue
			}
			leftCopy := left[i]
			rightCopy := right[j]
			bestLeft = &leftCopy
			bestRight = &rightCopy
			bestScore = score
		}
	}
	return bestLeft, bestRight, bestScore
}

func impactEditTargets(doc docDocument, evidence *ImpactEvidence, reasons []string) []ImpactEditTarget {
	if evidence == nil || stringsTrimSpace(evidence.DocSourceRef) == "" {
		return nil
	}

	reason := ""
	if len(reasons) > 0 {
		reason = reasons[0]
	}
	bullets := []string{
		fmt.Sprintf("Review %s in %s against %s / %s before editing downstream references.", defaultString(stringsTrimSpace(evidence.DocSection), "(body)"), doc.Record.Ref, evidence.SpecRef, defaultString(stringsTrimSpace(evidence.SpecSection), "(body)")),
	}
	if evidence.SpecExcerpt != "" {
		bullets = append(bullets, "Carry forward the accepted spec excerpt rather than paraphrasing the change from memory.")
	}
	return []ImpactEditTarget{
		{
			SourceRef:        evidence.DocSourceRef,
			Section:          evidence.DocSection,
			Excerpt:          evidence.DocExcerpt,
			Reason:           reason,
			SuggestedBullets: bullets,
		},
	}
}

func isValidChangeType(changeType string) bool {
	switch changeType {
	case "accepted", "modified", "deprecated":
		return true
	default:
		return false
	}
}

func refKind(ref string) string {
	switch {
	case stringsHasPrefix(ref, "code://"):
		return "code"
	case stringsHasPrefix(ref, "config://"):
		return "config"
	default:
		return "ref"
	}
}
