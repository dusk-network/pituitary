package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
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

func TestImproveDependencyUnavailableMessageIncludesConfiguredAPIKeyEnv(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Runtime: config.Runtime{
			Embedder: config.RuntimeProvider{APIKeyEnv: "OPENAI_API_KEY"},
		},
	}

	message := improveDependencyUnavailableMessage(cfg, &index.DependencyUnavailableError{
		Message: "missing API key for runtime.embedder",
	})
	if !strings.Contains(message, "OPENAI_API_KEY") {
		t.Fatalf("improveDependencyUnavailableMessage() = %q, want env var name", message)
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

func writeOperationWorkspace(t *testing.T, enableAnalysisProvider bool) string {
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

	var runtimeAnalysis string
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

	configPath := filepath.Join(repo, "pituitary.toml")
	if err := os.WriteFile(configPath, []byte(strings.TrimSpace(`
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0
`+runtimeAnalysis+`
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
