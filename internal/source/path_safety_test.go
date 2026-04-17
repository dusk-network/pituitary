package source

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPathWithinRootAllowsLocal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	inside := filepath.Join(root, "docs", "guide.md")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(inside, []byte("hi"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if !pathWithinRoot(root, inside) {
		t.Fatalf("pathWithinRoot(root, inside) = false, want true")
	}
	if !pathWithinRoot(root, root) {
		t.Fatalf("pathWithinRoot(root, root) = false, want true")
	}
}

func TestPathWithinRootRejectsTraversal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "other.md")
	if err := os.WriteFile(outside, []byte("hi"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if pathWithinRoot(root, outside) {
		t.Fatalf("pathWithinRoot should reject path in a sibling tempdir")
	}
}

// TestPathWithinRootRejectsSymlinkEscape confirms the fix for the lexical-only
// containment check: a symlink inside the workspace pointing outside must not
// pass the check, even though filepath.Rel would accept it.
func TestPathWithinRootRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows; skipping")
	}
	t.Parallel()

	root := t.TempDir()
	target := t.TempDir()
	outsideFile := filepath.Join(target, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	linkPath := filepath.Join(root, "escape")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	// The escape-via-symlink path is "<root>/escape/secret.txt", but symlink
	// resolution shows it actually lives under `target`, not under `root`.
	probed := filepath.Join(linkPath, "secret.txt")
	if pathWithinRoot(root, probed) {
		t.Fatalf("pathWithinRoot should reject a symlink-escaped path, got true for %q", probed)
	}
}

func TestPathWithinRootHandlesNonexistentPath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fresh := filepath.Join(root, "not", "yet", "there.md")
	if !pathWithinRoot(root, fresh) {
		t.Fatalf("pathWithinRoot should accept a nonexistent path inside root")
	}
	outside := filepath.Join(t.TempDir(), "not", "yet", "there.md")
	if pathWithinRoot(root, outside) {
		t.Fatalf("pathWithinRoot should reject a nonexistent path outside root")
	}
}
