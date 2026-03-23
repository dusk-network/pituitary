package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunIndexValidatesConfig(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runIndex() wrote unexpected stderr: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "indexed 0 artifact(s), 0 chunk(s), and 0 edge(s)") {
		t.Fatalf("runIndex() output %q does not contain rebuild counts", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); err != nil {
		t.Fatalf("runIndex() did not create database: %v", err)
	}
}

func TestRunIndexRequiresRebuild(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex(nil, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runIndex() exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runIndex() wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `pituitary index: one of --rebuild or --dry-run is required`) {
		t.Fatalf("runIndex() stderr %q does not contain mode requirement", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); !os.IsNotExist(err) {
		t.Fatalf("runIndex() created database without --rebuild: %v", err)
	}
}

func TestRunIndexRejectsConflictingModes(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild", "--dry-run"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runIndex() exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runIndex() wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `pituitary index: exactly one of --rebuild or --dry-run is allowed`) {
		t.Fatalf("runIndex() stderr %q does not contain conflicting-mode message", stderr.String())
	}
}

func TestRunIndexReportsInvalidConfig(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "github"
kind = "spec_bundle"
path = "specs"
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runIndex() exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runIndex() wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `pituitary index: invalid config:`) {
		t.Fatalf("runIndex() stderr %q does not contain invalid-config prefix", stderr.String())
	}
	if !strings.Contains(stderr.String(), `unsupported adapter "github"`) {
		t.Fatalf("runIndex() stderr %q does not contain adapter detail", stderr.String())
	}
}

func TestRunIndexReportsMalformedSource(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	mustWriteFileCmd(t, filepath.Join(repo, "specs", "broken", "spec.toml"), `
id = "SPEC-200"
title = "Broken"
status = "draft"
domain = "api"
body = "body.md"
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runIndex() exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runIndex() wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `pituitary index: source load failed:`) && !strings.Contains(stderr.String(), `pituitary index: rebuild failed:`) {
		t.Fatalf("runIndex() stderr %q does not contain failure prefix", stderr.String())
	}
	if !strings.Contains(stderr.String(), `specs/broken/body.md`) {
		t.Fatalf("runIndex() stderr %q does not contain missing body path", stderr.String())
	}
}

func TestRunIndexReportsConfigPathAndLineForParseErrors(t *testing.T) {
	repo := t.TempDir()
	configPath := filepath.Join(repo, "pituitary.toml")
	mustWriteFileCmd(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = [
  "guides/*.md"
[runtime.embedder]
provider = "fixture"
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runIndex() exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runIndex() wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), configPath) {
		t.Fatalf("runIndex() stderr %q does not contain config path", stderr.String())
	}
	if !strings.Contains(stderr.String(), "line ") {
		t.Fatalf("runIndex() stderr %q does not contain line information", stderr.String())
	}
	if !strings.Contains(stderr.String(), "unterminated array") {
		t.Fatalf("runIndex() stderr %q does not contain parse detail", stderr.String())
	}
}

func TestRunIndexRejectsOpenAICompatibleEmbedderWithoutEndpoint(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "openai_compatible"
model = "text-embedding-3-small"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runIndex() exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runIndex() wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `pituitary index: invalid config:`) {
		t.Fatalf("runIndex() stderr %q does not contain config-error prefix", stderr.String())
	}
	if !strings.Contains(stderr.String(), `runtime.embedder.endpoint: value is required for provider "openai_compatible"`) {
		t.Fatalf("runIndex() stderr %q does not contain missing-endpoint detail", stderr.String())
	}
}

func TestRunIndexJSON(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runIndex() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			Rebuild bool `json:"rebuild"`
		} `json:"request"`
		Result struct {
			ArtifactCount int    `json:"artifact_count"`
			ChunkCount    int    `json:"chunk_count"`
			EdgeCount     int    `json:"edge_count"`
			IndexPath     string `json:"index_path"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal index payload: %v", err)
	}
	if !payload.Request.Rebuild {
		t.Fatalf("request = %+v, want rebuild=true", payload.Request)
	}
	if payload.Result.ArtifactCount != 0 || payload.Result.ChunkCount != 0 || payload.Result.EdgeCount != 0 {
		t.Fatalf("result = %+v, want zero-count rebuild", payload.Result)
	}
	if payload.Result.IndexPath == "" {
		t.Fatalf("result = %+v, want non-empty index path", payload.Result)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunIndexVerboseTextReportsSourceCountsAndProgress(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild", "--verbose"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex(--verbose) exit code = %d, want 0", exitCode)
	}
	out := stdout.String()
	if !strings.Contains(out, "indexed 5 artifact(s), 17 chunk(s), and 8 edge(s)") {
		t.Fatalf("runIndex(--verbose) output %q does not contain rebuild summary", out)
	}
	if !strings.Contains(out, "source: specs | spec_bundle | root: specs | items: 3 | specs: 3") {
		t.Fatalf("runIndex(--verbose) output %q does not contain spec source summary", out)
	}
	if !strings.Contains(out, "source: docs | markdown_docs | root: docs | items: 2 | docs: 2") {
		t.Fatalf("runIndex(--verbose) output %q does not contain doc source summary", out)
	}

	progress := stderr.String()
	if !strings.Contains(progress, "pituitary index: chunking") {
		t.Fatalf("runIndex(--verbose) stderr %q does not contain chunking progress", progress)
	}
	if !strings.Contains(progress, "pituitary index: embedding") {
		t.Fatalf("runIndex(--verbose) stderr %q does not contain embedding progress", progress)
	}
}

func TestRunIndexVerboseJSONIncludesSourceSummariesAndNoProgressOutput(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild", "--verbose", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex(--verbose --format json) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runIndex(--verbose --format json) wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			Rebuild bool `json:"rebuild"`
			Verbose bool `json:"verbose"`
		} `json:"request"`
		Result struct {
			ArtifactCount int `json:"artifact_count"`
			Sources       []struct {
				Name      string `json:"name"`
				Kind      string `json:"kind"`
				Path      string `json:"path"`
				ItemCount int    `json:"item_count"`
				SpecCount int    `json:"spec_count"`
				DocCount  int    `json:"doc_count"`
			} `json:"sources"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal verbose index payload: %v", err)
	}
	if !payload.Request.Rebuild || !payload.Request.Verbose {
		t.Fatalf("request = %+v, want rebuild=true verbose=true", payload.Request)
	}
	if payload.Result.ArtifactCount != 5 {
		t.Fatalf("result = %+v, want 5 artifacts", payload.Result)
	}
	if len(payload.Result.Sources) != 2 {
		t.Fatalf("result.sources = %+v, want 2 summaries", payload.Result.Sources)
	}
	if payload.Result.Sources[0].Name != "specs" || payload.Result.Sources[0].SpecCount != 3 {
		t.Fatalf("result.sources[0] = %+v, want specs summary", payload.Result.Sources[0])
	}
	if payload.Result.Sources[1].Name != "docs" || payload.Result.Sources[1].DocCount != 2 {
		t.Fatalf("result.sources[1] = %+v, want docs summary", payload.Result.Sources[1])
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunIndexDryRunTextDoesNotCreateDatabase(t *testing.T) {
	repo := writeSearchWorkspace(t)
	indexPath := filepath.Join(repo, ".pituitary", "pituitary.db")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--dry-run"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex(--dry-run) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runIndex(--dry-run) wrote unexpected stderr: %q", stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "dry run validated 5 artifact(s), 17 chunk(s), and 8 edge(s)") {
		t.Fatalf("runIndex(--dry-run) output %q does not contain dry-run summary", out)
	}
	if !strings.Contains(out, "database write: skipped") {
		t.Fatalf("runIndex(--dry-run) output %q does not report skipped write", out)
	}
	if _, err := os.Stat(indexPath); !os.IsNotExist(err) {
		t.Fatalf("runIndex(--dry-run) created database: %v", err)
	}
}

func TestRunIndexDryRunJSONDoesNotCreateDatabase(t *testing.T) {
	repo := writeSearchWorkspace(t)
	indexPath := filepath.Join(repo, ".pituitary", "pituitary.db")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--dry-run", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex(--dry-run) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runIndex(--dry-run) wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			Rebuild bool `json:"rebuild"`
			DryRun  bool `json:"dry_run"`
		} `json:"request"`
		Result struct {
			DryRun        bool   `json:"dry_run"`
			ArtifactCount int    `json:"artifact_count"`
			SpecCount     int    `json:"spec_count"`
			DocCount      int    `json:"doc_count"`
			ChunkCount    int    `json:"chunk_count"`
			EdgeCount     int    `json:"edge_count"`
			IndexPath     string `json:"index_path"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal dry-run payload: %v", err)
	}
	if payload.Request.Rebuild || !payload.Request.DryRun {
		t.Fatalf("request = %+v, want rebuild=false dry_run=true", payload.Request)
	}
	if !payload.Result.DryRun {
		t.Fatalf("result = %+v, want dry_run=true", payload.Result)
	}
	if payload.Result.ArtifactCount != 5 || payload.Result.SpecCount != 3 || payload.Result.DocCount != 2 {
		t.Fatalf("result = %+v, want 5 artifacts / 3 specs / 2 docs", payload.Result)
	}
	if payload.Result.ChunkCount != 17 || payload.Result.EdgeCount != 8 {
		t.Fatalf("result = %+v, want 17 chunks / 8 edges", payload.Result)
	}
	if payload.Result.IndexPath == "" {
		t.Fatalf("result = %+v, want non-empty index path", payload.Result)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if _, err := os.Stat(indexPath); !os.IsNotExist(err) {
		t.Fatalf("runIndex(--dry-run) created database: %v", err)
	}
}

func TestRunIndexDryRunDoesNotModifyExistingDatabase(t *testing.T) {
	repo := writeSearchWorkspace(t)
	indexPath := filepath.Join(repo, ".pituitary", "pituitary.db")

	var rebuildStdout bytes.Buffer
	var rebuildStderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &rebuildStdout, &rebuildStderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex(--rebuild) exit code = %d, want 0 (stderr: %q)", exitCode, rebuildStderr.String())
	}

	before := fileHashCmd(t, indexPath)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--dry-run"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex(--dry-run) exit code = %d, want 0", exitCode)
	}
	after := fileHashCmd(t, indexPath)
	if before != after {
		t.Fatalf("runIndex(--dry-run) modified existing database")
	}
}

func mustWriteIndexFixture(t *testing.T, dir, content string) {
	t.Helper()
	path := filepath.Join(dir, "pituitary.toml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustMkdirAllCmd(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFileCmd(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func withWorkingDir(t *testing.T, dir string, fn func() int) int {
	t.Helper()

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	return fn()
}

func fileHashCmd(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
