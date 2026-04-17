package source

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadBoundedFileRejectsOversize(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "big.md")
	payload := bytes.Repeat([]byte("x"), 2048)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if _, err := readBoundedFile(path, 1024); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("readBoundedFile(oversize) err = %v, want size limit error", err)
	}
}

func TestReadBoundedFileReadsUnderLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "small.md")
	if err := os.WriteFile(path, []byte("hi"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := readBoundedFile(path, 1024)
	if err != nil {
		t.Fatalf("readBoundedFile err = %v", err)
	}
	if string(got) != "hi" {
		t.Fatalf("readBoundedFile content = %q, want %q", string(got), "hi")
	}
}
