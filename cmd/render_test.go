package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestRenderPreviewSourcesResultIncludesFiles(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	renderPreviewSourcesResult(&stdout, &source.PreviewResult{
		Sources: []source.SourcePreview{
			{
				Name:      "docs",
				Kind:      "markdown_docs",
				Path:      ".",
				Files:     []string{"docs/guides/api-rate-limits.md"},
				ItemCount: 1,
				Items: []source.PreviewItem{
					{
						ArtifactKind: "doc",
						Path:         "docs/guides/api-rate-limits.md",
					},
				},
			},
		},
	})

	output := stdout.String()
	if !strings.Contains(output, "files: docs/guides/api-rate-limits.md") {
		t.Fatalf("renderPreviewSourcesResult() output %q does not contain files selector", output)
	}
}

func TestRenderCommandTableSearchSpecs(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer

	err := renderCommandTable(&stdout, "search-specs", &index.SearchSpecResult{
		Matches: []index.SearchSpecMatch{
			{
				Ref:            "SPEC-042",
				Title:          "Tenant-aware rate limiting",
				SectionHeading: "Per-tenant quotas",
				Score:          0.9876,
			},
		},
	})
	if err != nil {
		t.Fatalf("renderCommandTable() error = %v, want nil", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"pituitary search-specs: search spec sections semantically",
		"REF",
		"TITLE",
		"SECTION",
		"SCORE",
		"SPEC-042",
		"Tenant-aware rate limiting",
		"Per-tenant quotas",
		"0.988",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderCommandTable() output %q does not contain %q", output, want)
		}
	}
}

func TestRenderCommandMarkdownReviewSpec(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer

	err := renderCommandMarkdown(&stdout, "review-spec", &analysis.ReviewResult{
		SpecRef: "SPEC-042",
		Overlap: &analysis.OverlapResult{
			Recommendation: "proceed_with_supersedes",
			Overlaps: []analysis.OverlapItem{
				{Ref: "SPEC-008", Title: "Legacy Rate Limiting", Relationship: "extends", Score: 0.922},
			},
		},
		Comparison: &analysis.CompareResult{
			Comparison: analysis.Comparison{
				Recommendation: "prefer_spec_042",
				Tradeoffs: []analysis.ComparisonTradeoff{
					{Topic: "scope", Summary: "SPEC-042 uses tenant-scoped limits."},
				},
			},
		},
		Impact: &analysis.AnalyzeImpactResult{
			AffectedSpecs: []analysis.ImpactedSpec{{Ref: "SPEC-055"}},
			AffectedDocs:  []analysis.ImpactedDoc{{Ref: "doc://guides/api-rate-limits"}},
		},
		DocDrift: &analysis.DocDriftResult{
			DriftItems: []analysis.DriftItem{
				{
					DocRef: "doc://guides/api-rate-limits",
					Findings: []analysis.DriftFinding{
						{SpecRef: "SPEC-042", Code: "default_limit_mismatch", Message: "document reports a different default limit", Expected: "200", Observed: "100"},
					},
				},
			},
		},
		DocRemediation: &analysis.DocRemediationResult{
			Items: []analysis.DocRemediationItem{
				{
					DocRef: "doc://guides/api-rate-limits",
					Suggestions: []analysis.DocRemediationSuggestion{
						{
							SpecRef: "SPEC-042",
							Code:    "default_limit_mismatch",
							Summary: "update the documented default rate limit to the accepted value",
							Evidence: analysis.DocRemediationEvidence{
								SpecExcerpt: "Enforce a default limit of 200 requests per minute.",
								DocExcerpt:  "The default limit is 100 requests per minute for each API key.",
							},
							SuggestedEdit: analysis.DocSuggestedEdit{
								Action:  "replace_claim",
								Replace: "100 requests per minute",
								With:    "200 requests per minute",
							},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("renderCommandMarkdown() error = %v, want nil", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"# Review Spec Report",
		"## Overlap",
		"`SPEC-008`",
		"## Comparison",
		"## Impact",
		"`SPEC-055`",
		"## Doc Drift",
		"## Doc Remediation",
		"Suggested edit: replace",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderCommandMarkdown() output %q does not contain %q", output, want)
		}
	}
}
