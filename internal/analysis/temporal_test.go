package analysis

import (
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestAnalysisCommandsNormalizeTimestampAtDateAndRejectInvalidDates(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	timestamp := "2030-01-02T03:04:05Z"
	if _, err := AnalyzeImpact(cfg, AnalyzeImpactRequest{
		SpecRef:    "SPEC-042",
		ChangeType: "accepted",
		AtDate:     timestamp,
	}); err != nil {
		t.Fatalf("AnalyzeImpact(timestamp at_date) error = %v", err)
	}
	if _, err := CheckDocDrift(cfg, DocDriftRequest{
		Scope:  "all",
		AtDate: timestamp,
	}); err != nil {
		t.Fatalf("CheckDocDrift(timestamp at_date) error = %v", err)
	}
	if _, err := CheckCompliance(cfg, ComplianceRequest{
		DiffText: complianceTemporalDiff(),
		AtDate:   timestamp,
	}); err != nil {
		t.Fatalf("CheckCompliance(timestamp at_date) error = %v", err)
	}

	invalid := "2026-02-30"
	if _, err := AnalyzeImpact(cfg, AnalyzeImpactRequest{SpecRef: "SPEC-042", ChangeType: "accepted", AtDate: invalid}); err == nil || !strings.Contains(err.Error(), "invalid at_date") {
		t.Fatalf("AnalyzeImpact invalid at_date error = %v, want invalid at_date", err)
	}
	if _, err := CheckDocDrift(cfg, DocDriftRequest{Scope: "all", AtDate: invalid}); err == nil || !strings.Contains(err.Error(), "invalid at_date") {
		t.Fatalf("CheckDocDrift invalid at_date error = %v, want invalid at_date", err)
	}
	if _, err := CheckCompliance(cfg, ComplianceRequest{DiffText: complianceTemporalDiff(), AtDate: invalid}); err == nil || !strings.Contains(err.Error(), "invalid at_date") {
		t.Fatalf("CheckCompliance invalid at_date error = %v, want invalid at_date", err)
	}
}

func complianceTemporalDiff() string {
	return strings.TrimSpace(`
diff --git a/src/api/middleware/ratelimiter.go b/src/api/middleware/ratelimiter.go
index 0000000..1111111 100644
--- a/src/api/middleware/ratelimiter.go
+++ b/src/api/middleware/ratelimiter.go
@@ -0,0 +1,6 @@
+package middleware
+
+// Apply limits per tenant rather than per API key.
+// Enforce a default limit of 200 requests per minute.
+// Use a sliding-window limiter.
+func buildLimiter() {}
`)
}
