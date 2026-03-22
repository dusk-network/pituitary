package cmd

import (
	"bytes"
	"encoding/json"
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
				Ref string `json:"ref"`
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
