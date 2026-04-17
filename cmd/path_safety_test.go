package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestCLIPathWithinRootRejectsSymlinkEscape confirms that the CLI containment
// check resolves symlinks before applying the lexical check, so a symlink
// inside the workspace pointing outside cannot bypass the guard.
func TestCLIPathWithinRootRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows; skipping")
	}
	t.Parallel()

	root := t.TempDir()
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	linkPath := filepath.Join(root, "escape")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	probed := filepath.Join(linkPath, "secret.txt")
	if cliPathWithinRoot(root, probed) {
		t.Fatalf("cliPathWithinRoot accepted symlink-escaped path %q", probed)
	}
}

func TestCLIPathWithinRootAllowsLocal(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	inside := filepath.Join(root, "sub", "file.txt")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(inside, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !cliPathWithinRoot(root, inside) {
		t.Fatalf("cliPathWithinRoot rejected a workspace-local path")
	}
}
