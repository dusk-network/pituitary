package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAcceptsExplicitSchemaVersion(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
schema_version = 3

[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.SchemaVersion, CurrentSchemaVersion; got != want {
		t.Fatalf("schema version = %d, want %d", got, want)
	}
}

func TestLoadRejectsUnsupportedSchemaVersion(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
schema_version = 9

[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want schema-version failure")
	}
	if !strings.Contains(err.Error(), "unsupported schema_version 9") {
		t.Fatalf("Load() error = %q, want schema-version detail", err)
	}
	if !strings.Contains(err.Error(), "newer Pituitary version") || !strings.Contains(err.Error(), "upgrade Pituitary") {
		t.Fatalf("Load() error = %q, want newer-version guidance", err)
	}
	if strings.Contains(err.Error(), "migrate-config") {
		t.Fatalf("Load() error = %q, want no migrate-config hint for unsupported schema", err)
	}
}

func TestLoadRejectsOlderUnsupportedSchemaVersionWithRecoveryGuidance(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
schema_version = 2

[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want schema-version failure")
	}
	if !strings.Contains(err.Error(), "unsupported schema_version 2") {
		t.Fatalf("Load() error = %q, want schema-version detail", err)
	}
	if !strings.Contains(err.Error(), "older than this Pituitary version") || !strings.Contains(err.Error(), "cannot be migrated automatically") {
		t.Fatalf("Load() error = %q, want older-version guidance", err)
	}
	if strings.Contains(err.Error(), "migrate-config") {
		t.Fatalf("Load() error = %q, want no migrate-config hint for unsupported schema", err)
	}
}

func TestLoadRejectsLegacyProjectConfigWithMigrationHint(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[project]
id = "ccd"
name = "Continuous Context Development"
specs_dir = "specs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want legacy-config failure")
	}
	if !strings.Contains(err.Error(), "legacy config schema detected") {
		t.Fatalf("Load() error = %q, want legacy-config detail", err)
	}
	if !strings.Contains(err.Error(), "migrate-config") {
		t.Fatalf("Load() error = %q, want migrate-config hint", err)
	}
}

func TestMigrateFileRewritesLegacyProjectConfig(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[project]
id = "ccd"
name = "Continuous Context Development"
specs_dir = "specs"
`)

	result, err := MigrateFile(configPath)
	if err != nil {
		t.Fatalf("MigrateFile() error = %v", err)
	}
	if got, want := result.DetectedSchema, legacyProjectSchemaName; got != want {
		t.Fatalf("detected schema = %q, want %q", got, want)
	}
	if got, want := result.Config.SchemaVersion, CurrentSchemaVersion; got != want {
		t.Fatalf("schema version = %d, want %d", got, want)
	}
	if got, want := result.Config.Workspace.Root, "."; got != want {
		t.Fatalf("workspace root = %q, want %q", got, want)
	}
	if got, want := result.Config.Sources[0].Path, "specs"; got != want {
		t.Fatalf("spec source path = %q, want %q", got, want)
	}

	rendered, err := Render(result.Config)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(rendered, "schema_version = 3") {
		t.Fatalf("rendered config %q does not contain schema_version", rendered)
	}
	if !strings.Contains(rendered, "kind = \"spec_bundle\"") {
		t.Fatalf("rendered config %q does not contain spec source", rendered)
	}
}

func TestMigrateFileNormalizesCurrentUnversionedConfig(t *testing.T) {
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
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	result, err := MigrateFile(configPath)
	if err != nil {
		t.Fatalf("MigrateFile() error = %v", err)
	}
	if got, want := result.DetectedSchema, "current.unversioned"; got != want {
		t.Fatalf("detected schema = %q, want %q", got, want)
	}
	if len(result.Notes) == 0 || !strings.Contains(result.Notes[0], "added explicit schema_version") {
		t.Fatalf("notes = %#v, want explicit schema-version note", result.Notes)
	}
}
