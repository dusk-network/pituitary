package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
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

func TestRunPreviewSourcesJSONAdapter(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "json-specs"
adapter = "json"
kind = "json_spec"
path = "schemas"
files = ["rate-limit.json"]

[sources.options]
title_pointer = "/info/title"
`)
	mustWriteFileCmd(t, filepath.Join(repo, "schemas", "rate-limit.json"), `{
  "info": {
    "title": "Rate Limit Schema"
  }
}`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runPreviewSources([]string{"--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runPreviewSources() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runPreviewSources() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Sources []struct {
				Name      string `json:"name"`
				Adapter   string `json:"adapter"`
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
	if got, want := len(payload.Result.Sources), 1; got != want {
		t.Fatalf("source count = %d, want %d", got, want)
	}
	source := payload.Result.Sources[0]
	if got, want := source.Adapter, "json"; got != want {
		t.Fatalf("source adapter = %q, want %q", got, want)
	}
	if got, want := source.Kind, "json_spec"; got != want {
		t.Fatalf("source kind = %q, want %q", got, want)
	}
	if got, want := source.ItemCount, 1; got != want {
		t.Fatalf("item count = %d, want %d", got, want)
	}
	if got, want := source.Items[0].ArtifactKind, "spec"; got != want {
		t.Fatalf("artifact kind = %q, want %q", got, want)
	}
	if got, want := source.Items[0].Path, "schemas/rate-limit.json"; got != want {
		t.Fatalf("item path = %q, want %q", got, want)
	}
}
