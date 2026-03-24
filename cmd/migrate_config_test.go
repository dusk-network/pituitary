package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunMigrateConfigJSONPreview(t *testing.T) {
	repo := t.TempDir()
	configPath := filepath.Join(repo, "pituitary.toml")
	mustWriteFileCmd(t, configPath, `
[project]
id = "ccd"
name = "Continuous Context Development"
specs_dir = "specs"
`)
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runMigrateConfig([]string{"--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runMigrateConfig() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runMigrateConfig() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request migrateConfigRequest `json:"request"`
		Result  struct {
			ConfigPath          string   `json:"config_path"`
			DetectedSchema      string   `json:"detected_schema"`
			TargetSchemaVersion int      `json:"target_schema_version"`
			WroteConfig         bool     `json:"wrote_config"`
			Notes               []string `json:"notes"`
			Config              string   `json:"config"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal migrate-config payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if got, want := payload.Request.Path, defaultConfigName; got != want {
		t.Fatalf("request path = %q, want %q", got, want)
	}
	if payload.Result.WroteConfig {
		t.Fatalf("result = %+v, want preview without write", payload.Result)
	}
	if got, want := payload.Result.DetectedSchema, "project.v1"; got != want {
		t.Fatalf("detected schema = %q, want %q", got, want)
	}
	if got, want := payload.Result.TargetSchemaVersion, 2; got != want {
		t.Fatalf("target schema version = %d, want %d", got, want)
	}
	if !strings.Contains(payload.Result.Config, "schema_version = 2") {
		t.Fatalf("migrated config %q does not contain schema_version", payload.Result.Config)
	}
}

func TestRunMigrateConfigWriteOverwritesFile(t *testing.T) {
	repo := t.TempDir()
	configPath := filepath.Join(repo, "pituitary.toml")
	mustWriteFileCmd(t, configPath, `
[project]
id = "ccd"
name = "Continuous Context Development"
specs_dir = "specs"
`)
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runMigrateConfig([]string{"--write"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runMigrateConfig(--write) exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runMigrateConfig(--write) wrote unexpected stderr: %q", stderr.String())
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read migrated config: %v", err)
	}
	if !strings.Contains(string(content), "schema_version = 2") {
		t.Fatalf("written config %q does not contain schema_version", string(content))
	}
	if strings.Contains(string(content), "[project]") {
		t.Fatalf("written config %q still contains legacy project section", string(content))
	}
}

func TestRunMigrateConfigHelpDoesNotAdvertiseConfigResolution(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"migrate-config", "--help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("Run(migrate-config, --help) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(migrate-config, --help) wrote unexpected stderr: %q", stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "shared config resolution:") {
		t.Fatalf("migrate-config help %q unexpectedly advertises config resolution", out)
	}
	if !strings.Contains(out, "usage: pituitary migrate-config [--path PATH] [--write] [--format FORMAT]") {
		t.Fatalf("migrate-config help %q missing usage line", out)
	}
}
