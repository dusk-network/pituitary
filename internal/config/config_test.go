package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadResolvesWorkspaceAndSourcePaths(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	workspace := filepath.Join(repo, "workspace")
	mustMkdirAll(t, filepath.Join(workspace, "specs"))
	mustMkdirAll(t, filepath.Join(workspace, "docs"))

	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.Workspace.RootPath, filepath.Clean(workspace); got != want {
		t.Fatalf("workspace root path = %q, want %q", got, want)
	}
	if got, want := cfg.Workspace.ResolvedIndexPath, filepath.Join(workspace, ".pituitary", "pituitary.db"); got != want {
		t.Fatalf("resolved index path = %q, want %q", got, want)
	}
	if got, want := cfg.Sources[0].ResolvedPath, filepath.Join(workspace, "specs"); got != want {
		t.Fatalf("spec source path = %q, want %q", got, want)
	}
	if got, want := cfg.Sources[1].ResolvedPath, filepath.Join(workspace, "docs"); got != want {
		t.Fatalf("doc source path = %q, want %q", got, want)
	}
}

func TestLoadResolvesLocalPituitaryConfigRelativeToRepoRoot(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, ".pituitary"))
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	mustMkdirAll(t, filepath.Join(repo, "docs"))

	configPath := filepath.Join(repo, ".pituitary", "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.ConfigDir, filepath.Clean(repo); got != want {
		t.Fatalf("config dir = %q, want %q", got, want)
	}
	if got, want := cfg.Workspace.RootPath, filepath.Clean(repo); got != want {
		t.Fatalf("workspace root path = %q, want %q", got, want)
	}
	if got, want := cfg.Workspace.ResolvedIndexPath, filepath.Join(repo, ".pituitary", "pituitary.db"); got != want {
		t.Fatalf("resolved index path = %q, want %q", got, want)
	}
	if got, want := cfg.Sources[0].ResolvedPath, filepath.Join(repo, "specs"); got != want {
		t.Fatalf("spec source path = %q, want %q", got, want)
	}
	if got, want := cfg.Sources[1].ResolvedPath, filepath.Join(repo, "docs"); got != want {
		t.Fatalf("doc source path = %q, want %q", got, want)
	}
}

func TestLoadDefaultsBootstrapRuntimeContract(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.Runtime.Embedder.Provider, "fixture"; got != want {
		t.Fatalf("runtime.embedder.provider = %q, want %q", got, want)
	}
	if got, want := cfg.Runtime.Embedder.Model, "fixture-8d"; got != want {
		t.Fatalf("runtime.embedder.model = %q, want %q", got, want)
	}
	if got, want := cfg.Runtime.Analysis.Provider, "disabled"; got != want {
		t.Fatalf("runtime.analysis.provider = %q, want %q", got, want)
	}
}
func TestLoadPreservesSourceSelectors(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "docs", "guides"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
files = ["guides/api-rate-limits.md"]
include = ["guides/*.md", "runbooks/*.md"]
exclude = ["runbooks/draft-*.md"]
`)
	writeFile(t, filepath.Join(repo, "docs", "guides", "api-rate-limits.md"), "# API Rate Limits\n")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.Sources[0].Files, []string{"guides/api-rate-limits.md"}; !equalStringSlices(got, want) {
		t.Fatalf("files = %#v, want %#v", got, want)
	}
	if got, want := cfg.Sources[0].Include, []string{"guides/*.md", "runbooks/*.md"}; !equalStringSlices(got, want) {
		t.Fatalf("include = %#v, want %#v", got, want)
	}
	if got, want := cfg.Sources[0].Exclude, []string{"runbooks/draft-*.md"}; !equalStringSlices(got, want) {
		t.Fatalf("exclude = %#v, want %#v", got, want)
	}
}

func TestLoadAcceptsMarkdownContractSourceKind(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "contracts"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "contracts"
adapter = "filesystem"
	kind = "markdown_contract"
	path = "contracts"
	files = ["auth/session-policy.md"]
`)
	mustMkdirAll(t, filepath.Join(repo, "contracts", "auth"))
	writeFile(t, filepath.Join(repo, "contracts", "auth", "session-policy.md"), "# Session Policy\n")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.Sources[0].Kind, SourceKindMarkdownContract; got != want {
		t.Fatalf("source kind = %q, want %q", got, want)
	}
	if got, want := cfg.Sources[0].Files, []string{"auth/session-policy.md"}; !equalStringSlices(got, want) {
		t.Fatalf("files = %#v, want %#v", got, want)
	}
}

func TestLoadRejectsInvalidSourceSelectorPattern(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "docs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["["]
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want selector validation error")
	}
	if !strings.Contains(err.Error(), `source "docs".include: invalid pattern "["`) {
		t.Fatalf("Load() error = %q, want selector validation details", err)
	}
}

func TestLoadRejectsInvalidSourceFileSelector(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "docs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
files = ["../guide.md"]
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want invalid file selector error")
	}
	if !strings.Contains(err.Error(), `source "docs".files[0]: "../guide.md" escapes the source root`) {
		t.Fatalf("Load() error = %q, want invalid source-file selector detail", err)
	}
}

func TestLoadRejectsNonMarkdownFilesForMarkdownContract(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "contracts"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "contracts"
adapter = "filesystem"
kind = "markdown_contract"
path = "contracts"
files = ["auth/spec.toml"]
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want markdown_contract selector validation error")
	}
	if !strings.Contains(err.Error(), `must point to a markdown file for kind "markdown_contract"`) {
		t.Fatalf("Load() error = %q, want markdown selector details", err)
	}
}

func TestLoadRejectsSourceFileKindMismatch(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs", "rate-limit-v2"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
files = ["rate-limit-v2/body.md"]
`)
	writeFile(t, filepath.Join(repo, "specs", "rate-limit-v2", "body.md"), "Body\n")

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want selector kind mismatch error")
	}
	if !strings.Contains(err.Error(), `must point to a spec.toml file`) {
		t.Fatalf("Load() error = %q, want selector kind mismatch detail", err)
	}
}

func TestLoadRejectsUnterminatedSourceSelectorArray(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "docs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = [
  "guides/*.md",
  "runbooks/*.md"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want unterminated array error")
	}
	if !strings.Contains(err.Error(), `sources.include: unterminated array`) {
		t.Fatalf("Load() error = %q, want unterminated selector-array detail", err)
	}
}

func TestLoadReportsConfigPathAndLineForUnterminatedSourceSelectorArray(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "docs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = [
  "guides/*.md"
[runtime.embedder]
provider = "fixture"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want parse error")
	}
	if !strings.Contains(err.Error(), configPath) {
		t.Fatalf("Load() error = %q, want config path", err)
	}
	if !strings.Contains(err.Error(), "line ") {
		t.Fatalf("Load() error = %q, want line detail", err)
	}
	if !strings.Contains(err.Error(), `sources.include: unterminated array`) {
		t.Fatalf("Load() error = %q, want unterminated selector-array detail", err)
	}
}

func TestLoadAcceptsOpenAICompatibleEmbedderProvider(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "openai_compatible"
model = "pituitary-embed"
endpoint = "http://100.92.91.40:1234/v1"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if got, want := cfg.Runtime.Embedder.Provider, RuntimeProviderOpenAI; got != want {
		t.Fatalf("runtime.embedder.provider = %q, want %q", got, want)
	}
	if got, want := cfg.Runtime.Embedder.Endpoint, "http://100.92.91.40:1234/v1"; got != want {
		t.Fatalf("runtime.embedder.endpoint = %q, want %q", got, want)
	}
}

func TestLoadRejectsOpenAICompatibleEmbedderWithoutEndpoint(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "openai_compatible"
model = "pituitary-embed"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want runtime validation error")
	}
	if !strings.Contains(err.Error(), `runtime.embedder.endpoint: value is required for provider "openai_compatible"`) {
		t.Fatalf("Load() error = %q, want embedder endpoint detail", err)
	}
}

func TestLoadRejectsUnsupportedEmbedderProvider(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "anthropic"
model = "text-embedding-3-small"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want runtime validation error")
	}
	if !strings.Contains(err.Error(), `runtime.embedder.provider: unsupported provider "anthropic"`) {
		t.Fatalf("Load() error = %q, want embedder provider detail", err)
	}
	if !strings.Contains(err.Error(), `supported providers: "fixture", "openai_compatible"`) {
		t.Fatalf("Load() error = %q, want provider list", err)
	}
}

func TestLoadRejectsUnsupportedAnalysisProvider(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.analysis]
provider = "anthropic"
model = "claude-sonnet-4-6"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want runtime validation error")
	}
	if !strings.Contains(err.Error(), `runtime.analysis.provider: unsupported provider "anthropic"`) {
		t.Fatalf("Load() error = %q, want analysis provider detail", err)
	}
	if !strings.Contains(err.Error(), `supports only "disabled"`) {
		t.Fatalf("Load() error = %q, want bootstrap support detail", err)
	}
}

func TestLoadRejectsUnknownAdapter(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "github"
kind = "spec_bundle"
path = "specs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), `source "specs".adapter: unsupported adapter "github"`) {
		t.Fatalf("Load() error = %q, want unknown adapter details", err)
	}
}

func TestLoadRejectsMissingSourcePath(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "missing-docs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), `source "docs".path: "missing-docs" does not exist`) {
		t.Fatalf("Load() error = %q, want missing path details", err)
	}
}

func TestLoadRejectsIndexPathThatIsDirectory(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	mustMkdirAll(t, filepath.Join(repo, ".pituitary"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), `workspace.index_path: ".pituitary" resolves to a directory`) {
		t.Fatalf("Load() error = %q, want directory validation", err)
	}
}

func TestLoadRejectsUnknownSection(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[github]
repo = "dusk-network/pituitary"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want parse error")
	}
	if !strings.Contains(err.Error(), `unsupported section "github"`) {
		t.Fatalf("Load() error = %q, want unsupported section message", err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func equalStringSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
