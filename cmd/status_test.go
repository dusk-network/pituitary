package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunStatusReportsMissingIndex(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runStatus(nil, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "index: missing") {
		t.Fatalf("runStatus() output %q does not report missing index", out)
	}
	if !strings.Contains(out, filepath.Join(repo, ".pituitary", "pituitary.db")) {
		t.Fatalf("runStatus() output %q does not contain resolved index path", out)
	}
}

func TestRunStatusJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var rebuildStdout bytes.Buffer
	var rebuildStderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &rebuildStdout, &rebuildStderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, rebuildStderr.String())
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
		return runStatus([]string{"--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct{} `json:"request"`
		Result  struct {
			ConfigPath  string `json:"config_path"`
			IndexPath   string `json:"index_path"`
			IndexExists bool   `json:"index_exists"`
			SpecCount   int    `json:"spec_count"`
			DocCount    int    `json:"doc_count"`
			ChunkCount  int    `json:"chunk_count"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if payload.Result.ConfigPath == "" || payload.Result.IndexPath == "" {
		t.Fatalf("result = %+v, want non-empty config and index paths", payload.Result)
	}
	if !payload.Result.IndexExists {
		t.Fatalf("result = %+v, want index_exists=true", payload.Result)
	}
	if payload.Result.SpecCount != 3 || payload.Result.DocCount != 2 || payload.Result.ChunkCount != 17 {
		t.Fatalf("result = %+v, want 3 specs, 2 docs, 17 chunks", payload.Result)
	}
}

func TestRunStatusReportsConfigError(t *testing.T) {
	repo := t.TempDir()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runStatus(nil, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runStatus() exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "pituitary status: no config found") {
		t.Fatalf("runStatus() stderr %q does not contain config discovery error", stderr.String())
	}
}
