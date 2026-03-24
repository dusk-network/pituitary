package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCheckTerminologyReportsAnchoredFindings(t *testing.T) {
	t.Parallel()

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
		"evidence: SPEC-LOCALITY | Kernel Locality Contract / Core Model",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runCheckTerminology() output %q does not contain %q", out, want)
		}
	}
	if strings.Contains(out, "repo-compatibility") {
		t.Fatalf("runCheckTerminology() output %q unexpectedly contains compatibility-only doc", out)
	}
}

func TestRunCheckTerminologyRequiresTerms(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runCheckTerminology([]string{"--scope", "docs"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("runCheckTerminology() exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(stderr.String(), "at least one term is required") {
		t.Fatalf("runCheckTerminology() stderr = %q, want term validation", stderr.String())
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
