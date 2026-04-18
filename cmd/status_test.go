package cmd

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestRunStatusReportsMissingIndex(t *testing.T) {
	repo := writeSearchWorkspace(t)

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

	out := stdout.String()
	if !strings.Contains(out, "index: missing") {
		t.Fatalf("runStatus() output %q does not report missing index", out)
	}
	if !strings.Contains(out, "config resolution:") {
		t.Fatalf("runStatus() output %q does not explain config resolution", out)
	}
	if !strings.Contains(out, "artifact ignore patterns: .pituitary/") {
		t.Fatalf("runStatus() output %q does not contain artifact ignore guidance", out)
	}
	if !strings.Contains(out, filepath.Join(repo, ".pituitary", "pituitary.db")) {
		t.Fatalf("runStatus() output %q does not contain resolved index path", out)
	}
}

func TestRunStatusReportsFixtureGuidanceForLargerCorpus(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var rebuildStdout bytes.Buffer
	var rebuildStderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &rebuildStdout, &rebuildStderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, rebuildStderr.String())
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
		return runStatus(nil, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `runtime.embedder is still "fixture" on 5 indexed artifact(s)`) {
		t.Fatalf("runStatus() output %q does not contain fixture guidance", out)
	}
	if !strings.Contains(out, "`pituitary status --check-runtime embedder`") {
		t.Fatalf("runStatus() output %q does not contain runtime probe guidance", out)
	}
}

func TestRunStatusCompactSuppressesVerboseOutput(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var rebuildStdout bytes.Buffer
	var rebuildStderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &rebuildStdout, &rebuildStderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, rebuildStderr.String())
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
		return runStatus([]string{"--compact"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for _, want := range []string{
		"━━◈ status",
		"3 specs  2 docs  17 chunks",
		`runtime.embedder is still "fixture" on 5 indexed artifact(s)`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runStatus() compact output %q does not contain %q", out, want)
		}
	}
	for _, unwanted := range []string{
		"config resolution:",
		"artifact ignore patterns:",
		"RUNTIME CONFIG",
	} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("runStatus() compact output %q unexpectedly contains %q", out, unwanted)
		}
	}
}

func TestRunStatusJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var rebuildStdout bytes.Buffer
	var rebuildStderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &rebuildStdout, &rebuildStderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, rebuildStderr.String())
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
		return runStatus([]string{"--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct{} `json:"request"`
		Result  struct {
			WorkspaceRoot string `json:"workspace_root"`
			ConfigPath    string `json:"config_path"`
			RuntimeConfig struct {
				Embedder struct {
					Provider  string `json:"provider"`
					Model     string `json:"model"`
					TimeoutMS int    `json:"timeout_ms"`
				} `json:"embedder"`
				Analysis struct {
					Provider string `json:"provider"`
				} `json:"analysis"`
			} `json:"runtime_config"`
			ConfigResolution struct {
				SelectedBy string `json:"selected_by"`
				Reason     string `json:"reason"`
				Candidates []struct {
					Source string `json:"source"`
					Status string `json:"status"`
					Path   string `json:"path"`
				} `json:"candidates"`
			} `json:"config_resolution"`
			IndexPath   string `json:"index_path"`
			IndexExists bool   `json:"index_exists"`
			Freshness   struct {
				State string `json:"state"`
			} `json:"freshness"`
			SpecCount         int `json:"spec_count"`
			DocCount          int `json:"doc_count"`
			ChunkCount        int `json:"chunk_count"`
			ArtifactLocations struct {
				IndexDir               string   `json:"index_dir"`
				DiscoverConfigPath     string   `json:"discover_config_path"`
				CanonicalizeBundleRoot string   `json:"canonicalize_bundle_root"`
				IgnorePatterns         []string `json:"ignore_patterns"`
				RelocationHints        []string `json:"relocation_hints"`
			} `json:"artifact_locations"`
			Guidance []string `json:"guidance"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if payload.Result.WorkspaceRoot == "" || payload.Result.ConfigPath == "" || payload.Result.IndexPath == "" {
		t.Fatalf("result = %+v, want non-empty workspace, config, and index paths", payload.Result)
	}
	if got, want := payload.Result.RuntimeConfig.Embedder.Provider, "fixture"; got != want {
		t.Fatalf("result.runtime_config.embedder.provider = %q, want %q", got, want)
	}
	if got, want := payload.Result.RuntimeConfig.Embedder.Model, "fixture-8d"; got != want {
		t.Fatalf("result.runtime_config.embedder.model = %q, want %q", got, want)
	}
	if payload.Result.RuntimeConfig.Embedder.TimeoutMS == 0 {
		t.Fatalf("result.runtime_config.embedder.timeout_ms = %d, want non-zero default", payload.Result.RuntimeConfig.Embedder.TimeoutMS)
	}
	if got, want := payload.Result.RuntimeConfig.Analysis.Provider, "disabled"; got != want {
		t.Fatalf("result.runtime_config.analysis.provider = %q, want %q", got, want)
	}
	if !payload.Result.IndexExists {
		t.Fatalf("result = %+v, want index_exists=true", payload.Result)
	}
	if got, want := payload.Result.Freshness.State, "fresh"; got != want {
		t.Fatalf("result.freshness.state = %q, want %q", got, want)
	}
	if got, want := payload.Result.ConfigResolution.SelectedBy, configSourceDiscovery; got != want {
		t.Fatalf("config_resolution.selected_by = %q, want %q", got, want)
	}
	if payload.Result.ConfigResolution.Reason == "" || len(payload.Result.ConfigResolution.Candidates) < 4 {
		t.Fatalf("config_resolution = %+v, want reason and candidates", payload.Result.ConfigResolution)
	}
	if payload.Result.ArtifactLocations.IndexDir == "" ||
		payload.Result.ArtifactLocations.DiscoverConfigPath == "" ||
		payload.Result.ArtifactLocations.CanonicalizeBundleRoot == "" {
		t.Fatalf("artifact_locations = %+v, want explicit artifact paths", payload.Result.ArtifactLocations)
	}
	if len(payload.Result.ArtifactLocations.IgnorePatterns) == 0 || payload.Result.ArtifactLocations.IgnorePatterns[0] != ".pituitary/" {
		t.Fatalf("artifact_locations.ignore_patterns = %v, want .pituitary/", payload.Result.ArtifactLocations.IgnorePatterns)
	}
	if len(payload.Result.ArtifactLocations.RelocationHints) < 3 {
		t.Fatalf("artifact_locations.relocation_hints = %v, want relocation guidance", payload.Result.ArtifactLocations.RelocationHints)
	}
	if len(payload.Result.Guidance) != 1 || !strings.Contains(payload.Result.Guidance[0], `runtime.embedder is still "fixture" on 5 indexed artifact(s)`) {
		t.Fatalf("result.guidance = %v, want fixture guidance", payload.Result.Guidance)
	}
	if payload.Result.SpecCount != 3 || payload.Result.DocCount != 2 || payload.Result.ChunkCount != 17 {
		t.Fatalf("result = %+v, want 3 specs, 2 docs, 17 chunks", payload.Result)
	}
}

func TestRunStatusJSONIncludesGovernanceHotspots(t *testing.T) {
	repo := writeGovernanceHotspotStatusWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runStatus([]string{"--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0 (stdout: %q, stderr: %q)", exitCode, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			GovernanceHotspots struct {
				HighFanOutSpecs []struct {
					Ref            string `json:"ref"`
					AppliesToCount int    `json:"applies_to_count"`
				} `json:"high_fan_out_specs"`
				WeakLinkArtifacts []struct {
					Ref                string `json:"ref"`
					ExtractedEdgeCount int    `json:"extracted_edge_count"`
					InferredEdgeCount  int    `json:"inferred_edge_count"`
					AmbiguousEdgeCount int    `json:"ambiguous_edge_count"`
				} `json:"weak_link_artifacts"`
				MultiGovernedArtifacts []struct {
					Ref                string   `json:"ref"`
					GoverningSpecCount int      `json:"governing_spec_count"`
					GoverningSpecs     []string `json:"governing_specs"`
				} `json:"multi_governed_artifacts"`
			} `json:"governance_hotspots"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if len(payload.Result.GovernanceHotspots.HighFanOutSpecs) == 0 {
		t.Fatalf("high_fan_out_specs = %+v, want hotspot output", payload.Result.GovernanceHotspots)
	}
	if got, want := payload.Result.GovernanceHotspots.HighFanOutSpecs[0].Ref, "SPEC-300"; got != want {
		t.Fatalf("high_fan_out_specs[0].ref = %q, want %q", got, want)
	}
	if got, want := payload.Result.GovernanceHotspots.HighFanOutSpecs[0].AppliesToCount, 4; got != want {
		t.Fatalf("high_fan_out_specs[0].applies_to_count = %d, want %d", got, want)
	}
	if got, want := payload.Result.GovernanceHotspots.WeakLinkArtifacts[0].Ref, "code://src/service/weak.go"; got != want {
		t.Fatalf("weak_link_artifacts[0].ref = %q, want %q", got, want)
	}
	if payload.Result.GovernanceHotspots.WeakLinkArtifacts[0].ExtractedEdgeCount != 0 {
		t.Fatalf("weak_link_artifacts[0] = %+v, want zero extracted edges", payload.Result.GovernanceHotspots.WeakLinkArtifacts[0])
	}
	var handlerHotspot *struct {
		Ref                string   `json:"ref"`
		GoverningSpecCount int      `json:"governing_spec_count"`
		GoverningSpecs     []string `json:"governing_specs"`
	}
	for i := range payload.Result.GovernanceHotspots.MultiGovernedArtifacts {
		artifact := &payload.Result.GovernanceHotspots.MultiGovernedArtifacts[i]
		if artifact.Ref == "code://src/service/handler.go" {
			handlerHotspot = artifact
			break
		}
	}
	if handlerHotspot == nil {
		t.Fatalf("multi_governed_artifacts = %+v, want handler hotspot", payload.Result.GovernanceHotspots.MultiGovernedArtifacts)
	}
	if got, want := handlerHotspot.GoverningSpecCount, 2; got != want {
		t.Fatalf("multi_governed_artifacts[0].governing_spec_count = %d, want %d", got, want)
	}
}

func TestRunStatusJSONIncludesRepoCoverage(t *testing.T) {
	repo := writeMultiRepoSearchWorkspace(t)

	var rebuildStdout bytes.Buffer
	var rebuildStderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &rebuildStdout, &rebuildStderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, rebuildStderr.String())
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
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
			Repos []struct {
				Repo      string `json:"repo"`
				ItemCount int    `json:"item_count"`
				SpecCount int    `json:"spec_count"`
				DocCount  int    `json:"doc_count"`
			} `json:"repo_coverage"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if got, want := len(payload.Result.Repos), 2; got != want {
		t.Fatalf("repo coverage = %+v, want %d repos", payload.Result.Repos, want)
	}
	for _, repo := range payload.Result.Repos {
		if repo.Repo != "primary" && repo.Repo != "shared" {
			t.Fatalf("unexpected repo coverage entry %+v", repo)
		}
		if repo.ItemCount != 2 || repo.SpecCount != 1 || repo.DocCount != 1 {
			t.Fatalf("repo coverage entry %+v, want 2 items / 1 spec / 1 doc", repo)
		}
	}
}

func TestRunStatusReportsStaleIndexFreshness(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var rebuildStdout bytes.Buffer
	var rebuildStderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &rebuildStdout, &rebuildStderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, rebuildStderr.String())
	}
	mustWriteFileCmd(t, filepath.Join(repo, "docs", "guides", "api-rate-limits.md"), `
# API Rate Limits

This guide changed after indexing.
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
		return runStatus(nil, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "index freshness: stale") {
		t.Fatalf("runStatus() output %q does not report stale freshness", out)
	}
	if !strings.Contains(out, "index content fingerprint") {
		t.Fatalf("runStatus() output %q does not contain content fingerprint reason", out)
	}
	if !strings.Contains(out, "pituitary index --update") && !strings.Contains(out, "pituitary index --rebuild") {
		t.Fatalf("runStatus() output %q does not contain update or rebuild guidance", out)
	}
}

func TestRunStatusJSONIncludesRuntimeProbeResults(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runStatus([]string{"--format", "json", "--check-runtime", "all"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			CheckRuntime string `json:"check_runtime"`
		} `json:"request"`
		Result struct {
			Runtime struct {
				Scope  string `json:"scope"`
				Checks []struct {
					Name     string `json:"name"`
					Profile  string `json:"profile"`
					Provider string `json:"provider"`
					Timeout  int    `json:"timeout_ms"`
					Status   string `json:"status"`
					Message  string `json:"message"`
				} `json:"checks"`
			} `json:"runtime"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if got, want := payload.Request.CheckRuntime, "all"; got != want {
		t.Fatalf("request.check_runtime = %q, want %q", got, want)
	}
	if got, want := payload.Result.Runtime.Scope, "all"; got != want {
		t.Fatalf("result.runtime.scope = %q, want %q", got, want)
	}
	if len(payload.Result.Runtime.Checks) != 2 {
		t.Fatalf("len(result.runtime.checks) = %d, want 2", len(payload.Result.Runtime.Checks))
	}
	if got, want := payload.Result.Runtime.Checks[0].Name, "runtime.embedder"; got != want {
		t.Fatalf("checks[0].name = %q, want %q", got, want)
	}
	if got, want := payload.Result.Runtime.Checks[0].Status, "ready"; got != want {
		t.Fatalf("checks[0].status = %q, want %q", got, want)
	}
	if payload.Result.Runtime.Checks[0].Profile != "" {
		t.Fatalf("checks[0].profile = %q, want empty for fixture default config", payload.Result.Runtime.Checks[0].Profile)
	}
	if payload.Result.Runtime.Checks[0].Timeout == 0 {
		t.Fatalf("checks[0].timeout_ms = %d, want non-zero default", payload.Result.Runtime.Checks[0].Timeout)
	}
	if got, want := payload.Result.Runtime.Checks[1].Name, "runtime.analysis"; got != want {
		t.Fatalf("checks[1].name = %q, want %q", got, want)
	}
	if got, want := payload.Result.Runtime.Checks[1].Status, "disabled"; got != want {
		t.Fatalf("checks[1].status = %q, want %q", got, want)
	}
}

func writeGovernanceHotspotStatusWorkspace(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	indexPath := filepath.Join(repo, ".pituitary", "pituitary.db")
	configPath := filepath.Join(repo, "pituitary.toml")
	mustWriteFileCmd(t, configPath, fmt.Sprintf(`
[workspace]
root = "."
index_path = "%s"

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
`, filepath.ToSlash(indexPath)))
	mustWriteFileCmd(t, filepath.Join(repo, "specs", "spec-100", "spec.toml"), `
id = "SPEC-100"
title = "Handler Governance"
status = "accepted"
domain = "api"
body = "body.md"
applies_to = [
  "code://src/service/handler.go",
  "code://src/service/shared.go",
]
`)
	mustWriteFileCmd(t, filepath.Join(repo, "specs", "spec-100", "body.md"), `
# Handler Governance
`)
	mustWriteFileCmd(t, filepath.Join(repo, "specs", "spec-200", "spec.toml"), `
id = "SPEC-200"
title = "Worker Governance"
status = "accepted"
domain = "api"
body = "body.md"
applies_to = [
  "code://src/service/handler.go",
  "code://src/service/worker.go",
]
`)
	mustWriteFileCmd(t, filepath.Join(repo, "specs", "spec-200", "body.md"), `
# Worker Governance
`)
	mustWriteFileCmd(t, filepath.Join(repo, "specs", "spec-300", "spec.toml"), `
id = "SPEC-300"
title = "Fanout Governance"
status = "accepted"
domain = "api"
body = "body.md"
applies_to = [
  "code://src/service/fanout-a.go",
  "code://src/service/fanout-b.go",
  "code://src/service/fanout-c.go",
  "code://src/service/fanout-d.go",
]
`)
	mustWriteFileCmd(t, filepath.Join(repo, "specs", "spec-300", "body.md"), `
# Fanout Governance
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}
	insertGovernanceHotspotStatusEdge(t, cfg.Workspace.ResolvedIndexPath, "SPEC-100", "code://src/service/weak.go", "inferred", "inferred", 0.7)
	insertGovernanceHotspotStatusEdge(t, cfg.Workspace.ResolvedIndexPath, "SPEC-200", "code://src/service/weak.go", "inferred", "ambiguous", 0.5)
	return repo
}

func insertGovernanceHotspotStatusEdge(t *testing.T, indexPath, fromRef, toRef, edgeSource, confidence string, confidenceScore float64) {
	t.Helper()

	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(indexPath)+"?mode=rw")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(
		`INSERT OR REPLACE INTO edges (from_ref, to_ref, edge_type, edge_source, confidence, confidence_score) VALUES (?, ?, 'applies_to', ?, ?, ?)`,
		fromRef,
		toRef,
		edgeSource,
		confidence,
		confidenceScore,
	); err != nil {
		t.Fatalf("insert hotspot edge %s -> %s: %v", fromRef, toRef, err)
	}
}

func TestRunStatusJSONIncludesResolvedRuntimeProfiles(t *testing.T) {
	repo := t.TempDir()
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.profiles.local-lm-studio]
provider = "openai_compatible"
endpoint = "http://127.0.0.1:1234/v1"
timeout_ms = 30000
max_retries = 1

[runtime.embedder]
profile = "local-lm-studio"
model = "nomic-embed-text-v1.5"

[runtime.analysis]
profile = "local-lm-studio"
model = "qwen3.5-35b"
timeout_ms = 120000

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

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
			RuntimeConfig struct {
				Embedder struct {
					Profile    string `json:"profile"`
					Provider   string `json:"provider"`
					Model      string `json:"model"`
					Endpoint   string `json:"endpoint"`
					TimeoutMS  int    `json:"timeout_ms"`
					MaxRetries int    `json:"max_retries"`
				} `json:"embedder"`
				Analysis struct {
					Profile   string `json:"profile"`
					Provider  string `json:"provider"`
					Model     string `json:"model"`
					Endpoint  string `json:"endpoint"`
					TimeoutMS int    `json:"timeout_ms"`
				} `json:"analysis"`
			} `json:"runtime_config"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if got, want := payload.Result.RuntimeConfig.Embedder.Profile, "local-lm-studio"; got != want {
		t.Fatalf("result.runtime_config.embedder.profile = %q, want %q", got, want)
	}
	if got, want := payload.Result.RuntimeConfig.Embedder.Provider, "openai_compatible"; got != want {
		t.Fatalf("result.runtime_config.embedder.provider = %q, want %q", got, want)
	}
	if got, want := payload.Result.RuntimeConfig.Embedder.TimeoutMS, 30000; got != want {
		t.Fatalf("result.runtime_config.embedder.timeout_ms = %d, want %d", got, want)
	}
	if got, want := payload.Result.RuntimeConfig.Analysis.Profile, "local-lm-studio"; got != want {
		t.Fatalf("result.runtime_config.analysis.profile = %q, want %q", got, want)
	}
	if got, want := payload.Result.RuntimeConfig.Analysis.Model, "qwen3.5-35b"; got != want {
		t.Fatalf("result.runtime_config.analysis.model = %q, want %q", got, want)
	}
	if got, want := payload.Result.RuntimeConfig.Analysis.TimeoutMS, 120000; got != want {
		t.Fatalf("result.runtime_config.analysis.timeout_ms = %d, want %d", got, want)
	}
}

func TestRunStatusShowsContextualizerDisabledByDefault(t *testing.T) {
	repo := t.TempDir()
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runStatus(nil, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0 (stderr=%q)", exitCode, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "runtime.chunking.contextualizer: disabled") {
		t.Fatalf("runStatus() output %q does not report contextualizer disabled state", out)
	}
}

func TestRunStatusShowsContextualizerFormatWhenEnabled(t *testing.T) {
	repo := t.TempDir()
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[runtime.chunking.contextualizer]
format = "title_ancestry"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runStatus(nil, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0 (stderr=%q)", exitCode, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "runtime.chunking.contextualizer: format=title_ancestry") {
		t.Fatalf("runStatus() output %q does not report active contextualizer format", out)
	}
	if strings.Contains(out, "runtime.chunking.contextualizer: disabled") {
		t.Fatalf("runStatus() output %q incorrectly reports disabled when contextualizer is set", out)
	}
}

func TestRunStatusJSONSurfacesContextualizerForBothPostures(t *testing.T) {
	// The JSON status payload carries the contextualizer field
	// unconditionally (no omitempty) so machine consumers can
	// distinguish "disabled" (empty string) from "field absent /
	// older binary / schema drift". Covers both postures in one
	// test so regression on either path fails loud.
	type jsonPayload struct {
		Result struct {
			RuntimeConfig struct {
				Contextualizer string `json:"contextualizer"`
			} `json:"runtime_config"`
		} `json:"result"`
	}

	cases := []struct {
		name          string
		fixtureBlock  string
		wantFormatVal string
	}{
		{
			name:          "enabled surfaces format",
			fixtureBlock:  "[runtime.chunking.contextualizer]\nformat = \"ref_title\"\n",
			wantFormatVal: "ref_title",
		},
		{
			name:          "disabled surfaces explicit empty string",
			fixtureBlock:  "",
			wantFormatVal: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			mustMkdirAllCmd(t, filepath.Join(repo, "specs"))
			mustWriteIndexFixture(t, repo, fmt.Sprintf(`
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

%s
[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`, tc.fixtureBlock))

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := withWorkingDir(t, repo, func() int {
				return runStatus([]string{"--format", "json"}, &stdout, &stderr)
			})
			if exitCode != 0 {
				t.Fatalf("runStatus() exit code = %d, want 0 (stderr=%q)", exitCode, stderr.String())
			}

			var payload jsonPayload
			if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
				t.Fatalf("unmarshal status json: %v (stdout=%q)", err, stdout.String())
			}
			if got := payload.Result.RuntimeConfig.Contextualizer; got != tc.wantFormatVal {
				t.Fatalf("result.runtime_config.contextualizer = %q, want %q", got, tc.wantFormatVal)
			}

			// Regression: the field itself must be present in the
			// raw JSON even on the disabled path.
			if !strings.Contains(stdout.String(), `"contextualizer":"`) &&
				!strings.Contains(stdout.String(), `"contextualizer": "`) {
				t.Fatalf("status JSON does not carry contextualizer key:\n%s", stdout.String())
			}
		})
	}
}

func TestRunStatusTextIncludesResolvedRuntimeProfiles(t *testing.T) {
	repo := t.TempDir()
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.profiles.local-lm-studio]
provider = "openai_compatible"
endpoint = "http://127.0.0.1:1234/v1"
timeout_ms = 30000
max_retries = 1

[runtime.embedder]
profile = "local-lm-studio"
model = "nomic-embed-text-v1.5"

[runtime.analysis]
profile = "local-lm-studio"
model = "qwen3.5-35b"
timeout_ms = 120000

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

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

	out := stdout.String()
	for _, want := range []string{
		"RUNTIME CONFIG",
		"profile: local-lm-studio",
		"model: nomic-embed-text-v1.5",
		"timeout_ms: 120000",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runStatus() output %q does not contain %q", out, want)
		}
	}
}

func TestRunStatusRuntimeProbeReportsDependencyUnavailable(t *testing.T) {
	repo := t.TempDir()
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"error": "Model unloaded..",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	mustWriteIndexFixture(t, repo, fmt.Sprintf(`
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "openai_compatible"
model = "pituitary-embed"
endpoint = %q
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`, server.URL+"/v1"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runStatus([]string{"--format", "json", "--check-runtime", "embedder"}, &stdout, &stderr)
	})
	if exitCode != 3 {
		t.Fatalf("runStatus() exit code = %d, want 3", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			CheckRuntime string `json:"check_runtime"`
		} `json:"request"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status error payload: %v", err)
	}
	if got, want := payload.Request.CheckRuntime, "embedder"; got != want {
		t.Fatalf("request.check_runtime = %q, want %q", got, want)
	}
	if len(payload.Errors) != 1 {
		t.Fatalf("len(errors) = %d, want 1", len(payload.Errors))
	}
	if got, want := payload.Errors[0].Code, "dependency_unavailable"; got != want {
		t.Fatalf("errors[0].code = %q, want %q", got, want)
	}
	if !strings.Contains(payload.Errors[0].Message, "load or pin model") {
		t.Fatalf("errors[0].message = %q, want model loading guidance", payload.Errors[0].Message)
	}
	if got, want := payload.Errors[0].Details["runtime"], "runtime.embedder"; got != want {
		t.Fatalf("errors[0].details.runtime = %#v, want %q", got, want)
	}
	if got, want := payload.Errors[0].Details["request_type"], "embeddings"; got != want {
		t.Fatalf("errors[0].details.request_type = %#v, want %q", got, want)
	}
	if got, want := payload.Errors[0].Details["failure_class"], "dependency_unavailable"; got != want {
		t.Fatalf("errors[0].details.failure_class = %#v, want %q", got, want)
	}
}

func TestRunStatusReportsConfigError(t *testing.T) {
	repo := t.TempDir()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runStatus(nil, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runStatus() exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "pituitary status: no config found") {
		t.Fatalf("runStatus() stderr %q does not contain config discovery error", stderr.String())
	}
}

func TestRunStatusJSONExplainsShadowedDiscoveredConfig(t *testing.T) {
	repo := t.TempDir()
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks(%q) error = %v", repo, err)
	}
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))
	mustMkdirAllCmd(t, filepath.Join(repo, ".pituitary"))
	mustWriteIndexFixture(t, filepath.Join(repo, ".pituitary"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
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
			ConfigPath       string `json:"config_path"`
			ConfigResolution struct {
				SelectedBy string `json:"selected_by"`
				Reason     string `json:"reason"`
				Candidates []struct {
					Path   string `json:"path"`
					Status string `json:"status"`
				} `json:"candidates"`
			} `json:"config_resolution"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if got, want := payload.Result.ConfigPath, filepath.Join(resolvedRepo, ".pituitary", "pituitary.toml"); got != want {
		t.Fatalf("config_path = %q, want %q", got, want)
	}
	if got, want := payload.Result.ConfigResolution.SelectedBy, configSourceDiscovery; got != want {
		t.Fatalf("selected_by = %q, want %q", got, want)
	}
	var foundShadowed bool
	for _, candidate := range payload.Result.ConfigResolution.Candidates {
		if candidate.Path == filepath.Join(resolvedRepo, "pituitary.toml") && candidate.Status == "shadowed" {
			foundShadowed = true
			break
		}
	}
	if !foundShadowed {
		t.Fatalf("candidates = %+v, want shadowed root config", payload.Result.ConfigResolution.Candidates)
	}
	if !strings.Contains(payload.Result.ConfigResolution.Reason, filepath.ToSlash(filepath.Join(resolvedRepo, "pituitary.toml"))) {
		t.Fatalf("reason = %q, want shadowed root config path", payload.Result.ConfigResolution.Reason)
	}
}

func TestRunStatusJSONWarnsWhenLocalConfigShadowsParentMultirepoConfig(t *testing.T) {
	root, primary, _ := writeShadowedMultiRepoStatusWorkspace(t)
	nested := filepath.Join(primary, "pkg", "nested")
	mustMkdirAllCmd(t, nested)
	resolvedPrimary, err := filepath.EvalSymlinks(primary)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks(%q) error = %v", primary, err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, nested, func() int {
		return runStatus([]string{"--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0 (stdout: %q, stderr: %q)", exitCode, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			ConfigPath string   `json:"config_path"`
			Guidance   []string `json:"guidance"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	assertEquivalentPath(t, "config_path", payload.Result.ConfigPath, filepath.Join(resolvedPrimary, ".pituitary", "pituitary.toml"))
	if len(payload.Result.Guidance) == 0 {
		t.Fatalf("guidance = %v, want multirepo shadowing warning", payload.Result.Guidance)
	}
	sharedConfigPath := filepath.Join(root, ".pituitary", "pituitary.toml")
	if !strings.Contains(payload.Result.Guidance[0], filepath.ToSlash(sharedConfigPath)) {
		t.Fatalf("guidance = %q, want shared config path", payload.Result.Guidance[0])
	}
	if !strings.Contains(payload.Result.Guidance[0], "--config") {
		t.Fatalf("guidance = %q, want --config suggestion", payload.Result.Guidance[0])
	}
}

func TestRunStatusJSONUsesSelectedSharedConfigForArtifactLocations(t *testing.T) {
	root, primary, sharedConfigPath := writeShadowedMultiRepoStatusWorkspace(t)
	nested := filepath.Join(primary, "pkg", "nested")
	mustMkdirAllCmd(t, nested)
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks(%q) error = %v", root, err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, nested, func() int {
		return runStatus([]string{"--config", sharedConfigPath, "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() exit code = %d, want 0 (stdout: %q, stderr: %q)", exitCode, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			ArtifactLocations struct {
				IndexDir               string   `json:"index_dir"`
				DiscoverConfigPath     string   `json:"discover_config_path"`
				CanonicalizeBundleRoot string   `json:"canonicalize_bundle_root"`
				IgnorePatterns         []string `json:"ignore_patterns"`
			} `json:"artifact_locations"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	assertEquivalentPath(t, "artifact_locations.index_dir", payload.Result.ArtifactLocations.IndexDir, filepath.Join(resolvedRoot, ".pituitary"))
	assertEquivalentPath(t, "artifact_locations.discover_config_path", payload.Result.ArtifactLocations.DiscoverConfigPath, sharedConfigPath)
	assertEquivalentPath(t, "artifact_locations.canonicalize_bundle_root", payload.Result.ArtifactLocations.CanonicalizeBundleRoot, filepath.Join(resolvedRoot, ".pituitary", "canonicalized"))
	if len(payload.Result.ArtifactLocations.IgnorePatterns) != 0 {
		t.Fatalf("artifact_locations.ignore_patterns = %v, want none for shared artifact dir outside workspace root", payload.Result.ArtifactLocations.IgnorePatterns)
	}
}

func writeShadowedMultiRepoStatusWorkspace(t *testing.T) (string, string, string) {
	t.Helper()

	root := t.TempDir()
	primary := filepath.Join(root, "primary")
	shared := filepath.Join(root, "shared")
	mustMkdirAllCmd(t, filepath.Join(root, ".pituitary"))
	mustMkdirAllCmd(t, filepath.Join(primary, ".pituitary"))
	mustMkdirAllCmd(t, filepath.Join(primary, "specs"))
	mustMkdirAllCmd(t, filepath.Join(shared, "docs"))

	sharedConfigDir := filepath.Join(root, ".pituitary")
	sharedConfigPath := filepath.Join(sharedConfigDir, "pituitary.toml")
	mustWriteIndexFixture(t, sharedConfigDir, `
[workspace]
root = "`+filepath.ToSlash(primary)+`"
repo_id = "primary"
index_path = "`+filepath.ToSlash(filepath.Join(root, ".pituitary", "pituitary.db"))+`"

[[workspace.repos]]
id = "shared"
root = "`+filepath.ToSlash(shared)+`"

[[sources]]
name = "primary-specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "shared-docs"
adapter = "filesystem"
kind = "markdown_docs"
repo = "shared"
path = "docs"
`)

	mustWriteIndexFixture(t, filepath.Join(primary, ".pituitary"), `
[workspace]
root = "`+filepath.ToSlash(primary)+`"
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	return root, primary, sharedConfigPath
}

func assertEquivalentPath(t *testing.T, label, got, want string) {
	t.Helper()
	if comparablePath(got) != comparablePath(want) {
		t.Fatalf("%s = %q, want %q", label, got, want)
	}
}

func comparablePath(path string) string {
	path = filepath.Clean(path)
	current := path
	suffix := make([]string, 0, 4)
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func TestStatusArtifactLocationsIncludesMultirepoParent(t *testing.T) {
	parentConfig := "/workspace/.pituitary/pituitary.toml"
	resolution := &configResolution{
		ShadowedMultirepoConfig: parentConfig,
	}
	workspaceRoot := t.TempDir()
	configPath := filepath.Join(workspaceRoot, ".pituitary", "pituitary.toml")
	indexPath := filepath.Join(workspaceRoot, ".pituitary", "pituitary.db")
	result := buildStatusArtifactLocations(workspaceRoot, configPath, indexPath, resolution)
	if result == nil {
		t.Fatal("buildStatusArtifactLocations returned nil")
	}
	if got, want := result.MultirepoParent, filepath.ToSlash(parentConfig); got != want {
		t.Fatalf("MultirepoParent = %q, want %q", got, want)
	}
}

func TestStatusArtifactLocationsOmitsMultirepoParentWhenNotShadowed(t *testing.T) {
	resolution := &configResolution{}
	workspaceRoot := t.TempDir()
	configPath := filepath.Join(workspaceRoot, ".pituitary", "pituitary.toml")
	indexPath := filepath.Join(workspaceRoot, ".pituitary", "pituitary.db")
	result := buildStatusArtifactLocations(workspaceRoot, configPath, indexPath, resolution)
	if result == nil {
		t.Fatal("buildStatusArtifactLocations returned nil")
	}
	if result.MultirepoParent != "" {
		t.Fatalf("MultirepoParent = %q, want empty", result.MultirepoParent)
	}
}
