package analysis

import (
	"testing"
)

func TestNormalizeMetadataInferenceCleansDomain(t *testing.T) {
	t.Parallel()

	result := &MetadataInferenceResult{
		Domain: &InferredStringField{Value: "  Rate Limiting  ", Confidence: 0.8},
	}
	normalizeMetadataInference(result)
	if result.Domain.Value != "rate limiting" {
		t.Errorf("domain = %q, want 'rate limiting'", result.Domain.Value)
	}
}

func TestNormalizeMetadataInferenceRemovesEmptyDomain(t *testing.T) {
	t.Parallel()

	result := &MetadataInferenceResult{
		Domain: &InferredStringField{Value: "  ", Confidence: 0.8},
	}
	normalizeMetadataInference(result)
	if result.Domain != nil {
		t.Errorf("expected nil domain for empty value")
	}
}

func TestNormalizeMetadataInferenceValidatesStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		status  string
		wantNil bool
	}{
		{"draft", false},
		{"review", false},
		{"accepted", false},
		{"superseded", false},
		{"deprecated", false},
		{"active", true},
		{"invalid", true},
		{"", true},
	}

	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			result := &MetadataInferenceResult{
				Status: &InferredStringField{Value: tc.status, Confidence: 0.7},
			}
			normalizeMetadataInference(result)
			if tc.wantNil && result.Status != nil {
				t.Errorf("expected nil status for %q", tc.status)
			}
			if !tc.wantNil && result.Status == nil {
				t.Errorf("expected non-nil status for %q", tc.status)
			}
		})
	}
}

func TestNormalizeMetadataInferenceClampsConfidence(t *testing.T) {
	t.Parallel()

	result := &MetadataInferenceResult{
		Domain: &InferredStringField{Value: "auth", Confidence: 1.5},
		Tags:   &InferredStringsField{Values: []string{"security"}, Confidence: -0.3},
	}
	normalizeMetadataInference(result)
	if result.Domain.Confidence != 1.0 {
		t.Errorf("domain confidence = %f, want 1.0", result.Domain.Confidence)
	}
	if result.Tags.Confidence != 0.0 {
		t.Errorf("tags confidence = %f, want 0.0", result.Tags.Confidence)
	}
}

func TestNormalizeMetadataInferenceRemovesEmptyTags(t *testing.T) {
	t.Parallel()

	result := &MetadataInferenceResult{
		Tags: &InferredStringsField{Values: []string{"", "  "}, Confidence: 0.6},
	}
	normalizeMetadataInference(result)
	if result.Tags != nil {
		t.Errorf("expected nil tags when all values are empty")
	}
}

func TestNormalizeInferredRelationsFiltersEmpty(t *testing.T) {
	t.Parallel()

	relations := []InferredRelationField{
		{Ref: "rate-limiting", Confidence: 0.8, Reason: "related"},
		{Ref: "", Confidence: 0.9, Reason: "empty ref"},
		{Ref: "  ", Confidence: 0.7, Reason: "whitespace ref"},
		{Ref: "auth-spec", Confidence: 1.5, Reason: "over confidence"},
	}
	result := normalizeInferredRelations(relations)
	if len(result) != 2 {
		t.Fatalf("got %d relations, want 2", len(result))
	}
	if result[0].Ref != "rate-limiting" {
		t.Errorf("result[0].Ref = %q, want rate-limiting", result[0].Ref)
	}
	if result[1].Confidence != 1.0 {
		t.Errorf("result[1].Confidence = %f, want 1.0 (clamped)", result[1].Confidence)
	}
}

func TestClampConfidence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input float64
		want  float64
	}{
		{0.5, 0.5},
		{0.0, 0.0},
		{1.0, 1.0},
		{1.5, 1.0},
		{-0.3, 0.0},
	}
	for _, tc := range cases {
		got := clampConfidence(tc.input)
		if got != tc.want {
			t.Errorf("clampConfidence(%f) = %f, want %f", tc.input, got, tc.want)
		}
	}
}
