package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRunIndexSupportsGlobalConfigFlag(t *testing.T) {
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

	configPath := filepath.Join(repo, "pituitary.toml")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, t.TempDir(), func() int {
		return Run([]string{"--config", configPath, "index", "--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run() wrote unexpected stderr: %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); err != nil {
		t.Fatalf("Run() did not create database via global config: %v", err)
	}
}

func TestRunIndexSupportsPerCommandConfigFlag(t *testing.T) {
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

	configPath := filepath.Join(repo, "pituitary.toml")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, t.TempDir(), func() int {
		return runIndex([]string{"--config", configPath, "--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runIndex() wrote unexpected stderr: %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); err != nil {
		t.Fatalf("runIndex() did not create database via per-command config: %v", err)
	}
}

func TestRunIndexUsesPituitaryConfigEnv(t *testing.T) {
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

	t.Setenv(configEnvVar, filepath.Join(repo, "pituitary.toml"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, t.TempDir(), func() int {
		return runIndex([]string{"--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runIndex() wrote unexpected stderr: %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); err != nil {
		t.Fatalf("runIndex() did not create database via %s: %v", configEnvVar, err)
	}
}

func TestRunIndexDiscoversLocalConfigFromParentDirectoryAndPrefersIt(t *testing.T) {
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
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))

	nested := filepath.Join(repo, "pkg", "nested")
	mustMkdirAllCmd(t, nested)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, nested, func() int {
		return runIndex([]string{"--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runIndex() wrote unexpected stderr: %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); err != nil {
		t.Fatalf("runIndex() did not create database via discovered local config: %v", err)
	}
}
