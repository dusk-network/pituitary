package cmd

import (
	"bytes"
	"encoding/json"
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

func TestRunCheckCompliancePathJSONNoGoverningSpec(t *testing.T) {
	repo := writeSearchWorkspace(t)
	writeComplianceSourceFile(t, repo, "src/worker/jobs/reconciler.go", `
package jobs

func Run() {}
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		rebuildSearchWorkspaceIndex(t)
		return runCheckCompliance([]string{
			"--path", "src/worker/jobs/reconciler.go",
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
				Path    string `json:"path"`
				Code    string `json:"code"`
				Message string `json:"message"`
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
	if payload.Result.Unspecified[0].Code != "no_governing_spec" {
		t.Fatalf("unspecified finding = %+v, want no_governing_spec", payload.Result.Unspecified[0])
	}
	if payload.Result.Unspecified[0].Path != "src/worker/jobs/reconciler.go" {
		t.Fatalf("unspecified finding = %+v, want reconciler path", payload.Result.Unspecified[0])
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
