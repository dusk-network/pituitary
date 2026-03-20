package cmd

import (
	"bytes"
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
	if !strings.Contains(stderr.String(), `pituitary index: --rebuild is required`) {
		t.Fatalf("runIndex() stderr %q does not contain rebuild requirement", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); !os.IsNotExist(err) {
		t.Fatalf("runIndex() created database without --rebuild: %v", err)
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

func TestRunIndexReportsDependencyUnavailable(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "openai_compatible"
model = "text-embedding-3-small"
api_key_env = "PITUITARY_API_KEY"

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
	if exitCode != 3 {
		t.Fatalf("runIndex() exit code = %d, want 3", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runIndex() wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `pituitary index: dependency unavailable:`) {
		t.Fatalf("runIndex() stderr %q does not contain dependency-unavailable prefix", stderr.String())
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
