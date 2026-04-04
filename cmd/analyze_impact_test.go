package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAnalyzeImpactJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runAnalyzeImpact([]string{"--spec-ref", "SPEC-042", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runAnalyzeImpact() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runAnalyzeImpact() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			SpecRef    string `json:"spec_ref"`
			ChangeType string `json:"change_type"`
		} `json:"request"`
		Result struct {
			AffectedSpecs []struct {
				Ref string `json:"ref"`
			} `json:"affected_specs"`
			AffectedRefs []struct {
				Ref string `json:"ref"`
			} `json:"affected_refs"`
			AffectedDocs []struct {
				Ref            string `json:"ref"`
				Classification string `json:"classification"`
				Evidence       struct {
					SpecSourceRef string `json:"spec_source_ref"`
					DocSourceRef  string `json:"doc_source_ref"`
					LinkReason    string `json:"link_reason"`
				} `json:"evidence"`
				SuggestedTargets []struct {
					SourceRef string `json:"source_ref"`
					Section   string `json:"section"`
				} `json:"suggested_targets"`
			} `json:"affected_docs"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal impact payload: %v", err)
	}
	if payload.Request.SpecRef != "SPEC-042" || payload.Request.ChangeType != "accepted" {
		t.Fatalf("request = %+v, want SPEC-042/accepted", payload.Request)
	}
	if len(payload.Result.AffectedSpecs) == 0 || payload.Result.AffectedSpecs[0].Ref != "SPEC-055" {
		t.Fatalf("affected_specs = %+v, want SPEC-055 first", payload.Result.AffectedSpecs)
	}
	if len(payload.Result.AffectedRefs) == 0 || len(payload.Result.AffectedDocs) == 0 {
		t.Fatalf("impact result missing refs/docs: %+v", payload.Result)
	}
	foundStructuredDoc := false
	for _, doc := range payload.Result.AffectedDocs {
		if doc.Ref == "" {
			continue
		}
		if doc.Classification != "" && doc.Evidence.SpecSourceRef != "" && doc.Evidence.DocSourceRef != "" && doc.Evidence.LinkReason != "" && len(doc.SuggestedTargets) > 0 && doc.SuggestedTargets[0].Section != "" {
			foundStructuredDoc = true
			break
		}
	}
	if !foundStructuredDoc {
		t.Fatalf("affected_docs = %+v, want structured evidence and suggested targets", payload.Result.AffectedDocs)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunAnalyzeImpactWithPathJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runAnalyzeImpact([]string{"--path", "specs/rate-limit-v2/body.md", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runAnalyzeImpact() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runAnalyzeImpact() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			SpecRef string `json:"spec_ref"`
		} `json:"request"`
		Result struct {
			SpecRef string `json:"spec_ref"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal impact payload: %v", err)
	}
	if payload.Request.SpecRef != "SPEC-042" || payload.Result.SpecRef != "SPEC-042" {
		t.Fatalf("payload spec_ref = request=%q result=%q, want SPEC-042", payload.Request.SpecRef, payload.Result.SpecRef)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunAnalyzeImpactWithWorkspaceRelativePathFromSubdirectory(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return withWorkingDir(t, filepath.Join(repo, "docs"), func() int {
			return runAnalyzeImpact([]string{"--path", "specs/rate-limit-v2/body.md", "--format", "json"}, &stdout, &stderr)
		})
	})
	if exitCode != 0 {
		t.Fatalf("runAnalyzeImpact() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runAnalyzeImpact() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			SpecRef string `json:"spec_ref"`
		} `json:"request"`
		Result struct {
			SpecRef string `json:"spec_ref"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal impact payload: %v", err)
	}
	if payload.Request.SpecRef != "SPEC-042" || payload.Result.SpecRef != "SPEC-042" {
		t.Fatalf("payload spec_ref = request=%q result=%q, want SPEC-042", payload.Request.SpecRef, payload.Result.SpecRef)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunAnalyzeImpactWarnsOnWeakInferredMetadata(t *testing.T) {
	repo := writePathFirstWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runAnalyzeImpact([]string{"--path", "rfcs/service-sla.md", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runAnalyzeImpact() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runAnalyzeImpact() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Warnings []cliIssue `json:"warnings"`
		Errors   []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal impact payload: %v", err)
	}
	if len(payload.Warnings) == 0 || payload.Warnings[0].Code != "low_confidence_inference" {
		t.Fatalf("warnings = %+v, want low_confidence_inference warning", payload.Warnings)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunAnalyzeImpactWithRequestFileJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)
	mustWriteJSONFileCmd(t, filepath.Join(repo, "impact-request.json"), map[string]any{
		"spec_ref":    "SPEC-042",
		"change_type": "deprecated",
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runAnalyzeImpact([]string{"--request-file", "impact-request.json", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runAnalyzeImpact() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runAnalyzeImpact() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			SpecRef    string `json:"spec_ref"`
			ChangeType string `json:"change_type"`
		} `json:"request"`
		Result struct {
			ChangeType string `json:"change_type"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal impact request-file payload: %v", err)
	}
	if payload.Request.SpecRef != "SPEC-042" || payload.Request.ChangeType != "deprecated" || payload.Result.ChangeType != "deprecated" {
		t.Fatalf("payload = %+v, want request-file change type propagated", payload)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunAnalyzeImpactTextIncludesCrossRepoArtifacts(t *testing.T) {
	repo := writeMultiRepoSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runAnalyzeImpact([]string{"--spec-ref", "SPEC-100"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runAnalyzeImpact() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runAnalyzeImpact() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"affected specs: 1",
		"SPEC-200 | repo: shared | depends_on | Shared Repo Rollout",
		"affected docs:",
		"doc://shared/guides/api-rate-limits | repo: shared | source: docs/guides/api-rate-limits.md",
		"evidence:",
		"target:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runAnalyzeImpact() output %q does not contain %q", out, want)
		}
	}
}

func TestRunAnalyzeImpactSummaryJSONIncludesRankedSummary(t *testing.T) {
	repo := writeMultiRepoSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runAnalyzeImpact([]string{"--spec-ref", "SPEC-100", "--summary", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runAnalyzeImpact() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runAnalyzeImpact() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			SpecRef string `json:"spec_ref"`
			Summary bool   `json:"summary"`
		} `json:"request"`
		Result struct {
			SummaryOnly   bool `json:"summary_only"`
			RankedSummary []struct {
				Rank        int     `json:"rank"`
				Kind        string  `json:"kind"`
				Ref         string  `json:"ref"`
				Repo        string  `json:"repo"`
				Score       float64 `json:"score"`
				Why         string  `json:"why"`
				ReviewFirst string  `json:"review_first"`
			} `json:"ranked_summary"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal impact payload: %v", err)
	}
	if !payload.Request.Summary || payload.Request.SpecRef != "SPEC-100" {
		t.Fatalf("request = %+v, want summary=true for SPEC-100", payload.Request)
	}
	if !payload.Result.SummaryOnly {
		t.Fatalf("result = %+v, want summary_only=true", payload.Result)
	}
	if len(payload.Result.RankedSummary) == 0 {
		t.Fatal("ranked_summary is empty, want prioritized items")
	}
	first := payload.Result.RankedSummary[0]
	if first.Rank != 1 || first.Kind != "spec" || first.Ref != "SPEC-200" || first.Repo != "shared" {
		t.Fatalf("first ranked summary item = %+v, want shared dependent spec first", first)
	}
	foundDoc := false
	for _, item := range payload.Result.RankedSummary {
		if item.Kind == "doc" && item.Ref == "doc://shared/guides/api-rate-limits" && item.ReviewFirst != "" && item.Why != "" {
			foundDoc = true
			break
		}
	}
	if !foundDoc {
		t.Fatalf("ranked_summary = %+v, want shared doc follow-up entry", payload.Result.RankedSummary)
	}
}

func TestRunAnalyzeImpactSummaryTextShowsOnlyRankedItems(t *testing.T) {
	repo := writeMultiRepoSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runAnalyzeImpact([]string{"--spec-ref", "SPEC-100", "--summary"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runAnalyzeImpact() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runAnalyzeImpact() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"ranked summary:",
		"1. spec SPEC-200 | repo: shared",
		"review first:",
		"doc://shared/guides/api-rate-limits",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runAnalyzeImpact(--summary) output %q does not contain %q", out, want)
		}
	}
	for _, unwanted := range []string{
		"affected specs:",
		"affected refs:",
		"affected docs:",
	} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("runAnalyzeImpact(--summary) output %q unexpectedly contains %q", out, unwanted)
		}
	}
}
