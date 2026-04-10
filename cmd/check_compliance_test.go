package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCheckCompliancePathJSONCompliant(t *testing.T) {
	repo := writeSearchWorkspace(t)
	writeComplianceSourceFile(t, repo, "src/api/middleware/ratelimiter.go", `
package middleware

// Apply limits per tenant rather than per API key.
// Enforce a default limit of 200 requests per minute.
// Allow short bursts above the steady-state tenant limit.
// Use a sliding-window limiter and tenant-specific overrides.
func buildLimiter() {}
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		rebuildSearchWorkspaceIndex(t)
		return runCheckCompliance([]string{
			"--path", "src/api/middleware/ratelimiter.go",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckCompliance() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckCompliance() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			Paths []string `json:"paths"`
		} `json:"request"`
		Result struct {
			Paths         []string `json:"paths"`
			RelevantSpecs []struct {
				SpecRef string   `json:"spec_ref"`
				Basis   []string `json:"basis"`
			} `json:"relevant_specs"`
			Compliant []struct {
				Path           string `json:"path"`
				SpecRef        string `json:"spec_ref"`
				SectionHeading string `json:"section_heading"`
			} `json:"compliant"`
			Conflicts   []any `json:"conflicts"`
			Unspecified []any `json:"unspecified"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal compliance payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("payload errors = %+v, want none", payload.Errors)
	}
	if len(payload.Request.Paths) != 1 || payload.Request.Paths[0] != "src/api/middleware/ratelimiter.go" {
		t.Fatalf("request.paths = %v, want ratelimiter path", payload.Request.Paths)
	}
	if len(payload.Result.Paths) != 1 || payload.Result.Paths[0] != "src/api/middleware/ratelimiter.go" {
		t.Fatalf("result.paths = %v, want normalized ratelimiter path", payload.Result.Paths)
	}
	if len(payload.Result.RelevantSpecs) != 2 {
		t.Fatalf("relevant_specs = %+v, want two explicit governing specs", payload.Result.RelevantSpecs)
	}
	if len(payload.Result.Compliant) == 0 {
		t.Fatal("result.compliant is empty, want compliant findings")
	}
	if len(payload.Result.Conflicts) != 0 {
		t.Fatalf("result.conflicts = %+v, want none", payload.Result.Conflicts)
	}
	if len(payload.Result.Unspecified) != 0 {
		t.Fatalf("result.unspecified = %+v, want none", payload.Result.Unspecified)
	}
	if payload.Result.Compliant[0].SpecRef == "" || payload.Result.Compliant[0].SectionHeading == "" {
		t.Fatalf("top compliant finding = %+v, want spec ref and section heading", payload.Result.Compliant[0])
	}
}

func TestRunCheckCompliancePathJSONConflict(t *testing.T) {
	repo := writeSearchWorkspace(t)
	writeComplianceSourceFile(t, repo, "src/api/middleware/ratelimiter.go", `
package middleware

// Apply limits per API key.
// Enforce a default limit of 100 requests per minute.
// Use a fixed-window limiter.
func buildLimiter() {}
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		rebuildSearchWorkspaceIndex(t)
		return runCheckCompliance([]string{
			"--path", "src/api/middleware/ratelimiter.go",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckCompliance() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckCompliance() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Conflicts []struct {
				Path           string `json:"path"`
				SpecRef        string `json:"spec_ref"`
				SectionHeading string `json:"section_heading"`
				Code           string `json:"code"`
				Expected       string `json:"expected"`
				Observed       string `json:"observed"`
			} `json:"conflicts"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal conflict payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("payload errors = %+v, want none", payload.Errors)
	}
	if len(payload.Result.Conflicts) == 0 {
		t.Fatal("result.conflicts is empty, want at least one conflict")
	}
	if payload.Result.Conflicts[0].Path != "src/api/middleware/ratelimiter.go" {
		t.Fatalf("top conflict = %+v, want ratelimiter path", payload.Result.Conflicts[0])
	}
	if payload.Result.Conflicts[0].SpecRef != "SPEC-042" {
		t.Fatalf("top conflict = %+v, want SPEC-042", payload.Result.Conflicts[0])
	}
	if payload.Result.Conflicts[0].SectionHeading == "" {
		t.Fatalf("top conflict = %+v, want section heading", payload.Result.Conflicts[0])
	}
	if payload.Result.Conflicts[0].Code == "" {
		t.Fatalf("top conflict = %+v, want stable code", payload.Result.Conflicts[0])
	}
}

func TestRunCheckCompliancePathJSONWeakTraceability(t *testing.T) {
	repo := writeSearchWorkspace(t)
	writeComplianceSourceFile(t, repo, "notes/ungoverned.txt", `
zxqv aurora lattice
plinth ember quartz
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		rebuildSearchWorkspaceIndex(t)
		return runCheckCompliance([]string{
			"--path", "notes/ungoverned.txt",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckCompliance() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckCompliance() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Compliant   []any `json:"compliant"`
			Conflicts   []any `json:"conflicts"`
			Unspecified []struct {
				Path           string `json:"path"`
				Code           string `json:"code"`
				Message        string `json:"message"`
				Traceability   string `json:"traceability"`
				LimitingFactor string `json:"limiting_factor"`
				Suggestion     string `json:"suggestion"`
			} `json:"unspecified"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal no-spec payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("payload errors = %+v, want none", payload.Errors)
	}
	if len(payload.Result.Compliant) != 0 || len(payload.Result.Conflicts) != 0 {
		t.Fatalf("result = %+v, want only unspecified findings", payload.Result)
	}
	if len(payload.Result.Unspecified) != 1 {
		t.Fatalf("result.unspecified = %+v, want one no-spec finding", payload.Result.Unspecified)
	}
	if payload.Result.Unspecified[0].Code != "weak_traceability" {
		t.Fatalf("unspecified finding = %+v, want weak_traceability", payload.Result.Unspecified[0])
	}
	if payload.Result.Unspecified[0].Path != "notes/ungoverned.txt" {
		t.Fatalf("unspecified finding = %+v, want ungoverned path", payload.Result.Unspecified[0])
	}
	if payload.Result.Unspecified[0].Traceability != "weak_semantic_retrieval" {
		t.Fatalf("unspecified finding = %+v, want weak_semantic_retrieval traceability", payload.Result.Unspecified[0])
	}
	if payload.Result.Unspecified[0].LimitingFactor != "spec_metadata_gap" {
		t.Fatalf("unspecified finding = %+v, want spec_metadata_gap limiting factor", payload.Result.Unspecified[0])
	}
	if !strings.Contains(payload.Result.Unspecified[0].Suggestion, `applies_to = ["code://notes/ungoverned.txt"]`) {
		t.Fatalf("unspecified finding = %+v, want applies_to suggestion", payload.Result.Unspecified[0])
	}
}

func TestRunCheckComplianceWithRequestFileJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)
	writeComplianceSourceFile(t, repo, "src/api/middleware/ratelimiter.go", `
package middleware

// Apply limits per tenant rather than per API key.
// Enforce a default limit of 200 requests per minute.
// Allow short bursts above the steady-state tenant limit.
// Use a sliding-window limiter and tenant-specific overrides.
func buildLimiter() {}
`)
	mustWriteJSONFileCmd(t, filepath.Join(repo, "compliance-request.json"), map[string]any{
		"paths": []string{"src/api/middleware/ratelimiter.go"},
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		rebuildSearchWorkspaceIndex(t)
		return runCheckCompliance([]string{"--request-file", "compliance-request.json", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckCompliance() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckCompliance() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			Paths []string `json:"paths"`
		} `json:"request"`
		Result struct {
			Compliant []struct {
				Path string `json:"path"`
			} `json:"compliant"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal compliance request-file payload: %v", err)
	}
	if len(payload.Request.Paths) != 1 || payload.Request.Paths[0] != "src/api/middleware/ratelimiter.go" {
		t.Fatalf("request.paths = %v, want ratelimiter path", payload.Request.Paths)
	}
	if len(payload.Result.Compliant) == 0 {
		t.Fatal("result.compliant is empty, want compliant findings")
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunCheckComplianceRejectsDiffFileOutsideWorkspace(t *testing.T) {
	repo := writeSearchWorkspace(t)
	outside := filepath.Join(t.TempDir(), "change.diff")
	mustWriteFileCmd(t, outside, "diff --git a/a b/a\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runCheckCompliance([]string{"--diff-file", outside, "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runCheckCompliance() exit code = %d, want 2", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckCompliance() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal compliance outside-workspace payload: %v", err)
	}
	if len(payload.Errors) != 1 || payload.Errors[0].Code != "validation_error" {
		t.Fatalf("errors = %+v, want one validation_error", payload.Errors)
	}
	if got := payload.Errors[0].Message; !strings.Contains(got, "outside workspace root") {
		t.Fatalf("error message = %q, want workspace-root validation", got)
	}
}

func TestRunCheckCompliancePathJSONTraceabilityGap(t *testing.T) {
	repo := writeSearchWorkspace(t)
	writeComplianceSourceFile(t, repo, "src/api/middleware/tenant_limiter.go", `
package middleware

// Apply limits per tenant rather than per API key.
// Enforce a default limit of 200 requests per minute.
// Allow short bursts above the steady-state tenant limit.
func buildTenantLimiter() {}
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		rebuildSearchWorkspaceIndex(t)
		return runCheckCompliance([]string{
			"--path", "src/api/middleware/tenant_limiter.go",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckCompliance() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckCompliance() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Unspecified []struct {
				Path           string `json:"path"`
				SpecRef        string `json:"spec_ref"`
				Code           string `json:"code"`
				Traceability   string `json:"traceability"`
				LimitingFactor string `json:"limiting_factor"`
				Suggestion     string `json:"suggestion"`
			} `json:"unspecified"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal traceability-gap payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("payload errors = %+v, want none", payload.Errors)
	}
	if len(payload.Result.Unspecified) != 1 {
		t.Fatalf("result.unspecified = %+v, want one traceability gap finding", payload.Result.Unspecified)
	}
	if payload.Result.Unspecified[0].Code != "traceability_gap" {
		t.Fatalf("unspecified finding = %+v, want traceability_gap", payload.Result.Unspecified[0])
	}
	if payload.Result.Unspecified[0].SpecRef == "" {
		t.Fatalf("unspecified finding = %+v, want nearest accepted spec ref", payload.Result.Unspecified[0])
	}
	if payload.Result.Unspecified[0].Traceability != "semantic_neighbor_without_applies_to" {
		t.Fatalf("unspecified finding = %+v, want semantic_neighbor_without_applies_to", payload.Result.Unspecified[0])
	}
	if payload.Result.Unspecified[0].LimitingFactor != "spec_metadata_gap" {
		t.Fatalf("unspecified finding = %+v, want spec_metadata_gap limiting factor", payload.Result.Unspecified[0])
	}
	if !strings.Contains(payload.Result.Unspecified[0].Suggestion, `applies_to = ["code://src/api/middleware/tenant_limiter.go"]`) {
		t.Fatalf("unspecified finding = %+v, want applies_to suggestion for tenant_limiter", payload.Result.Unspecified[0])
	}
}

func TestRunCheckCompliancePathJSONInsufficientEvidenceExplainsTraceability(t *testing.T) {
	repo := writeSearchWorkspace(t)
	writeComplianceSourceFile(t, repo, "src/api/middleware/ratelimiter.go", `
package middleware

func buildLimiter() {}
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		rebuildSearchWorkspaceIndex(t)
		return runCheckCompliance([]string{
			"--path", "src/api/middleware/ratelimiter.go",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckCompliance() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckCompliance() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Unspecified []struct {
				SpecRef        string `json:"spec_ref"`
				Code           string `json:"code"`
				Traceability   string `json:"traceability"`
				LimitingFactor string `json:"limiting_factor"`
				Suggestion     string `json:"suggestion"`
			} `json:"unspecified"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal insufficient-evidence payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("payload errors = %+v, want none", payload.Errors)
	}
	if len(payload.Result.Unspecified) == 0 {
		t.Fatal("result.unspecified is empty, want insufficient_evidence findings")
	}
	for _, item := range payload.Result.Unspecified {
		if item.Code != "insufficient_evidence" {
			t.Fatalf("unspecified finding = %+v, want insufficient_evidence", item)
		}
		if item.Traceability != "explicit_applies_to" {
			t.Fatalf("unspecified finding = %+v, want explicit_applies_to", item)
		}
		if item.LimitingFactor != "code_evidence_gap" {
			t.Fatalf("unspecified finding = %+v, want code_evidence_gap limiting factor", item)
		}
		if !strings.Contains(item.Suggestion, "already governs") {
			t.Fatalf("unspecified finding = %+v, want explicit guidance", item)
		}
	}
}

func TestRunCheckComplianceJSONIncludesUnspecifiedSummaryBreakout(t *testing.T) {
	repo := writeSearchWorkspace(t)
	writeComplianceSourceFile(t, repo, "src/api/middleware/ratelimiter.go", `
package middleware

func buildLimiter() {}
`)
	writeComplianceSourceFile(t, repo, "notes/ungoverned.txt", `
zxqv aurora lattice
plinth ember quartz
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		rebuildSearchWorkspaceIndex(t)
		return runCheckCompliance([]string{
			"--path", "src/api/middleware/ratelimiter.go",
			"--path", "notes/ungoverned.txt",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckCompliance() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckCompliance() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Unspecified []struct {
				Path         string `json:"path"`
				Traceability string `json:"traceability"`
			} `json:"unspecified"`
			UnspecifiedSummary struct {
				Total                     int `json:"total"`
				MissingGovernanceEdge     int `json:"missing_governance_edge"`
				ExplicitButUnderexercised int `json:"explicit_but_underexercised"`
			} `json:"unspecified_summary"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal unspecified-summary payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("payload errors = %+v, want none", payload.Errors)
	}
	if got, want := payload.Result.UnspecifiedSummary.Total, len(payload.Result.Unspecified); got != want {
		t.Fatalf("unspecified_summary.total = %d, want %d", got, want)
	}
	if got, want := payload.Result.UnspecifiedSummary.MissingGovernanceEdge, 1; got != want {
		t.Fatalf("missing_governance_edge = %d, want %d", got, want)
	}
	if got, want := payload.Result.UnspecifiedSummary.ExplicitButUnderexercised, 2; got != want {
		t.Fatalf("explicit_but_underexercised = %d, want %d", got, want)
	}
}

func TestRunCheckComplianceJSONIncludesTopSuggestionsAndTimings(t *testing.T) {
	repo := writeSearchWorkspace(t)
	writeComplianceSourceFile(t, repo, "src/api/middleware/ratelimiter.go", `
package middleware

func buildLimiter() {}
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		rebuildSearchWorkspaceIndex(t)
		return runCheckCompliance([]string{
			"--path", "src/api/middleware/ratelimiter.go",
			"--format", "json",
			"--timings",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckCompliance() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckCompliance() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			TopSuggestions []string `json:"top_suggestions"`
		} `json:"result"`
		Timings struct {
			TotalMS    int64 `json:"total_ms"`
			IndexingMS int64 `json:"indexing_ms"`
		} `json:"timings"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal compliance timings payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("payload errors = %+v, want none", payload.Errors)
	}
	if len(payload.Result.TopSuggestions) == 0 || !strings.Contains(payload.Result.TopSuggestions[0], "already governs") {
		t.Fatalf("top_suggestions = %+v, want surfaced compliance guidance", payload.Result.TopSuggestions)
	}
	if payload.Timings.TotalMS <= 0 {
		t.Fatalf("timings.total_ms = %d, want > 0", payload.Timings.TotalMS)
	}
	if payload.Timings.IndexingMS <= 0 {
		t.Fatalf("timings.indexing_ms = %d, want > 0", payload.Timings.IndexingMS)
	}
}

func TestRunCheckComplianceJSONTimingsIncludeEmbeddingCalls(t *testing.T) {
	repo := writeSearchWorkspace(t)
	writeComplianceSourceFile(t, repo, "notes/ungoverned.txt", `
zxqv aurora lattice
plinth ember quartz
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		rebuildSearchWorkspaceIndex(t)
		return runCheckCompliance([]string{
			"--path", "notes/ungoverned.txt",
			"--format", "json",
			"--timings",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckCompliance() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckCompliance() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Timings struct {
			EmbeddingMS    int64 `json:"embedding_ms"`
			EmbeddingCalls int   `json:"embedding_calls"`
		} `json:"timings"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal compliance embedding timings payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("payload errors = %+v, want none", payload.Errors)
	}
	if payload.Timings.EmbeddingMS <= 0 || payload.Timings.EmbeddingCalls <= 0 {
		t.Fatalf("timings = %+v, want semantic lookup embedding timings", payload.Timings)
	}
}

func TestRunCheckComplianceDiffFromStdinJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	oldStdin := cliStdin
	cliStdin = strings.NewReader(strings.TrimSpace(`
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
`))
	t.Cleanup(func() {
		cliStdin = oldStdin
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		rebuildSearchWorkspaceIndex(t)
		return runCheckCompliance([]string{
			"--diff-file", "-",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckCompliance(--diff-file -) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckCompliance(--diff-file -) wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			DiffFile string `json:"diff_file"`
		} `json:"request"`
		Result struct {
			Paths     []string `json:"paths"`
			Compliant []struct {
				Path    string `json:"path"`
				SpecRef string `json:"spec_ref"`
			} `json:"compliant"`
			Errors []cliIssue `json:"errors"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal diff payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("payload errors = %+v, want none", payload.Errors)
	}
	if payload.Request.DiffFile != "-" {
		t.Fatalf("request.diff_file = %q, want -", payload.Request.DiffFile)
	}
	if len(payload.Result.Paths) != 1 || payload.Result.Paths[0] != "src/api/middleware/ratelimiter.go" {
		t.Fatalf("result.paths = %v, want ratelimiter path", payload.Result.Paths)
	}
	if len(payload.Result.Compliant) == 0 || payload.Result.Compliant[0].SpecRef == "" {
		t.Fatalf("result.compliant = %+v, want compliant findings with spec refs", payload.Result.Compliant)
	}
}

func TestRunCheckComplianceDeletionDiffDoesNotFlagRemovedContent(t *testing.T) {
	repo := writeSearchWorkspace(t)

	oldStdin := cliStdin
	cliStdin = strings.NewReader(strings.TrimSpace(`
diff --git a/src/api/middleware/ratelimiter.go b/src/api/middleware/ratelimiter.go
deleted file mode 100644
index 1111111..0000000
--- a/src/api/middleware/ratelimiter.go
+++ /dev/null
@@ -1,6 +0,0 @@
-package middleware
-
-// Apply limits per API key.
-// Enforce a default limit of 100 requests per minute.
-// Use a fixed-window limiter.
-func buildLimiter() {}
`))
	t.Cleanup(func() {
		cliStdin = oldStdin
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		rebuildSearchWorkspaceIndex(t)
		return runCheckCompliance([]string{
			"--diff-file", "-",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckCompliance(--diff-file -) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckCompliance(--diff-file -) wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Paths       []string `json:"paths"`
			Compliant   []any    `json:"compliant"`
			Conflicts   []any    `json:"conflicts"`
			Unspecified []struct {
				Path           string `json:"path"`
				SpecRef        string `json:"spec_ref"`
				Code           string `json:"code"`
				LimitingFactor string `json:"limiting_factor"`
			} `json:"unspecified"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal deletion diff payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("payload errors = %+v, want none", payload.Errors)
	}
	if len(payload.Result.Paths) != 1 || payload.Result.Paths[0] != "src/api/middleware/ratelimiter.go" {
		t.Fatalf("result.paths = %v, want ratelimiter path", payload.Result.Paths)
	}
	if len(payload.Result.Conflicts) != 0 {
		t.Fatalf("result.conflicts = %+v, want no conflicts for deleted content", payload.Result.Conflicts)
	}
	if len(payload.Result.Compliant) != 0 {
		t.Fatalf("result.compliant = %+v, want no compliant findings for deleted content", payload.Result.Compliant)
	}
	if len(payload.Result.Unspecified) == 0 {
		t.Fatal("result.unspecified is empty, want removed_content findings")
	}
	for _, item := range payload.Result.Unspecified {
		if item.Path != "src/api/middleware/ratelimiter.go" {
			t.Fatalf("unspecified finding = %+v, want ratelimiter path", item)
		}
		if item.SpecRef == "" {
			t.Fatalf("unspecified finding = %+v, want explicit governing spec ref", item)
		}
		if item.Code != "removed_content" {
			t.Fatalf("unspecified finding = %+v, want removed_content", item)
		}
		if item.LimitingFactor != "code_evidence_gap" {
			t.Fatalf("unspecified finding = %+v, want code_evidence_gap limiting factor", item)
		}
	}
}

func TestRunCheckComplianceCollapsesDuplicateMirrorTargets(t *testing.T) {
	repo := writeSearchWorkspace(t)
	content := `
package middleware

// Apply limits per tenant rather than per API key.
// Enforce a default limit of 200 requests per minute.
// Allow short bursts above the steady-state tenant limit.
// Use a sliding-window limiter and tenant-specific overrides.
func buildLimiter() {}
`
	writeComplianceSourceFile(t, repo, "src/api/middleware/ratelimiter.go", content)
	writeComplianceSourceFile(t, repo, ".claude/skills/ratelimiter.go", content)
	writeComplianceSourceFile(t, repo, ".gemini/skills/ratelimiter.go", content)

	oldStdin := cliStdin
	cliStdin = strings.NewReader(strings.TrimSpace(`
diff --git a/src/api/middleware/ratelimiter.go b/src/api/middleware/ratelimiter.go
index 0000000..1111111 100644
--- a/src/api/middleware/ratelimiter.go
+++ b/src/api/middleware/ratelimiter.go
@@ -0,0 +1,7 @@
+package middleware
+
+// Apply limits per tenant rather than per API key.
+// Enforce a default limit of 200 requests per minute.
+// Allow short bursts above the steady-state tenant limit.
+// Use a sliding-window limiter and tenant-specific overrides.
+func buildLimiter() {}
diff --git a/.claude/skills/ratelimiter.go b/.claude/skills/ratelimiter.go
index 0000000..1111111 100644
--- a/.claude/skills/ratelimiter.go
+++ b/.claude/skills/ratelimiter.go
@@ -0,0 +1,7 @@
+package middleware
+
+// Apply limits per tenant rather than per API key.
+// Enforce a default limit of 200 requests per minute.
+// Allow short bursts above the steady-state tenant limit.
+// Use a sliding-window limiter and tenant-specific overrides.
+func buildLimiter() {}
diff --git a/.gemini/skills/ratelimiter.go b/.gemini/skills/ratelimiter.go
index 0000000..1111111 100644
--- a/.gemini/skills/ratelimiter.go
+++ b/.gemini/skills/ratelimiter.go
@@ -0,0 +1,7 @@
+package middleware
+
+// Apply limits per tenant rather than per API key.
+// Enforce a default limit of 200 requests per minute.
+// Allow short bursts above the steady-state tenant limit.
+// Use a sliding-window limiter and tenant-specific overrides.
+func buildLimiter() {}
`))
	t.Cleanup(func() {
		cliStdin = oldStdin
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		rebuildSearchWorkspaceIndex(t)
		return runCheckCompliance([]string{
			"--diff-file", "-",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckCompliance(--diff-file -) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckCompliance(--diff-file -) wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Paths     []string `json:"paths"`
			Compliant []struct {
				Path string `json:"path"`
			} `json:"compliant"`
			Unspecified []any `json:"unspecified"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal duplicate-target payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("payload errors = %+v, want none", payload.Errors)
	}
	if got, want := len(payload.Result.Paths), 3; got != want {
		t.Fatalf("len(result.paths) = %d, want %d", got, want)
	}
	if len(payload.Result.Compliant) == 0 {
		t.Fatalf("result.compliant = %+v, want compliant findings for the canonical target", payload.Result.Compliant)
	}
	if len(payload.Result.Unspecified) != 0 {
		t.Fatalf("result.unspecified = %+v, want duplicate mirror findings collapsed", payload.Result.Unspecified)
	}
}

func rebuildSearchWorkspaceIndex(t *testing.T) {
	t.Helper()
	if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0", code)
	}
}

func writeComplianceSourceFile(t *testing.T, repo, relPath, content string) {
	t.Helper()
	mustWriteFileCmd(t, repo+"/"+relPath, content)
}
