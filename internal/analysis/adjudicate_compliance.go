package analysis

import (
	"context"
	"fmt"
	"strings"
)

// complianceAdjudicator is an optional capability for analysis providers that
// support semantic code-compliance adjudication. Deterministic evaluation runs
// first; the adjudicator refines the result with model-backed reasoning when
// runtime.analysis is configured.
type complianceAdjudicator interface {
	AdjudicateCompliance(ctx context.Context, request complianceAdjudicationRequest) (*complianceAdjudicationResponse, error)
}

// complianceAdjudicationRequest groups the narrowed evidence sent to the
// analysis model for bounded adjudication.
type complianceAdjudicationRequest struct {
	Spec    analysisSpecPrompt   `json:"spec"`
	Targets []adjudicationTarget `json:"targets"`
}

// adjudicationTarget is one code path or diff section to adjudicate against
// the governing spec.
type adjudicationTarget struct {
	Path     string                  `json:"path"`
	Content  string                  `json:"content"`
	Sections []analysisSectionPrompt `json:"sections,omitempty"`
}

// complianceAdjudicationResponse is the structured response from the model.
type complianceAdjudicationResponse struct {
	Adjudications []complianceAdjudication `json:"adjudications"`
}

// complianceAdjudication is one model-produced compliance judgment.
type complianceAdjudication struct {
	Path            string  `json:"path"`
	Classification  string  `json:"classification"`
	ViolatedSection string  `json:"violated_section,omitempty"`
	Evidence        string  `json:"evidence"`
	Confidence      float64 `json:"confidence"`
	Message         string  `json:"message"`
	Expected        string  `json:"expected,omitempty"`
	Observed        string  `json:"observed,omitempty"`
}

const (
	openAICompatibleComplianceAdjudicateSystemPrompt = "You are Pituitary's compliance adjudicator. Given a specification and code targets, determine whether each target complies with the specification. Return only one JSON object with key adjudications containing an array. Each adjudication must have: path (the target path), classification (compliant, conflict, or unclear), evidence (brief factual basis), confidence (0.0 to 1.0), and message (concise human-readable summary). Optionally include violated_section, expected, and observed when a conflict is found. Focus on semantic compliance: look for violations that literal string matching would miss, such as missing required patterns, incorrect implementations, or behavioral divergence from the spec. Do not invent violations — only report what the evidence supports."
	adjudicationTargetContentLimit                   = 1200
	adjudicationMaxTargetsPerRequest                 = 6
)

// AdjudicateCompliance implements complianceAdjudicator for the OpenAI-compatible
// analysis provider.
func (p *openAICompatibleAnalysisProvider) AdjudicateCompliance(ctx context.Context, request complianceAdjudicationRequest) (*complianceAdjudicationResponse, error) {
	if len(request.Targets) == 0 {
		return &complianceAdjudicationResponse{}, nil
	}

	payload := complianceAdjudicatePrompt{
		Command: "check-compliance-adjudicate",
		Spec:    request.Spec,
		Targets: truncateAdjudicationTargets(request.Targets),
	}

	var response complianceAdjudicationResponse
	if err := p.completeJSON(ctx, openAICompatibleComplianceAdjudicateSystemPrompt, payload, &response); err != nil {
		return nil, err
	}
	response.Adjudications = normalizeAdjudications(response.Adjudications, request.Targets)
	return &response, nil
}

type complianceAdjudicatePrompt struct {
	Command string               `json:"command"`
	Spec    analysisSpecPrompt   `json:"spec"`
	Targets []adjudicationTarget `json:"targets"`
}

func truncateAdjudicationTargets(targets []adjudicationTarget) []adjudicationTarget {
	result := make([]adjudicationTarget, 0, minInt(len(targets), adjudicationMaxTargetsPerRequest))
	for _, target := range targets {
		if len(result) >= adjudicationMaxTargetsPerRequest {
			break
		}
		truncated := target
		truncated.Content = truncateForAnalysisPrompt(target.Content, adjudicationTargetContentLimit)
		if len(truncated.Sections) > analysisPromptSectionLimit {
			truncated.Sections = truncated.Sections[:analysisPromptSectionLimit]
		}
		for i := range truncated.Sections {
			truncated.Sections[i].Content = truncateForAnalysisPrompt(truncated.Sections[i].Content, analysisPromptSectionContentLimit)
		}
		result = append(result, truncated)
	}
	return result
}

func normalizeAdjudications(adjudications []complianceAdjudication, targets []adjudicationTarget) []complianceAdjudication {
	targetPaths := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		targetPaths[target.Path] = struct{}{}
	}

	result := make([]complianceAdjudication, 0, len(adjudications))
	for _, adj := range adjudications {
		path := strings.TrimSpace(adj.Path)
		if path == "" {
			continue
		}
		if _, ok := targetPaths[path]; !ok {
			continue
		}
		classification := strings.TrimSpace(adj.Classification)
		switch classification {
		case "compliant", "conflict", "unclear":
		default:
			continue
		}
		adj.Path = path
		adj.Classification = classification
		adj.Evidence = strings.TrimSpace(adj.Evidence)
		adj.Message = strings.TrimSpace(adj.Message)
		adj.ViolatedSection = strings.TrimSpace(adj.ViolatedSection)
		adj.Expected = strings.TrimSpace(adj.Expected)
		adj.Observed = strings.TrimSpace(adj.Observed)
		if adj.Confidence < 0 {
			adj.Confidence = 0
		}
		if adj.Confidence > 1 {
			adj.Confidence = 1
		}
		if adj.Message == "" {
			adj.Message = fmt.Sprintf("Model adjudication: %s", classification)
		}
		result = append(result, adj)
	}
	return result
}
