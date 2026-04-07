package ast

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestWalkWorkspace(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	files := map[string]string{
		"main.go":                   "package main",
		"lib/handler.py":            "def handle(): pass",
		"web/app.ts":                "export default {}",
		"README.md":                 "# hello",
		"node_modules/dep/index.js": "module.exports = {}",
		".git/HEAD":                 "ref: refs/heads/main",
		"vendor/lib/lib.go":         "package lib",
	}
	for path, content := range files {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := WalkWorkspace(dir)
	if err != nil {
		t.Fatalf("WalkWorkspace error: %v", err)
	}

	sort.Strings(got)
	want := []string{"lib/handler.py", "main.go", "web/app.ts"}
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("WalkWorkspace got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("file[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWalkWorkspaceEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got, err := WalkWorkspace(dir)
	if err != nil {
		t.Fatalf("WalkWorkspace error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result for empty dir, got %v", got)
	}
}
