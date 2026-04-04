package analysis

import (
	"encoding/json"
	"testing"
)

func TestNormalizeImpactClassificationsFiltersInvalidRefs(t *testing.T) {
	t.Parallel()

	items := []impactSeverityItem{
		{Ref: "SPEC-001", Kind: "spec", Title: "Auth"},
		{Ref: "doc://guides/auth", Kind: "doc", Title: "Auth Guide"},
	}
	classifications := []impactSeverityClassification{
		{Ref: "SPEC-001", Severity: "breaking", Confidence: 0.9, Reason: "invalidates auth contract"},
		{Ref: "SPEC-999", Severity: "behavioral", Confidence: 0.8, Reason: "unknown ref"},
		{Ref: "", Severity: "cosmetic", Confidence: 0.5, Reason: "empty ref"},
		{Ref: "doc://guides/auth", Severity: "cosmetic", Confidence: 0.7, Reason: "naming only"},
	}

	result := normalizeImpactClassifications(classifications, items)

	if len(result) != 2 {
		t.Fatalf("got %d classifications, want 2", len(result))
	}
	if result[0].Ref != "SPEC-001" || result[0].Severity != "breaking" {
		t.Errorf("result[0] = %+v, want SPEC-001 breaking", result[0])
	}
	if result[1].Ref != "doc://guides/auth" || result[1].Severity != "cosmetic" {
		t.Errorf("result[1] = %+v, want doc://guides/auth cosmetic", result[1])
	}
}

func TestNormalizeImpactClassificationsNormalizesUnknownSeverity(t *testing.T) {
	t.Parallel()

	items := []impactSeverityItem{
		{Ref: "SPEC-001", Kind: "spec", Title: "Auth"},
	}
	classifications := []impactSeverityClassification{
		{Ref: "SPEC-001", Severity: "critical", Confidence: 0.8, Reason: "invalid severity value"},
	}

	result := normalizeImpactClassifications(classifications, items)

	if len(result) != 1 {
		t.Fatalf("got %d classifications, want 1", len(result))
	}
	if result[0].Severity != ImpactSeverityUnknown {
		t.Errorf("severity = %q, want %q", result[0].Severity, ImpactSeverityUnknown)
	}
}

func TestNormalizeImpactClassificationsClampsConfidence(t *testing.T) {
	t.Parallel()

	items := []impactSeverityItem{
		{Ref: "SPEC-001", Kind: "spec"},
		{Ref: "SPEC-002", Kind: "spec"},
	}
	classifications := []impactSeverityClassification{
		{Ref: "SPEC-001", Severity: "breaking", Confidence: 1.5, Reason: "over"},
		{Ref: "SPEC-002", Severity: "behavioral", Confidence: -0.3, Reason: "under"},
	}

	result := normalizeImpactClassifications(classifications, items)

	if len(result) != 2 {
		t.Fatalf("got %d classifications, want 2", len(result))
	}
	if result[0].Confidence != 1.0 {
		t.Errorf("confidence = %f, want 1.0", result[0].Confidence)
	}
	if result[1].Confidence != 0.0 {
		t.Errorf("confidence = %f, want 0.0", result[1].Confidence)
	}
}

func TestTruncateImpactItemsRespectsLimit(t *testing.T) {
	t.Parallel()

	items := make([]impactSeverityItem, 20)
	for i := range items {
		items[i] = impactSeverityItem{Ref: "SPEC-" + string(rune('A'+i)), Kind: "spec"}
	}

	result := truncateImpactItems(items)
	if len(result) != impactSeverityMaxItems {
		t.Errorf("got %d items, want %d", len(result), impactSeverityMaxItems)
	}
}

func TestTruncateImpactItemsPassthroughWhenUnderLimit(t *testing.T) {
	t.Parallel()

	items := []impactSeverityItem{
		{Ref: "SPEC-001", Kind: "spec"},
		{Ref: "SPEC-002", Kind: "spec"},
	}

	result := truncateImpactItems(items)
	if len(result) != 2 {
		t.Errorf("got %d items, want 2", len(result))
	}
}

func TestImpactSeverityPromptMarshalJSON(t *testing.T) {
	t.Parallel()

	prompt := impactSeverityPrompt{
		Command: "analyze-impact-severity",
		Spec: analysisSpecPrompt{
			Ref:   "SPEC-042",
			Title: "Rate Limiting",
		},
		ChangeType: "accepted",
		Items: []impactSeverityItem{
			{Ref: "SPEC-055", Kind: "spec", Title: "Throttle Policy", Relationship: "depends_on"},
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
	if parsed["command"] != "analyze-impact-severity" {
		t.Errorf("command = %v, want analyze-impact-severity", parsed["command"])
	}
}

func TestBuildImpactSummarySeverityOverridesPriority(t *testing.T) {
	t.Parallel()

	specs := []ImpactedSpec{
		{Ref: "SPEC-A", Title: "Alpha", Relationship: "depends_on", Severity: ImpactSeverityCosmetic},
		{Ref: "SPEC-B", Title: "Beta", Relationship: "depends_on", Severity: ImpactSeverityBreaking},
	}
	docs := []ImpactedDoc{
		{Ref: "doc://x", Title: "Doc X", SourceRef: "x.md", Score: 0.9, Classification: impactDocClassificationSemanticNeighbor, Severity: ImpactSeverityBehavioral},
	}

	result := buildImpactSummary(specs, docs, 5)
	if len(result) != 3 {
		t.Fatalf("got %d summary items, want 3", len(result))
	}
	// Breaking should come first.
	if result[0].Ref != "SPEC-B" {
		t.Errorf("rank 1 ref = %q, want SPEC-B (breaking)", result[0].Ref)
	}
	if result[0].Severity != ImpactSeverityBreaking {
		t.Errorf("rank 1 severity = %q, want breaking", result[0].Severity)
	}
	// Behavioral second.
	if result[1].Ref != "doc://x" {
		t.Errorf("rank 2 ref = %q, want doc://x (behavioral)", result[1].Ref)
	}
	// Cosmetic last.
	if result[2].Ref != "SPEC-A" {
		t.Errorf("rank 3 ref = %q, want SPEC-A (cosmetic)", result[2].Ref)
	}
}
