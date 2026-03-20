package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dusk-network/pituitary/internal/analysis"
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
