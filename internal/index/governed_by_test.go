package index

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestGovernedByContextResolvesIndexedDocAppliesToRefs(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	configPath := filepath.Join(t.TempDir(), "pituitary.toml")
	indexPath := filepath.Join(t.TempDir(), "pituitary.db")

	mustWriteFile(t, filepath.Join(repo, "specs", "reference-adapter", "spec.toml"), `
id = "SPEC-DOC"
title = "Reference Adapter"
status = "accepted"
domain = "docs"
body = "body.md"
applies_to = ["doc://guides/reference-adapter"]
`)
	mustWriteFile(t, filepath.Join(repo, "specs", "reference-adapter", "body.md"), `
## Requirements

Reference adapter documentation is governed explicitly.
`)
	mustWriteFile(t, filepath.Join(repo, "docs", "guides", "reference-adapter.md"), `
# Reference Adapter

This guide is governed explicitly.
`)
	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(repo)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

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
include = ["guides/*.md"]
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	result, err := GovernedByContext(context.Background(), cfg.Workspace.ResolvedIndexPath, "docs/guides/reference-adapter.md", "", "")
	if err != nil {
		t.Fatalf("GovernedByContext() error = %v", err)
	}
	if result.Path != "docs/guides/reference-adapter.md" {
		t.Fatalf("result.Path = %q, want normalized doc path", result.Path)
	}
	if !containsGovernedRef(result.Refs, "doc://guides/reference-adapter") {
		t.Fatalf("result.Refs = %v, want indexed doc ref candidate", result.Refs)
	}
	if len(result.Specs) != 1 || result.Specs[0].Ref != "SPEC-DOC" {
		t.Fatalf("result.Specs = %+v, want SPEC-DOC", result.Specs)
	}
}

func containsGovernedRef(refs []string, want string) bool {
	for _, ref := range refs {
		if ref == want {
			return true
		}
	}
	return false
}
