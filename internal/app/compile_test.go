package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestCompileTerminologyPlansEdits(t *testing.T) {
	t.Parallel()

	configPath := writeCompileTerminologyWorkspace(t)
	operation := CompileTerminology(context.Background(), configPath, CompileRequest{
		Scope: "docs",
	})
	if operation.Issue != nil {
		t.Fatalf("CompileTerminology() issue = %+v, want nil", operation.Issue)
	}
	if operation.Result == nil {
		t.Fatal("CompileTerminology() result = nil, want structured result")
	}
	if operation.Result.Applied {
		t.Fatal("CompileTerminology() result.applied = true, want false")
	}
	if operation.Result.PlannedFileCount < 1 {
		t.Fatalf("planned_file_count = %d, want at least 1", operation.Result.PlannedFileCount)
	}
	if operation.Result.PlannedEditCount < 1 {
		t.Fatalf("planned_edit_count = %d, want at least 1", operation.Result.PlannedEditCount)
	}

	// Verify that the source file is unchanged (dry-run).
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	kernelPath := filepath.Join(cfg.Workspace.RootPath, "docs", "guides", "repo-kernel.md")
	content, err := os.ReadFile(kernelPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if !strings.Contains(string(content), "repo") {
		t.Fatal("dry-run should NOT have modified the file")
	}

	// Verify edits are structured correctly.
	var foundEdits bool
	for _, file := range operation.Result.Files {
		for _, edit := range file.Edits {
			foundEdits = true
			if edit.Code != "terminology_compile" {
				t.Fatalf("edit.code = %q, want terminology_compile", edit.Code)
			}
			if edit.Action != "replace_term" {
				t.Fatalf("edit.action = %q, want replace_term", edit.Action)
			}
			if edit.Replace == "" || edit.With == "" {
				t.Fatalf("edit replace=%q with=%q, want non-empty", edit.Replace, edit.With)
			}
		}
	}
	if !foundEdits {
		t.Fatal("expected at least one edit in the result files")
	}

	if len(operation.Result.Guidance) == 0 || !strings.Contains(operation.Result.Guidance[0], "--yes") {
		t.Fatalf("guidance = %v, want apply guidance", operation.Result.Guidance)
	}
}

func TestCompileTerminologyAppliesEdits(t *testing.T) {
	t.Parallel()

	configPath := writeCompileTerminologyWorkspace(t)
	operation := CompileTerminology(context.Background(), configPath, CompileRequest{
		Scope: "docs",
		Apply: true,
	})
	if operation.Issue != nil {
		t.Fatalf("CompileTerminology() issue = %+v, want nil", operation.Issue)
	}
	if operation.Result == nil {
		t.Fatal("CompileTerminology() result = nil, want structured result")
	}
	if !operation.Result.Applied {
		t.Fatal("CompileTerminology() result.applied = false, want true")
	}
	if operation.Result.AppliedFileCount < 1 {
		t.Fatalf("applied_file_count = %d, want at least 1", operation.Result.AppliedFileCount)
	}
	if operation.Result.AppliedEditCount < 1 {
		t.Fatalf("applied_edit_count = %d, want at least 1", operation.Result.AppliedEditCount)
	}
	if len(operation.Result.Guidance) == 0 || !strings.Contains(operation.Result.Guidance[0], "pituitary index --rebuild") {
		t.Fatalf("guidance = %v, want rebuild guidance", operation.Result.Guidance)
	}

	// Verify the file was actually changed.
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	kernelPath := filepath.Join(cfg.Workspace.RootPath, "docs", "guides", "repo-kernel.md")
	content, err := os.ReadFile(kernelPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	updated := string(content)

	// The displaced term "repo" should be replaced by "locality".
	if strings.Contains(updated, "each repo") {
		t.Fatal("expected displaced term 'repo' to be replaced in the file")
	}
	// The preferred term should be present.
	if !strings.Contains(updated, "locality") {
		t.Fatal("expected preferred term 'locality' to appear in the file")
	}
}

func TestCompileTerminologySkipsTolerated(t *testing.T) {
	t.Parallel()

	configPath := writeCompileTerminologyWorkspace(t)
	operation := CompileTerminology(context.Background(), configPath, CompileRequest{
		Scope: "docs",
	})
	if operation.Issue != nil {
		t.Fatalf("CompileTerminology() issue = %+v, want nil", operation.Issue)
	}
	if operation.Result == nil {
		t.Fatal("CompileTerminology() result = nil, want structured result")
	}

	// The compatibility doc should NOT appear in findings (tolerated findings
	// are separated into the tolerated slice by CheckTerminology, so they
	// never reach the compile pipeline).
	for _, file := range operation.Result.Files {
		if strings.Contains(file.Ref, "repo-compatibility") {
			t.Fatalf("tolerated doc %q should not produce compile edits", file.Ref)
		}
	}
}

func TestCompileTerminologySkipsCodeBlocksAndPaths(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustWriteCompileFile(t, filepath.Join(repo, "specs", "kernel-locality", "spec.toml"), `
id = "SPEC-LOCALITY"
title = "Kernel Locality Contract"
status = "accepted"
domain = "kernel"
body = "body.md"
`)
	mustWriteCompileFile(t, filepath.Join(repo, "specs", "kernel-locality", "body.md"), `
# Kernel Locality Contract

## Core Model

The kernel keeps continuity in clone-local state.
`)
	// Doc with term in prose, code block, inline code, and path context.
	mustWriteCompileFile(t, filepath.Join(repo, "docs", "guides", "mixed-contexts.md"), strings.Join([]string{
		"# Mixed Contexts",
		"",
		"The repo is the main workspace.",
		"",
		"```",
		"cd ~/devel/repo/src",
		"```",
		"",
		"See `repo.json` for config.",
		"",
		"Check /opt/repo/bin for binaries.",
		"",
		"Also see repo-server for details.",
	}, "\n"))

	configContent := strings.TrimSpace(`
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[terminology.policies]]
preferred = "locality"
historical_aliases = ["repo"]
docs_severity = "error"
specs_severity = "warning"

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
include = ["guides/*.md"]
`) + "\n"

	configPath := filepath.Join(repo, "pituitary.toml")
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig: %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild: %v", err)
	}

	operation := CompileTerminology(context.Background(), configPath, CompileRequest{
		Scope: "all",
		Apply: true,
	})
	if operation.Issue != nil {
		t.Fatalf("CompileTerminology() issue = %+v", operation.Issue)
	}

	content, err := os.ReadFile(filepath.Join(repo, "docs", "guides", "mixed-contexts.md"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(content)

	// Prose "repo" should be replaced.
	if strings.Contains(body, "The repo is") {
		t.Error("prose 'repo' should have been replaced")
	}
	if !strings.Contains(body, "The locality is") {
		t.Error("expected 'locality' in prose position")
	}

	// Code block content should be unchanged.
	if !strings.Contains(body, "cd ~/devel/repo/src") {
		t.Error("code block content should not be modified")
	}

	// Inline code should be unchanged.
	if !strings.Contains(body, "`repo.json`") {
		t.Error("inline code should not be modified")
	}

	// Path context should be unchanged.
	if !strings.Contains(body, "/opt/repo/bin") {
		t.Error("path context should not be modified")
	}

	// Hyphenated compound should be unchanged.
	if !strings.Contains(body, "repo-server") {
		t.Error("hyphenated compound should not be modified")
	}
}

func TestPreserveCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		original    string
		replacement string
		want        string
	}{
		{"Repo", "locality", "Locality"},
		{"REPO", "locality", "LOCALITY"},
		{"repo", "locality", "locality"},
		{"Workflow", "continuity", "Continuity"},
		{"WORKFLOW", "continuity", "CONTINUITY"},
		{"workflow", "continuity", "continuity"},
		{"", "locality", "locality"},
		{"Repo", "", ""},
	}

	for _, tc := range tests {
		got := preserveCase(tc.original, tc.replacement)
		if got != tc.want {
			t.Errorf("preserveCase(%q, %q) = %q, want %q", tc.original, tc.replacement, got, tc.want)
		}
	}
}

func writeCompileTerminologyWorkspace(t *testing.T) string {
	t.Helper()

	repo := t.TempDir()
	mustWriteCompileFile(t, filepath.Join(repo, "specs", "kernel-locality", "spec.toml"), `
id = "SPEC-LOCALITY"
title = "Kernel Locality Contract"
status = "accepted"
domain = "kernel"
body = "body.md"
`)
	mustWriteCompileFile(t, filepath.Join(repo, "specs", "kernel-locality", "body.md"), `
# Kernel Locality Contract

## Core Model

The kernel keeps continuity in clone-local state.
The runtime is locality-centric and treats repository adapters as optional extensions.
`)
	mustWriteCompileFile(t, filepath.Join(repo, "docs", "guides", "repo-kernel.md"), `
# Repo Kernel Guide

The kernel keeps workflow continuity in each repo.
Repository storage is the default operator model.
`)
	mustWriteCompileFile(t, filepath.Join(repo, "docs", "guides", "repo-compatibility.md"), `
# Repo Compatibility Notes

Legacy repo references remain available only as a compatibility alias during migration to locality.
`)

	configContent := `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[terminology.policies]]
preferred = "locality"
historical_aliases = ["repo"]
forbidden_current = ["repository"]
docs_severity = "error"
specs_severity = "warning"

[[terminology.policies]]
preferred = "continuity"
deprecated_terms = ["workflow"]
docs_severity = "error"
specs_severity = "warning"

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
include = ["guides/*.md"]
`

	configPath := filepath.Join(repo, "pituitary.toml")
	if err := os.WriteFile(configPath, []byte(strings.TrimSpace(configContent)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

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

	return configPath
}

func mustWriteCompileFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
