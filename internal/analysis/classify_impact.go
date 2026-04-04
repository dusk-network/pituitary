package analysis

import (
	"context"
	"strings"
)

// impactSeverityClassifier is an optional capability for analysis providers
// that support classifying the severity of spec change impact.
type impactSeverityClassifier interface {
	ClassifyImpactSeverity(ctx context.Context, request impactSeverityRequest) (*impactSeverityResponse, error)
}

type impactSeverityRequest struct {
	Spec       analysisSpecPrompt   `json:"spec"`
	ChangeType string               `json:"change_type"`
	Items      []impactSeverityItem `json:"items"`
}

type impactSeverityItem struct {
	Ref          string `json:"ref"`
	Kind         string `json:"kind"`
	Title        string `json:"title"`
	Relationship string `json:"relationship,omitempty"`
	Excerpt      string `json:"excerpt,omitempty"`
}

type impactSeverityResponse struct {
	Classifications []impactSeverityClassification `json:"classifications"`
}

type impactSeverityClassification struct {
	Ref        string  `json:"ref"`
	Severity   string  `json:"severity"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

const (
	ImpactSeverityBreaking   = "breaking"
	ImpactSeverityBehavioral = "behavioral"
	ImpactSeverityCosmetic   = "cosmetic"
	ImpactSeverityUnknown    = "unknown"

	openAICompatibleImpactSeveritySystemPrompt = "You are Pituitary's impact severity classifier. Given a spec and its change type, classify the severity of impact on each affected item. Return only one JSON object with key classifications containing an array. Each classification must have: ref (the affected item ref), severity (breaking, behavioral, cosmetic, or unknown), confidence (0.0 to 1.0), and reason (brief justification). Breaking means the change invalidates assumptions or contracts in the affected item. Behavioral means the change alters behavior but doesn't break contracts. Cosmetic means the change only affects naming, formatting, or documentation without functional impact."
	impactSeverityMaxItems                     = 12
)

// ClassifyImpactSeverity implements impactSeverityClassifier for the
// OpenAI-compatible analysis provider.
func (p *openAICompatibleAnalysisProvider) ClassifyImpactSeverity(ctx context.Context, request impactSeverityRequest) (*impactSeverityResponse, error) {
	if len(request.Items) == 0 {
		return &impactSeverityResponse{}, nil
	}

	payload := impactSeverityPrompt{
		Command:    "analyze-impact-severity",
		Spec:       request.Spec,
		ChangeType: request.ChangeType,
		Items:      truncateImpactItems(request.Items),
	}

	var response impactSeverityResponse
	if err := p.completeJSON(ctx, openAICompatibleImpactSeveritySystemPrompt, payload, &response); err != nil {
		return nil, err
	}
	response.Classifications = normalizeImpactClassifications(response.Classifications, request.Items)
	return &response, nil
}

type impactSeverityPrompt struct {
	Command    string               `json:"command"`
	Spec       analysisSpecPrompt   `json:"spec"`
	ChangeType string               `json:"change_type"`
	Items      []impactSeverityItem `json:"items"`
}

func truncateImpactItems(items []impactSeverityItem) []impactSeverityItem {
	if len(items) <= impactSeverityMaxItems {
		return items
	}
	return items[:impactSeverityMaxItems]
}

func normalizeImpactClassifications(classifications []impactSeverityClassification, items []impactSeverityItem) []impactSeverityClassification {
	validRefs := make(map[string]struct{}, len(items))
	for _, item := range items {
		validRefs[item.Ref] = struct{}{}
	}

	result := make([]impactSeverityClassification, 0, len(classifications))
	for _, c := range classifications {
		ref := strings.TrimSpace(c.Ref)
		if ref == "" {
			continue
		}
		if _, ok := validRefs[ref]; !ok {
			continue
		}
		severity := strings.TrimSpace(c.Severity)
		switch severity {
		case ImpactSeverityBreaking, ImpactSeverityBehavioral, ImpactSeverityCosmetic:
		default:
			severity = ImpactSeverityUnknown
		}
		c.Ref = ref
		c.Severity = severity
		c.Reason = strings.TrimSpace(c.Reason)
		if c.Confidence < 0 {
			c.Confidence = 0
		}
		if c.Confidence > 1 {
			c.Confidence = 1
		}
		result = append(result, c)
	}
	return result
}

// classifyImpactSeverities enriches the impact result with model-backed
// severity classifications. Errors are silently ignored — severity is
// non-fatal enrichment.
func classifyImpactSeverities(ctx context.Context, classifier impactSeverityClassifier, candidate *specDocument, result *AnalyzeImpactResult) {
	specPrompt := analysisSpecFromDocument(*candidate)

	items := make([]impactSeverityItem, 0, len(result.AffectedSpecs)+len(result.AffectedDocs))
	for _, spec := range result.AffectedSpecs {
		items = append(items, impactSeverityItem{
			Ref:          spec.Ref,
			Kind:         "spec",
			Title:        spec.Title,
			Relationship: spec.Relationship,
		})
	}
	for _, doc := range result.AffectedDocs {
		excerpt := ""
		if doc.Evidence != nil {
			excerpt = doc.Evidence.DocExcerpt
		}
		items = append(items, impactSeverityItem{
			Ref:     doc.Ref,
			Kind:    "doc",
			Title:   doc.Title,
			Excerpt: excerpt,
		})
	}
	if len(items) == 0 {
		return
	}

	response, err := classifier.ClassifyImpactSeverity(ctx, impactSeverityRequest{
		Spec:       specPrompt,
		ChangeType: result.ChangeType,
		Items:      items,
	})
	if err != nil || response == nil {
		return
	}

	classificationByRef := make(map[string]impactSeverityClassification, len(response.Classifications))
	for _, c := range response.Classifications {
		classificationByRef[c.Ref] = c
	}

	for i := range result.AffectedSpecs {
		if c, ok := classificationByRef[result.AffectedSpecs[i].Ref]; ok {
			result.AffectedSpecs[i].Severity = c.Severity
			result.AffectedSpecs[i].SeverityConfidence = c.Confidence
			result.AffectedSpecs[i].SeverityReason = c.Reason
		}
	}
	for i := range result.AffectedDocs {
		if c, ok := classificationByRef[result.AffectedDocs[i].Ref]; ok {
			result.AffectedDocs[i].Severity = c.Severity
			result.AffectedDocs[i].SeverityConfidence = c.Confidence
			result.AffectedDocs[i].SeverityReason = c.Reason
		}
	}
}
