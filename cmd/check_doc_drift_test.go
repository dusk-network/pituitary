package cmd

import (
	"bytes"
	"encoding/json"
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
	if len(payload.Result.Assessments) == 0 || payload.Result.Assessments[0].Status != "drift" {
		t.Fatalf("assessments = %+v, want at least one drift assessment", payload.Result.Assessments)
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
		"status: drift",
		"confidence:",
		"rationale:",
		"spec evidence:",
		"doc evidence:",
		"remediation:",
		"suggested edit:",
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
