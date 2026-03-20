package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadResolvesWorkspaceAndSourcePaths(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	workspace := filepath.Join(repo, "workspace")
	mustMkdirAll(t, filepath.Join(workspace, "specs"))
	mustMkdirAll(t, filepath.Join(workspace, "docs"))

	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

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
`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.Workspace.RootPath, filepath.Clean(workspace); got != want {
		t.Fatalf("workspace root path = %q, want %q", got, want)
	}
	if got, want := cfg.Workspace.ResolvedIndexPath, filepath.Join(workspace, ".pituitary", "pituitary.db"); got != want {
		t.Fatalf("resolved index path = %q, want %q", got, want)
	}
	if got, want := cfg.Sources[0].ResolvedPath, filepath.Join(workspace, "specs"); got != want {
		t.Fatalf("spec source path = %q, want %q", got, want)
	}
	if got, want := cfg.Sources[1].ResolvedPath, filepath.Join(workspace, "docs"); got != want {
		t.Fatalf("doc source path = %q, want %q", got, want)
	}
}

func TestLoadRejectsUnknownAdapter(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "github"
kind = "spec_bundle"
path = "specs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), `source "specs".adapter: unsupported adapter "github"`) {
		t.Fatalf("Load() error = %q, want unknown adapter details", err)
	}
}

func TestLoadRejectsMissingSourcePath(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "missing-docs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), `source "docs".path: "missing-docs" does not exist`) {
		t.Fatalf("Load() error = %q, want missing path details", err)
	}
}

func TestLoadRejectsIndexPathThatIsDirectory(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	mustMkdirAll(t, filepath.Join(repo, ".pituitary"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), `workspace.index_path: ".pituitary" resolves to a directory`) {
		t.Fatalf("Load() error = %q, want directory validation", err)
	}
}

func TestLoadRejectsUnknownSection(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[github]
repo = "dusk-network/pituitary"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want parse error")
	}
	if !strings.Contains(err.Error(), `unsupported section "github"`) {
		t.Fatalf("Load() error = %q, want unsupported section message", err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
