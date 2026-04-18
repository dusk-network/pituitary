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

	"github.com/dusk-network/pituitary/internal/config"
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

func TestRunIndexWithJSONAdapter(t *testing.T) {
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
name = "json-specs"
adapter = "json"
kind = "json_spec"
path = "schemas"

[sources.options]
title_pointer = "/info/title"
status_pointer = "/meta/status"
domain_pointer = "/meta/domain"
`)
	mustWriteFileCmd(t, filepath.Join(repo, "schemas", "rate-limit.json"), `{
  "meta": {
    "status": "accepted",
    "domain": "api"
  },
  "info": {
    "title": "JSON Rate Limits"
  },
  "limits": {
    "requests_per_minute": 120
  }
}`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	progress := stderr.String()
	if !strings.Contains(progress, "pituitary index: chunking") {
		t.Fatalf("runIndex() stderr %q does not contain chunking progress", progress)
	}
	if !strings.Contains(progress, "pituitary index: embedding") {
		t.Fatalf("runIndex() stderr %q does not contain embedding progress", progress)
	}
	if !strings.Contains(stdout.String(), "indexed 1 artifact(s)") {
		t.Fatalf("runIndex() output %q does not contain JSON adapter counts", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); err != nil {
		t.Fatalf("runIndex() did not create database: %v", err)
	}
}

func TestRunIndexReportsRepoCoverage(t *testing.T) {
	repo := writeMultiRepoSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"repo: primary | items: 2 | specs: 1 | docs: 1",
		"repo: shared | items: 2 | specs: 1 | docs: 1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runIndex() output %q does not contain %q", out, want)
		}
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
	if !strings.Contains(stderr.String(), `pituitary index: exactly one of --rebuild, --update, or --dry-run is required`) {
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
	if !strings.Contains(stderr.String(), `pituitary index: exactly one of --rebuild, --update, or --dry-run is required`) {
		t.Fatalf("runIndex() stderr %q does not contain conflicting-mode message", stderr.String())
	}
}

func TestRunIndexShowDeltaRequiresUpdate(t *testing.T) {
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
		return runIndex([]string{"--rebuild", "--show-delta"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runIndex() exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(stderr.String(), `--show-delta is only valid with --update`) {
		t.Fatalf("runIndex() stderr %q does not contain show-delta validation message", stderr.String())
	}
}

func TestRunIndexRejectsUnknownAdapterInConfig(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "missing"
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
	if !strings.Contains(stderr.String(), `unknown adapter "missing"`) {
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
	if !strings.Contains(stderr.String(), "array terminator") {
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

func TestRunIndexEmitsContextualizerConfigLineInTextMode(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.chunking.contextualizer]
format = "title_ancestry"

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
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr=%q)", exitCode, stderr.String())
	}

	progress := stderr.String()
	want := "pituitary index: chunking contextualizer active (format=title_ancestry)"
	if !strings.Contains(progress, want) {
		t.Fatalf("runIndex() stderr %q does not contain %q", progress, want)
	}
}

func TestRunIndexJSONRebuildKeepsStderrProgressOnlyWhenContextualizerEnabled(t *testing.T) {
	// Regression: the JSON stderr stream is a strict NDJSON of
	// rebuild_progress events (see decodeIndexProgressEvents). The
	// contextualizer announcement is deliberately text-mode only —
	// injecting a second event type would break strict parsers.
	// Machine consumers should read contextualizer posture from
	// `pituitary status --format json`, not from the rebuild stream.
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.chunking.contextualizer]
format = "ref_title"

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
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr=%q)", exitCode, stderr.String())
	}

	if strings.Contains(stderr.String(), "contextualizer") {
		t.Fatalf("runIndex() JSON stderr %q leaked a contextualizer config line into the progress-only stream", stderr.String())
	}
}

func TestEmitRebuildContextualizerConfigTextOnly(t *testing.T) {
	t.Parallel()

	// Direct helper test: locks both the text-only output and the
	// JSON-mode silence so a well-meaning future change that adds a
	// JSON event type back has to update both this test and the
	// decodeIndexProgressEvents strict decoder together. Nothing
	// else in cmd/ exercises the JSON-mode helper path directly.
	cfg := &config.Config{}
	cfg.Runtime.Chunking.Contextualizer.Format = config.ChunkContextualizerFormatTitleAncestry

	var textOut bytes.Buffer
	emitRebuildContextualizerConfig(cfg, commandFormatText, &textOut)
	if got, want := textOut.String(), "pituitary index: chunking contextualizer active (format=title_ancestry)\n"; got != want {
		t.Fatalf("text mode emit = %q, want %q", got, want)
	}

	var jsonOut bytes.Buffer
	emitRebuildContextualizerConfig(cfg, commandFormatJSON, &jsonOut)
	if jsonOut.Len() != 0 {
		t.Fatalf("json mode emit = %q, want empty (progress-only stream)", jsonOut.String())
	}

	var disabledOut bytes.Buffer
	emitRebuildContextualizerConfig(&config.Config{}, commandFormatText, &disabledOut)
	if disabledOut.Len() != 0 {
		t.Fatalf("disabled emit = %q, want empty", disabledOut.String())
	}

	var nilOut bytes.Buffer
	emitRebuildContextualizerConfig(nil, commandFormatText, &nilOut)
	if nilOut.Len() != 0 {
		t.Fatalf("nil cfg emit = %q, want empty", nilOut.String())
	}
}

func TestRunIndexIsSilentAboutContextualizerWhenDisabled(t *testing.T) {
	// Regression: the zero-config path must not emit a contextualizer
	// line. Quiet-on-default output is part of the #347 contract so
	// existing rebuild scripts that grep stderr don't see new noise
	// after the contextualizer feature is available but not opted in.
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
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr=%q)", exitCode, stderr.String())
	}

	if strings.Contains(stderr.String(), "contextualizer") {
		t.Fatalf("runIndex() stderr %q leaked a contextualizer line on the disabled path", stderr.String())
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
	if !strings.Contains(out, "indexed 5 artifact(s), 17 chunk(s), and 9 edge(s)") {
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

func TestRunIndexVerboseJSONIncludesSourceSummariesAndNDJSONProgress(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild", "--verbose", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex(--verbose --format json) exit code = %d, want 0", exitCode)
	}
	events := decodeIndexProgressEvents(t, stderr.String())
	if len(events) == 0 {
		t.Fatal("runIndex(--verbose --format json) wrote no progress events")
	}
	if !hasIndexProgressPhase(events, "chunking") {
		t.Fatalf("progress events = %+v, want chunking phase", events)
	}
	if !hasIndexProgressPhase(events, "embedding") {
		t.Fatalf("progress events = %+v, want embedding phase", events)
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
	if !strings.Contains(out, "dry run validated 5 artifact(s), 17 chunk(s), and 9 edge(s)") {
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
	if payload.Result.ChunkCount != 17 || payload.Result.EdgeCount != 9 {
		t.Fatalf("result = %+v, want 17 chunks / 9 edges", payload.Result)
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

func TestRunIndexRebuildReusesUnchangedCorpus(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var firstStdout bytes.Buffer
	var firstStderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &firstStdout, &firstStderr)
	})
	if exitCode != 0 {
		t.Fatalf("initial runIndex(--rebuild) exit code = %d, want 0 (stderr: %q)", exitCode, firstStderr.String())
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("second runIndex(--rebuild) exit code = %d, want 0", exitCode)
	}
	events := decodeIndexProgressEvents(t, stderr.String())
	if len(events) == 0 {
		t.Fatal("second runIndex(--rebuild) wrote no progress events")
	}
	if !hasIndexProgressPhase(events, "chunking") {
		t.Fatalf("progress events = %+v, want chunking phase", events)
	}
	if hasIndexProgressPhase(events, "embedding") {
		t.Fatalf("progress events = %+v, want reused rebuild to avoid embedding phase", events)
	}

	var payload struct {
		Request struct {
			Rebuild bool `json:"rebuild"`
			Full    bool `json:"full"`
		} `json:"request"`
		Result struct {
			FullRebuild         bool `json:"full_rebuild"`
			ArtifactCount       int  `json:"artifact_count"`
			ChunkCount          int  `json:"chunk_count"`
			ReusedArtifactCount int  `json:"reused_artifact_count"`
			ReusedChunkCount    int  `json:"reused_chunk_count"`
			EmbeddedChunkCount  int  `json:"embedded_chunk_count"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal incremental rebuild payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if !payload.Request.Rebuild || payload.Request.Full {
		t.Fatalf("request = %+v, want rebuild=true full=false", payload.Request)
	}
	if payload.Result.FullRebuild {
		t.Fatalf("result = %+v, want incremental rebuild mode", payload.Result)
	}
	if payload.Result.ArtifactCount != 5 || payload.Result.ChunkCount != 17 {
		t.Fatalf("result = %+v, want 5 artifacts and 17 chunks", payload.Result)
	}
	if payload.Result.ReusedArtifactCount != 5 || payload.Result.ReusedChunkCount != 17 || payload.Result.EmbeddedChunkCount != 0 {
		t.Fatalf("result = %+v, want full vector reuse on unchanged corpus", payload.Result)
	}
}

func TestRunIndexFullForcesReembedding(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var firstStdout bytes.Buffer
	var firstStderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &firstStdout, &firstStderr)
	})
	if exitCode != 0 {
		t.Fatalf("initial runIndex(--rebuild) exit code = %d, want 0 (stderr: %q)", exitCode, firstStderr.String())
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild", "--full", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex(--rebuild --full) exit code = %d, want 0", exitCode)
	}
	events := decodeIndexProgressEvents(t, stderr.String())
	if len(events) == 0 {
		t.Fatal("runIndex(--rebuild --full) wrote no progress events")
	}
	if !hasIndexProgressPhase(events, "chunking") || !hasIndexProgressPhase(events, "embedding") {
		t.Fatalf("progress events = %+v, want chunking and embedding phases", events)
	}

	var payload struct {
		Request struct {
			Full bool `json:"full"`
		} `json:"request"`
		Result struct {
			FullRebuild         bool `json:"full_rebuild"`
			ReusedArtifactCount int  `json:"reused_artifact_count"`
			ReusedChunkCount    int  `json:"reused_chunk_count"`
			EmbeddedChunkCount  int  `json:"embedded_chunk_count"`
			ChunkCount          int  `json:"chunk_count"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal full rebuild payload: %v", err)
	}
	if !payload.Request.Full || !payload.Result.FullRebuild {
		t.Fatalf("payload = %+v, want explicit full rebuild mode", payload)
	}
	if payload.Result.ReusedArtifactCount != 0 || payload.Result.ReusedChunkCount != 0 || payload.Result.EmbeddedChunkCount != payload.Result.ChunkCount {
		t.Fatalf("result = %+v, want zero reuse and full re-embedding", payload.Result)
	}
}

func TestRunIndexRebuildReusesOnlyUnchangedSections(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var firstStdout bytes.Buffer
	var firstStderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &firstStdout, &firstStderr)
	})
	if exitCode != 0 {
		t.Fatalf("initial runIndex(--rebuild) exit code = %d, want 0 (stderr: %q)", exitCode, firstStderr.String())
	}

	mustWriteFileCmd(t, filepath.Join(repo, "docs", "guides", "api-rate-limits.md"), `
# Public API Rate Limits

## Default Limit

Tenant-scoped rate limits default to 200 requests per minute.

## Configuration

Limits are configured in src/api/config/limits.yaml.

## Operational Notes

Avoid per-api-key fallback modes for accepted tenants.

## Rollout Notes

Canary the new limiter before the full rollout.
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("incremental runIndex(--rebuild) exit code = %d, want 0", exitCode)
	}
	events := decodeIndexProgressEvents(t, stderr.String())
	if len(events) == 0 {
		t.Fatal("incremental runIndex(--rebuild) wrote no progress events")
	}
	if !hasIndexProgressPhase(events, "chunking") || !hasIndexProgressPhase(events, "embedding") {
		t.Fatalf("progress events = %+v, want chunking and embedding phases", events)
	}

	var payload struct {
		Result struct {
			ChunkCount          int `json:"chunk_count"`
			ReusedArtifactCount int `json:"reused_artifact_count"`
			ReusedChunkCount    int `json:"reused_chunk_count"`
			EmbeddedChunkCount  int `json:"embedded_chunk_count"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal partial reuse payload: %v", err)
	}
	if payload.Result.ReusedArtifactCount == 0 {
		t.Fatalf("result = %+v, want unchanged artifacts to be reused", payload.Result)
	}
	if payload.Result.ReusedChunkCount == 0 || payload.Result.EmbeddedChunkCount == 0 {
		t.Fatalf("result = %+v, want both reused and embedded chunks after one doc changes", payload.Result)
	}
	if payload.Result.EmbeddedChunkCount >= payload.Result.ChunkCount {
		t.Fatalf("result = %+v, want only a subset of chunks to be re-embedded", payload.Result)
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

type indexProgressJSONEvent struct {
	Event        string `json:"event"`
	Command      string `json:"command"`
	Phase        string `json:"phase"`
	ArtifactKind string `json:"artifact_kind"`
	ArtifactRef  string `json:"artifact_ref"`
	Current      int    `json:"current"`
	Total        int    `json:"total"`
	ChunkCount   int    `json:"chunk_count"`
}

func decodeIndexProgressEvents(t *testing.T, stderr string) []indexProgressJSONEvent {
	t.Helper()
	trimmed := strings.TrimSpace(stderr)
	if trimmed == "" {
		return nil
	}

	lines := strings.Split(trimmed, "\n")
	events := make([]indexProgressJSONEvent, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event indexProgressJSONEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("unmarshal index progress event %q: %v", line, err)
		}
		if event.Event != "rebuild_progress" || event.Command != "index" {
			t.Fatalf("progress event = %+v, want rebuild_progress/index metadata", event)
		}
		events = append(events, event)
	}
	return events
}

func hasIndexProgressPhase(events []indexProgressJSONEvent, phase string) bool {
	for _, event := range events {
		if event.Phase == phase {
			return true
		}
	}
	return false
}
