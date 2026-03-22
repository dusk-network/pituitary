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
	if !strings.Contains(out, "usage: pituitary discover [--path PATH] [--write] [--format FORMAT]") {
		t.Fatalf("discover help %q missing usage line", out)
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
