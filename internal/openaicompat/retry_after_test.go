package openaicompat

import (
	"strconv"
	"testing"
	"time"
)

func TestRetryAfterDurationCapsLargeValues(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		in   string
	}{
		{"billion_seconds", "999999999"},
		{"one_year", strconv.Itoa(60 * 60 * 24 * 365)},
		{"just_over_cap", strconv.Itoa(int(maxRetryAfter/time.Second) + 10)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := retryAfterDuration(tc.in)
			if got > maxRetryAfter {
				t.Fatalf("retryAfterDuration(%q) = %v, exceeds cap %v", tc.in, got, maxRetryAfter)
			}
			if got <= 0 {
				t.Fatalf("retryAfterDuration(%q) = %v, want positive delay", tc.in, got)
			}
		})
	}
}

func TestRetryAfterDurationPassesSmallValues(t *testing.T) {
	t.Parallel()

	got := retryAfterDuration("5")
	if got != 5*time.Second {
		t.Fatalf("retryAfterDuration(\"5\") = %v, want 5s", got)
	}
}

func TestRetryAfterDurationEmpty(t *testing.T) {
	t.Parallel()

	if got := retryAfterDuration(""); got != 0 {
		t.Fatalf("retryAfterDuration(\"\") = %v, want 0", got)
	}
}

func TestRetryAfterDurationHTTPDate(t *testing.T) {
	t.Parallel()

	// A date far in the future should clamp to the cap rather than stall for years.
	far := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC1123)
	got := retryAfterDuration(far)
	if got > maxRetryAfter {
		t.Fatalf("retryAfterDuration(far-future HTTP date) = %v, exceeds cap %v", got, maxRetryAfter)
	}
}
