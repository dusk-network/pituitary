package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCompileDryRunJSONPlansEdits(t *testing.T) {
	repo := writeCompileWorkspaceCmd(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCompile([]string{"--scope", "all", "--dry-run", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCompile() exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCompile() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			Scope  string `json:"scope"`
			DryRun bool   `json:"dry_run"`
		} `json:"request"`
		Result struct {
			Scope            string `json:"scope"`
			Applied          bool   `json:"applied"`
			PlannedFileCount int    `json:"planned_file_count"`
			PlannedEditCount int    `json:"planned_edit_count"`
			Files            []struct {
				Ref    string `json:"ref"`
				Path   string `json:"path"`
				Status string `json:"status"`
				Edits  []struct {
					Code   string `json:"code"`
					Action string `json:"action"`
					Before string `json:"before"`
					After  string `json:"after"`
				} `json:"edits"`
				Reason string `json:"reason"`
			} `json:"files"`
			Guidance []string `json:"guidance"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\nraw: %s", err, stdout.String())
	}
	if payload.Request.Scope != "all" || !payload.Request.DryRun {
		t.Fatalf("request = %+v, want scope=all and dry_run=true", payload.Request)
	}
	if payload.Result.Applied {
		t.Fatalf("result.applied = true, want false")
	}
	if payload.Result.PlannedEditCount < 1 {
		t.Fatalf("result.planned_edit_count = %d, want at least 1", payload.Result.PlannedEditCount)
	}

	// Verify we have at least one file with planned edits.
	var foundPlanned bool
	for _, file := range payload.Result.Files {
		if file.Status == "planned" && len(file.Edits) > 0 {
			foundPlanned = true
			for _, edit := range file.Edits {
				if edit.Action != "replace_term" {
					t.Fatalf("edit.action = %q, want replace_term", edit.Action)
				}
			}
		}
	}
	if !foundPlanned {
		t.Fatalf("result.files = %+v, want at least one planned file with edits", payload.Result.Files)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunCompileYesAppliesEdits(t *testing.T) {
	repo := writeCompileWorkspaceCmd(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runCompile([]string{"--scope", "all", "--yes", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCompile() exit code = %d, want 0\nstdout: %s\nstderr: %s", exitCode, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCompile() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Applied          bool `json:"applied"`
			AppliedFileCount int  `json:"applied_file_count"`
			AppliedEditCount int  `json:"applied_edit_count"`
			Files            []struct {
				Path   string `json:"path"`
				Status string `json:"status"`
				Edits  []struct {
					Before string `json:"before"`
					After  string `json:"after"`
				} `json:"edits"`
			} `json:"files"`
			Guidance []string `json:"guidance"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\nraw: %s", err, stdout.String())
	}
	if !payload.Result.Applied {
		t.Fatalf("result.applied = false, want true")
	}
	if payload.Result.AppliedEditCount < 1 {
		t.Fatalf("result.applied_edit_count = %d, want at least 1", payload.Result.AppliedEditCount)
	}

	// Verify at least one file was applied.
	var foundApplied bool
	for _, file := range payload.Result.Files {
		if file.Status == "applied" {
			foundApplied = true
		}
	}
	if !foundApplied {
		t.Fatalf("result.files = %+v, want at least one applied file", payload.Result.Files)
	}

	// Verify the doc file was actually changed on disk.
	kernelDoc, err := os.ReadFile(filepath.Join(repo, "docs", "guides", "repo-kernel.md"))
	if err != nil {
		t.Fatalf("read updated doc: %v", err)
	}
	content := string(kernelDoc)
	// The terminology policy says "repo" -> "locality", so the doc should contain "locality" now.
	if !strings.Contains(content, "locality") {
		t.Fatalf("updated doc %q does not contain expected replacement term 'locality'", content)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunCompileRequiresScopeFlag(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runCompile([]string{"--dry-run", "--format", "json"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("runCompile() exit code = %d, want 2", exitCode)
	}

	var payload struct {
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\nraw: %s", err, stdout.String())
	}
	if len(payload.Errors) == 0 {
		t.Fatal("errors is empty, want scope validation error")
	}
	if !strings.Contains(payload.Errors[0].Message, "--scope") {
		t.Fatalf("error message = %q, want --scope required", payload.Errors[0].Message)
	}
}

func writeCompileWorkspaceCmd(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	mustWriteFileCmd(t, root+"/specs/kernel-locality/spec.toml", `
id = "SPEC-LOCALITY"
title = "Kernel Locality Contract"
status = "accepted"
domain = "kernel"
body = "body.md"
`)
	mustWriteFileCmd(t, root+"/specs/kernel-locality/body.md", `
# Kernel Locality Contract

## Core Model

The kernel keeps continuity in clone-local state.
The runtime is locality-centric and treats repository adapters as optional extensions.
`)
	mustWriteFileCmd(t, root+"/docs/guides/repo-kernel.md", `
# Repo Kernel Guide

The kernel keeps workflow continuity in each repo.
Repository storage is the default operator model.
`)
	mustWriteFileCmd(t, root+"/docs/guides/repo-compatibility.md", `
# Repo Compatibility Notes

Legacy repo references remain available only as a compatibility alias during migration to locality.
`)
	mustWriteIndexFixture(t, root, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[terminology.policies]]
preferred = "locality"
historical_aliases = ["repo"]
forbidden_current = ["repository"]
docs_severity = "error"
specs_severity = "warning"

[[terminology.policies]]
preferred = "continuity"
deprecated_terms = ["workflow"]
docs_severity = "error"
specs_severity = "warning"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
`)
	return root
}
