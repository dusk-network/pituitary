package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestRunStatusJSONIncludesRuntimeProbeResults(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runStatus([]string{"--format", "json", "--check-runtime", "all"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			CheckRuntime string `json:"check_runtime"`
		} `json:"request"`
		Result struct {
			Runtime struct {
				Scope  string `json:"scope"`
				Checks []struct {
					Name     string `json:"name"`
					Provider string `json:"provider"`
					Status   string `json:"status"`
					Message  string `json:"message"`
				} `json:"checks"`
			} `json:"runtime"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if got, want := payload.Request.CheckRuntime, "all"; got != want {
		t.Fatalf("request.check_runtime = %q, want %q", got, want)
	}
	if got, want := payload.Result.Runtime.Scope, "all"; got != want {
		t.Fatalf("result.runtime.scope = %q, want %q", got, want)
	}
	if len(payload.Result.Runtime.Checks) != 2 {
		t.Fatalf("len(result.runtime.checks) = %d, want 2", len(payload.Result.Runtime.Checks))
	}
	if got, want := payload.Result.Runtime.Checks[0].Name, "runtime.embedder"; got != want {
		t.Fatalf("checks[0].name = %q, want %q", got, want)
	}
	if got, want := payload.Result.Runtime.Checks[0].Status, "ready"; got != want {
		t.Fatalf("checks[0].status = %q, want %q", got, want)
	}
	if got, want := payload.Result.Runtime.Checks[1].Name, "runtime.analysis"; got != want {
		t.Fatalf("checks[1].name = %q, want %q", got, want)
	}
	if got, want := payload.Result.Runtime.Checks[1].Status, "disabled"; got != want {
		t.Fatalf("checks[1].status = %q, want %q", got, want)
	}
}

func TestRunStatusRuntimeProbeReportsDependencyUnavailable(t *testing.T) {
	repo := t.TempDir()
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"error": "Model unloaded..",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	mustWriteIndexFixture(t, repo, fmt.Sprintf(`
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "openai_compatible"
model = "pituitary-embed"
endpoint = %q
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`, server.URL+"/v1"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runStatus([]string{"--format", "json", "--check-runtime", "embedder"}, &stdout, &stderr)
	})
	if exitCode != 3 {
		t.Fatalf("runStatus() exit code = %d, want 3", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			CheckRuntime string `json:"check_runtime"`
		} `json:"request"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status error payload: %v", err)
	}
	if got, want := payload.Request.CheckRuntime, "embedder"; got != want {
		t.Fatalf("request.check_runtime = %q, want %q", got, want)
	}
	if len(payload.Errors) != 1 {
		t.Fatalf("len(errors) = %d, want 1", len(payload.Errors))
	}
	if got, want := payload.Errors[0].Code, "dependency_unavailable"; got != want {
		t.Fatalf("errors[0].code = %q, want %q", got, want)
	}
	if !strings.Contains(payload.Errors[0].Message, "load or pin model") {
		t.Fatalf("errors[0].message = %q, want model loading guidance", payload.Errors[0].Message)
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
