package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveSourceFilePathRejectsTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	for _, tc := range []struct {
		name      string
		sourceRef string
	}{
		{"parent_with_file_prefix", "file://../../etc/evil"},
		{"parent_without_prefix", "../../etc/evil"},
		{"slashed_parent", "file://..//..//etc//evil"},
		{"trailing_parent", "file://docs/../../outside"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := resolveSourceFilePath(root, tc.sourceRef); err == nil {
				t.Fatalf("resolveSourceFilePath(%q) error = nil, want containment error", tc.sourceRef)
			}
		})
	}
}

func TestResolveSourceFilePathAllowsWorkspaceLocal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	got, err := resolveSourceFilePath(root, "file://docs/guide.md")
	if err != nil {
		t.Fatalf("resolveSourceFilePath() error = %v, want nil", err)
	}
	want := filepath.Join(root, "docs", "guide.md")
	if got != want {
		t.Fatalf("resolveSourceFilePath() = %q, want %q", got, want)
	}
}

func TestResolveSourceFilePathEmpty(t *testing.T) {
	t.Parallel()

	if _, err := resolveSourceFilePath("/tmp", "file://"); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("resolveSourceFilePath(empty) error = %v, want empty-ref error", err)
	}
}

// TestResolveSourceFilePathRejectsSymlinkEscape verifies the symlink-safe
// containment check added in the hardening PR: even when the lexical path is
// workspace-local, a workspace-internal symlink pointing outside the root must
// be rejected.
func TestResolveSourceFilePathRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows; skipping")
	}
	t.Parallel()

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "target.md"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := resolveSourceFilePath(root, "file://escape/target.md")
	if err == nil {
		t.Fatalf("resolveSourceFilePath() accepted a symlink-escaped source_ref; want containment error")
	}
	if !strings.Contains(err.Error(), "outside workspace root") {
		t.Fatalf("resolveSourceFilePath() err = %v, want 'outside workspace root'", err)
	}
}
