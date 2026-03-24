package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/runtimeprobe"
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

func TestRenderExplainFileResultIncludesInference(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	renderExplainFileResult(&stdout, &source.ExplainFileResult{
		AbsolutePath:  "/tmp/repo/rfcs/service-sla.md",
		WorkspacePath: "rfcs/service-sla.md",
		Summary: source.ExplainFileSummary{
			Status:    "indexed",
			IndexedBy: []string{"contracts"},
		},
		Sources: []source.SourceFileExplanation{
			{
				Name:            "contracts",
				Kind:            "markdown_contract",
				Path:            "rfcs",
				RelativePath:    "service-sla.md",
				UnderSourceRoot: true,
				Selected:        true,
				ArtifactKind:    "spec",
				Reason:          "indexed_markdown_contract",
				InferredSpec: &source.ExplainedInferredSpec{
					Ref:    "contract://rfcs/service-sla",
					Title:  "Service SLA",
					Status: "draft",
				},
			},
		},
	})

	output := stdout.String()
	for _, want := range []string{
		"summary: indexed",
		"indexed by: contracts",
		"indexed_markdown_contract",
		"inferred ref: contract://rfcs/service-sla",
		"inferred title: Service SLA",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderExplainFileResult() output %q does not contain %q", output, want)
		}
	}
}

func TestRenderStatusResultIncludesRuntimeProbe(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	renderStatusResult(&stdout, &statusResult{
		WorkspaceRoot: "/tmp/repo",
		ConfigPath:    "/tmp/repo/pituitary.toml",
		ConfigResolution: &configResolution{
			SelectedBy: configSourceDiscovery,
			Reason:     "working-directory search found /tmp/repo/pituitary.toml",
			Candidates: []configResolutionCandidate{
				{Precedence: 1, Source: configSourceCommandFlag, Status: "not_set", Detail: "command-local --config was not provided"},
				{Precedence: 2, Source: configSourceGlobalFlag, Status: "not_set", Detail: "global --config was not provided"},
				{Precedence: 3, Source: configSourceEnv, Status: "not_set", Detail: "PITUITARY_CONFIG is not set"},
				{Precedence: 4, Source: configSourceDiscovery, Status: "selected", Path: "/tmp/repo/pituitary.toml", Detail: "found during working-directory search in /tmp/repo"},
			},
		},
		IndexPath:   "/tmp/repo/.pituitary/pituitary.db",
		IndexExists: true,
		SpecCount:   3,
		DocCount:    2,
		ChunkCount:  17,
		ArtifactLocations: &statusArtifactLocation{
			IndexDir:               "/tmp/repo/.pituitary",
			DiscoverConfigPath:     "/tmp/repo/.pituitary/pituitary.toml",
			CanonicalizeBundleRoot: "/tmp/repo/.pituitary/canonicalized",
			IgnorePatterns:         []string{".pituitary/"},
			RelocationHints: []string{
				"set [workspace].index_path to move the SQLite index",
				"use `pituitary discover --config-path PATH --write` to place generated config elsewhere",
				"use `pituitary canonicalize --bundle-dir PATH` to place generated bundles elsewhere",
			},
		},
		Runtime: &runtimeprobe.Result{
			Scope: "all",
			Checks: []runtimeprobe.Check{
				{
					Name:     "runtime.embedder",
					Provider: "openai_compatible",
					Model:    "pituitary-embed",
					Endpoint: "http://localhost:1234/v1",
					Status:   "ready",
				},
				{
					Name:     "runtime.analysis",
					Provider: "disabled",
					Status:   "disabled",
					Message:  "runtime.analysis is disabled in config",
				},
			},
		},
	})

	output := stdout.String()
	for _, want := range []string{
		"workspace: /tmp/repo",
		"config resolution: working-directory search found /tmp/repo/pituitary.toml",
		"config candidates:",
		"1. command-local --config | not_set",
		"4. working-directory search | selected | /tmp/repo/pituitary.toml",
		"index: present",
		"artifact index dir: /tmp/repo/.pituitary",
		"artifact discover --write default: /tmp/repo/.pituitary/pituitary.toml",
		"artifact canonicalize default: /tmp/repo/.pituitary/canonicalized",
		"artifact ignore patterns: .pituitary/",
		"artifact relocation: set [workspace].index_path to move the SQLite index",
		"runtime probe: all",
		"runtime: runtime.embedder | ready | provider: openai_compatible | model: pituitary-embed | endpoint: http://localhost:1234/v1",
		"runtime: runtime.analysis | disabled | provider: disabled",
		"runtime note: runtime.analysis is disabled in config",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderStatusResult() output %q does not contain %q", output, want)
		}
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

func TestRenderTerminologyAuditResultIncludesEvidence(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	renderTerminologyAuditResult(&stdout, &analysis.TerminologyAuditResult{
		Scope: analysis.TerminologyAuditScope{
			Mode:          "spec_ref",
			ArtifactKinds: []string{"doc", "spec"},
			SpecRef:       "SPEC-LOCALITY",
		},
		Terms:          []string{"repo", "workflow"},
		CanonicalTerms: []string{"locality", "continuity"},
		AnchorSpecs: []analysis.TerminologyAnchorSpec{
			{Ref: "SPEC-LOCALITY", Title: "Kernel Locality Contract", Status: "accepted"},
		},
		Findings: []analysis.TerminologyFinding{
			{
				Ref:       "doc://guides/repo-kernel",
				Kind:      "doc",
				Title:     "Repo Kernel Guide",
				SourceRef: "docs/guides/repo-kernel.md",
				Terms:     []string{"repo", "workflow"},
				Sections: []analysis.TerminologySectionFinding{
					{
						Section: "Core Model",
						Terms:   []string{"repo"},
						Excerpt: "The kernel keeps workflow continuity in each repo.",
						Evidence: &analysis.TerminologyEvidence{
							SpecRef: "SPEC-LOCALITY",
							Section: "Core Model",
							Excerpt: "The kernel keeps continuity in clone-local state.",
							Score:   0.812,
						},
					},
				},
			},
		},
	})

	output := stdout.String()
	for _, want := range []string{
		"scope: spec_ref",
		"artifact kinds: doc, spec",
		"anchor spec: SPEC-LOCALITY",
		"terms: repo, workflow",
		"canonical terms: locality, continuity",
		"evidence specs: SPEC-LOCALITY",
		"doc://guides/repo-kernel | doc | Repo Kernel Guide | terms: repo, workflow",
		"evidence: SPEC-LOCALITY | Core Model | 0.812",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderTerminologyAuditResult() output %q does not contain %q", output, want)
		}
	}
}

func TestRenderDocDriftResultIncludesEvidenceAndConfidence(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	renderDocDriftResult(&stdout, &analysis.DocDriftResult{
		Assessments: []analysis.DocDriftAssessment{
			{
				DocRef:    "doc://guides/api-rate-limits",
				Title:     "API Rate Limits",
				SourceRef: "docs/guides/api-rate-limits.md",
				Status:    "drift",
				SpecRefs:  []string{"SPEC-042"},
				Rationale: "accepted spec sets 200 requests per minute, but the doc still states 100 requests per minute",
				Evidence: &analysis.DriftEvidence{
					SpecRef:     "SPEC-042",
					SpecSection: "Defaults",
					SpecExcerpt: "The default rate limit is 200 requests per minute.",
					DocSection:  "Quickstart",
					DocExcerpt:  "The default rate limit is 100 requests per minute.",
				},
				Confidence: &analysis.DriftConfidence{
					Level: "high",
					Score: 0.911,
					Basis: "finding is backed by explicit doc/spec excerpts that also align semantically",
				},
			},
			{
				DocRef:    "doc://runbooks/rate-limit-rollout",
				Title:     "Rate Limit Rollout",
				SourceRef: "docs/runbooks/rate-limit-rollout.md",
				Status:    "aligned",
				SpecRefs:  []string{"SPEC-042"},
				Rationale: "matched accepted spec SPEC-042 and found no deterministic contradiction in the reviewed sections",
				Evidence: &analysis.DriftEvidence{
					SpecRef:     "SPEC-042",
					SpecSection: "Rollout",
					SpecExcerpt: "Rollout steps should keep tenant-scoped defaults intact.",
					DocSection:  "Rollout",
					DocExcerpt:  "Rollout steps keep tenant-scoped defaults intact.",
				},
				Confidence: &analysis.DriftConfidence{
					Level: "medium",
					Score: 0.744,
					Basis: "nearest accepted spec and doc sections agree semantically, but no explicit contradiction was detected",
				},
			},
		},
		DriftItems: []analysis.DriftItem{
			{
				DocRef:    "doc://guides/api-rate-limits",
				Title:     "API Rate Limits",
				SourceRef: "docs/guides/api-rate-limits.md",
				SpecRefs:  []string{"SPEC-042"},
				Findings: []analysis.DriftFinding{
					{
						SpecRef:   "SPEC-042",
						Code:      "default_limit_mismatch",
						Message:   "document reports a different default limit",
						Rationale: "accepted spec sets 200 requests per minute, but the doc still states 100 requests per minute",
						Expected:  "200",
						Observed:  "100",
						Evidence: &analysis.DriftEvidence{
							SpecRef:     "SPEC-042",
							SpecSection: "Defaults",
							SpecExcerpt: "The default rate limit is 200 requests per minute.",
							DocSection:  "Quickstart",
							DocExcerpt:  "The default rate limit is 100 requests per minute.",
						},
						Confidence: &analysis.DriftConfidence{
							Level: "high",
							Score: 0.911,
							Basis: "finding is backed by explicit doc/spec excerpts that also align semantically",
						},
					},
				},
			},
		},
		Remediation: &analysis.DocRemediationResult{
			Items: []analysis.DocRemediationItem{
				{
					DocRef:    "doc://guides/api-rate-limits",
					Title:     "API Rate Limits",
					SourceRef: "docs/guides/api-rate-limits.md",
					Suggestions: []analysis.DocRemediationSuggestion{
						{
							SpecRef: "SPEC-042",
							Code:    "default_limit_mismatch",
							Summary: "update the documented default rate limit to the accepted value",
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

	output := stdout.String()
	for _, want := range []string{
		"status: drift | confidence: high (0.911)",
		"status: aligned | confidence: medium (0.744)",
		"rationale:",
		"spec evidence: SPEC-042 | Defaults",
		"doc evidence: Quickstart",
		"finding: default_limit_mismatch | confidence: high",
		"confidence basis:",
		"suggested edit: replace \"100 requests per minute\" with \"200 requests per minute\"",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderDocDriftResult() output %q does not contain %q", output, want)
		}
	}
}

func TestRenderReviewResultIncludesTopImpactSummaries(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	renderReviewResult(&stdout, &analysis.ReviewResult{
		SpecRef: "SPEC-042",
		Impact: &analysis.AnalyzeImpactResult{
			AffectedSpecs: []analysis.ImpactedSpec{
				{Ref: "SPEC-055", Title: "Tenant Overrides Rollout", Relationship: "depends_on"},
				{Ref: "SPEC-008", Title: "Legacy Rate Limiting", Relationship: "supersedes", Historical: true},
			},
			AffectedDocs: []analysis.ImpactedDoc{
				{Ref: "doc://guides/api-rate-limits", Title: "API Rate Limits", SourceRef: "file://docs/guides/api-rate-limits.md", Score: 0.912},
				{Ref: "doc://runbooks/rate-limit-rollout", Title: "Rate Limit Rollout", SourceRef: "file://docs/runbooks/rate-limit-rollout.md", Score: 0.701},
			},
		},
	})

	output := stdout.String()
	for _, want := range []string{
		"impact: 2 spec(s), 0 ref(s), 2 doc(s)",
		"top impacted specs:",
		"- SPEC-055 | Tenant Overrides Rollout | depends_on",
		"- SPEC-008 | Legacy Rate Limiting | supersedes | historical",
		"top impacted docs:",
		"- doc://guides/api-rate-limits | API Rate Limits | 0.912 | file://docs/guides/api-rate-limits.md",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderReviewResult() output %q does not contain %q", output, want)
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
			AffectedSpecs: []analysis.ImpactedSpec{{Ref: "SPEC-055", Title: "Tenant Overrides Rollout", Relationship: "depends_on"}},
			AffectedDocs:  []analysis.ImpactedDoc{{Ref: "doc://guides/api-rate-limits", Title: "API Rate Limits", SourceRef: "file://docs/guides/api-rate-limits.md", Score: 0.912}},
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
		"Top impacted specs",
		"`SPEC-055` Tenant Overrides Rollout (depends_on)",
		"Top impacted docs",
		"`doc://guides/api-rate-limits` API Rate Limits (score 0.912, file://docs/guides/api-rate-limits.md)",
		"## Doc Drift",
		"## Doc Remediation",
		"Suggested edit: replace",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderCommandMarkdown() output %q does not contain %q", output, want)
		}
	}
}
