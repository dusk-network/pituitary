package source

import (
	"path/filepath"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
)

func TestExplainFileReportsExcludedMarkdownDoc(t *testing.T) {
	t.Parallel()

	repoRoot := repoRoot(t)
	cfg, err := config.Load(filepath.Join(repoRoot, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := ExplainFile(cfg, filepath.Join(repoRoot, "docs", "development", "testing-guide.md"))
	if err != nil {
		t.Fatalf("ExplainFile() error = %v", err)
	}
	if got, want := result.Summary.Status, "excluded"; got != want {
		t.Fatalf("summary status = %q, want %q", got, want)
	}
	docsSource, ok := findSourceExplanation(result.Sources, func(src SourceFileExplanation) bool {
		return src.Name == "docs"
	})
	if !ok {
		t.Fatal("did not find docs source in result.Sources")
	}
	if got, want := docsSource.Reason, explainReasonNotMatchedByInclude; got != want {
		t.Fatalf("docs source reason = %q, want %q", got, want)
	}
	if got, want := docsSource.RelativePath, "development/testing-guide.md"; got != want {
		t.Fatalf("docs source relative path = %q, want %q", got, want)
	}
}

func TestExplainFileReportsFileSelectorExclusion(t *testing.T) {
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
	mustWriteFile(t, filepath.Join(repo, "contracts", "auth", "session-policy.md"), `
# Session Policy
Status: accepted
`)
	mustWriteFile(t, filepath.Join(repo, "contracts", "platform", "tenant-rate-limits.md"), `
# Tenant Rate Limits
Status: draft
`)

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := ExplainFile(cfg, filepath.Join(repo, "contracts", "auth", "session-policy.md"))
	if err != nil {
		t.Fatalf("ExplainFile() error = %v", err)
	}
	if got, want := result.Summary.Status, "excluded"; got != want {
		t.Fatalf("summary status = %q, want %q", got, want)
	}
	if got, want := result.Sources[0].Reason, explainReasonNotListedInFiles; got != want {
		t.Fatalf("contract source reason = %q, want %q", got, want)
	}
}

func TestExplainFileReportsIndexedMarkdownContract(t *testing.T) {
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
Depends On:
- SPEC-042
Applies To:
- code://src/auth/session_policy.go

# Session Policy

All interactive sessions must use tenant-scoped policy evaluation.
`)

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := ExplainFile(cfg, filepath.Join(repo, "contracts", "auth", "session-policy.md"))
	if err != nil {
		t.Fatalf("ExplainFile() error = %v", err)
	}
	if got, want := result.Summary.Status, "indexed"; got != want {
		t.Fatalf("summary status = %q, want %q", got, want)
	}
	if got, want := result.Sources[0].Reason, explainReasonIndexedMarkdownContract; got != want {
		t.Fatalf("contract source reason = %q, want %q", got, want)
	}
	if result.Sources[0].InferredSpec == nil {
		t.Fatal("contract inferred spec = nil, want metadata")
	}
	if got, want := result.Sources[0].InferredSpec.Ref, "RFC-AUTH-001"; got != want {
		t.Fatalf("inferred ref = %q, want %q", got, want)
	}
	if got, want := result.Sources[0].InferredSpec.Status, "accepted"; got != want {
		t.Fatalf("inferred status = %q, want %q", got, want)
	}
}

func TestExplainFileReportsBundleMemberNotIndexedDirectly(t *testing.T) {
	t.Parallel()

	repoRoot := repoRoot(t)
	cfg, err := config.Load(filepath.Join(repoRoot, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := ExplainFile(cfg, filepath.Join(repoRoot, "specs", "rate-limit-v2", "body.md"))
	if err != nil {
		t.Fatalf("ExplainFile() error = %v", err)
	}
	if got, want := result.Summary.Status, "not_indexed"; got != want {
		t.Fatalf("summary status = %q, want %q", got, want)
	}
	specSource, ok := findSourceExplanation(result.Sources, func(src SourceFileExplanation) bool {
		return src.BundlePath == "specs/rate-limit-v2/spec.toml"
	})
	if !ok {
		t.Fatalf("spec source with bundle path %q not found", "specs/rate-limit-v2/spec.toml")
	}
	if got, want := specSource.Reason, explainReasonBundleMemberNotIndexed; got != want {
		t.Fatalf("spec source reason = %q, want %q", got, want)
	}
	if got, want := specSource.BundlePath, "specs/rate-limit-v2/spec.toml"; got != want {
		t.Fatalf("bundle path = %q, want %q", got, want)
	}
}

func TestExplainFileReportsRepoContextForSecondaryRepo(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	primary := filepath.Join(root, "primary")
	shared := filepath.Join(root, "shared")
	mustWriteFile(t, filepath.Join(root, "pituitary.toml"), `
[workspace]
root = "`+filepath.ToSlash(primary)+`"
repo_id = "primary"
index_path = "`+filepath.ToSlash(filepath.Join(root, ".pituitary", "pituitary.db"))+`"

[[workspace.repos]]
id = "shared"
root = "`+filepath.ToSlash(shared)+`"

[[sources]]
name = "primary-docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["*.md"]

[[sources]]
name = "shared-docs"
adapter = "filesystem"
kind = "markdown_docs"
repo = "shared"
path = "docs"
include = ["*.md"]
`)
	mustWriteFile(t, filepath.Join(primary, "docs", "guide.md"), "# Primary Guide\n")
	mustWriteFile(t, filepath.Join(shared, "docs", "api.md"), "# Shared API\n")

	cfg, err := config.Load(filepath.Join(root, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	t.Run("primary repo file has primary repo_id", func(t *testing.T) {
		result, err := ExplainFile(cfg, filepath.Join(primary, "docs", "guide.md"))
		if err != nil {
			t.Fatalf("ExplainFile() error = %v", err)
		}
		if got, want := result.RepoID, "primary"; got != want {
			t.Fatalf("RepoID = %q, want %q", got, want)
		}
		if got, want := result.WorkspacePath, "docs/guide.md"; got != want {
			t.Fatalf("WorkspacePath = %q, want %q", got, want)
		}
	})

	t.Run("secondary repo file has secondary repo_id and scoped workspace_path", func(t *testing.T) {
		result, err := ExplainFile(cfg, filepath.Join(shared, "docs", "api.md"))
		if err != nil {
			t.Fatalf("ExplainFile() error = %v", err)
		}
		if got, want := result.RepoID, "shared"; got != want {
			t.Fatalf("RepoID = %q, want %q", got, want)
		}
		if got, want := result.WorkspacePath, "shared:docs/api.md"; got != want {
			t.Fatalf("WorkspacePath = %q, want %q", got, want)
		}
	})
}

func findSourceExplanation(sources []SourceFileExplanation, match func(SourceFileExplanation) bool) (SourceFileExplanation, bool) {
	for _, source := range sources {
		if match(source) {
			return source, true
		}
	}
	return SourceFileExplanation{}, false
}
