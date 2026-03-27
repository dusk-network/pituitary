package source

import (
	"path/filepath"
	"strings"
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
	mustWriteFile(t, filepath.Join(repo, "docs", "misc", "preferences.md"), `
# Preferences
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

	if got, want := len(result.Sources), 4; got != want {
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
	if got, want := result.Sources[3].Kind, config.SourceKindMarkdownDocs; got != want {
		t.Fatalf("fourth source kind = %q, want %q", got, want)
	}
	if got, want := result.Sources[3].Name, "project-docs"; got != want {
		t.Fatalf("fourth source name = %q, want %q", got, want)
	}
	if got := result.Sources[3].ItemCount; got != 1 {
		t.Fatalf("project-docs source item count = %d, want 1", got)
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
	if result.Preview == nil || len(result.Preview.Sources) != 4 {
		t.Fatalf("preview = %+v, want 4 sources", result.Preview)
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

func TestDiscoverDetectsIntentArtifacts(t *testing.T) {
	repo := t.TempDir()

	wellKnown := map[string]string{
		"CLAUDE.md":       "# CLAUDE\n\nAgent instructions.",
		"AGENTS.md":       "# AGENTS\n\nCanonical AI policy.",
		"ARCHITECTURE.md": "# Architecture\n\nSystem design overview.",
		"CONTRIBUTING.md": "# Contributing\n\nHow to contribute.",
		"README.md":       "# README\n\nProject landing page.",
	}
	for name, content := range wellKnown {
		mustWriteFile(t, filepath.Join(repo, name), content)
	}

	result, err := DiscoverWorkspace(DiscoverOptions{RootPath: repo})
	if err != nil {
		t.Fatalf("DiscoverWorkspace() error = %v", err)
	}

	// Well-known files should appear in their own "project-docs" source,
	// separate from regular docs, to avoid shifting the common source root.
	var projectDocsSrc *DiscoveredSource
	for i, src := range result.Sources {
		if src.Name == "project-docs" {
			projectDocsSrc = &result.Sources[i]
			break
		}
	}
	if projectDocsSrc == nil {
		t.Fatalf("expected a 'project-docs' source, got sources: %+v", result.Sources)
	}
	if projectDocsSrc.Kind != config.SourceKindMarkdownDocs {
		t.Fatalf("project-docs kind = %q, want %q", projectDocsSrc.Kind, config.SourceKindMarkdownDocs)
	}

	projectItems := map[string]bool{}
	for _, item := range projectDocsSrc.Items {
		projectItems[item.Path] = true
	}
	for name := range wellKnown {
		if !projectItems[name] {
			t.Errorf("expected %s to be in project-docs source, got items: %v", name, projectItems)
		}
	}
}

func TestDiscoverWorkspaceSkipsConflictingMarkdownContractRefs(t *testing.T) {
	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "rfcs", "primary.md"), `
# Primary Contract

Ref: DUPE-001
Status: draft
Domain: api
`)
	mustWriteFile(t, filepath.Join(repo, "notes.md"), `
# Root Proposal

Ref: DUPE-001
Status: draft
Domain: api
`)

	result, err := DiscoverWorkspace(DiscoverOptions{RootPath: repo})
	if err != nil {
		t.Fatalf("DiscoverWorkspace() error = %v", err)
	}

	if got, want := len(result.Warnings), 1; got != want {
		t.Fatalf("warning count = %d, want %d", got, want)
	}
	warning := result.Warnings[0]
	if got, want := warning.Code, "duplicate_spec_ref_skipped"; got != want {
		t.Fatalf("warning code = %q, want %q", got, want)
	}
	if got, want := warning.Ref, "DUPE-001"; got != want {
		t.Fatalf("warning ref = %q, want %q", got, want)
	}
	if got, want := warning.KeptPath, "rfcs/primary.md"; got != want {
		t.Fatalf("warning kept_path = %q, want %q", got, want)
	}
	if got, want := warning.SkippedPath, "notes.md"; got != want {
		t.Fatalf("warning skipped_path = %q, want %q", got, want)
	}
	if got, want := warning.Reason, "higher discovery score"; got != want {
		t.Fatalf("warning reason = %q, want %q", got, want)
	}
	if got, want := len(result.Sources), 1; got != want {
		t.Fatalf("source count = %d, want %d", got, want)
	}
	if got, want := result.Sources[0].ItemCount, 1; got != want {
		t.Fatalf("contracts item count = %d, want %d", got, want)
	}
	if !strings.Contains(result.Config, "primary.md") {
		t.Fatalf("generated config %q does not include kept contract path", result.Config)
	}
	if strings.Contains(result.Config, "notes.md") {
		t.Fatalf("generated config %q unexpectedly includes skipped contract path", result.Config)
	}
}

func TestDiscoverWorkspaceSilentlySkipsTrueDuplicateMarkdownContractRefs(t *testing.T) {
	repo := t.TempDir()
	body := `
# Shared Contract

Ref: DUPE-001
Status: draft
Domain: api
`
	mustWriteFile(t, filepath.Join(repo, "rfcs", "primary.md"), body)
	mustWriteFile(t, filepath.Join(repo, "notes.md"), body)

	result, err := DiscoverWorkspace(DiscoverOptions{RootPath: repo})
	if err != nil {
		t.Fatalf("DiscoverWorkspace() error = %v", err)
	}

	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %+v, want none for same-content duplicates", result.Warnings)
	}
	if got, want := len(result.Sources), 1; got != want {
		t.Fatalf("source count = %d, want %d", got, want)
	}
	if got, want := result.Sources[0].ItemCount, 1; got != want {
		t.Fatalf("contracts item count = %d, want %d", got, want)
	}
	if !strings.Contains(result.Config, "primary.md") {
		t.Fatalf("generated config %q does not include kept contract path", result.Config)
	}
	if strings.Contains(result.Config, "notes.md") {
		t.Fatalf("generated config %q unexpectedly includes skipped duplicate path", result.Config)
	}
}
