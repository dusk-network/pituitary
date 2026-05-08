package analysis

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
)

func TestLoadPathComplianceTargetsContext_RejectsOversizedFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := &config.Config{Workspace: config.Workspace{RootPath: root}}

	rel := "src/big.go"
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write 1 byte over the limit; LimitReader stops at maxCompliancePathBytes+1.
	oversized := bytes.Repeat([]byte("a"), maxCompliancePathBytes+1)
	if err := os.WriteFile(abs, oversized, 0o644); err != nil {
		t.Fatalf("write oversized: %v", err)
	}

	_, err := loadPathComplianceTargetsContext(t.Context(), cfg, []string{rel})
	if err == nil {
		t.Fatalf("loadPathComplianceTargetsContext: want size-limit error, got nil")
	}
	if !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("loadPathComplianceTargetsContext error = %v, want size-limit error", err)
	}
}

func TestLoadPathComplianceTargetsContext_RejectsBinaryFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := &config.Config{Workspace: config.Workspace{RootPath: root}}

	rel := "bin/blob.dat"
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// NUL byte inside the sniff window triggers the binary screen.
	if err := os.WriteFile(abs, []byte("text\x00more text"), 0o644); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	_, err := loadPathComplianceTargetsContext(t.Context(), cfg, []string{rel})
	if err == nil {
		t.Fatalf("loadPathComplianceTargetsContext: want binary-rejection error, got nil")
	}
	if !strings.Contains(err.Error(), "binary file") {
		t.Fatalf("loadPathComplianceTargetsContext error = %v, want binary-file error", err)
	}
}

func TestLoadPathComplianceTargetsContext_AcceptsSmallTextFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := &config.Config{Workspace: config.Workspace{RootPath: root}}

	rel := "src/handler.go"
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "package main\n\nfunc main() {}\n"
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	targets, err := loadPathComplianceTargetsContext(t.Context(), cfg, []string{rel})
	if err != nil {
		t.Fatalf("loadPathComplianceTargetsContext error = %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("len(targets) = %d, want 1", len(targets))
	}
	if got, want := targets[0].Path, rel; got != want {
		t.Fatalf("targets[0].Path = %q, want %q", got, want)
	}
	if got, want := targets[0].Content, body; got != want {
		t.Fatalf("targets[0].Content = %q, want %q", got, want)
	}
	if targets[0].DuplicateKey == "" {
		t.Fatalf("targets[0].DuplicateKey is empty; want sha256 digest")
	}
}

func TestLoadPathComplianceTargetsContext_RejectsSymlink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := &config.Config{Workspace: config.Workspace{RootPath: root}}

	target := filepath.Join(root, "real.go")
	if err := os.WriteFile(target, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	link := filepath.Join(root, "link.go")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	_, err := loadPathComplianceTargetsContext(t.Context(), cfg, []string{"link.go"})
	if err == nil {
		t.Fatalf("loadPathComplianceTargetsContext on symlink: want non-regular-file error, got nil")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("loadPathComplianceTargetsContext error = %v, want non-regular-file error", err)
	}
}

func TestReadBoundedCompliancePath_ReturnsExactSizeAtLimit(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "limit.txt")

	const limit int64 = 32
	atLimit := bytes.Repeat([]byte("x"), int(limit))
	if err := os.WriteFile(path, atLimit, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := readBoundedCompliancePath(path, limit)
	if err != nil {
		t.Fatalf("readBoundedCompliancePath at limit: err = %v", err)
	}
	if !bytes.Equal(got, atLimit) {
		t.Fatalf("readBoundedCompliancePath returned %d bytes, want %d", len(got), len(atLimit))
	}

	overLimit := bytes.Repeat([]byte("x"), int(limit)+1)
	if err := os.WriteFile(path, overLimit, 0o644); err != nil {
		t.Fatalf("write over: %v", err)
	}
	if _, err := readBoundedCompliancePath(path, limit); err == nil {
		t.Fatalf("readBoundedCompliancePath over limit: err = nil, want size-limit error")
	}
}

func TestLooksLikeBinary(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"empty", nil, false},
		{"plain text", []byte("hello world\n"), false},
		{"utf8 bom", []byte("\xEF\xBB\xBFhello"), false},
		{"nul byte at start", []byte("\x00abc"), true},
		{"nul byte in window", append([]byte(strings.Repeat("a", 100)), 0x00, 'x'), true},
		{"nul byte beyond window", append(bytes.Repeat([]byte("a"), binarySniffWindow), 0x00), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := looksLikeBinary(tc.data); got != tc.want {
				t.Fatalf("looksLikeBinary(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
