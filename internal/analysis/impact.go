package analysis

import (
	"context"
	"fmt"
	"sort"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
)

// AnalyzeImpactRequest is the normalized input for impact analysis.
type AnalyzeImpactRequest struct {
	SpecRef    string            `json:"spec_ref,omitempty"`
	SpecRecord *model.SpecRecord `json:"spec_record,omitempty"`
	ChangeType string            `json:"change_type"`
}

// ImpactedSpec reports one affected spec.
type ImpactedSpec struct {
	Ref          string                     `json:"ref"`
	Title        string                     `json:"title"`
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

// ImpactedDoc reports one semantically related document.
type ImpactedDoc struct {
	Ref       string  `json:"ref"`
	Title     string  `json:"title"`
	SourceRef string  `json:"source_ref"`
	Score     float64 `json:"score"`
}

// AnalyzeImpactResult is the structured impact-analysis response.
type AnalyzeImpactResult struct {
	SpecRef       string                     `json:"spec_ref"`
	SpecInference *model.InferenceConfidence `json:"spec_inference,omitempty"`
	ChangeType    string                     `json:"change_type"`
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

	return buildAnalyzeImpactResult(candidate, request.ChangeType, specs, docs), nil
}

func buildAnalyzeImpactResult(candidate *specDocument, changeType string, specs map[string]specDocument, docs map[string]docDocument) *AnalyzeImpactResult {
	relevantSpecs := make([]specDocument, 0, len(specs)+1)
	relevantSpecs = append(relevantSpecs, *candidate)
	for _, spec := range specs {
		relevantSpecs = append(relevantSpecs, spec)
	}
	return &AnalyzeImpactResult{
		SpecRef:       candidate.Record.Ref,
		SpecInference: candidate.Record.Inference,
		ChangeType:    changeType,
		AffectedSpecs: impactedSpecs(candidate.Record, specs),
		AffectedRefs:  impactedRefs(candidate.Record, specs),
		AffectedDocs:  impactedDocs(*candidate, docs),
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
				Status:       spec.Record.Status,
				Relationship: string(model.RelationDependsOn),
				Historical:   false,
				Inference:    spec.Record.Inference,
			})
		case relationExists(candidate.Relations, model.RelationSupersedes, spec.Record.Ref):
			result = append(result, ImpactedSpec{
				Ref:          spec.Record.Ref,
				Title:        spec.Record.Title,
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
		result = append(result, ImpactedDoc{
			Ref:       doc.Record.Ref,
			Title:     doc.Record.Title,
			SourceRef: doc.Record.SourceRef,
			Score:     roundScore(score),
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

func documentSimilarity(left, right []embeddedSection) float64 {
	return bestSectionOverlap(left, right)
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
