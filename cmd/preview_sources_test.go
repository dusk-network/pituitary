package cmd

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestRunPreviewSourcesJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runPreviewSources([]string{"--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runPreviewSources() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runPreviewSources() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Sources []struct {
				Name      string `json:"name"`
				Kind      string `json:"kind"`
				ItemCount int    `json:"item_count"`
				Items     []struct {
					ArtifactKind string `json:"artifact_kind"`
					Path         string `json:"path"`
				} `json:"items"`
			} `json:"sources"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal preview payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if got, want := len(payload.Result.Sources), 2; got != want {
		t.Fatalf("source count = %d, want %d", got, want)
	}
	if payload.Result.Sources[0].Name != "specs" || payload.Result.Sources[0].ItemCount != 3 {
		t.Fatalf("spec source = %+v, want 3 spec items", payload.Result.Sources[0])
	}
	if payload.Result.Sources[1].Name != "docs" || payload.Result.Sources[1].ItemCount != 2 {
		t.Fatalf("docs source = %+v, want 2 doc items", payload.Result.Sources[1])
	}
	if payload.Result.Sources[1].Items[0].ArtifactKind != "doc" {
		t.Fatalf("first doc item = %+v, want artifact_kind=doc", payload.Result.Sources[1].Items[0])
	}
}
