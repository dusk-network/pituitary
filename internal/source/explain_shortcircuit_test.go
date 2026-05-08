package source

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
)

// TestExplainFileExcludedMarkdownContractIsNotRead proves the explain-file
// short-circuit: when the selector excludes a markdown_contract path, the
// diagnostic must not buffer the file content into memory for inference.
//
// We exercise this by writing a file whose body would exceed the
// markdown body bound. If explainMarkdownContractSource still tried to read
// it, readBoundedFile would fail and ExplainFile would return an error.
// With the short-circuit in place, the read never happens and the call
// succeeds with an exclusion reason.
func TestExplainFileExcludedMarkdownContractIsNotRead(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "contracts"
adapter = "filesystem"
kind = "markdown_contract"
path = "contracts"
files = ["platform/tenant-rate-limits.md"]
`)
	mustWriteFile(t, filepath.Join(repo, "contracts", "platform", "tenant-rate-limits.md"), `
# Tenant Rate Limits
Status: draft
`)

	excluded := filepath.Join(repo, "contracts", "auth", "huge-policy.md")
	if err := os.MkdirAll(filepath.Dir(excluded), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	oversized := bytes.Repeat([]byte("a"), maxMarkdownBodyBytes+1)
	if err := os.WriteFile(excluded, oversized, 0o644); err != nil {
		t.Fatalf("write oversized: %v", err)
	}

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := ExplainFile(cfg, excluded)
	if err != nil {
		t.Fatalf("ExplainFile() error = %v (excluded contract should not be read)", err)
	}
	if got, want := result.Summary.Status, "excluded"; got != want {
		t.Fatalf("summary status = %q, want %q", got, want)
	}
	if len(result.Sources) == 0 {
		t.Fatalf("result.Sources is empty")
	}
	if got, want := result.Sources[0].Reason, explainReasonNotListedInFiles; got != want {
		t.Fatalf("contract source reason = %q, want %q", got, want)
	}
	if result.Sources[0].InferredSpec != nil {
		t.Fatalf("InferredSpec = %+v, want nil for excluded contract (no inference performed)", result.Sources[0].InferredSpec)
	}
}

// TestExplainFileSelectedMarkdownContractStillInfers verifies the happy path
// is unchanged: a selected contract still reports inferred metadata.
func TestExplainFileSelectedMarkdownContractStillInfers(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "contracts"
adapter = "filesystem"
kind = "markdown_contract"
path = "contracts"
`)
	mustWriteFile(t, filepath.Join(repo, "contracts", "auth", "session-policy.md"), `
Ref: RFC-AUTH-001
Status: accepted
Domain: identity

# Session Policy

All interactive sessions must use tenant-scoped policy evaluation.
`)

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	result, err := ExplainFile(cfg, filepath.Join(repo, "contracts", "auth", "session-policy.md"))
	if err != nil {
		t.Fatalf("ExplainFile error = %v", err)
	}
	if got, want := result.Summary.Status, "indexed"; got != want {
		t.Fatalf("summary status = %q, want %q", got, want)
	}
	if result.Sources[0].InferredSpec == nil {
		t.Fatalf("InferredSpec = nil, want inferred metadata for selected contract")
	}
	if got, want := result.Sources[0].InferredSpec.Ref, "RFC-AUTH-001"; got != want {
		t.Fatalf("InferredSpec.Ref = %q, want %q", got, want)
	}
}
