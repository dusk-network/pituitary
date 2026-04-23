package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRunBenchmarksOnFixtureWorkspace(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	report, err := runBenchmarks(context.Background(), filepath.Join(root, "pituitary.toml"), filepath.Join(root, "testdata", "bench"))
	if err != nil {
		t.Fatalf("runBenchmarks() error = %v", err)
	}
	if got, want := report.Summary.TotalCases, 4; got != want {
		t.Fatalf("summary.total_cases = %d, want %d", got, want)
	}
	if got, want := report.Summary.PassedCases, 4; got != want {
		t.Fatalf("summary.passed_cases = %d, want %d", got, want)
	}
	if got, want := report.Summary.CasesUsingAnalysisRuntime, 0; got != want {
		t.Fatalf("summary.cases_using_analysis_runtime = %d, want %d", got, want)
	}
}

func TestRunBenchmarksCapturesAnalysisRuntimeTraffic(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	workspace := t.TempDir()
	copyTree(t, filepath.Join(root, "specs", "rate-limit-legacy"), filepath.Join(workspace, "specs", "rate-limit-legacy"))
	copyTree(t, filepath.Join(root, "specs", "rate-limit-v2"), filepath.Join(workspace, "specs", "rate-limit-v2"))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": `{
							"shared_scope": ["domain:api"],
							"differences": [
								{"spec_ref": "SPEC-008", "items": ["Uses a fixed window."]},
								{"spec_ref": "SPEC-042", "items": ["Uses tenant-scoped overrides."]}
							],
							"tradeoffs": [{"topic": "migration", "summary": "SPEC-042 is the accepted successor."}],
							"compatibility": {"level": "superseding", "summary": "SPEC-042 replaces SPEC-008."},
							"recommendation": "prefer SPEC-042 as the accepted successor"
						}`,
					},
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(upstream.Close)

	configPath := filepath.Join(workspace, "pituitary.toml")
	mustWriteFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[runtime.analysis]
provider = "openai_compatible"
model = "pituitary-analysis"
endpoint = "`+upstream.URL+`"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	casesDir := filepath.Join(workspace, "cases")
	if err := os.MkdirAll(casesDir, 0o755); err != nil {
		t.Fatalf("mkdir cases dir: %v", err)
	}
	mustWriteFile(t, filepath.Join(casesDir, "compare.json"), `{
  "id": "compare-runtime-capture",
  "operation": "compare-specs",
  "request": {
    "spec_refs": ["SPEC-008", "SPEC-042"]
  },
  "expectations": {
    "must_include_spec_refs": ["SPEC-008", "SPEC-042"],
    "compatibility_level": "superseding",
    "recommendation_contains": ["SPEC-042"]
  }
}`)

	report, err := runBenchmarks(context.Background(), configPath, casesDir)
	if err != nil {
		t.Fatalf("runBenchmarks() error = %v", err)
	}
	if got, want := report.Summary.TotalCases, 1; got != want {
		t.Fatalf("summary.total_cases = %d, want %d", got, want)
	}
	if got, want := report.Summary.CasesUsingAnalysisRuntime, 1; got != want {
		t.Fatalf("summary.cases_using_analysis_runtime = %d, want %d", got, want)
	}
	caseResult := report.Cases[0]
	if !caseResult.Passed {
		t.Fatalf("case result = %+v, want pass", caseResult)
	}
	if caseResult.PromptSizeBytes == nil || *caseResult.PromptSizeBytes == 0 {
		t.Fatalf("prompt_size_bytes = %+v, want measured prompt bytes", caseResult.PromptSizeBytes)
	}
	if caseResult.ResponseSizeBytes == nil || *caseResult.ResponseSizeBytes == 0 {
		t.Fatalf("response_size_bytes = %+v, want measured response bytes", caseResult.ResponseSizeBytes)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	if err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}); err != nil {
		t.Fatalf("copyTree(%s, %s): %v", src, dst, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
