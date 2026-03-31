package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCheckDocDriftScopeAllJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckDocDrift([]string{"--scope", "all", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckDocDrift() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckDocDrift() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			Scope string `json:"scope"`
		} `json:"request"`
		Result struct {
			Scope struct {
				Mode string `json:"mode"`
			} `json:"scope"`
			DriftItems []struct {
				DocRef   string `json:"doc_ref"`
				Findings []struct {
					Code      string `json:"code"`
					Rationale string `json:"rationale"`
					Evidence  struct {
						SpecSection string `json:"spec_section"`
						DocSection  string `json:"doc_section"`
					} `json:"evidence"`
					Confidence struct {
						Level string `json:"level"`
					} `json:"confidence"`
				} `json:"findings"`
			} `json:"drift_items"`
			Assessments []struct {
				DocRef   string `json:"doc_ref"`
				Status   string `json:"status"`
				Evidence struct {
					SpecSection string `json:"spec_section"`
					DocSection  string `json:"doc_section"`
				} `json:"evidence"`
				Confidence struct {
					Level string `json:"level"`
				} `json:"confidence"`
			} `json:"assessments"`
			Remediation struct {
				Items []struct {
					DocRef      string `json:"doc_ref"`
					Suggestions []struct {
						SpecRef  string `json:"spec_ref"`
						Code     string `json:"code"`
						Evidence struct {
							SpecSection string `json:"spec_section"`
						} `json:"evidence"`
						SuggestedEdit struct {
							Action string `json:"action"`
						} `json:"suggested_edit"`
					} `json:"suggestions"`
				} `json:"items"`
			} `json:"remediation"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal drift payload: %v", err)
	}
	if payload.Request.Scope != "all" || payload.Result.Scope.Mode != "all" {
		t.Fatalf("scope request=%+v result=%+v, want all/all", payload.Request, payload.Result.Scope)
	}
	if len(payload.Result.DriftItems) != 1 || payload.Result.DriftItems[0].DocRef != "doc://guides/api-rate-limits" {
		t.Fatalf("drift_items = %+v, want only guide drift", payload.Result.DriftItems)
	}
	if len(payload.Result.DriftItems[0].Findings) == 0 ||
		payload.Result.DriftItems[0].Findings[0].Rationale == "" ||
		payload.Result.DriftItems[0].Findings[0].Evidence.SpecSection == "" ||
		payload.Result.DriftItems[0].Findings[0].Evidence.DocSection == "" ||
		payload.Result.DriftItems[0].Findings[0].Confidence.Level == "" {
		t.Fatalf("top drift finding = %+v, want rationale, evidence, and confidence", payload.Result.DriftItems[0].Findings)
	}
	var foundDrift, foundAligned bool
	for _, assessment := range payload.Result.Assessments {
		switch {
		case assessment.DocRef == "doc://guides/api-rate-limits" && assessment.Status == "drift":
			foundDrift = true
		case assessment.DocRef == "doc://runbooks/rate-limit-rollout" && assessment.Status == "aligned":
			foundAligned = true
		}
	}
	if !foundDrift || !foundAligned {
		t.Fatalf("assessments = %+v, want guide drift and runbook aligned assessments", payload.Result.Assessments)
	}
	if len(payload.Result.Remediation.Items) != 1 || payload.Result.Remediation.Items[0].DocRef != "doc://guides/api-rate-limits" {
		t.Fatalf("remediation = %+v, want guide remediation", payload.Result.Remediation)
	}
	if len(payload.Result.Remediation.Items[0].Suggestions) == 0 {
		t.Fatalf("remediation suggestions = %+v, want actionable suggestions", payload.Result.Remediation.Items[0])
	}
	if payload.Result.Remediation.Items[0].Suggestions[0].SpecRef == "" ||
		payload.Result.Remediation.Items[0].Suggestions[0].Evidence.SpecSection == "" ||
		payload.Result.Remediation.Items[0].Suggestions[0].SuggestedEdit.Action == "" {
		t.Fatalf("top remediation suggestion = %+v, want stable evidence and suggested edit", payload.Result.Remediation.Items[0].Suggestions[0])
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunCheckDocDriftWithDiffFileJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	oldStdin := cliStdin
	cliStdin = strings.NewReader(strings.TrimSpace(`
diff --git a/src/api/middleware/ratelimiter.go b/src/api/middleware/ratelimiter.go
--- a/src/api/middleware/ratelimiter.go
+++ b/src/api/middleware/ratelimiter.go
@@ -1,2 +1,2 @@
-const defaultLimit = 100
+const defaultLimit = 200
`))
	defer func() {
		cliStdin = oldStdin
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckDocDrift([]string{"--diff-file", "-", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckDocDrift() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckDocDrift() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			DiffFile string `json:"diff_file"`
		} `json:"request"`
		Result struct {
			Scope struct {
				Mode string `json:"mode"`
			} `json:"scope"`
			ChangedFiles []struct {
				Path string `json:"path"`
			} `json:"changed_files"`
			ImplicatedSpecs []struct {
				Ref     string   `json:"ref"`
				Reasons []string `json:"reasons"`
			} `json:"implicated_specs"`
			ImplicatedDocs []struct {
				DocRef    string   `json:"doc_ref"`
				Reasons   []string `json:"reasons"`
				SourceRef string   `json:"source_ref"`
			} `json:"implicated_docs"`
			DriftItems []struct {
				DocRef string `json:"doc_ref"`
			} `json:"drift_items"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal diff drift payload: %v", err)
	}
	if got, want := payload.Request.DiffFile, "-"; got != want {
		t.Fatalf("request.diff_file = %q, want %q", got, want)
	}
	if got, want := payload.Result.Scope.Mode, "diff"; got != want {
		t.Fatalf("result.scope.mode = %q, want %q", got, want)
	}
	if len(payload.Result.ChangedFiles) != 1 || payload.Result.ChangedFiles[0].Path != "src/api/middleware/ratelimiter.go" {
		t.Fatalf("changed_files = %+v, want ratelimiter.go", payload.Result.ChangedFiles)
	}
	if len(payload.Result.ImplicatedSpecs) == 0 || payload.Result.ImplicatedSpecs[0].Ref != "SPEC-042" {
		t.Fatalf("implicated_specs = %+v, want SPEC-042", payload.Result.ImplicatedSpecs)
	}
	if len(payload.Result.ImplicatedSpecs[0].Reasons) == 0 {
		t.Fatalf("implicated_spec reasons = %+v, want linkage evidence", payload.Result.ImplicatedSpecs[0].Reasons)
	}
	if len(payload.Result.ImplicatedDocs) == 0 || payload.Result.ImplicatedDocs[0].DocRef == "" || len(payload.Result.ImplicatedDocs[0].Reasons) == 0 {
		t.Fatalf("implicated_docs = %+v, want docs with reasons", payload.Result.ImplicatedDocs)
	}
	if len(payload.Result.DriftItems) == 0 || payload.Result.DriftItems[0].DocRef != "doc://guides/api-rate-limits" {
		t.Fatalf("drift_items = %+v, want guide drift", payload.Result.DriftItems)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunCheckDocDriftWithDiffFileText(t *testing.T) {
	repo := writeSearchWorkspace(t)

	oldStdin := cliStdin
	cliStdin = strings.NewReader(strings.TrimSpace(`
diff --git a/src/api/middleware/ratelimiter.go b/src/api/middleware/ratelimiter.go
--- a/src/api/middleware/ratelimiter.go
+++ b/src/api/middleware/ratelimiter.go
@@ -1,2 +1,2 @@
-const defaultLimit = 100
+const defaultLimit = 200
`))
	defer func() {
		cliStdin = oldStdin
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckDocDrift([]string{"--diff-file", "-"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckDocDrift() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckDocDrift() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"CHANGED FILES",
		"src/api/middleware/ratelimiter.go",
		"IMPLICATED SPECS",
		"SPEC-042",
		"IMPLICATED DOCS",
		"docs/guides/api-rate-limits.md",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runCheckDocDrift() output %q does not contain %q", out, want)
		}
	}
}

func TestRunCheckDocDriftTargetedRefsJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckDocDrift([]string{
			"--doc-ref", "doc://guides/api-rate-limits",
			"--doc-ref", "doc://runbooks/rate-limit-rollout",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckDocDrift() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckDocDrift() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			DocRefs []string `json:"doc_refs"`
		} `json:"request"`
		Result struct {
			Scope struct {
				Mode    string   `json:"mode"`
				DocRefs []string `json:"doc_refs"`
			} `json:"scope"`
			DriftItems []struct {
				DocRef string `json:"doc_ref"`
			} `json:"drift_items"`
			Assessments []struct {
				DocRef string `json:"doc_ref"`
				Status string `json:"status"`
			} `json:"assessments"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal targeted drift payload: %v", err)
	}
	if payload.Result.Scope.Mode != "doc_refs" || len(payload.Request.DocRefs) != 2 {
		t.Fatalf("request/result scope mismatch: request=%+v result=%+v", payload.Request, payload.Result.Scope)
	}
	if len(payload.Result.DriftItems) != 1 || payload.Result.DriftItems[0].DocRef != "doc://guides/api-rate-limits" {
		t.Fatalf("drift_items = %+v, want only guide drift", payload.Result.DriftItems)
	}
	var foundAligned bool
	for _, assessment := range payload.Result.Assessments {
		if assessment.DocRef == "doc://runbooks/rate-limit-rollout" && assessment.Status == "aligned" {
			foundAligned = true
			break
		}
	}
	if !foundAligned {
		t.Fatalf("assessments = %+v, want aligned runbook assessment", payload.Result.Assessments)
	}
}

func TestRunCheckDocDriftTextIncludesRemediation(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckDocDrift([]string{"--scope", "all"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckDocDrift() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckDocDrift() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"━━◈ check-doc-drift",
		"██ DRIFT",
		"██ OK",
		"default limit mismatch",
		"pituitary fix --path",
		"review-spec --format html",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runCheckDocDrift() output %q does not contain %q", out, want)
		}
	}
}

func TestRunCheckDocDriftTextIncludesRepoIdentity(t *testing.T) {
	repo := writeMultiRepoSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckDocDrift([]string{"--scope", "all"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckDocDrift() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckDocDrift() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"[shared] docs/guides/api-rate-limits.md",
		"██ DRIFT",
		"deterministic remediation is available, but `pituitary fix --path` only targets primary-workspace docs",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runCheckDocDrift() output %q does not contain %q", out, want)
		}
	}
}

func TestRunCheckDocDriftWarnsOnWeakAcceptedContracts(t *testing.T) {
	repo := writeWeakAcceptedContractWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckDocDrift([]string{"--scope", "all", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckDocDrift() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckDocDrift() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Warnings []cliIssue `json:"warnings"`
		Errors   []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal drift payload: %v", err)
	}
	if len(payload.Warnings) == 0 || payload.Warnings[0].Code != "low_confidence_inference" {
		t.Fatalf("warnings = %+v, want low_confidence_inference warning", payload.Warnings)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunCheckDocDriftWithRequestFileJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)
	mustWriteJSONFileCmd(t, filepath.Join(repo, "doc-drift-request.json"), map[string]any{
		"doc_refs": []string{
			"doc://guides/api-rate-limits",
			"doc://runbooks/rate-limit-rollout",
		},
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckDocDrift([]string{"--request-file", "doc-drift-request.json", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckDocDrift() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckDocDrift() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			DocRefs []string `json:"doc_refs"`
		} `json:"request"`
		Result struct {
			Scope struct {
				Mode string `json:"mode"`
			} `json:"scope"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal doc-drift request-file payload: %v", err)
	}
	if len(payload.Request.DocRefs) != 2 || payload.Result.Scope.Mode != "doc_refs" {
		t.Fatalf("payload = %+v, want doc_refs scope", payload)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunCheckDocDriftWithRequestFileDiffFileJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)
	mustWriteJSONFileCmd(t, filepath.Join(repo, "doc-drift-request.json"), map[string]any{
		"diff_file": "-",
	})

	oldStdin := cliStdin
	cliStdin = strings.NewReader(strings.TrimSpace(`
diff --git a/src/api/middleware/ratelimiter.go b/src/api/middleware/ratelimiter.go
--- a/src/api/middleware/ratelimiter.go
+++ b/src/api/middleware/ratelimiter.go
@@ -1,2 +1,2 @@
-const defaultLimit = 100
+const defaultLimit = 200
`))
	defer func() {
		cliStdin = oldStdin
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckDocDrift([]string{"--request-file", "doc-drift-request.json", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckDocDrift() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckDocDrift() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			DiffFile string `json:"diff_file"`
		} `json:"request"`
		Result struct {
			Scope struct {
				Mode string `json:"mode"`
			} `json:"scope"`
			ChangedFiles []struct {
				Path string `json:"path"`
			} `json:"changed_files"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal doc-drift request-file diff payload: %v", err)
	}
	if got, want := payload.Request.DiffFile, "-"; got != want {
		t.Fatalf("request.diff_file = %q, want %q", got, want)
	}
	if got, want := payload.Result.Scope.Mode, "diff"; got != want {
		t.Fatalf("result.scope.mode = %q, want %q", got, want)
	}
	if len(payload.Result.ChangedFiles) != 1 || payload.Result.ChangedFiles[0].Path != "src/api/middleware/ratelimiter.go" {
		t.Fatalf("changed_files = %+v, want ratelimiter.go", payload.Result.ChangedFiles)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunCheckDocDriftWithRequestFileDiffFileErrorJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)
	mustWriteJSONFileCmd(t, filepath.Join(repo, "doc-drift-request.json"), map[string]any{
		"diff_file": "missing.patch",
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runCheckDocDrift([]string{"--request-file", "doc-drift-request.json", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runCheckDocDrift() exit code = %d, want 2", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckDocDrift() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			DiffFile string `json:"diff_file"`
		} `json:"request"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal doc-drift request-file diff error payload: %v", err)
	}
	if got, want := payload.Request.DiffFile, "missing.patch"; got != want {
		t.Fatalf("request.diff_file = %q, want %q", got, want)
	}
	if len(payload.Errors) != 1 || !strings.Contains(payload.Errors[0].Message, "missing.patch") {
		t.Fatalf("errors = %+v, want diff-file validation error", payload.Errors)
	}
}
