package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/model"
)

func TestRunReviewSpecWithSpecRefJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runReviewSpec([]string{"--spec-ref", "SPEC-042", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runReviewSpec() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runReviewSpec() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			SpecRef string `json:"spec_ref"`
		} `json:"request"`
		Result struct {
			SpecRef string `json:"spec_ref"`
			Overlap struct {
				Overlaps []struct {
					Ref string `json:"ref"`
				} `json:"overlaps"`
			} `json:"overlap"`
			Comparison struct {
				SpecRefs []string `json:"spec_refs"`
			} `json:"comparison"`
			Impact struct {
				AffectedSpecs []struct {
					Ref string `json:"ref"`
				} `json:"affected_specs"`
			} `json:"impact"`
			DocDrift struct {
				Scope struct {
					Mode string `json:"mode"`
				} `json:"scope"`
				DriftItems []struct {
					DocRef string `json:"doc_ref"`
				} `json:"drift_items"`
			} `json:"doc_drift"`
			DocRemediation struct {
				Items []struct {
					DocRef      string `json:"doc_ref"`
					Suggestions []struct {
						SpecRef string `json:"spec_ref"`
						Code    string `json:"code"`
					} `json:"suggestions"`
				} `json:"items"`
			} `json:"doc_remediation"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal review payload: %v", err)
	}
	if payload.Request.SpecRef != "SPEC-042" || payload.Result.SpecRef != "SPEC-042" {
		t.Fatalf("payload spec refs = request=%+v result=%+v, want SPEC-042", payload.Request, payload.Result)
	}
	if len(payload.Result.Overlap.Overlaps) == 0 || payload.Result.Overlap.Overlaps[0].Ref != "SPEC-008" {
		t.Fatalf("overlap = %+v, want SPEC-008 first", payload.Result.Overlap)
	}
	if len(payload.Result.Comparison.SpecRefs) != 2 {
		t.Fatalf("comparison = %+v, want composed comparison", payload.Result.Comparison)
	}
	if payload.Result.Comparison.SpecRefs[0] != "SPEC-042" || payload.Result.Comparison.SpecRefs[1] != "SPEC-008" {
		t.Fatalf("comparison = %+v, want [SPEC-042 SPEC-008]", payload.Result.Comparison)
	}
	if len(payload.Result.Impact.AffectedSpecs) == 0 || payload.Result.Impact.AffectedSpecs[0].Ref != "SPEC-055" {
		t.Fatalf("impact = %+v, want SPEC-055 impacted", payload.Result.Impact)
	}
	if payload.Result.DocDrift.Scope.Mode != "doc_refs" || len(payload.Result.DocDrift.DriftItems) != 1 || payload.Result.DocDrift.DriftItems[0].DocRef != "doc://guides/api-rate-limits" {
		t.Fatalf("doc_drift = %+v, want targeted guide drift", payload.Result.DocDrift)
	}
	if len(payload.Result.DocRemediation.Items) != 1 || payload.Result.DocRemediation.Items[0].DocRef != "doc://guides/api-rate-limits" {
		t.Fatalf("doc_remediation = %+v, want targeted guide remediation", payload.Result.DocRemediation)
	}
	if len(payload.Result.DocRemediation.Items[0].Suggestions) == 0 || payload.Result.DocRemediation.Items[0].Suggestions[0].SpecRef == "" {
		t.Fatalf("doc_remediation = %+v, want stable remediation suggestions", payload.Result.DocRemediation)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunReviewSpecWithMarkdownContractPathJSON(t *testing.T) {
	repo := writePathFirstWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runReviewSpec([]string{"--path", "rfcs/service-sla.md", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runReviewSpec() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runReviewSpec() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			SpecRef string `json:"spec_ref"`
		} `json:"request"`
		Result struct {
			SpecRef string `json:"spec_ref"`
			Overlap struct {
				Overlaps []struct {
					Ref string `json:"ref"`
				} `json:"overlaps"`
			} `json:"overlap"`
		} `json:"result"`
		Warnings []cliIssue `json:"warnings"`
		Errors   []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal review payload: %v", err)
	}
	if payload.Request.SpecRef != "contract://rfcs/service-sla" || payload.Result.SpecRef != "contract://rfcs/service-sla" {
		t.Fatalf("payload spec refs = request=%q result=%q, want contract://rfcs/service-sla", payload.Request.SpecRef, payload.Result.SpecRef)
	}
	if len(payload.Result.Overlap.Overlaps) == 0 || payload.Result.Overlap.Overlaps[0].Ref != "SPEC-042" {
		t.Fatalf("overlap = %+v, want SPEC-042 first", payload.Result.Overlap)
	}
	if len(payload.Warnings) == 0 || payload.Warnings[0].Code != "low_confidence_inference" {
		t.Fatalf("warnings = %+v, want low_confidence_inference warning", payload.Warnings)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunReviewSpecMarkdown(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runReviewSpec([]string{"--spec-ref", "SPEC-042", "--format", "markdown"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runReviewSpec(--format markdown) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runReviewSpec(--format markdown) wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"# Review Spec Report",
		"## Overlap",
		"`SPEC-008`",
		"## Comparison",
		"## Impact",
		"`SPEC-055`",
		"## Doc Drift",
		"`doc://guides/api-rate-limits`",
		"## Doc Remediation",
		"Suggested edit:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runReviewSpec(--format markdown) output %q does not contain %q", out, want)
		}
	}
}

func TestRunReviewSpecHTML(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runReviewSpec([]string{"--spec-ref", "SPEC-042", "--format", "html"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runReviewSpec(--format html) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runReviewSpec(--format html) wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"<!doctype html>",
		"<title>Pituitary Review Report: SPEC-042</title>",
		"<h2>Summary</h2>",
		"<h2>Recommended Next Actions</h2>",
		"<h2>Doc Drift</h2>",
		"<h2>Doc Remediation</h2>",
		"doc://guides/api-rate-limits",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runReviewSpec(--format html) output %q does not contain %q", out, want)
		}
	}
}

func TestRunReviewSpecTextIncludesTopImpactSummaries(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runReviewSpec([]string{"--spec-ref", "SPEC-042"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runReviewSpec() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runReviewSpec() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"━━◈ review-spec · SPEC-042",
		"IMPACT    2 specs · 2 refs · 2 docs",
		"SPEC-055",
		"doc://guides/api-rate-limits",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runReviewSpec() output %q does not contain %q", out, want)
		}
	}
}

func TestRunReviewSpecWithSpecRecordFileJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	record := model.SpecRecord{
		Ref:        "SPEC-900",
		Kind:       model.ArtifactKindSpec,
		Title:      "Draft Rate Limit Update",
		Status:     model.StatusDraft,
		Domain:     "api",
		SourceRef:  "file://drafts/spec-900/spec.toml",
		BodyFormat: model.BodyFormatMarkdown,
		BodyText: strings.TrimSpace(`
## Overview

This draft updates public API rate limiting.

## Requirements

- Apply limits per tenant rather than per API key.
- Enforce a default limit of 200 requests per minute.
- Allow tenant-specific overrides through configuration.

## Design Decisions

- Use a sliding-window limiter rather than a fixed-window counter.
- Keep the shared middleware path but load tenant-specific limits.
`),
		AppliesTo: []string{
			"code://src/api/middleware/ratelimiter.go",
			"config://src/api/config/limits.yaml",
		},
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "draft-spec.json"), data, 0o644); err != nil {
		t.Fatalf("write draft-spec.json: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runReviewSpec([]string{"--spec-record-file", "draft-spec.json", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runReviewSpec() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runReviewSpec() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			SpecRef string `json:"spec_ref"`
			Overlap struct {
				Overlaps []struct {
					Ref string `json:"ref"`
				} `json:"overlaps"`
			} `json:"overlap"`
			Comparison struct {
				SpecRefs []string `json:"spec_refs"`
			} `json:"comparison"`
			Impact struct {
				AffectedDocs []struct {
					Ref string `json:"ref"`
				} `json:"affected_docs"`
			} `json:"impact"`
			DocDrift struct {
				Scope struct {
					Mode string `json:"mode"`
				} `json:"scope"`
			} `json:"doc_drift"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal draft review payload: %v", err)
	}
	if payload.Result.SpecRef != "SPEC-900" {
		t.Fatalf("spec_ref = %q, want SPEC-900", payload.Result.SpecRef)
	}
	if len(payload.Result.Overlap.Overlaps) == 0 || payload.Result.Overlap.Overlaps[0].Ref != "SPEC-042" {
		t.Fatalf("overlap = %+v, want SPEC-042 first", payload.Result.Overlap)
	}
	if len(payload.Result.Comparison.SpecRefs) != 2 || payload.Result.Comparison.SpecRefs[0] != "SPEC-900" {
		t.Fatalf("comparison = %+v, want draft candidate first", payload.Result.Comparison)
	}
	if payload.Result.Comparison.SpecRefs[1] != "SPEC-042" {
		t.Fatalf("comparison = %+v, want [SPEC-900 SPEC-042]", payload.Result.Comparison)
	}
	if len(payload.Result.Impact.AffectedDocs) == 0 {
		t.Fatalf("impact = %+v, want targeted docs", payload.Result.Impact)
	}
	if payload.Result.DocDrift.Scope.Mode != "doc_refs" {
		t.Fatalf("doc_drift = %+v, want targeted doc_refs scope", payload.Result.DocDrift)
	}
}

func TestRunReviewSpecWithSpecRecordFromStdinJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	record := model.SpecRecord{
		Ref:        "SPEC-900",
		Kind:       model.ArtifactKindSpec,
		Title:      "Draft Rate Limit Update",
		Status:     model.StatusDraft,
		Domain:     "api",
		SourceRef:  "file://drafts/spec-900/spec.toml",
		BodyFormat: model.BodyFormatMarkdown,
		BodyText: strings.TrimSpace(`
## Overview

This draft updates public API rate limiting.

## Requirements

- Apply limits per tenant rather than per API key.
- Enforce a default limit of 200 requests per minute.
- Allow tenant-specific overrides through configuration.

## Design Decisions

- Use a sliding-window limiter rather than a fixed-window counter.
- Keep the shared middleware path but load tenant-specific limits.
`),
		AppliesTo: []string{
			"code://src/api/middleware/ratelimiter.go",
			"config://src/api/config/limits.yaml",
		},
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	oldStdin := cliStdin
	cliStdin = bytes.NewReader(data)
	t.Cleanup(func() {
		cliStdin = oldStdin
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runReviewSpec([]string{"--spec-record-file", "-", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runReviewSpec() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runReviewSpec() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			SpecRef string `json:"spec_ref"`
			Overlap struct {
				Overlaps []struct {
					Ref string `json:"ref"`
				} `json:"overlaps"`
			} `json:"overlap"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal stdin review payload: %v", err)
	}
	if payload.Result.SpecRef != "SPEC-900" {
		t.Fatalf("spec_ref = %q, want SPEC-900", payload.Result.SpecRef)
	}
	if len(payload.Result.Overlap.Overlaps) == 0 || payload.Result.Overlap.Overlaps[0].Ref != "SPEC-042" {
		t.Fatalf("overlap = %+v, want SPEC-042 first", payload.Result.Overlap)
	}
}
