package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunCompareSpecsJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCompareSpecs([]string{
			"--spec-ref", "SPEC-008",
			"--spec-ref", "SPEC-042",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCompareSpecs() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCompareSpecs() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			SpecRefs []string `json:"spec_refs"`
		} `json:"request"`
		Result struct {
			SpecRefs   []string `json:"spec_refs"`
			Comparison struct {
				SharedScope []string `json:"shared_scope"`
				Tradeoffs   []struct {
					Topic string `json:"topic"`
				} `json:"tradeoffs"`
				Recommendation string `json:"recommendation"`
			} `json:"comparison"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal compare payload: %v", err)
	}
	if len(payload.Request.SpecRefs) != 2 || len(payload.Result.SpecRefs) != 2 {
		t.Fatalf("spec_refs request=%v result=%v, want two refs", payload.Request.SpecRefs, payload.Result.SpecRefs)
	}
	if len(payload.Result.Comparison.SharedScope) == 0 {
		t.Fatalf("shared_scope = %v, want shared scope", payload.Result.Comparison.SharedScope)
	}
	if len(payload.Result.Comparison.Tradeoffs) == 0 {
		t.Fatalf("tradeoffs = %+v, want structured tradeoffs", payload.Result.Comparison.Tradeoffs)
	}
	if payload.Result.Comparison.Recommendation == "" {
		t.Fatal("recommendation is empty")
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunCompareSpecsRejectsMoreThanTwoRefs(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCompareSpecs([]string{
			"--spec-ref", "SPEC-008",
			"--spec-ref", "SPEC-042",
			"--spec-ref", "SPEC-055",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runCompareSpecs() exit code = %d, want 2", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCompareSpecs() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			SpecRefs []string `json:"spec_refs"`
		} `json:"request"`
		Result any        `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal compare payload: %v", err)
	}
	if len(payload.Request.SpecRefs) != 3 {
		t.Fatalf("request spec_refs = %v, want 3 refs", payload.Request.SpecRefs)
	}
	if payload.Result != nil {
		t.Fatalf("result = %#v, want nil", payload.Result)
	}
	if len(payload.Errors) != 1 {
		t.Fatalf("errors = %+v, want one validation error", payload.Errors)
	}
	if payload.Errors[0].Code != "validation_error" {
		t.Fatalf("error code = %q, want validation_error", payload.Errors[0].Code)
	}
	if !strings.Contains(payload.Errors[0].Message, "exactly two --spec-ref flags are required") {
		t.Fatalf("error message = %q, want exact-two validation", payload.Errors[0].Message)
	}
}
