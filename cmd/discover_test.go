package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDiscoverJSON(t *testing.T) {
	repo := writeDiscoveryWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runDiscover([]string{"--path", ".", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runDiscover() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runDiscover() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request discoverRequest `json:"request"`
		Result  struct {
			ConfigPath string `json:"config_path"`
			Sources    []struct {
				Kind      string `json:"kind"`
				ItemCount int    `json:"item_count"`
			} `json:"sources"`
			Preview struct {
				Sources []struct {
					ItemCount int `json:"item_count"`
				} `json:"sources"`
			} `json:"preview"`
			Config string `json:"config"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal discover payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if payload.Request.Path != "." || payload.Request.Write {
		t.Fatalf("request = %+v, want path-only discover request", payload.Request)
	}
	if got, want := len(payload.Result.Sources), 3; got != want {
		t.Fatalf("source count = %d, want %d", got, want)
	}
	if got := payload.Result.Sources[2].ItemCount; got != 2 {
		t.Fatalf("docs item count = %d, want 2", got)
	}
	if got, want := len(payload.Result.Preview.Sources), 3; got != want {
		t.Fatalf("preview source count = %d, want %d", got, want)
	}
	if !strings.Contains(payload.Result.Config, "kind = \"markdown_contract\"") {
		t.Fatalf("generated config %q does not contain markdown_contract source", payload.Result.Config)
	}
	if got := filepath.ToSlash(payload.Result.ConfigPath); !strings.HasSuffix(got, "/.pituitary/pituitary.toml") {
		t.Fatalf("config path = %q, want local .pituitary config suffix", got)
	}
}

func TestRunDiscoverWriteProducesUsableLocalConfig(t *testing.T) {
	repo := writeDiscoveryWorkspace(t)

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
	configPath := filepath.Join(repo, ".pituitary", "pituitary.toml")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("discovered config %s missing: %v", configPath, err)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = withWorkingDir(t, repo, func() int {
		return runPreviewSources([]string{"--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runPreviewSources() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex(--rebuild) exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); err != nil {
		t.Fatalf("rebuilt index missing: %v", err)
	}
}

func TestRunDiscoverWriteSupportsCustomConfigPath(t *testing.T) {
	repo := writeDiscoveryWorkspace(t)
	configPath := filepath.Join(repo, "tools", "pituitary.local.toml")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runDiscover([]string{"--path", ".", "--write", "--config-path", filepath.ToSlash(filepath.Join("tools", "pituitary.local.toml"))}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runDiscover(--write, --config-path) exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("custom discovered config %s missing: %v", configPath, err)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = withWorkingDir(t, repo, func() int {
		return Run([]string{"--config", configPath, "index", "--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("Run(--config, index --rebuild) exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); err != nil {
		t.Fatalf("rebuilt index missing after custom config: %v", err)
	}
}

func TestRunDiscoverWriteConfigWorksAcrossNestedIndexStatusAndAnalysis(t *testing.T) {
	repo := writeDiscoveryWorkspace(t)
	resolvedRepo, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks(%q) error = %v", repo, err)
	}
	nested := filepath.Join(repo, "pkg", "nested")
	mustMkdirAllCmd(t, nested)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runDiscover([]string{"--path", ".", "--write"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runDiscover(--write) exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = withWorkingDir(t, nested, func() int {
		return runIndex([]string{"--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex(--rebuild) from nested dir exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	for _, line := range strings.Split(stderr.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "pituitary index: ") {
			t.Fatalf("runIndex(--rebuild) from nested dir wrote unexpected stderr line %q (full stderr: %q)", line, stderr.String())
		}
	}
	if _, err := os.Stat(filepath.Join(resolvedRepo, ".pituitary", "pituitary.db")); err != nil {
		t.Fatalf("rebuilt index missing: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = withWorkingDir(t, nested, func() int {
		return runStatus([]string{}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus() from nested dir exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus() from nested dir wrote unexpected stderr: %q", stderr.String())
	}
	textOut := stdout.String()
	expectedConfigLine := "config: " + filepath.Join(resolvedRepo, ".pituitary", "pituitary.toml")
	expectedIndexLine := "index path: " + filepath.Join(resolvedRepo, ".pituitary", "pituitary.db")
	var foundConfigLine bool
	var foundIndexLine bool
	for _, line := range strings.Split(textOut, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, expectedConfigLine) {
			foundConfigLine = true
		}
		if strings.HasPrefix(line, expectedIndexLine) {
			foundIndexLine = true
		}
	}
	if !foundConfigLine {
		t.Fatalf("status output %q does not contain config line %q", textOut, expectedConfigLine)
	}
	if !foundIndexLine {
		t.Fatalf("status output %q does not contain index path line %q", textOut, expectedIndexLine)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = withWorkingDir(t, nested, func() int {
		return runStatus([]string{"--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runStatus(--format json) from nested dir exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runStatus(--format json) from nested dir wrote unexpected stderr: %q", stderr.String())
	}

	var statusPayload struct {
		Result struct {
			ConfigPath       string `json:"config_path"`
			IndexPath        string `json:"index_path"`
			IndexExists      bool   `json:"index_exists"`
			ConfigResolution struct {
				SelectedBy string `json:"selected_by"`
			} `json:"config_resolution"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &statusPayload); err != nil {
		t.Fatalf("unmarshal status payload: %v", err)
	}
	if len(statusPayload.Errors) != 0 {
		t.Fatalf("status errors = %+v, want none", statusPayload.Errors)
	}
	if got, want := statusPayload.Result.ConfigPath, filepath.Join(resolvedRepo, ".pituitary", "pituitary.toml"); got != want {
		t.Fatalf("status config_path = %q, want %q", got, want)
	}
	if got, want := statusPayload.Result.IndexPath, filepath.Join(resolvedRepo, ".pituitary", "pituitary.db"); got != want {
		t.Fatalf("status index_path = %q, want %q", got, want)
	}
	if !statusPayload.Result.IndexExists {
		t.Fatalf("status payload = %+v, want index_exists=true", statusPayload.Result)
	}
	if got, want := statusPayload.Result.ConfigResolution.SelectedBy, configSourceDiscovery; got != want {
		t.Fatalf("status selected_by = %q, want %q", got, want)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = withWorkingDir(t, nested, func() int {
		return runAnalyzeImpact([]string{"--spec-ref", "SPEC-042", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runAnalyzeImpact() from nested dir exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runAnalyzeImpact() from nested dir wrote unexpected stderr: %q", stderr.String())
	}

	var impactPayload struct {
		Result struct {
			SpecRef string `json:"spec_ref"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &impactPayload); err != nil {
		t.Fatalf("unmarshal impact payload: %v", err)
	}
	if len(impactPayload.Errors) != 0 {
		t.Fatalf("impact errors = %+v, want none", impactPayload.Errors)
	}
	if got, want := impactPayload.Result.SpecRef, "SPEC-042"; got != want {
		t.Fatalf("impact spec_ref = %q, want %q", got, want)
	}
}

func TestRunDiscoverHelpDoesNotAdvertiseConfigResolution(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"discover", "--help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("Run(discover, --help) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(discover, --help) wrote unexpected stderr: %q", stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "shared config resolution:") {
		t.Fatalf("discover help %q unexpectedly advertises config resolution", out)
	}
	if !strings.Contains(out, "usage: pituitary discover [--path PATH] [--config-path PATH] [--write] [--format FORMAT]") {
		t.Fatalf("discover help %q missing usage line", out)
	}
	if !strings.Contains(out, "--config-path VALUE") {
		t.Fatalf("discover help %q missing --config-path flag", out)
	}
}

func writeDiscoveryWorkspace(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	mustWriteFileCmd(t, filepath.Join(root, "specs", "rate-limit-v2", "spec.toml"), `
id = "SPEC-042"
title = "Per-Tenant API Rate Limits"
status = "accepted"
domain = "api"
body = "body.md"
applies_to = ["code://src/api/middleware/ratelimiter.go"]
`)
	mustWriteFileCmd(t, filepath.Join(root, "specs", "rate-limit-v2", "body.md"), `
# Per-Tenant API Rate Limits
`)
	mustWriteFileCmd(t, filepath.Join(root, "rfcs", "service-sla.md"), `
# Service SLA Contract

Status: review
Domain: api
Applies To:
- code://src/api/service/sla.go
`)
	mustWriteFileCmd(t, filepath.Join(root, "docs", "guides", "api-rate-limits.md"), `
# API Rate Limits Guide
`)
	mustWriteFileCmd(t, filepath.Join(root, "docs", "runbooks", "rate-limit-rollout.md"), `
# Rate Limit Rollout Runbook
`)
	mustWriteFileCmd(t, filepath.Join(root, "docs", "development", "testing-guide.md"), `
# Testing Guide
`)
	mustWriteFileCmd(t, filepath.Join(root, "README.md"), `
# Example Repo
`)
	return root
}
