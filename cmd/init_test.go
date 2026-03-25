package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInitDryRunJSON(t *testing.T) {
	repo := writeDiscoveryWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runInit([]string{"--path", ".", "--dry-run", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runInit(--dry-run) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runInit(--dry-run) wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request initRequest `json:"request"`
		Result  struct {
			ConfigAction string `json:"config_action"`
			Index        any    `json:"index"`
			Status       any    `json:"status"`
			Discover     struct {
				ConfigPath string        `json:"config_path"`
				Sources    []interface{} `json:"sources"`
			} `json:"discover"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal init payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if !payload.Request.DryRun || payload.Request.Path != "." {
		t.Fatalf("request = %+v, want dry-run path request", payload.Request)
	}
	if got, want := payload.Result.ConfigAction, "preview"; got != want {
		t.Fatalf("config_action = %q, want %q", got, want)
	}
	if got, want := len(payload.Result.Discover.Sources), 4; got != want {
		t.Fatalf("discover sources = %d, want %d", got, want)
	}
	if payload.Result.Index != nil {
		t.Fatalf("index = %+v, want nil in dry-run", payload.Result.Index)
	}
	if payload.Result.Status != nil {
		t.Fatalf("status = %+v, want nil in dry-run", payload.Result.Status)
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.toml")); !os.IsNotExist(err) {
		t.Fatalf("config unexpectedly exists after dry-run: %v", err)
	}
}

func TestRunInitWritesConfigRebuildsAndSummarizes(t *testing.T) {
	repo := writeDiscoveryWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runInit([]string{"--path", "."}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runInit() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	for _, line := range strings.Split(stderr.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "pituitary init: ") {
			t.Fatalf("runInit() wrote unexpected stderr line %q (full stderr: %q)", line, stderr.String())
		}
	}

	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.toml")); err != nil {
		t.Fatalf("discovered config missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); err != nil {
		t.Fatalf("rebuilt index missing: %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"━━◈ init",
		"config: ",
		"action: wrote",
		"index: 2 specs · 3 docs",
		"pituitary check-doc-drift --scope all",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("runInit() output %q does not contain %q", output, want)
		}
	}
}

func TestRunInitReportsFixtureGuidanceForLargerCorpus(t *testing.T) {
	repo := writeDiscoveryWorkspace(t)
	mustWriteFileCmd(t, filepath.Join(repo, "docs", "guides", "additional-guide.md"), `
# Additional Guide
`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runInit([]string{"--path", "."}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runInit() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, `runtime.embedder is still "fixture" on 6 indexed artifact(s)`) {
		t.Fatalf("runInit() output %q does not contain fixture guidance", output)
	}
	if !strings.Contains(output, "`pituitary status --check-runtime embedder`") {
		t.Fatalf("runInit() output %q does not contain runtime probe guidance", output)
	}
}

func TestRunInitRejectsExistingConfig(t *testing.T) {
	repo := writeDiscoveryWorkspace(t)
	mustMkdirAllCmd(t, filepath.Join(repo, ".pituitary"))
	mustWriteFileCmd(t, filepath.Join(repo, ".pituitary", "pituitary.toml"), "schema_version = 2\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runInit([]string{"--path", "."}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runInit() exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runInit() wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "config already exists") {
		t.Fatalf("runInit() stderr %q does not contain existing-config message", stderr.String())
	}
}
