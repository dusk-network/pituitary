package analysis

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/resultmeta"
)

const (
	maxReviewOutlineContextRefs       = 8
	maxReviewOutlineOverlapRefs       = 2
	maxReviewOutlineContextQueryBytes = 12000
)

// ReviewRequest is the normalized review-spec input.
type ReviewRequest struct {
	SpecRef             string            `json:"spec_ref,omitempty"`
	SpecRecord          *model.SpecRecord `json:"spec_record,omitempty"`
	OutlineContext      bool              `json:"outline_context,omitempty"`
	OutlineContextLimit int               `json:"outline_context_limit,omitempty"`
}

// ReviewResult is the composed review-spec response.
type ReviewResult struct {
	SpecRef        string                      `json:"spec_ref"`
	SpecInference  *model.InferenceConfidence  `json:"spec_inference,omitempty"`
	Overlap        *OverlapResult              `json:"overlap"`
	Comparison     *CompareResult              `json:"comparison"`
	Impact         *AnalyzeImpactResult        `json:"impact"`
	DocDrift       *DocDriftResult             `json:"doc_drift"`
	DocRemediation *DocRemediationResult       `json:"doc_remediation"`
	OutlineContext *index.OutlineContextResult `json:"outline_context,omitempty"`
	Warnings       []Warning                   `json:"warnings,omitempty"`
	ContentTrust   *resultmeta.ContentTrust    `json:"content_trust,omitempty"`
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
	overlapRequest := OverlapRequest{
		SpecRef:    request.SpecRef,
		SpecRecord: request.SpecRecord,
	}
	if err := validateOverlapRequest(overlapRequest); err != nil {
		return nil, err
	}

	repo, err := openAnalysisRepositoryContext(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer repo.Close()

	analyzer, err := newQualitativeAnalyzer(cfg.Runtime.Analysis)
	if err != nil {
		return nil, err
	}
	analyzer = qualitativeAnalyzerWithTimings(ctx, analyzer)

	candidate, overlap, overlapSpecs, err := buildReviewOverlap(repo, overlapRequest)
	if err != nil {
		return nil, err
	}

	comparison, err := buildReviewComparison(ctx, analyzer, candidate, overlap, overlapSpecs)
	if err != nil {
		return nil, err
	}

	impact, impactDocs, impactDocRefs, err := buildReviewImpact(repo, candidate)
	if err != nil {
		return nil, err
	}

	docDrift, err := buildReviewDocDrift(ctx, cfg, repo, analyzer, impact, impactDocs, impactDocRefs)
	if err != nil {
		return nil, err
	}

	docRemediation := &DocRemediationResult{Items: nil}
	if docDrift != nil && docDrift.Remediation != nil {
		docRemediation = docDrift.Remediation
	}

	var outlineContext *index.OutlineContextResult
	var outlineWarnings []Warning
	if request.OutlineContext {
		outlineContext, outlineWarnings = buildReviewOutlineContext(ctx, cfg, repo, request.OutlineContextLimit, *candidate, overlap, impact)
	}

	warnings := append(reviewWarnings(*candidate, impact, docDrift), outlineWarnings...)
	return &ReviewResult{
		SpecRef:        overlap.Candidate.Ref,
		SpecInference:  candidate.Record.Inference,
		Overlap:        overlap,
		Comparison:     comparison,
		Impact:         impact,
		DocDrift:       docDrift,
		DocRemediation: docRemediation,
		OutlineContext: outlineContext,
		Warnings:       uniqueWarnings(warnings),
		ContentTrust:   resultmeta.UntrustedWorkspaceText(),
	}, nil
}

func buildReviewOverlap(repo *analysisRepository, request OverlapRequest) (*specDocument, *OverlapResult, map[string]specDocument, error) {
	candidate, err := loadCandidate(repo, request, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	overlapTargetRefs, err := repo.overlapTargetRefs(*candidate)
	if err != nil {
		return nil, nil, nil, err
	}
	overlapSpecs, err := repo.loadSelectedSpecs(overlapTargetRefs)
	if err != nil {
		return nil, nil, nil, err
	}
	return candidate, buildOverlapResult(candidate, overlapSpecs), overlapSpecs, nil
}

func buildReviewComparison(ctx context.Context, analyzer qualitativeAnalyzer, candidate *specDocument, overlap *OverlapResult, overlapSpecs map[string]specDocument) (*CompareResult, error) {
	if overlap == nil || len(overlap.Overlaps) == 0 {
		return nil, nil
	}
	primaryOverlapRef := overlap.Overlaps[0].Ref
	comparison, err := buildCompareResult(ctx, analyzer, candidate, []string{candidate.Record.Ref, primaryOverlapRef}, selectSpecs(overlapSpecs, []string{primaryOverlapRef}))
	if err != nil {
		return nil, err
	}
	return comparison, nil
}

func buildReviewImpact(repo *analysisRepository, candidate *specDocument) (*AnalyzeImpactResult, map[string]docDocument, []string, error) {
	impactSpecRefs, err := repo.impactedSpecRefs(candidate.Record)
	if err != nil {
		return nil, nil, nil, err
	}
	impactSpecs, err := repo.loadSelectedSpecs(impactSpecRefs)
	if err != nil {
		return nil, nil, nil, err
	}
	impactDocRefs, err := repo.impactedDocRefs(*candidate)
	if err != nil {
		return nil, nil, nil, err
	}
	impactDocs, err := repo.loadSelectedDocs(impactDocRefs)
	if err != nil {
		return nil, nil, nil, err
	}
	return buildAnalyzeImpactResult(candidate, "accepted", false, impactSpecs, impactDocs), impactDocs, impactDocRefs, nil
}

func buildReviewDocDrift(ctx context.Context, cfg *config.Config, repo *analysisRepository, analyzer qualitativeAnalyzer, impact *AnalyzeImpactResult, impactDocs map[string]docDocument, impactDocRefs []string) (*DocDriftResult, error) {
	if impact == nil || len(impact.AffectedDocs) == 0 {
		return emptyReviewDocDrift(), nil
	}
	docDriftSpecRefs, err := repo.relevantDocDriftSpecRefs(impactDocs)
	if err != nil {
		return nil, err
	}
	docDriftSpecs, err := repo.loadSelectedSpecs(docDriftSpecRefs)
	if err != nil {
		return nil, err
	}
	return buildDocDriftResult(ctx, analyzer, newAnalysisRuntimeUsage(cfg.Runtime.Analysis), DocDriftScope{Mode: "doc_refs", DocRefs: uniqueStrings(impactDocRefs)}, impactDocs, docDriftSpecs, repo.loadAllSpecs)
}

func emptyReviewDocDrift() *DocDriftResult {
	return &DocDriftResult{
		Scope:      DocDriftScope{Mode: "doc_refs", DocRefs: []string{}},
		DriftItems: nil,
		Remediation: &DocRemediationResult{
			Items: nil,
		},
	}
}

func buildReviewOutlineContext(ctx context.Context, cfg *config.Config, repo *analysisRepository, limit int, candidate specDocument, overlap *OverlapResult, impact *AnalyzeImpactResult) (*index.OutlineContextResult, []Warning) {
	embedder, err := index.NewEmbedder(cfg.Runtime.Embedder)
	if err != nil {
		return nil, []Warning{outlineContextWarning(candidate.Record.Ref, err)}
	}
	result, err := index.RetrieveOutlineContextWithSnapshotContext(ctx, cfg, repo.snapshot, embedder, index.OutlineContextQuery{
		Query:          reviewOutlineContextQuery(candidate),
		Refs:           reviewOutlineContextRefs(candidate, overlap, impact),
		Limit:          limit,
		IncludeParent:  true,
		NeighborWindow: 1,
	})
	if err != nil {
		return nil, []Warning{outlineContextWarning(candidate.Record.Ref, err)}
	}
	return result, nil
}

func reviewOutlineContextQuery(candidate specDocument) string {
	query := strings.TrimSpace(candidate.Record.Title + "\n\n" + strings.TrimSpace(candidate.Record.BodyText))
	return truncateReviewOutlineQuery(query, maxReviewOutlineContextQueryBytes)
}

func truncateReviewOutlineQuery(content string, maxBytes int) string {
	if len(content) <= maxBytes {
		return content
	}
	for index, r := range content {
		next := index + utf8.RuneLen(r)
		if next > maxBytes {
			if index == 0 {
				return content[:next]
			}
			return strings.TrimSpace(content[:index])
		}
	}
	return content
}

func outlineContextWarning(ref string, err error) Warning {
	return Warning{
		Code:    "outline_context_unavailable",
		Message: fmt.Sprintf("outline context unavailable: %v", err),
		Ref:     ref,
	}
}

func reviewOutlineContextRefs(candidate specDocument, overlap *OverlapResult, impact *AnalyzeImpactResult) []string {
	refs := []string{candidate.Record.Ref}
	if overlap != nil {
		addedOverlapRefs := 0
		for _, item := range overlap.Overlaps {
			refs = append(refs, item.Ref)
			addedOverlapRefs++
			if addedOverlapRefs >= maxReviewOutlineOverlapRefs {
				break
			}
		}
	}
	if impact != nil {
		for _, doc := range impact.AffectedDocs {
			if len(refs) >= maxReviewOutlineContextRefs {
				break
			}
			refs = append(refs, doc.Ref)
		}
	}
	return uniqueStrings(refs)
}

func reviewWarnings(candidate specDocument, impact *AnalyzeImpactResult, docDrift *DocDriftResult) []Warning {
	warnings := buildSpecInferenceWarnings("review", candidate)
	if impact != nil {
		warnings = append(warnings, impact.Warnings...)
	}
	if docDrift != nil {
		warnings = append(warnings, docDrift.Warnings...)
	}
	return uniqueWarnings(warnings)
}

func uniqueWarnings(warnings []Warning) []Warning {
	seen := make(map[string]struct{}, len(warnings))
	result := make([]Warning, 0, len(warnings))
	for _, warning := range warnings {
		key := warning.Ref + "\x00" + warning.Code + "\x00" + warning.Message
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, warning)
	}
	return result
}
