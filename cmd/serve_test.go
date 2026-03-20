package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunServeHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runServe([]string{"--help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("runServe() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runServe() wrote unexpected stderr: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "pituitary serve") {
		t.Fatalf("runServe() help output %q does not mention command", stdout.String())
	}
}

func TestRunServeRejectsUnsupportedTransport(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runServe([]string{"--transport", "http"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("runServe() exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runServe() wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `unsupported transport "http"`) {
		t.Fatalf("runServe() stderr %q does not contain unsupported transport detail", stderr.String())
	}
}

func TestRunServeRejectsMissingIndex(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "specs"), 0o755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}

	configPath := filepath.Join(root, "pituitary.toml")
	if err := os.WriteFile(configPath, []byte(`
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
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runServe([]string{"--config", configPath}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("runServe() exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runServe() wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "does not exist") {
		t.Fatalf("runServe() stderr %q does not contain missing-index detail", stderr.String())
	}
}
