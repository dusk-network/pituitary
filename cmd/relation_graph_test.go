package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunIndexDryRunRejectsInvalidRelationGraph(t *testing.T) {
	repo := writeRelationGraphWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--dry-run"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runIndex(--dry-run) exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runIndex(--dry-run) wrote unexpected stdout: %q", stdout.String())
	}
	for _, want := range []string{
		"pituitary index: relation graph invalid:",
		"depends_on cycle detected:",
		"SPEC-100",
		"SPEC-101",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("runIndex(--dry-run) stderr %q does not contain %q", stderr.String(), want)
		}
	}
}

func TestRunStatusJSONIncludesRelationGraphFindings(t *testing.T) {
	repo := writeRelationGraphWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runStatus([]string{"--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			RelationGraph struct {
				State    string `json:"state"`
				Findings []struct {
					Code         string   `json:"code"`
					RelationType string   `json:"relation_type"`
					Refs         []string `json:"refs"`
					Message      string   `json:"message"`
				} `json:"findings"`
			} `json:"relation_graph"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if got, want := payload.Result.RelationGraph.State, "invalid"; got != want {
		t.Fatalf("result.relation_graph.state = %q, want %q", got, want)
	}
	if len(payload.Result.RelationGraph.Findings) == 0 {
		t.Fatal("result.relation_graph.findings is empty, want cycle detail")
	}
	if payload.Result.RelationGraph.Findings[0].Code == "" || payload.Result.RelationGraph.Findings[0].Message == "" {
		t.Fatalf("top relation graph finding = %+v, want stable code and message", payload.Result.RelationGraph.Findings[0])
	}
}

func TestRunStatusTextIncludesRelationGraphFindings(t *testing.T) {
	repo := writeRelationGraphWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runStatus(nil, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}
	for _, want := range []string{
		"relation graph: invalid",
		"relation issue: depends_on cycle detected:",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("runStatus() output %q does not contain %q", stdout.String(), want)
		}
	}
}

func writeRelationGraphWorkspace(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	mustWriteFileCmd(t, filepath.Join(repo, "specs", "alpha", "spec.toml"), `
id = "SPEC-100"
title = "Alpha"
status = "accepted"
domain = "kernel"
body = "body.md"
depends_on = ["SPEC-101"]
`)
	mustWriteFileCmd(t, filepath.Join(repo, "specs", "alpha", "body.md"), `
# Alpha

## Requirements

- Alpha depends on beta.
`)
	mustWriteFileCmd(t, filepath.Join(repo, "specs", "beta", "spec.toml"), `
id = "SPEC-101"
title = "Beta"
status = "accepted"
domain = "kernel"
body = "body.md"
depends_on = ["SPEC-100"]
`)
	mustWriteFileCmd(t, filepath.Join(repo, "specs", "beta", "body.md"), `
# Beta

## Requirements

- Beta depends on alpha.
`)
	return repo
}
