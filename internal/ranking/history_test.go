package ranking

import (
	"math"
	"testing"
)

func TestSearchPrefersHistoricalContext(t *testing.T) {
	t.Parallel()

	if !SearchPrefersHistoricalContext("show provenance for the legacy rollout") {
		t.Fatal("SearchPrefersHistoricalContext() = false, want true for explicit historical query")
	}
	if SearchPrefersHistoricalContext("tenant locality continuity kernel semantics") {
		t.Fatal("SearchPrefersHistoricalContext() = true, want false for default active query")
	}
}

func TestIsHistoricalSectionHeadingRecognizesCommonPatterns(t *testing.T) {
	t.Parallel()

	cases := []string{
		"Historical provenance",
		"Runtime Contract / Historical Context",
		"Guide / Change History",
		"Spec / Legacy Context",
	}
	for _, heading := range cases {
		if !IsHistoricalSectionHeading(heading) {
			t.Fatalf("IsHistoricalSectionHeading(%q) = false, want true", heading)
		}
	}
	if IsHistoricalSectionHeading("Requirements / Active Runtime Contract") {
		t.Fatal("IsHistoricalSectionHeading() = true, want false for active normative heading")
	}
}

func TestAdjustHistoricalSectionScoreDownranksByDefault(t *testing.T) {
	t.Parallel()

	got := AdjustHistoricalSectionScore(0.9, "Spec / Historical provenance", false)
	if got >= 0.9 {
		t.Fatalf("adjusted score = %.3f, want historical down-rank", got)
	}
	if want := 0.63; math.Abs(got-want) > 1e-9 {
		t.Fatalf("adjusted score = %.3f, want %.3f", got, want)
	}
	if got := AdjustHistoricalSectionScore(0.9, "Spec / Historical provenance", true); got != 0.9 {
		t.Fatalf("explicit historical query adjusted score = %.3f, want 0.900", got)
	}
	if got := AdjustHistoricalSectionScore(0.9, "Spec / Requirements", false); got != 0.9 {
		t.Fatalf("active section adjusted score = %.3f, want 0.900", got)
	}
}
