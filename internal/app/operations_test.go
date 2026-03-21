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
