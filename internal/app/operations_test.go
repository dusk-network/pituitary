package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/openaicompat"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestSearchSpecsNormalizesRequestBeforeConfigLoad(t *testing.T) {
	t.Parallel()

	operation := SearchSpecs(context.Background(), filepath.Join(t.TempDir(), "pituitary.toml"), index.SearchSpecRequest{
		Query: "  rate limiting  ",
	})
	if operation.Issue == nil {
		t.Fatal("SearchSpecs() issue = nil, want config error")
	}
	if operation.Issue.Code != CodeConfigError {
		t.Fatalf("SearchSpecs() issue.code = %q, want %q", operation.Issue.Code, CodeConfigError)
	}
	if operation.Request.Query != "rate limiting" {
		t.Fatalf("SearchSpecs() request.query = %q, want normalized query", operation.Request.Query)
	}
	if operation.Request.Limit == nil || *operation.Request.Limit != 10 {
		t.Fatalf("SearchSpecs() request.limit = %#v, want default limit 10", operation.Request.Limit)
	}
	if len(operation.Request.Filters.Statuses) != 3 {
		t.Fatalf("SearchSpecs() request.statuses = %v, want default active statuses", operation.Request.Filters.Statuses)
	}
}

func TestSearchSpecsClassifiesRequestValidationBeforeConfigLoad(t *testing.T) {
	t.Parallel()

	operation := SearchSpecs(context.Background(), filepath.Join(t.TempDir(), "pituitary.toml"), index.SearchSpecRequest{
		Query: "rate limiting",
		Filters: index.SearchSpecFilters{
			Statuses: []string{"broken"},
		},
	})
	if operation.Issue == nil {
		t.Fatal("SearchSpecs() issue = nil, want validation error")
	}
	if operation.Issue.Code != CodeValidationError {
		t.Fatalf("SearchSpecs() issue.code = %q, want %q", operation.Issue.Code, CodeValidationError)
	}
}

func TestAnalyzeImpactClassifiesMissingConfig(t *testing.T) {
	t.Parallel()

	operation := AnalyzeImpact(context.Background(), filepath.Join(t.TempDir(), "pituitary.toml"), analysis.AnalyzeImpactRequest{
		SpecRef:    "SPEC-042",
		ChangeType: "accepted",
	})
	if operation.Issue == nil {
		t.Fatal("AnalyzeImpact() issue = nil, want config error")
	}
	if operation.Issue.Code != CodeConfigError {
		t.Fatalf("AnalyzeImpact() issue.code = %q, want %q", operation.Issue.Code, CodeConfigError)
	}
}

func TestSearchSpecsReportsMissingIndexActionably(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "specs"), 0o755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	configPath := filepath.Join(repo, "pituitary.toml")
	if err := os.WriteFile(configPath, []byte(strings.TrimSpace(`
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	operation := SearchSpecs(context.Background(), configPath, index.SearchSpecRequest{
		Query: "rate limiting",
	})
	if operation.Issue == nil {
		t.Fatal("SearchSpecs() issue = nil, want missing-index error")
	}
	if operation.Issue.Code != CodeConfigError {
		t.Fatalf("SearchSpecs() issue.code = %q, want %q", operation.Issue.Code, CodeConfigError)
	}
	if !strings.Contains(operation.Issue.Message, "pituitary index --rebuild") {
		t.Fatalf("SearchSpecs() issue.message = %q, want rebuild guidance", operation.Issue.Message)
	}
	if !strings.Contains(operation.Issue.Message, filepath.Join(repo, ".pituitary", "pituitary.db")) {
		t.Fatalf("SearchSpecs() issue.message = %q, want resolved index path", operation.Issue.Message)
	}
}

func TestSearchSpecsRejectsStaleIndexBeforeReturningResults(t *testing.T) {
	t.Parallel()

	configPath := writeOperationWorkspace(t, false)
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Workspace.RootPath, "docs", "guides", "api-rate-limits.md"), []byte(strings.TrimSpace(`
# API Rate Limits

This guide changed after indexing.
`)+"\n"), 0o644); err != nil {
		t.Fatalf("write stale doc: %v", err)
	}

	operation := SearchSpecs(context.Background(), configPath, index.SearchSpecRequest{
		Query: "rate limiting",
	})
	if operation.Issue == nil {
		t.Fatal("SearchSpecs() issue = nil, want stale-index error")
	}
	if operation.Issue.Code != CodeConfigError {
		t.Fatalf("SearchSpecs() issue.code = %q, want %q", operation.Issue.Code, CodeConfigError)
	}
	if !strings.Contains(operation.Issue.Message, "index is stale") {
		t.Fatalf("SearchSpecs() issue.message = %q, want stale-index message", operation.Issue.Message)
	}
	if !strings.Contains(operation.Issue.Message, "content fingerprint") {
		t.Fatalf("SearchSpecs() issue.message = %q, want content fingerprint detail", operation.Issue.Message)
	}
	if !strings.Contains(operation.Issue.Message, "pituitary index --rebuild") {
		t.Fatalf("SearchSpecs() issue.message = %q, want rebuild guidance", operation.Issue.Message)
	}
}

func TestClassifyExecutionErrorDefaultsToValidationError(t *testing.T) {
	t.Parallel()

	issue := classifyExecutionError(nil, errors.New("boom"), operationExecutionPolicy{})
	if issue.Code != CodeValidationError {
		t.Fatalf("classifyExecutionError() code = %q, want %q", issue.Code, CodeValidationError)
	}
	if issue.ExitCode != 2 {
		t.Fatalf("classifyExecutionError() exitCode = %d, want 2", issue.ExitCode)
	}
	if issue.Message != "boom" {
		t.Fatalf("classifyExecutionError() message = %q, want raw error", issue.Message)
	}
}

func TestOperationsShareNotFoundClassification(t *testing.T) {
	t.Parallel()

	configPath := writeOperationWorkspace(t, false)

	checks := map[string]*Issue{
		"compare": CompareSpecs(context.Background(), configPath, analysis.CompareRequest{
			SpecRefs: []string{"SPEC-404", "SPEC-042"},
		}).Issue,
		"impact": AnalyzeImpact(context.Background(), configPath, analysis.AnalyzeImpactRequest{
			SpecRef:    "SPEC-404",
			ChangeType: "accepted",
		}).Issue,
		"review": ReviewSpec(context.Background(), configPath, analysis.ReviewRequest{
			SpecRef: "SPEC-404",
		}).Issue,
	}

	for name, issue := range checks {
		if issue == nil {
			t.Fatalf("%s issue = nil, want not_found classification", name)
		}
		if issue.Code != CodeNotFound {
			t.Fatalf("%s issue.code = %q, want %q", name, issue.Code, CodeNotFound)
		}
		if issue.ExitCode != 2 {
			t.Fatalf("%s issue.exitCode = %d, want 2", name, issue.ExitCode)
		}
		if !strings.Contains(issue.Message, "SPEC-404") {
			t.Fatalf("%s issue.message = %q, want missing spec ref detail", name, issue.Message)
		}
	}
}

func TestFormatDependencyUnavailableMessageIncludesConfiguredAPIKeyEnv(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Runtime: config.Runtime{
			Embedder: config.RuntimeProvider{APIKeyEnv: "OPENAI_API_KEY"},
		},
	}

	message := FormatDependencyUnavailableMessage(cfg, &index.DependencyUnavailableError{
		Message: "missing API key for runtime.embedder",
	})
	if !strings.Contains(message, "OPENAI_API_KEY") {
		t.Fatalf("FormatDependencyUnavailableMessage() = %q, want env var name", message)
	}
}

func TestFormatDependencyUnavailableMessageUsesRuntimeSpecificAPIKeyEnv(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Runtime: config.Runtime{
			Embedder: config.RuntimeProvider{APIKeyEnv: "OPENAI_API_KEY"},
			Analysis: config.RuntimeProvider{APIKeyEnv: "PITUITARY_ANALYSIS_KEY"},
		},
	}

	message := FormatDependencyUnavailableMessage(cfg, &index.DependencyUnavailableError{
		Message: "missing API key for runtime.analysis",
	})
	if !strings.Contains(message, "PITUITARY_ANALYSIS_KEY") {
		t.Fatalf("FormatDependencyUnavailableMessage() = %q, want runtime-specific env var", message)
	}
	if strings.Contains(message, "OPENAI_API_KEY") {
		t.Fatalf("FormatDependencyUnavailableMessage() = %q, did not want embedder env var", message)
	}
}

func TestClassifyExecutionErrorCarriesDependencyDiagnostics(t *testing.T) {
	t.Parallel()

	err := openaicompat.NewDependencyUnavailableStatusWithDetails(openaicompat.FailureDetails{
		Runtime:      "runtime.embedder",
		Provider:     config.RuntimeProviderOpenAI,
		Model:        "pituitary-embed",
		Endpoint:     "http://127.0.0.1:1234/v1",
		RequestType:  "embeddings",
		FailureClass: openaicompat.FailureClassServer,
		HTTPStatus:   http.StatusInternalServerError,
		TimeoutMS:    1000,
		MaxRetries:   2,
		BatchSize:    8,
		InputCount:   8,
	}, http.StatusInternalServerError, "runtime.embedder failed")

	issue := classifyExecutionError(nil, err, operationExecutionPolicy{})
	if issue.Code != CodeDependencyUnavailable {
		t.Fatalf("classifyExecutionError() code = %q, want %q", issue.Code, CodeDependencyUnavailable)
	}
	if got, want := issue.Details["runtime"], "runtime.embedder"; got != want {
		t.Fatalf("issue.details.runtime = %#v, want %q", got, want)
	}
	if got, want := issue.Details["provider"], config.RuntimeProviderOpenAI; got != want {
		t.Fatalf("issue.details.provider = %#v, want %q", got, want)
	}
	if got, want := issue.Details["failure_class"], openaicompat.FailureClassServer; got != want {
		t.Fatalf("issue.details.failure_class = %#v, want %q", got, want)
	}
	if got, want := issue.Details["http_status"], http.StatusInternalServerError; got != want {
		t.Fatalf("issue.details.http_status = %#v, want %d", got, want)
	}
}

func TestCompareSpecsClassifiesAnalysisProviderDependencyFailures(t *testing.T) {
	t.Parallel()

	configPath := writeOperationWorkspace(t, true)
	operation := CompareSpecs(context.Background(), configPath, analysis.CompareRequest{
		SpecRefs: []string{"SPEC-008", "SPEC-042"},
	})
	if operation.Issue == nil {
		t.Fatal("CompareSpecs() issue = nil, want dependency error")
	}
	if operation.Issue.Code != CodeDependencyUnavailable {
		t.Fatalf("CompareSpecs() issue.code = %q, want %q", operation.Issue.Code, CodeDependencyUnavailable)
	}
	if !strings.Contains(operation.Issue.Message, "PITUITARY_TEST_ANALYSIS_KEY") {
		t.Fatalf("CompareSpecs() issue.message = %q, want configured env var", operation.Issue.Message)
	}
}

func TestCompareSpecsReportsActionableReachabilityGuidanceForAnalysisProvider(t *testing.T) {
	t.Parallel()

	endpoint := unreachableLocalEndpoint(t)
	configPath := writeOperationWorkspaceWithRuntime(t, "", fmt.Sprintf(`
[runtime.analysis]
provider = "openai_compatible"
model = "pituitary-analysis"
endpoint = %q
timeout_ms = 1000
max_retries = 0
`, endpoint))

	operation := CompareSpecs(context.Background(), configPath, analysis.CompareRequest{
		SpecRefs: []string{"SPEC-008", "SPEC-042"},
	})
	if operation.Issue == nil {
		t.Fatal("CompareSpecs() issue = nil, want dependency error")
	}
	if operation.Issue.Code != CodeDependencyUnavailable {
		t.Fatalf("CompareSpecs() issue.code = %q, want %q", operation.Issue.Code, CodeDependencyUnavailable)
	}
	if operation.Issue.ExitCode != 3 {
		t.Fatalf("CompareSpecs() issue.exitCode = %d, want 3", operation.Issue.ExitCode)
	}
	wantDescriptor := fmt.Sprintf(`runtime.analysis (provider "openai_compatible", model "pituitary-analysis", endpoint %q) is unreachable`, endpoint)
	if !strings.Contains(operation.Issue.Message, wantDescriptor) {
		t.Fatalf("CompareSpecs() issue.message = %q, want runtime descriptor", operation.Issue.Message)
	}
	if !strings.Contains(operation.Issue.Message, "reachable from this machine") {
		t.Fatalf("CompareSpecs() issue.message = %q, want reachability guidance", operation.Issue.Message)
	}
	if !strings.Contains(operation.Issue.Message, "LM Studio") {
		t.Fatalf("CompareSpecs() issue.message = %q, want LM Studio hint", operation.Issue.Message)
	}
	if !strings.Contains(operation.Issue.Message, "Raw provider error:") {
		t.Fatalf("CompareSpecs() issue.message = %q, want raw provider detail", operation.Issue.Message)
	}
}

func TestCheckComplianceUsesExplicitTraceabilityWithoutLiveEmbedder(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("request path = %q, want /v1/embeddings", r.URL.Path)
		}

		var request struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		response := map[string]any{"data": []map[string]any{}}
		for i := range request.Input {
			response["data"] = append(response["data"].([]map[string]any), map[string]any{
				"index":     i,
				"embedding": []float64{float64(i + 1), float64(i + 2), float64(i + 3)},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))

	configPath := writeOperationWorkspaceWithRuntime(t, fmt.Sprintf(`
[runtime.embedder]
provider = "openai_compatible"
model = "pituitary-embed"
endpoint = %q
timeout_ms = 1000
max_retries = 0
`, server.URL+"/v1"), "")
	server.Close()

	repoRoot := filepath.Dir(configPath)
	codePath := filepath.Join(repoRoot, "src", "api", "middleware", "ratelimiter.go")
	if err := os.MkdirAll(filepath.Dir(codePath), 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.WriteFile(codePath, []byte(strings.TrimSpace(`
package middleware

// Apply limits per tenant rather than per API key.
// Enforce a default limit of 200 requests per minute.
// Allow short bursts above the steady-state tenant limit.
// Use a sliding-window limiter and tenant-specific overrides.
func buildLimiter() {}
`)+"\n"), 0o644); err != nil {
		t.Fatalf("write code path: %v", err)
	}

	operation := CheckCompliance(context.Background(), configPath, analysis.ComplianceRequest{
		Paths: []string{"src/api/middleware/ratelimiter.go"},
	})
	if operation.Issue != nil {
		t.Fatalf("CheckCompliance() issue = %+v, want success without live embedder", operation.Issue)
	}
	if operation.Result == nil {
		t.Fatal("CheckCompliance() result = nil, want structured result")
	}
	if len(operation.Result.Conflicts) != 0 {
		t.Fatalf("CheckCompliance() conflicts = %+v, want none", operation.Result.Conflicts)
	}
	if len(operation.Result.Compliant) == 0 {
		t.Fatal("CheckCompliance() compliant = empty, want explicit compliance findings")
	}
}

func TestCheckDocDriftClassifiesAnalysisProviderDependencyFailures(t *testing.T) {
	t.Parallel()

	configPath := writeOperationWorkspace(t, true)
	operation := CheckDocDrift(context.Background(), configPath, analysis.DocDriftRequest{
		Scope: "all",
	})
	if operation.Issue == nil {
		t.Fatal("CheckDocDrift() issue = nil, want dependency error")
	}
	if operation.Issue.Code != CodeDependencyUnavailable {
		t.Fatalf("CheckDocDrift() issue.code = %q, want %q", operation.Issue.Code, CodeDependencyUnavailable)
	}
	if !strings.Contains(operation.Issue.Message, "PITUITARY_TEST_ANALYSIS_KEY") {
		t.Fatalf("CheckDocDrift() issue.message = %q, want configured env var", operation.Issue.Message)
	}
}

func TestSearchSpecsReportsActionableUnloadedModelErrorForOpenAICompatibleEmbedder(t *testing.T) {
	t.Parallel()

	var failSearch atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("request path = %q, want /v1/embeddings", r.URL.Path)
		}
		if failSearch.Load() {
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]any{"error": "Model unloaded.."}); err != nil {
				t.Fatalf("encode failure response: %v", err)
			}
			return
		}

		var request struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		response := map[string]any{"data": []map[string]any{}}
		for i := range request.Input {
			response["data"] = append(response["data"].([]map[string]any), map[string]any{
				"index":     i,
				"embedding": []float64{float64(i + 1), float64(i + 2), float64(i + 3)},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode success response: %v", err)
		}
	}))
	defer server.Close()

	configPath := writeOperationWorkspaceWithRuntime(t, fmt.Sprintf(`
[runtime.embedder]
provider = "openai_compatible"
model = "pituitary-embed"
endpoint = %q
timeout_ms = 1000
max_retries = 0
`, server.URL+"/v1"), "")

	failSearch.Store(true)

	operation := SearchSpecs(context.Background(), configPath, index.SearchSpecRequest{
		Query: "rate limiting",
	})
	if operation.Issue == nil {
		t.Fatal("SearchSpecs() issue = nil, want dependency error")
	}
	if operation.Issue.Code != CodeDependencyUnavailable {
		t.Fatalf("SearchSpecs() issue.code = %q, want %q", operation.Issue.Code, CodeDependencyUnavailable)
	}
	if operation.Issue.ExitCode != 3 {
		t.Fatalf("SearchSpecs() issue.exitCode = %d, want 3", operation.Issue.ExitCode)
	}
	wantDescriptor := fmt.Sprintf(`runtime.embedder (provider "openai_compatible", model "pituitary-embed", endpoint %q) is unavailable because the configured model appears to be unloaded`, server.URL+"/v1")
	if !strings.Contains(operation.Issue.Message, wantDescriptor) {
		t.Fatalf("SearchSpecs() issue.message = %q, want runtime descriptor and unloaded-model guidance", operation.Issue.Message)
	}
	if !strings.Contains(operation.Issue.Message, "load or pin model") {
		t.Fatalf("SearchSpecs() issue.message = %q, want model loading guidance", operation.Issue.Message)
	}
	if !strings.Contains(operation.Issue.Message, "LM Studio") {
		t.Fatalf("SearchSpecs() issue.message = %q, want LM Studio hint", operation.Issue.Message)
	}
	if !strings.Contains(operation.Issue.Message, "Raw provider error:") || !strings.Contains(operation.Issue.Message, "Model unloaded..") {
		t.Fatalf("SearchSpecs() issue.message = %q, want raw provider detail", operation.Issue.Message)
	}
}

func writeOperationWorkspace(t *testing.T, enableAnalysisProvider bool) string {
	t.Helper()

	runtimeAnalysis := ""
	if enableAnalysisProvider {
		runtimeAnalysis = `
[runtime.analysis]
provider = "openai_compatible"
model = "pituitary-analysis"
endpoint = "http://127.0.0.1:9/v1"
api_key_env = "PITUITARY_TEST_ANALYSIS_KEY"
timeout_ms = 1000
max_retries = 0
`
	}

	return writeOperationWorkspaceWithRuntime(t, "", runtimeAnalysis)
}

func writeOperationWorkspaceWithRuntime(t *testing.T, runtimeEmbedder, runtimeAnalysis string) string {
	t.Helper()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "specs", "rate-limit-legacy"), 0o755); err != nil {
		t.Fatalf("mkdir legacy spec: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "specs", "rate-limit-v2"), 0o755); err != nil {
		t.Fatalf("mkdir v2 spec: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "docs", "guides"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	for _, name := range []string{"spec.toml", "body.md"} {
		copyOperationFixtureFile(t, filepath.Join("specs", "rate-limit-legacy", name), filepath.Join(repo, "specs", "rate-limit-legacy", name))
		copyOperationFixtureFile(t, filepath.Join("specs", "rate-limit-v2", name), filepath.Join(repo, "specs", "rate-limit-v2", name))
	}
	copyOperationFixtureFile(t, filepath.Join("docs", "guides", "api-rate-limits.md"), filepath.Join(repo, "docs", "guides", "api-rate-limits.md"))

	if strings.TrimSpace(runtimeEmbedder) == "" {
		runtimeEmbedder = `
[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0
`
	}

	configPath := filepath.Join(repo, "pituitary.toml")
	if err := os.WriteFile(configPath, []byte(strings.TrimSpace(`
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

`+runtimeEmbedder+runtimeAnalysis+`
[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
`)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}
	return configPath
}

func copyOperationFixtureFile(t *testing.T, srcRelative, dst string) {
	t.Helper()

	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	data, err := os.ReadFile(filepath.Join(repoRoot, srcRelative))
	if err != nil {
		t.Fatalf("read fixture %s: %v", srcRelative, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", dst, err)
	}
}

func unreachableLocalEndpoint(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen(): %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("listener.Close(): %v", err)
	}
	return "http://" + addr + "/v1"
}
