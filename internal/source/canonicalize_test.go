package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
)

func TestCanonicalizeMarkdownContractPreviewAndWrite(t *testing.T) {
	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "rfcs", "service-sla.md"), `
# Service SLA Contract

Status: review
Domain: api
Applies To:
- code://src/api/service/sla.go

## Overview

This contract captures the public API service-level agreement.
`)

	var result *CanonicalizeResult
	withWorkingDirSource(t, repo, func() {
		var err error
		result, err = CanonicalizeMarkdownContract(CanonicalizeOptions{
			Path: "rfcs/service-sla.md",
		})
		if err != nil {
			t.Fatalf("CanonicalizeMarkdownContract() error = %v", err)
		}
	})

	if got, want := result.Spec.Ref, "contract://rfcs/service-sla"; got != want {
		t.Fatalf("spec ref = %q, want %q", got, want)
	}
	if result.WroteBundle {
		t.Fatal("preview unexpectedly wrote bundle")
	}
	if got, want := result.BundleDir, ".pituitary/canonicalized/service-sla"; got != want {
		t.Fatalf("bundle dir = %q, want %q", got, want)
	}
	if len(result.Files) != 2 {
		t.Fatalf("generated files = %d, want 2", len(result.Files))
	}
	if !strings.Contains(result.Files[0].Content, `id = "contract://rfcs/service-sla"`) {
		t.Fatalf("spec.toml preview %q missing stable ref", result.Files[0].Content)
	}
	if strings.Contains(result.Files[1].Content, "Status: review") {
		t.Fatalf("body.md preview %q still contains lifted metadata", result.Files[1].Content)
	}

	withWorkingDirSource(t, repo, func() {
		if _, err := CanonicalizeMarkdownContract(CanonicalizeOptions{
			Path:  "rfcs/service-sla.md",
			Write: true,
		}); err != nil {
			t.Fatalf("CanonicalizeMarkdownContract(--write) error = %v", err)
		}
	})

	cfg := &config.Config{
		ConfigPath: filepath.Join(repo, ".pituitary", "pituitary.toml"),
		ConfigDir:  repo,
		Workspace: config.Workspace{
			Root:      ".",
			IndexPath: ".pituitary/pituitary.db",
		},
		Runtime: config.Runtime{
			Embedder: config.RuntimeProvider{Provider: "fixture", Model: "fixture-8d", TimeoutMS: 1000},
			Analysis: config.RuntimeProvider{Provider: "disabled", TimeoutMS: 1000},
		},
		Sources: []config.Source{
			{
				Name:    "canonicalized",
				Adapter: config.AdapterFilesystem,
				Kind:    config.SourceKindSpecBundle,
				Path:    ".pituitary/canonicalized",
				Files:   []string{"service-sla/spec.toml"},
			},
		},
	}
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("Validate(canonicalized cfg) error = %v", err)
	}

	loadResult, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig(canonicalized cfg) error = %v", err)
	}
	if got, want := len(loadResult.Specs), 1; got != want {
		t.Fatalf("loaded spec count = %d, want %d", got, want)
	}
	if got, want := loadResult.Specs[0].Ref, "contract://rfcs/service-sla"; got != want {
		t.Fatalf("loaded spec ref = %q, want %q", got, want)
	}
}

func withWorkingDirSource(t *testing.T, dir string, fn func()) {
	t.Helper()

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	fn()
}
