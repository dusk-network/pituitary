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
	if !strings.Contains(out, "config resolution:") {
		t.Fatalf("runStatus() output %q does not explain config resolution", out)
	}
	if !strings.Contains(out, "artifact ignore patterns: .pituitary/") {
		t.Fatalf("runStatus() output %q does not contain artifact ignore guidance", out)
	}
	if !strings.Contains(out, filepath.Join(repo, ".pituitary", "pituitary.db")) {
		t.Fatalf("runStatus() output %q does not contain resolved index path", out)
	}
}

func TestRunStatusReportsFixtureGuidanceForLargerCorpus(t *testing.T) {
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
		return runStatus(nil, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `runtime.embedder is still "fixture" on 5 indexed artifact(s)`) {
		t.Fatalf("runStatus() output %q does not contain fixture guidance", out)
	}
	if !strings.Contains(out, "`pituitary status --check-runtime embedder`") {
		t.Fatalf("runStatus() output %q does not contain runtime probe guidance", out)
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
			WorkspaceRoot    string `json:"workspace_root"`
			ConfigPath       string `json:"config_path"`
			ConfigResolution struct {
				SelectedBy string `json:"selected_by"`
				Reason     string `json:"reason"`
				Candidates []struct {
					Source string `json:"source"`
					Status string `json:"status"`
					Path   string `json:"path"`
				} `json:"candidates"`
			} `json:"config_resolution"`
			IndexPath   string `json:"index_path"`
			IndexExists bool   `json:"index_exists"`
			Freshness   struct {
				State string `json:"state"`
			} `json:"freshness"`
			SpecCount         int `json:"spec_count"`
			DocCount          int `json:"doc_count"`
			ChunkCount        int `json:"chunk_count"`
			ArtifactLocations struct {
				IndexDir               string   `json:"index_dir"`
				DiscoverConfigPath     string   `json:"discover_config_path"`
				CanonicalizeBundleRoot string   `json:"canonicalize_bundle_root"`
				IgnorePatterns         []string `json:"ignore_patterns"`
				RelocationHints        []string `json:"relocation_hints"`
			} `json:"artifact_locations"`
			Guidance []string `json:"guidance"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if payload.Result.WorkspaceRoot == "" || payload.Result.ConfigPath == "" || payload.Result.IndexPath == "" {
		t.Fatalf("result = %+v, want non-empty workspace, config, and index paths", payload.Result)
	}
	if !payload.Result.IndexExists {
		t.Fatalf("result = %+v, want index_exists=true", payload.Result)
	}
	if got, want := payload.Result.Freshness.State, "fresh"; got != want {
		t.Fatalf("result.freshness.state = %q, want %q", got, want)
	}
	if got, want := payload.Result.ConfigResolution.SelectedBy, configSourceDiscovery; got != want {
		t.Fatalf("config_resolution.selected_by = %q, want %q", got, want)
	}
	if payload.Result.ConfigResolution.Reason == "" || len(payload.Result.ConfigResolution.Candidates) < 4 {
		t.Fatalf("config_resolution = %+v, want reason and candidates", payload.Result.ConfigResolution)
	}
	if payload.Result.ArtifactLocations.IndexDir == "" ||
		payload.Result.ArtifactLocations.DiscoverConfigPath == "" ||
		payload.Result.ArtifactLocations.CanonicalizeBundleRoot == "" {
		t.Fatalf("artifact_locations = %+v, want explicit artifact paths", payload.Result.ArtifactLocations)
	}
	if len(payload.Result.ArtifactLocations.IgnorePatterns) == 0 || payload.Result.ArtifactLocations.IgnorePatterns[0] != ".pituitary/" {
		t.Fatalf("artifact_locations.ignore_patterns = %v, want .pituitary/", payload.Result.ArtifactLocations.IgnorePatterns)
	}
	if len(payload.Result.ArtifactLocations.RelocationHints) < 3 {
		t.Fatalf("artifact_locations.relocation_hints = %v, want relocation guidance", payload.Result.ArtifactLocations.RelocationHints)
	}
	if len(payload.Result.Guidance) != 1 || !strings.Contains(payload.Result.Guidance[0], `runtime.embedder is still "fixture" on 5 indexed artifact(s)`) {
		t.Fatalf("result.guidance = %v, want fixture guidance", payload.Result.Guidance)
	}
	if payload.Result.SpecCount != 3 || payload.Result.DocCount != 2 || payload.Result.ChunkCount != 17 {
		t.Fatalf("result = %+v, want 3 specs, 2 docs, 17 chunks", payload.Result)
	}
}

func TestRunStatusReportsStaleIndexFreshness(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var rebuildStdout bytes.Buffer
	var rebuildStderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &rebuildStdout, &rebuildStderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, rebuildStderr.String())
	}
	mustWriteFileCmd(t, filepath.Join(repo, "docs", "guides", "api-rate-limits.md"), `
# API Rate Limits

This guide changed after indexing.
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
		return runStatus(nil, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "index freshness: stale") {
		t.Fatalf("runStatus() output %q does not report stale freshness", out)
	}
	if !strings.Contains(out, "index content fingerprint") {
		t.Fatalf("runStatus() output %q does not contain content fingerprint reason", out)
	}
	if !strings.Contains(out, "run `pituitary index --rebuild`") {
		t.Fatalf("runStatus() output %q does not contain rebuild guidance", out)
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
	if got, want := payload.Errors[0].Details["runtime"], "runtime.embedder"; got != want {
		t.Fatalf("errors[0].details.runtime = %#v, want %q", got, want)
	}
	if got, want := payload.Errors[0].Details["request_type"], "embeddings"; got != want {
		t.Fatalf("errors[0].details.request_type = %#v, want %q", got, want)
	}
	if got, want := payload.Errors[0].Details["failure_class"], "dependency_unavailable"; got != want {
		t.Fatalf("errors[0].details.failure_class = %#v, want %q", got, want)
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

func TestRunStatusJSONExplainsShadowedDiscoveredConfig(t *testing.T) {
	repo := t.TempDir()
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks(%q) error = %v", repo, err)
	}
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))
	mustMkdirAllCmd(t, filepath.Join(repo, ".pituitary"))
	mustWriteIndexFixture(t, filepath.Join(repo, ".pituitary"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runStatus([]string{"--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			ConfigPath       string `json:"config_path"`
			ConfigResolution struct {
				SelectedBy string `json:"selected_by"`
				Reason     string `json:"reason"`
				Candidates []struct {
					Path   string `json:"path"`
					Status string `json:"status"`
				} `json:"candidates"`
			} `json:"config_resolution"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if got, want := payload.Result.ConfigPath, filepath.Join(resolvedRepo, ".pituitary", "pituitary.toml"); got != want {
		t.Fatalf("config_path = %q, want %q", got, want)
	}
	if got, want := payload.Result.ConfigResolution.SelectedBy, configSourceDiscovery; got != want {
		t.Fatalf("selected_by = %q, want %q", got, want)
	}
	var foundShadowed bool
	for _, candidate := range payload.Result.ConfigResolution.Candidates {
		if candidate.Path == filepath.Join(resolvedRepo, "pituitary.toml") && candidate.Status == "shadowed" {
			foundShadowed = true
			break
		}
	}
	if !foundShadowed {
		t.Fatalf("candidates = %+v, want shadowed root config", payload.Result.ConfigResolution.Candidates)
	}
	if !strings.Contains(payload.Result.ConfigResolution.Reason, filepath.ToSlash(filepath.Join(resolvedRepo, "pituitary.toml"))) {
		t.Fatalf("reason = %q, want shadowed root config path", payload.Result.ConfigResolution.Reason)
	}
}
