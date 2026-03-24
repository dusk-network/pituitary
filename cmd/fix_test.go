package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFixDryRunJSONPlansDeterministicEdits(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runFix([]string{"--path", "docs/guides/api-rate-limits.md", "--dry-run", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runFix() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runFix() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			Path   string `json:"path"`
			DryRun bool   `json:"dry_run"`
		} `json:"request"`
		Result struct {
			Selector         string `json:"selector"`
			Applied          bool   `json:"applied"`
			PlannedFileCount int    `json:"planned_file_count"`
			PlannedEditCount int    `json:"planned_edit_count"`
			Files            []struct {
				DocRef string `json:"doc_ref"`
				Path   string `json:"path"`
				Status string `json:"status"`
				Edits  []struct {
					Code   string `json:"code"`
					Action string `json:"action"`
					After  string `json:"after"`
				} `json:"edits"`
				Reason string `json:"reason"`
			} `json:"files"`
			Guidance []string `json:"guidance"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload.Request.Path != "docs/guides/api-rate-limits.md" || !payload.Request.DryRun {
		t.Fatalf("request = %+v, want path+dry_run", payload.Request)
	}
	if payload.Result.Selector != "docs/guides/api-rate-limits.md" {
		t.Fatalf("result.selector = %q, want doc path selector", payload.Result.Selector)
	}
	if payload.Result.Applied {
		t.Fatalf("result.applied = true, want false")
	}
	if payload.Result.PlannedFileCount != 1 {
		t.Fatalf("result.planned_file_count = %d, want 1", payload.Result.PlannedFileCount)
	}
	if payload.Result.PlannedEditCount < 3 {
		t.Fatalf("result.planned_edit_count = %d, want at least 3", payload.Result.PlannedEditCount)
	}
	if len(payload.Result.Files) != 1 {
		t.Fatalf("result.files = %+v, want one file", payload.Result.Files)
	}
	file := payload.Result.Files[0]
	if got, want := file.DocRef, "doc://guides/api-rate-limits"; got != want {
		t.Fatalf("file.doc_ref = %q, want %q", got, want)
	}
	if !strings.HasSuffix(filepath.ToSlash(file.Path), "docs/guides/api-rate-limits.md") {
		t.Fatalf("file.path = %q, want docs/guides/api-rate-limits.md suffix", file.Path)
	}
	if file.Status != "planned" {
		t.Fatalf("file.status = %q, want planned", file.Status)
	}
	if len(file.Edits) < 3 {
		t.Fatalf("file.edits = %+v, want multiple planned edits", file.Edits)
	}
	var foundSliding bool
	for _, edit := range file.Edits {
		if edit.Action != "replace_claim" {
			t.Fatalf("edit.action = %q, want replace_claim", edit.Action)
		}
		if strings.Contains(edit.After, "sliding-window") {
			foundSliding = true
		}
	}
	if !foundSliding {
		t.Fatalf("file.edits = %+v, want sliding-window replacement", file.Edits)
	}
	if len(payload.Result.Guidance) == 0 || !strings.Contains(payload.Result.Guidance[0], "--yes") {
		t.Fatalf("result.guidance = %v, want apply guidance", payload.Result.Guidance)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunFixAppliesEditsWithYes(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runFix([]string{"--path", "docs/guides/api-rate-limits.md", "--yes"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runFix() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runFix() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"━━◈ fix",
		"api-rate-limits.md",
		"applied",
		"pituitary index --rebuild",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runFix() output %q does not contain %q", out, want)
		}
	}

	updatedBytes, err := os.ReadFile(filepath.Join(repo, "docs", "guides", "api-rate-limits.md"))
	if err != nil {
		t.Fatalf("read updated guide: %v", err)
	}
	updated := string(updatedBytes)
	for _, want := range []string{
		"sliding-window rate limiter",
		"200 requests per minute",
		"tenant-specific overrides are supported through configuration",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated guide %q does not contain %q", updated, want)
		}
	}
	for _, stale := range []string{
		"fixed-window rate limiter",
		"100 requests per minute",
		"tenant-specific overrides are not supported",
	} {
		if strings.Contains(updated, stale) {
			t.Fatalf("updated guide %q still contains stale text %q", updated, stale)
		}
	}
}

func TestRunFixRejectsInteractiveRunWithoutTTY(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	_ = stdinWriter.Close()
	oldStdin := os.Stdin
	os.Stdin = stdinReader
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = stdinReader.Close()
	})

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runFix([]string{"--path", "docs/guides/api-rate-limits.md"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runFix() exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runFix() wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "requires a TTY") {
		t.Fatalf("runFix() stderr %q does not contain TTY guidance", stderr.String())
	}
}

func TestRunFixRejectsStaleIndex(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		mustWriteFileCmd(t, filepath.Join(repo, "docs", "guides", "api-rate-limits.md"), `
# Public API Rate Limits

This guide changed after indexing.
`)
		return runFix([]string{"--path", "docs/guides/api-rate-limits.md", "--dry-run"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runFix() exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runFix() wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "index is stale") {
		t.Fatalf("runFix() stderr %q does not contain stale-index guidance", stderr.String())
	}
	if !strings.Contains(stderr.String(), "pituitary index --rebuild") {
		t.Fatalf("runFix() stderr %q does not contain rebuild guidance", stderr.String())
	}
}
