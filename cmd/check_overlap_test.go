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
