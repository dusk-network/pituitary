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

// TestReadAllLimitedEnforcesLimitFromReader verifies the limit is enforced at
// read time (via io.LimitReader), not via a pre-read Stat. This closes the
// TOCTOU window where a file could grow or be replaced between the size check
// and the actual read.
func TestReadAllLimitedEnforcesLimitFromReader(t *testing.T) {
	t.Parallel()

	// An unbounded source (endless 'x' stream) must still be rejected.
	src := bytes.NewReader(bytes.Repeat([]byte("x"), 4096))
	if _, err := readAllLimited(src, 1024, "test"); err == nil {
		t.Fatalf("readAllLimited accepted oversize reader; want size limit error")
	}

	// A source exactly at the limit must be accepted.
	src = bytes.NewReader(bytes.Repeat([]byte("y"), 1024))
	got, err := readAllLimited(src, 1024, "test")
	if err != nil {
		t.Fatalf("readAllLimited rejected at-limit reader: %v", err)
	}
	if len(got) != 1024 {
		t.Fatalf("readAllLimited len = %d, want 1024", len(got))
	}
}
