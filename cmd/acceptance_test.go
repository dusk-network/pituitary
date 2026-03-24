package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestAcceptanceDiscoverIndexAndAnalyzeWorkflow(t *testing.T) {
	repo := writeAcceptanceWorkflowWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runDiscover([]string{"--path", ".", "--write"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runDiscover(--write) exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runDiscover(--write) wrote unexpected stderr: %q", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex(--rebuild) exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = withWorkingDir(t, repo, func() int {
		return runStatus([]string{"--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus(--format json) exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus(--format json) wrote unexpected stderr: %q", stderr.String())
	}

	var statusPayload struct {
		Result struct {
			ConfigResolution struct {
				SelectedBy string `json:"selected_by"`
			} `json:"config_resolution"`
			Freshness struct {
				State string `json:"state"`
			} `json:"freshness"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &statusPayload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if len(statusPayload.Errors) != 0 {
		t.Fatalf("status errors = %+v, want none", statusPayload.Errors)
	}
	if got, want := statusPayload.Result.ConfigResolution.SelectedBy, configSourceDiscovery; got != want {
		t.Fatalf("status selected_by = %q, want %q", got, want)
	}
	if got, want := statusPayload.Result.Freshness.State, "fresh"; got != want {
		t.Fatalf("status freshness.state = %q, want %q", got, want)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = withWorkingDir(t, repo, func() int {
		return runAnalyzeImpact([]string{"--spec-ref", "SPEC-LOCALITY", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runAnalyzeImpact() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runAnalyzeImpact() wrote unexpected stderr: %q", stderr.String())
	}

	var impactPayload struct {
		Result struct {
			SpecRef      string `json:"spec_ref"`
			AffectedDocs []struct {
				Ref string `json:"ref"`
			} `json:"affected_docs"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &impactPayload); err != nil {
		t.Fatalf("unmarshal impact payload: %v", err)
	}
	if len(impactPayload.Errors) != 0 {
		t.Fatalf("impact errors = %+v, want none", impactPayload.Errors)
	}
	if got, want := impactPayload.Result.SpecRef, "SPEC-LOCALITY"; got != want {
		t.Fatalf("impact spec_ref = %q, want %q", got, want)
	}
	if len(impactPayload.Result.AffectedDocs) == 0 {
		t.Fatalf("impact affected_docs = %+v, want non-empty", impactPayload.Result.AffectedDocs)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = withWorkingDir(t, repo, func() int {
		return runCheckDocDrift([]string{"--scope", "all", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckDocDrift() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckDocDrift() wrote unexpected stderr: %q", stderr.String())
	}

	var driftPayload struct {
		Result struct {
			DriftItems []struct {
				DocRef string `json:"doc_ref"`
			} `json:"drift_items"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &driftPayload); err != nil {
		t.Fatalf("unmarshal doc drift payload: %v", err)
	}
	if len(driftPayload.Errors) != 0 {
		t.Fatalf("doc drift errors = %+v, want none", driftPayload.Errors)
	}
	var foundRuntimeCache bool
	for _, item := range driftPayload.Result.DriftItems {
		if strings.Contains(item.DocRef, "runtime-cache") {
			foundRuntimeCache = true
			break
		}
	}
	if !foundRuntimeCache {
		t.Fatalf("doc drift items = %+v, want runtime-cache guide drift", driftPayload.Result.DriftItems)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = withWorkingDir(t, repo, func() int {
		return runCheckTerminology([]string{
			"--term", "repo",
			"--canonical-term", "locality",
			"--spec-ref", "SPEC-LOCALITY",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCheckTerminology() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCheckTerminology() wrote unexpected stderr: %q", stderr.String())
	}

	var terminologyPayload struct {
		Result struct {
			Findings []struct {
				Ref string `json:"ref"`
			} `json:"findings"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &terminologyPayload); err != nil {
		t.Fatalf("unmarshal terminology payload: %v", err)
	}
	if len(terminologyPayload.Errors) != 0 {
		t.Fatalf("terminology errors = %+v, want none", terminologyPayload.Errors)
	}
	var foundCanonicalMisuse, foundCompatibilityOnly bool
	for _, finding := range terminologyPayload.Result.Findings {
		switch finding.Ref {
		case "doc://guides/repo-kernel", "doc://repo-kernel":
			foundCanonicalMisuse = true
		case "doc://guides/repo-compatibility", "doc://repo-compatibility":
			foundCompatibilityOnly = true
		}
	}
	if !foundCanonicalMisuse {
		t.Fatalf("terminology findings = %+v, want repo-kernel", terminologyPayload.Result.Findings)
	}
	if foundCompatibilityOnly {
		t.Fatalf("terminology findings = %+v, did not want compatibility-only guide", terminologyPayload.Result.Findings)
	}
}

func writeAcceptanceWorkflowWorkspace(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	mustWriteFileCmd(t, filepath.Join(root, "specs", "kernel-locality", "spec.toml"), `
id = "SPEC-LOCALITY"
title = "Kernel Locality Contract"
status = "accepted"
domain = "kernel"
body = "body.md"
applies_to = ["config://state/locality"]
`)
	mustWriteFileCmd(t, filepath.Join(root, "specs", "kernel-locality", "body.md"), `
# Kernel Locality Contract

## Core Model

The kernel keeps continuity in clone-local state.
Use locality terminology in operator docs and guides.

## Runtime Artifacts

Legacy derived files such as `+"`work_queue.json`"+` and `+"`compiled_state.json`"+` are not part of the accepted runtime contract.
The kernel must not read `+"`work_queue.json`"+` as canonical runtime input.
`+"`compiled_state.json`"+` is not a required runtime artifact.
`)
	mustWriteFileCmd(t, filepath.Join(root, "docs", "guides", "locality-kernel.md"), `
# Locality Kernel Guide

The kernel keeps continuity in each locality.
Locality storage is the default operator model.
`)
	mustWriteFileCmd(t, filepath.Join(root, "docs", "guides", "repo-kernel.md"), `
# Repo Kernel Guide

The kernel keeps workflow continuity in each repo.
Repository storage is the default operator model.
`)
	mustWriteFileCmd(t, filepath.Join(root, "docs", "guides", "repo-compatibility.md"), `
# Repo Compatibility Notes

Legacy repo references remain available only as a compatibility alias during migration to locality.
`)
	mustWriteFileCmd(t, filepath.Join(root, "docs", "guides", "runtime-cache.md"), `
# Runtime Cache Guide

`+"`ccd start`"+` writes `+"`work_queue.json`"+` for the active clone and reads it on the next startup.
The clone-local runtime layout also keeps `+"`compiled_state.json`"+` alongside that cache.
`)
	return root
}
