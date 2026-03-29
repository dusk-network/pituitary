package source

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSpecBundleGeneratesNextIDAndWritesTemplate(t *testing.T) {
	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "specs", "rate-limit-v2", "spec.toml"), `
id = "SPEC-042"
title = "Per-Tenant Rate Limiting"
status = "accepted"
domain = "api"
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(repo, "specs", "rate-limit-v2", "body.md"), "# Rate Limiting\n")
	mustWriteFile(t, filepath.Join(repo, "specs", "burst-handling", "spec.toml"), `
id = "SPEC-055"
title = "Burst Handling"
status = "accepted"
domain = "api"
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(repo, "specs", "burst-handling", "body.md"), "# Burst Handling\n")

	result, err := NewSpecBundle(NewSpecBundleOptions{
		WorkspaceRoot: repo,
		SpecRoot:      "specs",
		Title:         "Queue shaping",
		Domain:        "api",
	})
	if err != nil {
		t.Fatalf("NewSpecBundle() error = %v", err)
	}
	if got, want := result.BundleDir, "specs/queue-shaping"; got != want {
		t.Fatalf("bundle dir = %q, want %q", got, want)
	}
	if got, want := result.Spec.Ref, "SPEC-056"; got != want {
		t.Fatalf("spec ref = %q, want %q", got, want)
	}
	if got, want := result.Spec.Status, "draft"; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if len(result.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(result.Files))
	}
	if !strings.Contains(result.Files[0].Content, `status = "draft"`) {
		t.Fatalf("spec.toml = %q, want draft status scaffold", result.Files[0].Content)
	}
	if !strings.Contains(result.Files[1].Content, "## Requirements") {
		t.Fatalf("body.md = %q, want requirements heading", result.Files[1].Content)
	}
	for _, path := range []string{
		filepath.Join(repo, "specs", "queue-shaping", "spec.toml"),
		filepath.Join(repo, "specs", "queue-shaping", "body.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected scaffold file %s: %v", path, err)
		}
	}
}

func TestNewSpecBundleWarnsWhenDomainIsMissing(t *testing.T) {
	repo := t.TempDir()

	result, err := NewSpecBundle(NewSpecBundleOptions{
		WorkspaceRoot: repo,
		SpecRoot:      "specs",
		Title:         "Draft spec",
	})
	if err != nil {
		t.Fatalf("NewSpecBundle() error = %v", err)
	}
	if got, want := result.Spec.Domain, "unknown"; got != want {
		t.Fatalf("domain = %q, want %q", got, want)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "placeholder_domain" {
		t.Fatalf("warnings = %+v, want placeholder_domain warning", result.Warnings)
	}
}
