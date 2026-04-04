package analysis

import (
	"encoding/json"
	"testing"
)

func TestNormalizeAdjudicationsFiltersInvalidClassifications(t *testing.T) {
	t.Parallel()

	targets := []adjudicationTarget{
		{Path: "api/handler.go"},
		{Path: "api/middleware.go"},
	}
	adjudications := []complianceAdjudication{
		{Path: "api/handler.go", Classification: "conflict", Evidence: "missing retry header", Confidence: 0.85, Message: "Missing Retry-After header on 429"},
		{Path: "api/handler.go", Classification: "invalid", Evidence: "bad class", Confidence: 0.5, Message: "bogus"},
		{Path: "unknown/file.go", Classification: "conflict", Evidence: "wrong path", Confidence: 0.9, Message: "path not in targets"},
		{Path: "api/middleware.go", Classification: "compliant", Evidence: "ok", Confidence: 0.95, Message: "looks good"},
		{Path: "", Classification: "conflict", Evidence: "empty", Confidence: 0.5, Message: "empty path"},
	}

	result := normalizeAdjudications(adjudications, targets)

	if len(result) != 2 {
		t.Fatalf("got %d adjudications, want 2", len(result))
	}
	if result[0].Path != "api/handler.go" || result[0].Classification != "conflict" {
		t.Errorf("result[0] = %+v, want handler.go conflict", result[0])
	}
	if result[1].Path != "api/middleware.go" || result[1].Classification != "compliant" {
		t.Errorf("result[1] = %+v, want middleware.go compliant", result[1])
	}
}

func TestNormalizeAdjudicationsClampsConfidence(t *testing.T) {
	t.Parallel()

	targets := []adjudicationTarget{{Path: "a.go"}}
	adjudications := []complianceAdjudication{
		{Path: "a.go", Classification: "conflict", Evidence: "e", Confidence: 1.5, Message: "over"},
		{Path: "a.go", Classification: "unclear", Evidence: "e", Confidence: -0.3, Message: "under"},
	}

	// normalizeAdjudications deduplicates by path so both should appear
	// since they have different classifications... actually the function
	// doesn't deduplicate, just normalizes.
	result := normalizeAdjudications(adjudications, targets)
	if len(result) != 2 {
		t.Fatalf("got %d adjudications, want 2", len(result))
	}
	if result[0].Confidence != 1.0 {
		t.Errorf("confidence = %f, want 1.0", result[0].Confidence)
	}
	if result[1].Confidence != 0.0 {
		t.Errorf("confidence = %f, want 0.0", result[1].Confidence)
	}
}

func TestTruncateAdjudicationTargetsRespectsLimit(t *testing.T) {
	t.Parallel()

	targets := make([]adjudicationTarget, 10)
	for i := range targets {
		targets[i] = adjudicationTarget{
			Path:    "file.go",
			Content: "content",
		}
	}
	result := truncateAdjudicationTargets(targets)
	if len(result) > adjudicationMaxTargetsPerRequest {
		t.Errorf("got %d targets, want at most %d", len(result), adjudicationMaxTargetsPerRequest)
	}
}

func TestComplianceAdjudicatePromptMarshalJSON(t *testing.T) {
	t.Parallel()

	prompt := complianceAdjudicatePrompt{
		Command: "check-compliance-adjudicate",
		Spec: analysisSpecPrompt{
			Ref:   "rate-limiting",
			Title: "Rate Limiting",
		},
		Targets: []adjudicationTarget{
			{Path: "api/handler.go", Content: "func handle(){}"},
		},
	}
	data, err := json.Marshal(prompt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty JSON")
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["command"] != "check-compliance-adjudicate" {
		t.Errorf("command = %v, want check-compliance-adjudicate", parsed["command"])
	}
}
