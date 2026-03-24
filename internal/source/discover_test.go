package source

import (
	"path/filepath"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
)

func TestDiscoverWorkspaceBuildsConservativeConfig(t *testing.T) {
	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "specs", "rate-limit-v2", "spec.toml"), `
id = "SPEC-042"
title = "Per-Tenant API Rate Limits"
status = "accepted"
domain = "api"
body = "body.md"
applies_to = ["code://src/api/middleware/ratelimiter.go"]
`)
	mustWriteFile(t, filepath.Join(repo, "specs", "rate-limit-v2", "body.md"), `
# Per-Tenant API Rate Limits
`)
	mustWriteFile(t, filepath.Join(repo, "rfcs", "service-sla.md"), `
# Service SLA Contract

Status: review
Domain: api
Applies To:
- code://src/api/service/sla.go
`)
	mustWriteFile(t, filepath.Join(repo, "docs", "guides", "api-rate-limits.md"), `
# API Rate Limits Guide
`)
	mustWriteFile(t, filepath.Join(repo, "docs", "runbooks", "rate-limit-rollout.md"), `
# Rate Limit Rollout Runbook
`)
	mustWriteFile(t, filepath.Join(repo, "docs", "reference", "rate-limit-keys.md"), `
# Rate Limit Reference
`)
	mustWriteFile(t, filepath.Join(repo, "docs", "development", "testing-guide.md"), `
# Testing Guide
`)
	mustWriteFile(t, filepath.Join(repo, "README.md"), `
# Example Repo
`)

	result, err := DiscoverWorkspace(DiscoverOptions{RootPath: repo})
	if err != nil {
		t.Fatalf("DiscoverWorkspace() error = %v", err)
	}

	if got, want := len(result.Sources), 3; got != want {
		t.Fatalf("source count = %d, want %d", got, want)
	}
	if got, want := result.Sources[0].Kind, config.SourceKindSpecBundle; got != want {
		t.Fatalf("first source kind = %q, want %q", got, want)
	}
	if got, want := result.Sources[1].Kind, config.SourceKindMarkdownContract; got != want {
		t.Fatalf("second source kind = %q, want %q", got, want)
	}
	if got, want := result.Sources[2].Kind, config.SourceKindMarkdownDocs; got != want {
		t.Fatalf("third source kind = %q, want %q", got, want)
	}
	if got := result.Sources[2].ItemCount; got != 3 {
		t.Fatalf("docs source item count = %d, want 3", got)
	}
	if got := result.Sources[1].Confidence; got != "high" {
		t.Fatalf("contract source confidence = %q, want high", got)
	}
	var foundReferenceDoc bool
	for _, item := range result.Sources[2].Items {
		if item.Path == "docs/reference/rate-limit-keys.md" {
			foundReferenceDoc = true
			break
		}
	}
	if !foundReferenceDoc {
		t.Fatalf("docs items = %+v, want docs/reference/rate-limit-keys.md included", result.Sources[2].Items)
	}
	if result.Preview == nil || len(result.Preview.Sources) != 3 {
		t.Fatalf("preview = %+v, want 3 sources", result.Preview)
	}
	if got := result.ConfigPath; got != filepath.Join(repo, ".pituitary", "pituitary.toml") {
		t.Fatalf("config path = %q, want local .pituitary config", got)
	}
	if got := result.Config; got == "" {
		t.Fatal("generated config = empty, want TOML")
	}
}

func TestDiscoverWorkspaceSupportsCustomConfigPath(t *testing.T) {
	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "specs", "rate-limit-v2", "spec.toml"), `
id = "SPEC-042"
title = "Per-Tenant API Rate Limits"
status = "accepted"
domain = "api"
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(repo, "specs", "rate-limit-v2", "body.md"), `
# Per-Tenant API Rate Limits
`)
	customConfigPath := filepath.Join(repo, "tools", "pituitary.local.toml")

	result, err := DiscoverWorkspace(DiscoverOptions{
		RootPath:   repo,
		ConfigPath: filepath.ToSlash(filepath.Join("tools", "pituitary.local.toml")),
		Write:      true,
	})
	if err != nil {
		t.Fatalf("DiscoverWorkspace() error = %v", err)
	}

	if got, want := result.ConfigPath, customConfigPath; got != want {
		t.Fatalf("config path = %q, want %q", got, want)
	}
	if !result.WroteConfig {
		t.Fatalf("result = %+v, want wrote_config=true", result)
	}

	cfg, err := config.Load(result.ConfigPath)
	if err != nil {
		t.Fatalf("config.Load(custom discovered config) error = %v", err)
	}
	if got, want := cfg.Workspace.RootPath, repo; got != want {
		t.Fatalf("workspace root path = %q, want %q", got, want)
	}
	if got, want := cfg.Workspace.ResolvedIndexPath, filepath.Join(repo, ".pituitary", "pituitary.db"); got != want {
		t.Fatalf("resolved index path = %q, want %q", got, want)
	}
}
