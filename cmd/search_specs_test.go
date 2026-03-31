package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSearchSpecsJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runSearchSpecs([]string{"--query", "rate limiting", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runSearchSpecs() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runSearchSpecs() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			Query   string `json:"query"`
			Limit   int    `json:"limit"`
			Filters struct {
				Statuses []string `json:"statuses"`
			} `json:"filters"`
		} `json:"request"`
		Result struct {
			Matches []struct {
				Ref            string `json:"ref"`
				SectionHeading string `json:"section_heading"`
			} `json:"matches"`
		} `json:"result"`
		Warnings []cliIssue `json:"warnings"`
		Errors   []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal search payload: %v", err)
	}
	if payload.Request.Query != "rate limiting" {
		t.Fatalf("payload request.query = %q, want %q", payload.Request.Query, "rate limiting")
	}
	if payload.Request.Limit != 10 {
		t.Fatalf("payload request.limit = %d, want 10", payload.Request.Limit)
	}
	if len(payload.Request.Filters.Statuses) != 3 {
		t.Fatalf("payload statuses = %v, want default active statuses", payload.Request.Filters.Statuses)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("payload errors = %+v, want none", payload.Errors)
	}
	if len(payload.Result.Matches) == 0 {
		t.Fatal("payload returned no matches")
	}
	if payload.Result.Matches[0].Ref == "" || payload.Result.Matches[0].SectionHeading == "" {
		t.Fatalf("top match = %+v, want stable ref and section heading", payload.Result.Matches[0])
	}
}

func TestRunSearchSpecsJSONIncludesRepoAndSource(t *testing.T) {
	repo := writeMultiRepoSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runSearchSpecs([]string{"--query", "shared repo rollout", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runSearchSpecs() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runSearchSpecs() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Matches []struct {
				Ref       string `json:"ref"`
				Repo      string `json:"repo"`
				SourceRef string `json:"source_ref"`
			} `json:"matches"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal search payload: %v", err)
	}
	if len(payload.Result.Matches) == 0 {
		t.Fatal("payload returned no matches")
	}
	if got, want := payload.Result.Matches[0].Ref, "SPEC-200"; got != want {
		t.Fatalf("top match ref = %q, want %q", got, want)
	}
	if got, want := payload.Result.Matches[0].Repo, "shared"; got != want {
		t.Fatalf("top match repo = %q, want %q", got, want)
	}
	if got, want := payload.Result.Matches[0].SourceRef, "file://specs/shared-rollout/spec.toml"; got != want {
		t.Fatalf("top match source_ref = %q, want %q", got, want)
	}
}

func TestRunSearchSpecsTable(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runSearchSpecs([]string{
			"--query", "fixed window rate limiting",
			"--status", "superseded",
			"--format", "table",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runSearchSpecs() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runSearchSpecs() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"pituitary search-specs: search spec sections semantically",
		"REF",
		"TITLE",
		"SECTION",
		"SCORE",
		"SPEC-008",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runSearchSpecs(--format table) output %q does not contain %q", out, want)
		}
	}
}

func TestRunSearchSpecsWithRequestFileJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)
	mustWriteJSONFileCmd(t, filepath.Join(repo, "search-request.json"), map[string]any{
		"query": "rate limiting",
		"filters": map[string]any{
			"domain":   "api",
			"statuses": []string{"accepted"},
		},
		"limit": 5,
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runSearchSpecs([]string{"--request-file", "search-request.json", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runSearchSpecs() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runSearchSpecs() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			Query   string `json:"query"`
			Filters struct {
				Domain   string   `json:"domain"`
				Statuses []string `json:"statuses"`
			} `json:"filters"`
			Limit int `json:"limit"`
		} `json:"request"`
		Result struct {
			Matches []struct {
				Ref string `json:"ref"`
			} `json:"matches"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal search request-file payload: %v", err)
	}
	if payload.Request.Query != "rate limiting" || payload.Request.Filters.Domain != "api" || payload.Request.Limit != 5 {
		t.Fatalf("request = %+v, want normalized request-file values", payload.Request)
	}
	if len(payload.Request.Filters.Statuses) != 1 || payload.Request.Filters.Statuses[0] != "accepted" {
		t.Fatalf("request filters = %+v, want accepted status", payload.Request.Filters)
	}
	if len(payload.Result.Matches) == 0 {
		t.Fatal("result.matches is empty, want indexed matches")
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunSearchSpecsRejectsRequestFileOutsideWorkspace(t *testing.T) {
	repo := writeSearchWorkspace(t)
	outside := filepath.Join(t.TempDir(), "search-request.json")
	mustWriteJSONFileCmd(t, outside, map[string]any{"query": "rate limiting"})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runSearchSpecs([]string{"--request-file", outside, "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runSearchSpecs() exit code = %d, want 2", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runSearchSpecs() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal search outside-workspace payload: %v", err)
	}
	if len(payload.Errors) != 1 || payload.Errors[0].Code != "validation_error" {
		t.Fatalf("errors = %+v, want one validation_error", payload.Errors)
	}
	if got := payload.Errors[0].Message; !strings.Contains(got, "outside workspace root") {
		t.Fatalf("error message = %q, want workspace-root validation", got)
	}
}

func TestRunSearchSpecsRejectsMissingQuery(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runSearchSpecs([]string{"--format", "json"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("runSearchSpecs() exit code = %d, want 2", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runSearchSpecs() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result any        `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	if payload.Result != nil {
		t.Fatalf("payload result = %#v, want nil", payload.Result)
	}
	if len(payload.Errors) != 1 || payload.Errors[0].Code != "validation_error" {
		t.Fatalf("payload errors = %+v, want one validation_error", payload.Errors)
	}
}

func TestRunSearchSpecsStatusFilterIncludesSuperseded(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		if code := runIndex([]string{"--rebuild"}, ioDiscard{}, ioDiscard{}); code != 0 {
			t.Fatalf("runIndex() exit code = %d, want 0", code)
		}
		return runSearchSpecs([]string{
			"--query", "fixed window rate limiting",
			"--status", "superseded",
			"--format", "json",
		}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runSearchSpecs() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runSearchSpecs() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Matches []struct {
				Ref string `json:"ref"`
			} `json:"matches"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal filtered payload: %v", err)
	}
	if len(payload.Result.Matches) == 0 {
		t.Fatal("filtered search returned no matches")
	}
	if payload.Result.Matches[0].Ref != "SPEC-008" {
		t.Fatalf("top filtered match = %+v, want SPEC-008", payload.Result.Matches[0])
	}
}

func TestRunSearchSpecsRejectsLimitAboveMaximum(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runSearchSpecs([]string{
		"--query", "rate limiting",
		"--limit", "51",
		"--format", "json",
	}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("runSearchSpecs() exit code = %d, want 2", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runSearchSpecs() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result any        `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	if payload.Result != nil {
		t.Fatalf("payload result = %#v, want nil", payload.Result)
	}
	if len(payload.Errors) != 1 || payload.Errors[0].Message != "limit must be less than or equal to 50" {
		t.Fatalf("payload errors = %+v, want maximum-limit validation", payload.Errors)
	}
}

func TestRunSearchSpecsReportsMissingIndexActionably(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runSearchSpecs([]string{"--query", "rate limiting", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runSearchSpecs() exit code = %d, want 2", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runSearchSpecs() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result any        `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal missing-index payload: %v", err)
	}
	if payload.Result != nil {
		t.Fatalf("payload result = %#v, want nil", payload.Result)
	}
	if len(payload.Errors) != 1 {
		t.Fatalf("payload errors = %+v, want one config error", payload.Errors)
	}
	if payload.Errors[0].Code != "config_error" {
		t.Fatalf("payload errors = %+v, want config_error", payload.Errors)
	}
	if !strings.Contains(payload.Errors[0].Message, "pituitary index --rebuild") {
		t.Fatalf("payload error message = %q, want rebuild guidance", payload.Errors[0].Message)
	}
	if !strings.Contains(payload.Errors[0].Message, filepath.Join(repo, ".pituitary", "pituitary.db")) {
		t.Fatalf("payload error message = %q, want resolved index path", payload.Errors[0].Message)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}

func writeSearchWorkspace(t *testing.T) string {
	t.Helper()

	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	root := t.TempDir()
	copyTree(t, filepath.Join(repoRoot, "specs"), filepath.Join(root, "specs"))
	copyTree(t, filepath.Join(repoRoot, "docs"), filepath.Join(root, "docs"))
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
include = ["guides/*.md", "runbooks/*.md"]
	`)
	return root
}

func writeMultiRepoSearchWorkspace(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	primary := filepath.Join(root, "primary")
	shared := filepath.Join(root, "shared")
	mustWriteIndexFixture(t, root, `
[workspace]
root = "`+filepath.ToSlash(primary)+`"
repo_id = "primary"
index_path = "`+filepath.ToSlash(filepath.Join(root, ".pituitary", "pituitary.db"))+`"

[[workspace.repos]]
id = "shared"
root = "`+filepath.ToSlash(shared)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "primary-specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "primary-docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]

[[sources]]
name = "shared-specs"
adapter = "filesystem"
kind = "spec_bundle"
repo = "shared"
path = "specs"

[[sources]]
name = "shared-docs"
adapter = "filesystem"
kind = "markdown_docs"
repo = "shared"
path = "docs"
include = ["guides/*.md"]
	`)
	mustWriteFileCmd(t, filepath.Join(primary, "specs", "tenant-rate-limits", "spec.toml"), `
id = "SPEC-100"
title = "Tenant Rate Limits"
status = "accepted"
domain = "api"
body = "body.md"
`)
	mustWriteFileCmd(t, filepath.Join(primary, "specs", "tenant-rate-limits", "body.md"), `
# Tenant Rate Limits

## Defaults

The default rate limit is 200 requests per minute.
`)
	mustWriteFileCmd(t, filepath.Join(primary, "docs", "guides", "api-rate-limits.md"), `
# API Rate Limits

The default rate limit is 200 requests per minute.
`)
	mustWriteFileCmd(t, filepath.Join(shared, "specs", "shared-rollout", "spec.toml"), `
id = "SPEC-200"
title = "Shared Repo Rollout"
status = "accepted"
domain = "api"
body = "body.md"
depends_on = ["SPEC-100"]
`)
	mustWriteFileCmd(t, filepath.Join(shared, "specs", "shared-rollout", "body.md"), `
# Shared Repo Rollout

## Dependencies

This rollout depends on SPEC-100.

## Tasks

Update shared consumers to respect the 200 requests per minute tenant default.
`)
	mustWriteFileCmd(t, filepath.Join(shared, "docs", "guides", "api-rate-limits.md"), `
# API Rate Limits

The default rate limit is 100 requests per minute.
`)
	return root
}

func writePathFirstWorkspace(t *testing.T) string {
	t.Helper()

	root := writeSearchWorkspace(t)
	mustWriteFileCmd(t, filepath.Join(root, "rfcs", "service-sla.md"), `
# Service Rate Limiting Contract

Domain: api
Applies To:
- code://src/api/middleware/ratelimiter.go
- config://src/api/config/limits.yaml

## Overview

This contract captures a tenant-aware rate-limiting policy for the public API.

## Requirements

- Apply limits per tenant rather than per API key.
- Enforce a default limit of 200 requests per minute.
- Allow tenant-specific overrides through configuration.
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
include = ["guides/*.md", "runbooks/*.md"]

[[sources]]
name = "contracts"
adapter = "filesystem"
kind = "markdown_contract"
path = "rfcs"
`)
	return root
}

func writeWeakAcceptedContractWorkspace(t *testing.T) string {
	t.Helper()

	root := writeSearchWorkspace(t)
	mustWriteFileCmd(t, filepath.Join(root, "rfcs", "tenant-rate-limits.md"), `
Status: accepted

# Tenant Rate Limits Contract

## Overview

This contract captures a tenant-aware rate-limiting policy for the public API.

## Requirements

- Apply limits per tenant rather than per API key.
- Enforce a default limit of 200 requests per minute.
- Allow tenant-specific overrides through configuration.
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
include = ["guides/*.md", "runbooks/*.md"]

[[sources]]
name = "contracts"
adapter = "filesystem"
kind = "markdown_contract"
path = "rfcs"
`)
	return root
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()

	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
}
