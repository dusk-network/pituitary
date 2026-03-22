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
				DocRef string `json:"doc_ref"`
			} `json:"drift_items"`
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
	if !strings.Contains(out, "remediation:") {
		t.Fatalf("runCheckDocDrift() output %q does not contain remediation summary", out)
	}
	if !strings.Contains(out, "suggested edit:") {
		t.Fatalf("runCheckDocDrift() output %q does not contain suggested edit guidance", out)
	}
}
