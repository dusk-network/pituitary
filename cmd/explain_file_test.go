package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestRunExplainFileJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)
	mustWriteFileCmd(t, filepath.Join(repo, "docs", "development", "testing-guide.md"), "# Testing Guide\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runExplainFile([]string{"docs/development/testing-guide.md", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runExplainFile() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runExplainFile() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			Path string `json:"path"`
		} `json:"request"`
		Result struct {
			WorkspacePath string `json:"workspace_path"`
			Summary       struct {
				Status string `json:"status"`
			} `json:"summary"`
			Sources []struct {
				Name         string `json:"name"`
				Reason       string `json:"reason"`
				Selected     bool   `json:"selected"`
				RelativePath string `json:"relative_path"`
			} `json:"sources"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal explain payload: %v", err)
	}
	if got, want := payload.Request.Path, "docs/development/testing-guide.md"; got != want {
		t.Fatalf("request path = %q, want %q", got, want)
	}
	if got, want := payload.Result.Summary.Status, "excluded"; got != want {
		t.Fatalf("summary status = %q, want %q", got, want)
	}
	if got, want := payload.Result.WorkspacePath, "docs/development/testing-guide.md"; got != want {
		t.Fatalf("workspace path = %q, want %q", got, want)
	}
	if len(payload.Result.Sources) != 2 {
		t.Fatalf("sources = %+v, want 2 explanations", payload.Result.Sources)
	}
	if payload.Result.Sources[1].Name != "docs" || payload.Result.Sources[1].Reason != "not_matched_by_include" || payload.Result.Sources[1].Selected {
		t.Fatalf("docs explanation = %+v, want excluded docs explanation", payload.Result.Sources[1])
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunExplainFileRejectsMissingPath(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runExplainFile([]string{"--format", "json"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("runExplainFile() exit code = %d, want 2", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runExplainFile() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result any        `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal explain error payload: %v", err)
	}
	if payload.Result != nil {
		t.Fatalf("result = %#v, want nil", payload.Result)
	}
	if len(payload.Errors) != 1 || payload.Errors[0].Code != "validation_error" {
		t.Fatalf("errors = %+v, want one validation_error", payload.Errors)
	}
}
