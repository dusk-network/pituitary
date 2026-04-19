package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/sdk"
)

func init() {
	sdk.Register("github", func() sdk.Adapter {
		return nil
	})
	sdk.Register(AdapterJSON, func() sdk.Adapter {
		return nil
	})
}

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

func TestLoadResolvesMultiRepoSources(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	primary := filepath.Join(root, "primary")
	shared := filepath.Join(root, "shared")
	mustMkdirAll(t, filepath.Join(primary, "specs"))
	mustMkdirAll(t, filepath.Join(shared, "docs"))

	configPath := filepath.Join(root, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "primary"
repo_id = "primary"
index_path = ".pituitary/pituitary.db"

[[workspace.repos]]
id = "shared"
root = "shared"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "shared-docs"
adapter = "filesystem"
kind = "markdown_docs"
repo = "shared"
path = "docs"
`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.Workspace.RootPath, filepath.Clean(primary); got != want {
		t.Fatalf("workspace root path = %q, want %q", got, want)
	}
	if got, want := cfg.Workspace.RepoID, "primary"; got != want {
		t.Fatalf("workspace repo_id = %q, want %q", got, want)
	}
	if got, want := len(cfg.Workspace.Repos), 1; got != want {
		t.Fatalf("workspace repos = %d, want %d", got, want)
	}
	if got, want := cfg.Workspace.Repos[0].RootPath, filepath.Clean(shared); got != want {
		t.Fatalf("workspace repo root path = %q, want %q", got, want)
	}
	if got, want := cfg.Sources[0].ResolvedRepo, "primary"; got != want {
		t.Fatalf("primary source repo = %q, want %q", got, want)
	}
	if got, want := cfg.Sources[0].RepoRootPath, filepath.Clean(primary); got != want {
		t.Fatalf("primary source repo root = %q, want %q", got, want)
	}
	if got, want := cfg.Sources[1].ResolvedRepo, "shared"; got != want {
		t.Fatalf("shared source repo = %q, want %q", got, want)
	}
	if got, want := cfg.Sources[1].RepoRootPath, filepath.Clean(shared); got != want {
		t.Fatalf("shared source repo root = %q, want %q", got, want)
	}
	if got, want := cfg.Sources[1].ResolvedPath, filepath.Join(shared, "docs"); got != want {
		t.Fatalf("shared source path = %q, want %q", got, want)
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

func TestLoadUnknownTerminologyPolicyFieldSuggestsNearestValidFields(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[terminology.policies]]
exclude_paths = ["CHANGELOG.md"]

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want unsupported terminology policy field failure")
	}
	if !strings.Contains(err.Error(), `unsupported terminology.policies field "exclude_paths"; did you mean one of:`) ||
		!strings.Contains(err.Error(), `"preferred"`) {
		t.Fatalf("Load() error = %q, want nearest-field suggestion", err)
	}
}

func TestLoadSourcePathErrorExplainsWorkspaceRootResolution(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "project", "docs"))
	configDir := filepath.Join(root, "tmp")
	mustMkdirAll(t, configDir)
	configPath := filepath.Join(configDir, "pit-fake.toml")
	writeFile(t, configPath, `
[workspace]
root = ".."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want source path resolution failure")
	}
	if !strings.Contains(err.Error(), `workspace.root ".." resolves relative to config base`) {
		t.Fatalf("Load() error = %q, want workspace.root resolution detail", err)
	}
	if !strings.Contains(err.Error(), `so source path "docs" resolves to`) {
		t.Fatalf("Load() error = %q, want derived source path detail", err)
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

func TestLoadResolvesRuntimeProfiles(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.profiles.local-lm-studio]
provider = "openai_compatible"
endpoint = "http://127.0.0.1:1234/v1"
timeout_ms = 30000
max_retries = 1

[runtime.embedder]
profile = "local-lm-studio"
model = "nomic-embed-text-v1.5"

[runtime.analysis]
profile = "local-lm-studio"
model = "qwen3.5-35b"
timeout_ms = 120000
max_response_tokens = 2048

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

	if got, want := len(cfg.Runtime.Profiles), 1; got != want {
		t.Fatalf("len(runtime.profiles) = %d, want %d", got, want)
	}
	profile := cfg.Runtime.Profiles["local-lm-studio"]
	if got, want := profile.Provider, RuntimeProviderOpenAI; got != want {
		t.Fatalf("runtime.profiles[local-lm-studio].provider = %q, want %q", got, want)
	}
	if got, want := cfg.Runtime.Embedder.Profile, "local-lm-studio"; got != want {
		t.Fatalf("runtime.embedder.profile = %q, want %q", got, want)
	}
	if got, want := cfg.Runtime.Embedder.Provider, RuntimeProviderOpenAI; got != want {
		t.Fatalf("runtime.embedder.provider = %q, want %q", got, want)
	}
	if got, want := cfg.Runtime.Embedder.Endpoint, "http://127.0.0.1:1234/v1"; got != want {
		t.Fatalf("runtime.embedder.endpoint = %q, want %q", got, want)
	}
	if got, want := cfg.Runtime.Embedder.TimeoutMS, 30000; got != want {
		t.Fatalf("runtime.embedder.timeout_ms = %d, want %d", got, want)
	}
	if got, want := cfg.Runtime.Analysis.Profile, "local-lm-studio"; got != want {
		t.Fatalf("runtime.analysis.profile = %q, want %q", got, want)
	}
	if got, want := cfg.Runtime.Analysis.Provider, RuntimeProviderOpenAI; got != want {
		t.Fatalf("runtime.analysis.provider = %q, want %q", got, want)
	}
	if got, want := cfg.Runtime.Analysis.TimeoutMS, 120000; got != want {
		t.Fatalf("runtime.analysis.timeout_ms = %d, want %d", got, want)
	}
	if got, want := cfg.Runtime.Analysis.MaxResponseTokens, 2048; got != want {
		t.Fatalf("runtime.analysis.max_response_tokens = %d, want %d", got, want)
	}
}

func TestLoadRejectsUnknownRuntimeProfile(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
profile = "missing"

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
	if !strings.Contains(err.Error(), `runtime.embedder.profile: unknown profile "missing"`) {
		t.Fatalf("Load() error = %q, want unknown profile detail", err)
	}
}

func TestParseRejectsDuplicateKeys(t *testing.T) {
	t.Parallel()

	t.Run("workspace scalar", func(t *testing.T) {
		t.Parallel()

		_, err := parse(bytes.NewBufferString(`
[workspace]
root = "."
root = "other"
`))
		if err == nil {
			t.Fatal("parse() error = nil, want duplicate workspace field error")
		}
		if !strings.Contains(err.Error(), `workspace.root`) || !strings.Contains(err.Error(), "already been defined") {
			t.Fatalf("parse() error = %q, want duplicate workspace.root details from TOML parser", err)
		}
	})

	t.Run("source array", func(t *testing.T) {
		t.Parallel()

		_, err := parse(bytes.NewBufferString(`
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
include = ["runbooks/*.md"]
`))
		if err == nil {
			t.Fatal("parse() error = nil, want duplicate sources array field error")
		}
		if !strings.Contains(err.Error(), `sources.include`) || !strings.Contains(err.Error(), "already been defined") {
			t.Fatalf("parse() error = %q, want duplicate sources.include details from TOML parser", err)
		}
	})
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

func TestLoadRejectsJSONSourceWithoutPath(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "json-specs"
adapter = "json"
kind = "json_spec"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want missing JSON path error")
	}
	if !strings.Contains(err.Error(), `source "json-specs".path: value is required`) {
		t.Fatalf("Load() error = %q, want missing JSON path detail", err)
	}
}

func TestLoadRejectsUnsupportedJSONKind(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "schemas"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "json-specs"
adapter = "json"
kind = "spec"
path = "schemas"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want unsupported JSON kind error")
	}
	if !strings.Contains(err.Error(), `source "json-specs".kind: unsupported kind "spec" for adapter "json"`) {
		t.Fatalf("Load() error = %q, want unsupported JSON kind detail", err)
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
	if !strings.Contains(err.Error(), `sources.include`) || !strings.Contains(err.Error(), "array terminator") {
		t.Fatalf("Load() error = %q, want unterminated selector-array detail from TOML parser", err)
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
	if !strings.Contains(err.Error(), `sources.include`) || !strings.Contains(err.Error(), "array terminator") {
		t.Fatalf("Load() error = %q, want unterminated selector-array detail from TOML parser", err)
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
	if !strings.Contains(err.Error(), `supported providers: "disabled", "openai_compatible"`) {
		t.Fatalf("Load() error = %q, want provider list", err)
	}
}

func TestLoadAcceptsOpenAIAnalysisProvider(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.analysis]
provider = "openai_compatible"
model = "pituitary-analysis"
endpoint = "http://127.0.0.1:1234/v1"
timeout_ms = 5000
max_retries = 1

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
	if got, want := cfg.Runtime.Analysis.Provider, RuntimeProviderOpenAI; got != want {
		t.Fatalf("runtime.analysis.provider = %q, want %q", got, want)
	}
	if got, want := cfg.Runtime.Analysis.Model, "pituitary-analysis"; got != want {
		t.Fatalf("runtime.analysis.model = %q, want %q", got, want)
	}
	if got, want := cfg.Runtime.Analysis.Endpoint, "http://127.0.0.1:1234/v1"; got != want {
		t.Fatalf("runtime.analysis.endpoint = %q, want %q", got, want)
	}
}

func TestLoadPreservesExplicitZeroRuntimeTimeouts(t *testing.T) {
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
endpoint = "http://127.0.0.1:1234/v1"
timeout_ms = 0

[runtime.analysis]
provider = "openai_compatible"
model = "pituitary-analysis"
endpoint = "http://127.0.0.1:1234/v1"
timeout_ms = 0

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
	if got, want := cfg.Runtime.Embedder.TimeoutMS, 0; got != want {
		t.Fatalf("runtime.embedder.timeout_ms = %d, want %d", got, want)
	}
	if got, want := cfg.Runtime.Analysis.TimeoutMS, 0; got != want {
		t.Fatalf("runtime.analysis.timeout_ms = %d, want %d", got, want)
	}
}

func TestLoadRejectsOpenAIAnalysisProviderWithoutEndpoint(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.analysis]
provider = "openai_compatible"
model = "pituitary-analysis"

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
	if !strings.Contains(err.Error(), `runtime.analysis.endpoint: value is required for provider "openai_compatible"`) {
		t.Fatalf("Load() error = %q, want missing analysis endpoint detail", err)
	}
}

func TestLoadAcceptsNonFilesystemAdapterOptions(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	configPath := filepath.Join(repo, "pituitary.toml")
	content := `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "github-issues"
adapter = "github"
kind = "issue"

[sources.options]
repo = "dusk-network/pituitary"
labels = ["spec", "rfc"]
state = "open"
per_page = 50
include_closed = false

[sources.options.headers]
accept = "application/vnd.github+json"
`

	raw, err := parse(strings.NewReader(strings.TrimSpace(content)))
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if got, want := raw.Workspace.Root, "."; got != want {
		t.Fatalf("raw workspace root = %q, want %q (raw = %#v)", got, want, raw)
	}
	if got, want := len(raw.Sources), 1; got != want {
		t.Fatalf("raw source count = %d, want %d (raw = %#v)", got, want, raw)
	}
	cfg, err := buildFromRaw(configPath, raw, true)
	if err != nil {
		t.Fatalf("buildFromRaw() error = %v", err)
	}
	source := cfg.Sources[0]
	if got, want := source.Path, ""; got != want {
		t.Fatalf("source path = %q, want %q", got, want)
	}
	if source.ResolvedPath != "" {
		t.Fatalf("resolved path = %q, want empty", source.ResolvedPath)
	}
	if got, want := source.Options["repo"], "dusk-network/pituitary"; got != want {
		t.Fatalf("repo option = %#v, want %#v", got, want)
	}
	labels, ok := source.Options["labels"].([]any)
	if !ok {
		t.Fatalf("labels option type = %T, want []any", source.Options["labels"])
	}
	if got, want := optionStrings(labels), []string{"spec", "rfc"}; !equalStringSlices(got, want) {
		t.Fatalf("labels option = %#v, want %#v", got, want)
	}
	if got, want := source.Options["per_page"], int64(50); got != want {
		t.Fatalf("per_page option = %#v, want %#v", got, want)
	}
	if got, want := source.Options["include_closed"], false; got != want {
		t.Fatalf("include_closed option = %#v, want %#v", got, want)
	}
	headers, ok := source.Options["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers option type = %T, want map[string]any", source.Options["headers"])
	}
	if got, want := headers["accept"], "application/vnd.github+json"; got != want {
		t.Fatalf("headers.accept = %#v, want %#v", got, want)
	}
}

func TestLoadRejectsUnknownAdapter(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "missing"
kind = "spec_bundle"
path = "specs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), `source "specs".adapter: unknown adapter "missing"`) {
		t.Fatalf("Load() error = %q, want unknown adapter details", err)
	}
	if !strings.Contains(err.Error(), `registered adapters: filesystem, github`) {
		t.Fatalf("Load() error = %q, want registered adapter details", err)
	}
}

func TestRenderRoundTripsSourceOptions(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "docs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	cfg := &Config{
		SchemaVersion: CurrentSchemaVersion,
		ConfigPath:    configPath,
		ConfigDir:     repo,
		Workspace: Workspace{
			Root:      ".",
			IndexPath: ".pituitary/pituitary.db",
		},
		Sources: []Source{
			{
				Name:    "docs",
				Adapter: AdapterFilesystem,
				Kind:    SourceKindMarkdownDocs,
				Path:    "docs",
				Options: map[string]any{
					"labels":         []string{"spec", "rfc"},
					"include_closed": false,
					"per_page":       50,
					"headers": map[string]any{
						"accept": "application/json",
					},
				},
			},
		},
	}

	rendered, err := Render(cfg)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	loaded, err := LoadFromText(rendered, configPath)
	if err != nil {
		t.Fatalf("LoadFromText() error = %v", err)
	}
	options := loaded.Sources[0].Options
	labels, ok := options["labels"].([]any)
	if !ok {
		t.Fatalf("labels option type = %T, want []any", options["labels"])
	}
	if got, want := optionStrings(labels), []string{"spec", "rfc"}; !equalStringSlices(got, want) {
		t.Fatalf("labels option = %#v, want %#v", got, want)
	}
	if got, want := options["include_closed"], false; got != want {
		t.Fatalf("include_closed option = %#v, want %#v", got, want)
	}
	if got, want := options["per_page"], int64(50); got != want {
		t.Fatalf("per_page option = %#v, want %#v", got, want)
	}
	headers, ok := options["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers option type = %T, want map[string]any", options["headers"])
	}
	if got, want := headers["accept"], "application/json"; got != want {
		t.Fatalf("headers.accept = %#v, want %#v", got, want)
	}
}

func TestLoadAcceptsSourceRole(t *testing.T) {
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
role = "historical"
path = "docs"
`)

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.Sources[0].Role, SourceRoleHistorical; got != want {
		t.Fatalf("source role = %q, want %q", got, want)
	}
}

func TestLoadRejectsInvalidSourceRole(t *testing.T) {
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
role = "activeish"
path = "docs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), `source "docs".role: unsupported role "activeish"`) {
		t.Fatalf("Load() error = %q, want unsupported role detail", err)
	}
}

func TestRenderRoundTripsSourceRole(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "docs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	cfg := &Config{
		SchemaVersion: CurrentSchemaVersion,
		ConfigPath:    configPath,
		ConfigDir:     repo,
		Workspace: Workspace{
			Root:      ".",
			IndexPath: ".pituitary/pituitary.db",
		},
		Sources: []Source{
			{
				Name:    "docs",
				Adapter: AdapterFilesystem,
				Kind:    SourceKindMarkdownDocs,
				Role:    SourceRoleMirror,
				Path:    "docs",
			},
		},
	}

	rendered, err := Render(cfg)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	loaded, err := LoadFromText(rendered, configPath)
	if err != nil {
		t.Fatalf("LoadFromText() error = %v", err)
	}
	if got, want := loaded.Sources[0].Role, SourceRoleMirror; got != want {
		t.Fatalf("loaded role = %q, want %q", got, want)
	}
}

func TestRenderRoundTripsWorkspaceRepos(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	primary := filepath.Join(root, "primary")
	shared := filepath.Join(root, "shared")
	mustMkdirAll(t, filepath.Join(primary, "specs"))
	mustMkdirAll(t, filepath.Join(shared, "docs"))

	configPath := filepath.Join(root, "pituitary.toml")
	cfg := &Config{
		SchemaVersion: CurrentSchemaVersion,
		ConfigPath:    configPath,
		ConfigDir:     root,
		Workspace: Workspace{
			Root:      "primary",
			RepoID:    "primary",
			IndexPath: ".pituitary/pituitary.db",
			Repos: []WorkspaceRepo{
				{ID: "shared", Root: "shared"},
			},
		},
		Sources: []Source{
			{
				Name:    "specs",
				Adapter: AdapterFilesystem,
				Kind:    SourceKindSpecBundle,
				Path:    "specs",
			},
			{
				Name:    "shared-docs",
				Adapter: AdapterFilesystem,
				Kind:    SourceKindMarkdownDocs,
				Repo:    "shared",
				Path:    "docs",
			},
		},
	}

	rendered, err := Render(cfg)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(rendered, "repo_id = \"primary\"") {
		t.Fatalf("rendered config %q does not contain workspace repo_id", rendered)
	}
	if !strings.Contains(rendered, "[[workspace.repos]]") || !strings.Contains(rendered, "id = \"shared\"") {
		t.Fatalf("rendered config %q does not contain workspace.repos entry", rendered)
	}
	if !strings.Contains(rendered, "repo = \"shared\"") {
		t.Fatalf("rendered config %q does not contain source repo", rendered)
	}

	loaded, err := LoadFromText(rendered, configPath)
	if err != nil {
		t.Fatalf("LoadFromText() error = %v", err)
	}
	if got, want := loaded.Workspace.RepoID, "primary"; got != want {
		t.Fatalf("loaded workspace repo_id = %q, want %q", got, want)
	}
	if got, want := len(loaded.Workspace.Repos), 1; got != want {
		t.Fatalf("loaded workspace repos = %d, want %d", got, want)
	}
	if got, want := loaded.Workspace.Repos[0].ID, "shared"; got != want {
		t.Fatalf("loaded workspace repo id = %q, want %q", got, want)
	}
	if got, want := loaded.Sources[1].Repo, "shared"; got != want {
		t.Fatalf("loaded source repo = %q, want %q", got, want)
	}
}

func TestRenderRoundTripsRuntimeProfiles(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	cfg := &Config{
		SchemaVersion: CurrentSchemaVersion,
		ConfigPath:    configPath,
		ConfigDir:     repo,
		Workspace: Workspace{
			Root:      ".",
			IndexPath: ".pituitary/pituitary.db",
		},
		Runtime: Runtime{
			Profiles: map[string]RuntimeProvider{
				"local-lm-studio": {
					Provider:   RuntimeProviderOpenAI,
					Endpoint:   "http://127.0.0.1:1234/v1",
					TimeoutMS:  30000,
					MaxRetries: 1,
				},
			},
			Embedder: RuntimeProvider{
				Profile:    "local-lm-studio",
				Provider:   RuntimeProviderOpenAI,
				Model:      "nomic-embed-text-v1.5",
				Endpoint:   "http://127.0.0.1:1234/v1",
				TimeoutMS:  30000,
				MaxRetries: 1,
			},
			Analysis: RuntimeProvider{
				Profile:           "local-lm-studio",
				Provider:          RuntimeProviderOpenAI,
				Model:             "qwen3.5-35b",
				Endpoint:          "http://127.0.0.1:1234/v1",
				TimeoutMS:         120000,
				MaxRetries:        1,
				MaxResponseTokens: 2048,
			},
		},
		Sources: []Source{
			{
				Name:    "specs",
				Adapter: AdapterFilesystem,
				Kind:    SourceKindSpecBundle,
				Path:    "specs",
			},
		},
	}

	rendered, err := Render(cfg)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(rendered, "[runtime.profiles.local-lm-studio]") {
		t.Fatalf("rendered config %q does not contain runtime profile table", rendered)
	}
	if !strings.Contains(rendered, "profile = \"local-lm-studio\"") {
		t.Fatalf("rendered config %q does not contain runtime profile selection", rendered)
	}

	loaded, err := LoadFromText(rendered, configPath)
	if err != nil {
		t.Fatalf("LoadFromText() error = %v", err)
	}
	if got, want := loaded.Runtime.Embedder.Profile, "local-lm-studio"; got != want {
		t.Fatalf("loaded runtime.embedder.profile = %q, want %q", got, want)
	}
	if got, want := loaded.Runtime.Embedder.Endpoint, "http://127.0.0.1:1234/v1"; got != want {
		t.Fatalf("loaded runtime.embedder.endpoint = %q, want %q", got, want)
	}
	if got, want := loaded.Runtime.Analysis.TimeoutMS, 120000; got != want {
		t.Fatalf("loaded runtime.analysis.timeout_ms = %d, want %d", got, want)
	}
	if got, want := loaded.Runtime.Analysis.MaxResponseTokens, 2048; got != want {
		t.Fatalf("loaded runtime.analysis.max_response_tokens = %d, want %d", got, want)
	}
}

func TestLoadTerminologyPolicies(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[terminology.policies]]
preferred = "locality"
historical_aliases = ["repo"]
deprecated_terms = ["repository"]
forbidden_current = ["repo mode"]
docs_severity = "error"

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
	if got, want := len(cfg.Terminology.Policies), 1; got != want {
		t.Fatalf("len(terminology.policies) = %d, want %d", got, want)
	}
	policy := cfg.Terminology.Policies[0]
	if got, want := policy.Preferred, "locality"; got != want {
		t.Fatalf("preferred = %q, want %q", got, want)
	}
	if got, want := policy.HistoricalAliases, []string{"repo"}; !equalStringSlices(got, want) {
		t.Fatalf("historical_aliases = %#v, want %#v", got, want)
	}
	if got, want := policy.DeprecatedTerms, []string{"repository"}; !equalStringSlices(got, want) {
		t.Fatalf("deprecated_terms = %#v, want %#v", got, want)
	}
	if got, want := policy.ForbiddenCurrent, []string{"repo mode"}; !equalStringSlices(got, want) {
		t.Fatalf("forbidden_current = %#v, want %#v", got, want)
	}
	if got, want := policy.DocsSeverity, TerminologySeverityError; got != want {
		t.Fatalf("docs_severity = %q, want %q", got, want)
	}
	if got, want := policy.SpecsSeverity, TerminologySeverityWarning; got != want {
		t.Fatalf("specs_severity = %q, want %q", got, want)
	}
}

func TestLoadTerminologyExcludePaths(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[terminology]
exclude_paths = ["CHANGELOG.md", "docs/archive/*.md"]

[[terminology.policies]]
preferred = "locality"
historical_aliases = ["repo"]

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
	if got, want := cfg.Terminology.ExcludePaths, []string{"CHANGELOG.md", "docs/archive/*.md"}; !equalStringSlices(got, want) {
		t.Fatalf("exclude_paths = %#v, want %#v", got, want)
	}
}

func TestLoadRejectsInvalidTerminologySeverity(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[terminology.policies]]
preferred = "locality"
historical_aliases = ["repo"]
docs_severity = "fatal"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want terminology validation error")
	}
	if !strings.Contains(err.Error(), `terminology.policies[0].docs_severity: unsupported severity "fatal"`) {
		t.Fatalf("Load() error = %q, want terminology severity detail", err)
	}
}

func TestRenderRoundTripsTerminologyPolicies(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	cfg := &Config{
		SchemaVersion: CurrentSchemaVersion,
		ConfigPath:    configPath,
		ConfigDir:     repo,
		Workspace: Workspace{
			Root:      ".",
			IndexPath: ".pituitary/pituitary.db",
		},
		Terminology: Terminology{
			ExcludePaths: []string{"CHANGELOG.md", "docs/archive/*.md"},
			Policies: []TerminologyPolicy{
				{
					Preferred:         "locality",
					HistoricalAliases: []string{"repo"},
					DeprecatedTerms:   []string{"repository"},
					ForbiddenCurrent:  []string{"repo mode"},
					DocsSeverity:      TerminologySeverityError,
					SpecsSeverity:     TerminologySeverityIgnore,
				},
			},
		},
		Sources: []Source{
			{
				Name:    "specs",
				Adapter: AdapterFilesystem,
				Kind:    SourceKindSpecBundle,
				Path:    "specs",
			},
		},
	}

	rendered, err := Render(cfg)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(rendered, "[[terminology.policies]]") || !strings.Contains(rendered, `preferred = "locality"`) {
		t.Fatalf("rendered config %q does not contain terminology policy", rendered)
	}

	loaded, err := LoadFromText(rendered, configPath)
	if err != nil {
		t.Fatalf("LoadFromText() error = %v", err)
	}
	if got, want := len(loaded.Terminology.Policies), 1; got != want {
		t.Fatalf("len(loaded terminology policies) = %d, want %d", got, want)
	}
	if got, want := loaded.Terminology.ExcludePaths, []string{"CHANGELOG.md", "docs/archive/*.md"}; !equalStringSlices(got, want) {
		t.Fatalf("loaded exclude_paths = %#v, want %#v", got, want)
	}
	policy := loaded.Terminology.Policies[0]
	if got, want := policy.Preferred, "locality"; got != want {
		t.Fatalf("loaded preferred = %q, want %q", got, want)
	}
	if got, want := policy.DocsSeverity, TerminologySeverityError; got != want {
		t.Fatalf("loaded docs_severity = %q, want %q", got, want)
	}
	if got, want := policy.SpecsSeverity, TerminologySeverityIgnore; got != want {
		t.Fatalf("loaded specs_severity = %q, want %q", got, want)
	}
}

func TestLoadTerminologyIncludeSemanticMatches(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[terminology]
include_semantic_matches = true

[[terminology.policies]]
preferred = "locality"
historical_aliases = ["repo"]

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
	if !cfg.Terminology.IncludeSemanticMatches {
		t.Fatalf("Terminology.IncludeSemanticMatches = false, want true")
	}

	// Round-trip: render + reload should preserve the flag.
	rendered, err := Render(cfg)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(rendered, "include_semantic_matches = true") {
		t.Fatalf("rendered config %q does not contain include_semantic_matches = true", rendered)
	}
	reloaded, err := LoadFromText(rendered, configPath)
	if err != nil {
		t.Fatalf("LoadFromText() error = %v", err)
	}
	if !reloaded.Terminology.IncludeSemanticMatches {
		t.Fatalf("round-trip lost IncludeSemanticMatches")
	}
}

func TestLoadTerminologyIncludeSemanticMatchesDefaultsFalse(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[terminology.policies]]
preferred = "locality"
historical_aliases = ["repo"]

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
	if cfg.Terminology.IncludeSemanticMatches {
		t.Fatalf("Terminology.IncludeSemanticMatches = true, want false (default)")
	}

	// Render should NOT emit the flag when false (keeps default configs minimal).
	rendered, err := Render(cfg)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if strings.Contains(rendered, "include_semantic_matches") {
		t.Fatalf("rendered config %q contains include_semantic_matches; should omit at default", rendered)
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

func TestLoadRejectsUnknownTerminologyField(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustMkdirAll(t, filepath.Join(repo, "specs"))
	configPath := filepath.Join(repo, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[terminology.policies]]
preferred = "locality"
historical_aliases = ["repo"]
unexpected = "value"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want parse error")
	}
	if !strings.Contains(err.Error(), `unsupported terminology.policies field "unexpected"`) {
		t.Fatalf("Load() error = %q, want unsupported terminology field message", err)
	}
}

func TestDeclaresMultirepoReposReturnsTrueForMultirepoConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
repo_id = "primary"
index_path = ".pituitary/pituitary.db"

[[workspace.repos]]
id = "shared"
root = "../shared"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
`)

	got, err := DeclaresMultirepoRepos(configPath)
	if err != nil {
		t.Fatalf("DeclaresMultirepoRepos() error = %v", err)
	}
	if !got {
		t.Fatal("DeclaresMultirepoRepos() = false, want true")
	}
}

func TestDeclaresMultirepoReposReturnsFalseForSingleRepoConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "pituitary.toml")
	writeFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
`)

	got, err := DeclaresMultirepoRepos(configPath)
	if err != nil {
		t.Fatalf("DeclaresMultirepoRepos() error = %v", err)
	}
	if got {
		t.Fatal("DeclaresMultirepoRepos() = true, want false")
	}
}

func TestDeclaresMultirepoReposReturnsErrorForMissingFile(t *testing.T) {
	t.Parallel()

	got, err := DeclaresMultirepoRepos(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err == nil {
		t.Fatal("DeclaresMultirepoRepos() error = nil, want error")
	}
	if got {
		t.Fatal("DeclaresMultirepoRepos() = true on error, want false")
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

func optionStrings(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil
		}
		result = append(result, text)
	}
	return result
}
