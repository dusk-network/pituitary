package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestRunCheckSpecFreshnessJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckSpecFreshness([]string{"--spec-ref", "SPEC-042", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckSpecFreshness() exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout.String(), stderr.String())
	}

	var payload struct {
		Request struct {
			SpecRef string `json:"spec_ref"`
		} `json:"request"`
		Result struct {
			Scope string `json:"scope"`
			Items []struct {
				SpecRef string `json:"spec_ref"`
				Verdict string `json:"verdict"`
			} `json:"items"`
			ContentTrust struct {
				Level string `json:"level"`
			} `json:"content_trust"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal freshness payload: %v\nraw: %s", err, stdout.String())
	}
	if payload.Request.SpecRef != "SPEC-042" {
		t.Fatalf("request.spec_ref = %q, want SPEC-042", payload.Request.SpecRef)
	}
	if payload.Result.Scope != "single" {
		t.Fatalf("result.scope = %q, want single", payload.Result.Scope)
	}
	if len(payload.Result.Items) != 1 {
		t.Fatalf("result.items = %d, want 1", len(payload.Result.Items))
	}
	if payload.Result.Items[0].SpecRef != "SPEC-042" {
		t.Fatalf("first item spec_ref = %q, want SPEC-042", payload.Result.Items[0].SpecRef)
	}
	if payload.Result.ContentTrust.Level != "untrusted" {
		t.Fatalf("content_trust.level = %q, want untrusted", payload.Result.ContentTrust.Level)
	}
}

func TestRunCheckSpecFreshnessText(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckSpecFreshness([]string{"--scope", "all", "--format", "text"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckSpecFreshness() exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout.String(), stderr.String())
	}

	output := stdout.String()
	if output == "" {
		t.Fatal("expected text output, got empty")
	}
}

func TestRunCheckSpecFreshnessRejectsPathAndSpecRef(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runCheckSpecFreshness([]string{
			"--path", "some/path.md",
			"--spec-ref", "SPEC-042",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runCheckSpecFreshness() exit code = %d, want 2", exitCode)
	}

	var payload struct {
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal mutex payload: %v", err)
	}
	if len(payload.Errors) != 1 || payload.Errors[0].Code != "validation_error" {
		t.Fatalf("errors = %+v, want one validation_error", payload.Errors)
	}
	if payload.Errors[0].Message != "at most one of --path or --spec-ref may be specified" {
		t.Fatalf("errors[0].message = %q, want mutex guard message", payload.Errors[0].Message)
	}
}

func TestRunCheckSpecFreshnessHelpFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runCheckSpecFreshness([]string{"--help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}
}
