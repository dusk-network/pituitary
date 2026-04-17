package app

import (
	"path/filepath"
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
