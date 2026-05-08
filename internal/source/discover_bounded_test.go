package source

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssessDiscoveredSpecBundle_RejectsOversizedSpecTOML(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	bundleDir := filepath.Join(repo, "specs", "huge")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// One byte over the spec.toml bound is enough to trip readBoundedFile.
	oversized := bytes.Repeat([]byte("a"), maxSpecTOMLBytes+1)
	if err := os.WriteFile(filepath.Join(bundleDir, "spec.toml"), oversized, 0o644); err != nil {
		t.Fatalf("write spec.toml: %v", err)
	}

	ok, _, reason := assessDiscoveredSpecBundle(repo, bundleDir)
	if ok {
		t.Fatalf("assessDiscoveredSpecBundle: ok = true, want false for oversized spec.toml")
	}
	if !strings.Contains(reason, "size limit") {
		t.Fatalf("assessDiscoveredSpecBundle reason = %q, want size-limit rejection", reason)
	}
}

func TestDiscoverSpecBundleIdentity_RejectsOversizedSpecTOML(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	bundleDir := filepath.Join(repo, "specs", "huge")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oversized := bytes.Repeat([]byte("a"), maxSpecTOMLBytes+1)
	if err := os.WriteFile(filepath.Join(bundleDir, "spec.toml"), oversized, 0o644); err != nil {
		t.Fatalf("write spec.toml: %v", err)
	}

	if _, _, err := discoverSpecBundleIdentity(bundleDir); err == nil {
		t.Fatalf("discoverSpecBundleIdentity: err = nil, want size-limit error")
	} else if !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("discoverSpecBundleIdentity error = %v, want size-limit error", err)
	}
}

func TestDiscoverSpecBundleIdentity_RejectsOversizedBody(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	bundleDir := filepath.Join(repo, "specs", "huge-body")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWriteFile(t, filepath.Join(bundleDir, "spec.toml"), `
id = "SPEC-HUGE"
title = "Huge Body"
status = "draft"
domain = "api"
body = "body.md"
`)
	oversized := bytes.Repeat([]byte("a"), maxMarkdownBodyBytes+1)
	if err := os.WriteFile(filepath.Join(bundleDir, "body.md"), oversized, 0o644); err != nil {
		t.Fatalf("write body: %v", err)
	}

	if _, _, err := discoverSpecBundleIdentity(bundleDir); err == nil {
		t.Fatalf("discoverSpecBundleIdentity: err = nil, want size-limit error for body")
	} else if !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("discoverSpecBundleIdentity error = %v, want size-limit error", err)
	}
}

func TestAssessDiscoveredSpecBundle_RejectsOversizedBodyByStat(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	bundleDir := filepath.Join(repo, "specs", "huge-body")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWriteFile(t, filepath.Join(bundleDir, "spec.toml"), `
id = "SPEC-HUGE"
title = "Huge Body"
status = "draft"
domain = "api"
body = "body.md"
`)
	oversized := bytes.Repeat([]byte("a"), maxMarkdownBodyBytes+1)
	if err := os.WriteFile(filepath.Join(bundleDir, "body.md"), oversized, 0o644); err != nil {
		t.Fatalf("write body: %v", err)
	}

	ok, _, reason := assessDiscoveredSpecBundle(repo, bundleDir)
	if ok {
		t.Fatalf("assessDiscoveredSpecBundle: ok = true, want false for oversized body")
	}
	if !strings.Contains(reason, "size limit") {
		t.Fatalf("assessDiscoveredSpecBundle reason = %q, want size-limit rejection", reason)
	}
}

func TestDiscoverWorkspace_OversizedBodyDoesNotFailDiscovery(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	// One bundle has an oversized body; another bundle is healthy. Discovery
	// must reject the bad bundle without aborting the entire scan.
	badBundle := filepath.Join(repo, "specs", "huge")
	if err := os.MkdirAll(badBundle, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mustWriteFile(t, filepath.Join(badBundle, "spec.toml"), `
id = "SPEC-HUGE"
title = "Huge Body"
status = "draft"
domain = "api"
body = "body.md"
`)
	oversized := bytes.Repeat([]byte("a"), maxMarkdownBodyBytes+1)
	if err := os.WriteFile(filepath.Join(badBundle, "body.md"), oversized, 0o644); err != nil {
		t.Fatalf("write oversized body: %v", err)
	}

	goodBundle := filepath.Join(repo, "specs", "ok")
	mustWriteFile(t, filepath.Join(goodBundle, "spec.toml"), `
id = "SPEC-OK"
title = "Healthy"
status = "draft"
domain = "api"
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(goodBundle, "body.md"), `
# Healthy

Body content.
`)

	result, err := DiscoverWorkspace(DiscoverOptions{RootPath: repo})
	if err != nil {
		t.Fatalf("DiscoverWorkspace error = %v", err)
	}
	if !strings.Contains(result.Config, "SPEC-OK") && !strings.Contains(result.Config, goodBundle) && !strings.Contains(result.Config, "specs/ok") {
		t.Fatalf("generated config %q does not include healthy bundle path", result.Config)
	}
}

func TestDiscoverMarkdownCandidates_SkipsSymlinks(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "README.md"), `
# Project

Landing page.
`)
	target := filepath.Join(repo, "README.md")
	link := filepath.Join(repo, "MIRROR.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	result, err := DiscoverWorkspace(DiscoverOptions{RootPath: repo})
	if err != nil {
		t.Fatalf("DiscoverWorkspace error = %v", err)
	}
	// The symlinked candidate must not appear in the generated config.
	if strings.Contains(result.Config, "MIRROR.md") {
		t.Fatalf("generated config %q unexpectedly includes symlinked markdown", result.Config)
	}
}

func TestDiscoverSpecBundles_SkipsSymlinkedSpecTOML(t *testing.T) {
	t.Parallel()

	// Place the real bundle OUTSIDE the workspace so discovery cannot find
	// it as a normal candidate. The only way SPEC-EXTERNAL can land in the
	// generated config is if discovery follows the in-workspace symlink --
	// which the regular-file guard must prevent.
	external := t.TempDir()
	mustWriteFile(t, filepath.Join(external, "spec.toml"), `
id = "SPEC-EXTERNAL"
title = "External"
status = "draft"
domain = "api"
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(external, "body.md"), `
# External

Body.
`)

	repo := t.TempDir()
	bundleDir := filepath.Join(repo, "specs", "via-symlink")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(filepath.Join(external, "spec.toml"), filepath.Join(bundleDir, "spec.toml")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	result, err := DiscoverWorkspace(DiscoverOptions{RootPath: repo})
	if err != nil && !strings.Contains(err.Error(), "no likely sources") {
		t.Fatalf("DiscoverWorkspace error = %v", err)
	}
	if err == nil && strings.Contains(result.Config, "SPEC-EXTERNAL") {
		t.Fatalf("generated config %q unexpectedly indexes symlinked spec.toml", result.Config)
	}
}

func TestClassifyMarkdownCandidate_RejectsOversizedMarkdown(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mdPath := filepath.Join(repo, "huge.md")
	oversized := bytes.Repeat([]byte("a"), maxMarkdownBodyBytes+1)
	if err := os.WriteFile(mdPath, oversized, 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	assessment, err := classifyMarkdownCandidate(repo, mdPath)
	if err != nil {
		t.Fatalf("classifyMarkdownCandidate: err = %v, want nil (read errors surface as Reason)", err)
	}
	if assessment.Selected {
		t.Fatalf("assessment.Selected = true, want false for oversized markdown")
	}
	if !strings.Contains(assessment.Reason, "size limit") {
		t.Fatalf("assessment.Reason = %q, want size-limit rejection", assessment.Reason)
	}
}
