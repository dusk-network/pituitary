package analysis

import (
	"context"
	"fmt"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
)

// ReviewRequest is the normalized review-spec input.
type ReviewRequest struct {
	SpecRef    string            `json:"spec_ref,omitempty"`
	SpecRecord *model.SpecRecord `json:"spec_record,omitempty"`
}

// ReviewResult is the composed review-spec response.
type ReviewResult struct {
	SpecRef    string               `json:"spec_ref"`
	Overlap    *OverlapResult       `json:"overlap"`
	Comparison *CompareResult       `json:"comparison"`
	Impact     *AnalyzeImpactResult `json:"impact"`
	DocDrift   *DocDriftResult      `json:"doc_drift"`
}

// ReviewSpec composes overlap, comparison, impact, and targeted doc-drift.
func ReviewSpec(cfg *config.Config, request ReviewRequest) (*ReviewResult, error) {
	return ReviewSpecContext(context.Background(), cfg, request)
}

// ReviewSpecContext composes overlap, comparison, impact, and targeted doc-drift.
func ReviewSpecContext(ctx context.Context, cfg *config.Config, request ReviewRequest) (*ReviewResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if err := validateOverlapRequest(OverlapRequest{
		SpecRef:    request.SpecRef,
		SpecRecord: request.SpecRecord,
	}); err != nil {
		return nil, err
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

	overlapTargetRefs, err := repo.overlapTargetRefs(*candidate)
	if err != nil {
		return nil, err
	}
	overlapSpecs, err := repo.loadSelectedSpecs(overlapTargetRefs)
	if err != nil {
		return nil, err
	}
	overlap := buildOverlapResult(candidate, overlapSpecs)

	var comparison *CompareResult
	if len(overlap.Overlaps) > 0 {
		primaryOverlapRef := overlap.Overlaps[0].Ref
		comparison = buildCompareResult(candidate, []string{candidate.Record.Ref, primaryOverlapRef}, selectSpecs(overlapSpecs, []string{primaryOverlapRef}))
	}

	impactSpecRefs, err := repo.impactedSpecRefs(candidate.Record)
	if err != nil {
		return nil, err
	}
	impactSpecs, err := repo.loadSelectedSpecs(impactSpecRefs)
	if err != nil {
		return nil, err
	}
	impactDocRefs, err := repo.impactedDocRefs(*candidate)
	if err != nil {
		return nil, err
	}
	impactDocs, err := repo.loadSelectedDocs(impactDocRefs)
	if err != nil {
		return nil, err
	}
	impact := buildAnalyzeImpactResult(candidate, "accepted", impactSpecs, impactDocs)

	docDrift := &DocDriftResult{
		Scope:      DocDriftScope{Mode: "doc_refs", DocRefs: []string{}},
		DriftItems: nil,
	}
	if impact != nil && len(impact.AffectedDocs) > 0 {
		docDriftSpecRefs, err := repo.relevantDocDriftSpecRefs(impactDocs)
		if err != nil {
			return nil, err
		}
		docDriftSpecs, err := repo.loadSelectedSpecs(docDriftSpecRefs)
		if err != nil {
			return nil, err
		}
		docDrift = buildDocDriftResult(DocDriftScope{Mode: "doc_refs", DocRefs: uniqueStrings(impactDocRefs)}, impactDocs, docDriftSpecs)
	}

	return &ReviewResult{
		SpecRef:    overlap.Candidate.Ref,
		Overlap:    overlap,
		Comparison: comparison,
		Impact:     impact,
		DocDrift:   docDrift,
	}, nil
}
