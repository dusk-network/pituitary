package ast

import "testing"

func TestScanSpecIdentifiers(t *testing.T) {
	t.Parallel()
	body := `## Overview
This spec governs the SlidingWindowLimiter and its NewSlidingWindowLimiter constructor.
The middleware lives in api/middleware and imports from rate_limiter_core.
Short names like x or id are too short to match.
`
	ids := ScanSpecIdentifiers(body)

	want := map[string]bool{
		"SlidingWindowLimiter":    true,
		"NewSlidingWindowLimiter": true,
		"rate_limiter_core":       true,
	}
	got := make(map[string]bool)
	for _, id := range ids {
		got[id] = true
	}

	for name := range want {
		if !got[name] {
			t.Errorf("expected identifier %q in scan results, got %v", name, ids)
		}
	}
	if got["x"] || got["id"] {
		t.Errorf("short identifiers should be excluded, got: %v", ids)
	}
}

func TestScanSpecIdentifiersFiltersCommonWords(t *testing.T) {
	t.Parallel()
	body := "This should replace existing configuration between systems."
	ids := ScanSpecIdentifiers(body)
	for _, id := range ids {
		if isCommonWord(id) {
			t.Errorf("common word %q should have been filtered", id)
		}
	}
}
