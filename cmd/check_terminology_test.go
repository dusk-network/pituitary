package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCheckTerminologyReportsAnchoredFindings(t *testing.T) {
	repo := writeTerminologyWorkspaceCmd(t)
	indexStdout := bytes.Buffer{}
	indexStderr := bytes.Buffer{}
	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &indexStdout, &indexStderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, indexStderr.String())
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
		return runCheckTerminology([]string{
			"--term", "repo",
			"--term", "workflow",
			"--canonical-term", "locality",
			"--canonical-term", "continuity",
			"--spec-ref", "SPEC-LOCALITY",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckTerminology() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckTerminology() stderr = %q, want empty", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"pituitary check-terminology: audit terminology consistency after conceptual changes",
		"anchor spec: SPEC-LOCALITY",
		"doc://guides/repo-kernel | doc | Repo Kernel Guide | terms: repo, workflow",
		"assessment: exact match in body text without compatibility-only markers",
		"match: repo | class: historical_alias | context: current_state | severity: error | replace with: locality",
		"evidence: SPEC-LOCALITY | Kernel Locality Contract / Core Model",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runCheckTerminology() output %q does not contain %q", out, want)
		}
	}
	if !strings.Contains(out, "tolerated historical uses: 1 artifact(s)") || !strings.Contains(out, "doc://guides/repo-compatibility") {
		t.Fatalf("runCheckTerminology() output %q does not contain tolerated compatibility guidance", out)
	}
}

func TestRunCheckTerminologyUsesConfiguredPoliciesWithoutTerms(t *testing.T) {
	repo := writeTerminologyWorkspaceCmd(t)
	indexStdout := bytes.Buffer{}
	indexStderr := bytes.Buffer{}
	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &indexStdout, &indexStderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, indexStderr.String())
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
		return runCheckTerminology([]string{"--scope", "docs", "--spec-ref", "SPEC-LOCALITY", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckTerminology() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckTerminology() stderr = %q, want empty", stderr.String())
	}

	var payload struct {
		Result struct {
			Findings []struct {
				Ref      string `json:"ref"`
				Sections []struct {
					Matches []struct {
						Term           string `json:"term"`
						Classification string `json:"classification"`
						Replacement    string `json:"replacement"`
						Tolerated      bool   `json:"tolerated"`
					} `json:"matches"`
				} `json:"sections"`
			} `json:"findings"`
			Tolerated []struct {
				Ref string `json:"ref"`
			} `json:"tolerated"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal configured terminology payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if len(payload.Result.Findings) != 1 || payload.Result.Findings[0].Ref != "doc://guides/repo-kernel" {
		t.Fatalf("findings = %+v, want repo-kernel", payload.Result.Findings)
	}
	if len(payload.Result.Tolerated) != 1 || payload.Result.Tolerated[0].Ref != "doc://guides/repo-compatibility" {
		t.Fatalf("tolerated = %+v, want compatibility doc", payload.Result.Tolerated)
	}
	if len(payload.Result.Findings[0].Sections) == 0 || len(payload.Result.Findings[0].Sections[0].Matches) == 0 {
		t.Fatalf("sections = %+v, want structured terminology matches", payload.Result.Findings[0].Sections)
	}
	first := payload.Result.Findings[0].Sections[0].Matches[0]
	if first.Classification == "" || first.Replacement == "" || first.Tolerated {
		t.Fatalf("first match = %+v, want actionable classified replacement", first)
	}
}

func TestRunCheckTerminologyWithRequestFileJSON(t *testing.T) {
	repo := writeTerminologyWorkspaceCmd(t)
	indexStdout := bytes.Buffer{}
	indexStderr := bytes.Buffer{}
	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &indexStdout, &indexStderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, indexStderr.String())
	}

	mustWriteJSONFileCmd(t, filepath.Join(repo, "terminology-request.json"), map[string]any{
		"terms":           []string{"repo", "workflow"},
		"canonical_terms": []string{"locality", "continuity"},
		"spec_ref":        "SPEC-LOCALITY",
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
		return runCheckTerminology([]string{"--request-file", "terminology-request.json", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckTerminology() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckTerminology() stderr = %q, want empty", stderr.String())
	}

	var payload struct {
		Request struct {
			Terms   []string `json:"terms"`
			SpecRef string   `json:"spec_ref"`
		} `json:"request"`
		Result struct {
			Findings []struct {
				Ref      string `json:"ref"`
				Sections []struct {
					Matches []struct {
						Classification string `json:"classification"`
						Replacement    string `json:"replacement"`
					} `json:"matches"`
				} `json:"sections"`
			} `json:"findings"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal terminology request-file payload: %v", err)
	}
	if len(payload.Request.Terms) != 2 || payload.Request.SpecRef != "SPEC-LOCALITY" {
		t.Fatalf("request = %+v, want request-file values", payload.Request)
	}
	if len(payload.Result.Findings) == 0 {
		t.Fatal("result.findings is empty, want terminology findings")
	}
	if len(payload.Result.Findings[0].Sections) == 0 || len(payload.Result.Findings[0].Sections[0].Matches) == 0 {
		t.Fatalf("result.findings = %+v, want structured terminology matches", payload.Result.Findings)
	}
	if got := payload.Result.Findings[0].Sections[0].Matches[0].Classification; got == "" {
		t.Fatalf("match classification = %q, want governance classification", got)
	}
	if got := payload.Result.Findings[0].Sections[0].Matches[0].Replacement; got == "" {
		t.Fatalf("match replacement = %q, want replacement suggestion", got)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func writeTerminologyWorkspaceCmd(t *testing.T) string {
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
