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

func TestRunCheckOverlapRejectsSpecRefAndPath(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runCheckOverlap([]string{
			"--spec-ref", "SPEC-042",
			"--path", "some/spec.toml",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runCheckOverlap() exit code = %d, want 2", exitCode)
	}

	var payload struct {
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal mutex payload: %v", err)
	}
	if len(payload.Errors) != 1 || payload.Errors[0].Code != "validation_error" {
		t.Fatalf("errors = %+v, want one validation_error", payload.Errors)
	}
	if !strings.Contains(payload.Errors[0].Message, "exactly one of --path, --spec-ref") {
		t.Fatalf("errors[0].message = %q, want mutex guard message", payload.Errors[0].Message)
	}
}

func TestRunCheckOverlapWithSpecRefJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckOverlap([]string{"--spec-ref", "SPEC-042", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckOverlap() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckOverlap() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			SpecRef string `json:"spec_ref"`
		} `json:"request"`
		Result struct {
			Candidate struct {
				Ref string `json:"ref"`
			} `json:"candidate"`
			Overlaps []struct {
				Ref          string `json:"ref"`
				Relationship string `json:"relationship"`
			} `json:"overlaps"`
			Recommendation string `json:"recommendation"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal overlap payload: %v", err)
	}
	if payload.Request.SpecRef != "SPEC-042" {
		t.Fatalf("request spec_ref = %q, want SPEC-042", payload.Request.SpecRef)
	}
	if payload.Result.Candidate.Ref != "SPEC-042" {
		t.Fatalf("candidate ref = %q, want SPEC-042", payload.Result.Candidate.Ref)
	}
	if len(payload.Result.Overlaps) == 0 || payload.Result.Overlaps[0].Ref != "SPEC-008" {
		t.Fatalf("overlaps = %+v, want SPEC-008 first", payload.Result.Overlaps)
	}
	if payload.Result.Recommendation != "proceed_with_supersedes" {
		t.Fatalf("recommendation = %q, want proceed_with_supersedes", payload.Result.Recommendation)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunCheckOverlapWithPathJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckOverlap([]string{"--path", "specs/rate-limit-v2", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckOverlap() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckOverlap() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			SpecRef string `json:"spec_ref"`
		} `json:"request"`
		Result struct {
			Candidate struct {
				Ref string `json:"ref"`
			} `json:"candidate"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal overlap payload: %v", err)
	}
	if payload.Request.SpecRef != "SPEC-042" {
		t.Fatalf("request spec_ref = %q, want SPEC-042", payload.Request.SpecRef)
	}
	if payload.Result.Candidate.Ref != "SPEC-042" {
		t.Fatalf("candidate ref = %q, want SPEC-042", payload.Result.Candidate.Ref)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunCheckOverlapReportsBoundaryReviewForMatureAcceptedSpecs(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckOverlap([]string{"--path", "specs/burst-handling", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckOverlap() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckOverlap() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Candidate struct {
				Ref string `json:"ref"`
			} `json:"candidate"`
			Overlaps []struct {
				Ref          string `json:"ref"`
				Relationship string `json:"relationship"`
				Guidance     string `json:"guidance"`
			} `json:"overlaps"`
			Recommendation string `json:"recommendation"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal overlap payload: %v", err)
	}
	if got, want := payload.Result.Candidate.Ref, "SPEC-055"; got != want {
		t.Fatalf("candidate ref = %q, want %q", got, want)
	}
	if len(payload.Result.Overlaps) == 0 || payload.Result.Overlaps[0].Ref != "SPEC-042" {
		t.Fatalf("overlaps = %+v, want SPEC-042 first", payload.Result.Overlaps)
	}
	if got, want := payload.Result.Overlaps[0].Relationship, "adjacent"; got != want {
		t.Fatalf("relationship = %q, want %q", got, want)
	}
	if got, want := payload.Result.Overlaps[0].Guidance, "boundary_review"; got != want {
		t.Fatalf("guidance = %q, want %q", got, want)
	}
	if got, want := payload.Result.Recommendation, "review_boundaries"; got != want {
		t.Fatalf("recommendation = %q, want %q", got, want)
	}
}

func TestRunCheckOverlapWithSpecRecordFileJSON(t *testing.T) {
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
		return runCheckOverlap([]string{"--spec-record-file", "draft-spec.json", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckOverlap() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckOverlap() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Candidate struct {
				Ref string `json:"ref"`
			} `json:"candidate"`
			Overlaps []struct {
				Ref string `json:"ref"`
			} `json:"overlaps"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal draft overlap payload: %v", err)
	}
	if payload.Result.Candidate.Ref != "SPEC-900" {
		t.Fatalf("candidate ref = %q, want SPEC-900", payload.Result.Candidate.Ref)
	}
	if len(payload.Result.Overlaps) == 0 || payload.Result.Overlaps[0].Ref != "SPEC-042" {
		t.Fatalf("overlaps = %+v, want SPEC-042 first", payload.Result.Overlaps)
	}
}

func TestRunCheckOverlapWithSpecRecordFromStdinJSON(t *testing.T) {
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
		return runCheckOverlap([]string{"--spec-record-file", "-", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckOverlap() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckOverlap() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Candidate struct {
				Ref string `json:"ref"`
			} `json:"candidate"`
			Overlaps []struct {
				Ref string `json:"ref"`
			} `json:"overlaps"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal stdin overlap payload: %v", err)
	}
	if payload.Result.Candidate.Ref != "SPEC-900" {
		t.Fatalf("candidate ref = %q, want SPEC-900", payload.Result.Candidate.Ref)
	}
	if len(payload.Result.Overlaps) == 0 || payload.Result.Overlaps[0].Ref != "SPEC-042" {
		t.Fatalf("overlaps = %+v, want SPEC-042 first", payload.Result.Overlaps)
	}
}

func TestRunCheckOverlapWithRequestFileJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)
	mustWriteJSONFileCmd(t, filepath.Join(repo, "overlap-request.json"), map[string]any{
		"spec_ref": "SPEC-042",
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckOverlap([]string{"--request-file", "overlap-request.json", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckOverlap() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckOverlap() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			SpecRef string `json:"spec_ref"`
		} `json:"request"`
		Result struct {
			Candidate struct {
				Ref string `json:"ref"`
			} `json:"candidate"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal overlap request-file payload: %v", err)
	}
	if payload.Request.SpecRef != "SPEC-042" || payload.Result.Candidate.Ref != "SPEC-042" {
		t.Fatalf("payload = %+v, want SPEC-042 request/candidate", payload)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunCheckOverlapRejectsSpecRecordOutsideWorkspace(t *testing.T) {
	repo := writeSearchWorkspace(t)
	outside := filepath.Join(t.TempDir(), "draft-spec.json")
	mustWriteJSONFileCmd(t, outside, map[string]any{
		"ref":    "SPEC-900",
		"title":  "Draft Spec",
		"status": "draft",
		"kind":   "spec",
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckOverlap([]string{"--spec-record-file", outside, "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runCheckOverlap() exit code = %d, want 2", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckOverlap() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal overlap outside-workspace payload: %v", err)
	}
	if len(payload.Errors) != 1 || payload.Errors[0].Code != "validation_error" {
		t.Fatalf("errors = %+v, want one validation_error", payload.Errors)
	}
	if got := payload.Errors[0].Message; !strings.Contains(got, "outside workspace root") {
		t.Fatalf("error message = %q, want workspace-root validation", got)
	}
}

func TestRunCheckOverlapReportsUnknownSpecRefActionably(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckOverlap([]string{"--spec-ref", "SPEC-999", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runCheckOverlap() exit code = %d, want 2", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckOverlap() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result any        `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal overlap error payload: %v", err)
	}
	if payload.Result != nil {
		t.Fatalf("payload result = %#v, want nil", payload.Result)
	}
	if len(payload.Errors) != 1 || payload.Errors[0].Code != "not_found" {
		t.Fatalf("payload errors = %+v, want one not_found error", payload.Errors)
	}
	if !strings.Contains(payload.Errors[0].Message, `unknown --spec-ref "SPEC-999"`) {
		t.Fatalf("payload error message = %q, want actionable spec-ref detail", payload.Errors[0].Message)
	}
	for _, want := range []string{"SPEC-008", "SPEC-042", "SPEC-055"} {
		if !strings.Contains(payload.Errors[0].Message, want) {
			t.Fatalf("payload error message = %q, want available ref %q", payload.Errors[0].Message, want)
		}
	}
	if !strings.Contains(payload.Errors[0].Message, "pituitary index --rebuild") {
		t.Fatalf("payload error message = %q, want rebuild guidance", payload.Errors[0].Message)
	}
}

func TestRunCheckOverlapReportsUnknownPathActionably(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCheckOverlap([]string{"--path", "specs/missing/spec.toml", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runCheckOverlap() exit code = %d, want 2", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckOverlap() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result any        `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal overlap error payload: %v", err)
	}
	if payload.Result != nil {
		t.Fatalf("payload result = %#v, want nil", payload.Result)
	}
	if len(payload.Errors) != 1 || payload.Errors[0].Code != "not_found" {
		t.Fatalf("payload errors = %+v, want one not_found error", payload.Errors)
	}
	if !strings.Contains(payload.Errors[0].Message, `unknown --path "specs/missing/spec.toml"`) {
		t.Fatalf("payload error message = %q, want actionable path detail", payload.Errors[0].Message)
	}
}
