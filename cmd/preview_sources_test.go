package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
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

func TestRunPreviewSourcesVerboseJSONIncludesSelectorDiagnostics(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
exclude = ["guides/private.md"]
`)
	mustWriteFileCmd(t, filepath.Join(repo, "docs", "guides", "public.md"), "# Public\n")
	mustWriteFileCmd(t, filepath.Join(repo, "docs", "guides", "private.md"), "# Private\n")
	mustWriteFileCmd(t, filepath.Join(repo, "docs", "notes", "other.md"), "# Other\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runPreviewSources([]string{"--verbose", "--format", "json"}, &stdout, &stderr)
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
				CandidateCount int `json:"candidate_count"`
				ItemCount      int `json:"item_count"`
				Items          []struct {
					Path           string   `json:"path"`
					IncludeMatches []string `json:"include_matches"`
				} `json:"items"`
				RejectedItems []struct {
					Path           string   `json:"path"`
					Reason         string   `json:"reason"`
					IncludeMatches []string `json:"include_matches"`
					ExcludeMatches []string `json:"exclude_matches"`
				} `json:"rejected_items"`
			} `json:"sources"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal verbose preview payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	source := payload.Result.Sources[0]
	if got, want := source.CandidateCount, 3; got != want {
		t.Fatalf("candidate_count = %d, want %d", got, want)
	}
	if got, want := source.ItemCount, 1; got != want {
		t.Fatalf("item_count = %d, want %d", got, want)
	}
	if got, want := source.Items[0].Path, "docs/guides/public.md"; got != want {
		t.Fatalf("selected item path = %q, want %q", got, want)
	}
	if len(source.Items[0].IncludeMatches) != 1 || source.Items[0].IncludeMatches[0] != "guides/*.md" {
		t.Fatalf("include_matches = %+v, want guides/*.md", source.Items[0].IncludeMatches)
	}
	if got, want := len(source.RejectedItems), 2; got != want {
		t.Fatalf("rejected_items = %+v, want %d entries", source.RejectedItems, want)
	}
}

func TestRunPreviewSourcesTextExplainsSelectorMismatch(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
`)
	mustWriteFileCmd(t, filepath.Join(repo, "docs", "notes", "other.md"), "# Other\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runPreviewSources([]string{}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runPreviewSources() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"candidate files: 1",
		"no matching items (candidate files were found under the source root, but the selectors excluded them)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("preview output %q does not contain %q", output, want)
		}
	}
}
