package source

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/diag"
	"github.com/dusk-network/pituitary/internal/model"
)

func TestLoadFromConfigNormalizesRepoFixtures(t *testing.T) {
	t.Parallel()

	repoRoot := repoRoot(t)
	cfg, err := config.Load(filepath.Join(repoRoot, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig() error = %v", err)
	}

	if got, want := len(result.Specs), 3; got != want {
		t.Fatalf("spec count = %d, want %d", got, want)
	}
	if got, want := len(result.Docs), 2; got != want {
		t.Fatalf("doc count = %d, want %d", got, want)
	}
	if got, want := len(result.Sources), 2; got != want {
		t.Fatalf("source summary count = %d, want %d", got, want)
	}
	if result.Sources[0].Name != "specs" || result.Sources[0].SpecCount != 3 || result.Sources[0].ItemCount != 3 {
		t.Fatalf("source summary[0] = %+v, want specs summary", result.Sources[0])
	}
	if result.Sources[1].Name != "docs" || result.Sources[1].DocCount != 2 || result.Sources[1].ItemCount != 2 {
		t.Fatalf("source summary[1] = %+v, want docs summary", result.Sources[1])
	}

	specsByRef := make(map[string]model.SpecRecord, len(result.Specs))
	for _, spec := range result.Specs {
		specsByRef[spec.Ref] = spec
		if spec.Kind != model.ArtifactKindSpec {
			t.Fatalf("spec %s kind = %q, want %q", spec.Ref, spec.Kind, model.ArtifactKindSpec)
		}
		if spec.SourceRef == "" || spec.ContentHash == "" {
			t.Fatalf("spec %s missing source_ref/content_hash", spec.Ref)
		}
		if spec.BodyFormat != model.BodyFormatMarkdown || spec.BodyText == "" {
			t.Fatalf("spec %s body not normalized", spec.Ref)
		}
	}

	legacy := specsByRef["SPEC-008"]
	if legacy.Status != model.StatusSuperseded {
		t.Fatalf("SPEC-008 status = %q, want %q", legacy.Status, model.StatusSuperseded)
	}
	if legacy.SourceRef != "file://specs/rate-limit-legacy/spec.toml" {
		t.Fatalf("SPEC-008 source_ref = %q", legacy.SourceRef)
	}

	v2 := specsByRef["SPEC-042"]
	if v2.SourceRef != "file://specs/rate-limit-v2/spec.toml" {
		t.Fatalf("SPEC-042 source_ref = %q", v2.SourceRef)
	}
	if !hasRelation(v2.Relations, model.RelationSupersedes, "SPEC-008") {
		t.Fatalf("SPEC-042 missing supersedes relation")
	}

	burst := specsByRef["SPEC-055"]
	if !hasRelation(burst.Relations, model.RelationDependsOn, "SPEC-042") {
		t.Fatalf("SPEC-055 missing depends_on relation")
	}
	if legacy.ContentHash == v2.ContentHash {
		t.Fatalf("legacy and v2 specs share the same content hash")
	}

	docsByRef := make(map[string]model.DocRecord, len(result.Docs))
	for _, doc := range result.Docs {
		docsByRef[doc.Ref] = doc
		if doc.Kind != model.ArtifactKindDoc {
			t.Fatalf("doc %s kind = %q, want %q", doc.Ref, doc.Kind, model.ArtifactKindDoc)
		}
		if doc.SourceRef == "" || doc.ContentHash == "" {
			t.Fatalf("doc %s missing source_ref/content_hash", doc.Ref)
		}
	}

	guide := docsByRef["doc://guides/api-rate-limits"]
	if guide.Title != "Public API Rate Limits" {
		t.Fatalf("guide title = %q", guide.Title)
	}
	if guide.SourceRef != "file://docs/guides/api-rate-limits.md" {
		t.Fatalf("guide source_ref = %q", guide.SourceRef)
	}

	runbook := docsByRef["doc://runbooks/rate-limit-rollout"]
	if runbook.Title != "Rate Limit Rollout Runbook" {
		t.Fatalf("runbook title = %q", runbook.Title)
	}

	resultAgain, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("second LoadFromConfig() error = %v", err)
	}
	if resultAgain.Specs[0].ContentHash == "" || resultAgain.Docs[0].ContentHash == "" {
		t.Fatal("reloaded records missing content hashes")
	}
	if specsByRef[resultAgain.Specs[0].Ref].ContentHash != resultAgain.Specs[0].ContentHash {
		t.Fatalf("spec content hash changed across reloads")
	}
}

func TestLoadFromConfigWithOptionsLogsEmptySourceMatches(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	cfg := &config.Config{
		Workspace: config.Workspace{
			Root:      ".",
			RootPath:  repo,
			IndexPath: ".pituitary/pituitary.db",
		},
		Sources: []config.Source{
			{
				Name:         "specs",
				Adapter:      config.AdapterFilesystem,
				Kind:         config.SourceKindSpecBundle,
				Path:         "specs",
				ResolvedPath: filepath.Join(repo, "specs"),
			},
		},
	}
	if err := os.MkdirAll(cfg.Sources[0].ResolvedPath, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	var stderr bytes.Buffer
	result, err := LoadFromConfigWithOptions(cfg, LoadOptions{
		Logger: diag.NewLogger(&stderr, diag.LevelInfo),
	})
	if err != nil {
		t.Fatalf("LoadFromConfigWithOptions() error = %v", err)
	}
	if got, want := len(result.Specs), 0; got != want {
		t.Fatalf("spec count = %d, want %d", got, want)
	}
	if !strings.Contains(stderr.String(), `pituitary warn: source: source "specs" (spec_bundle specs) matched 0 item(s)`) {
		t.Fatalf("stderr %q does not contain empty-source warning", stderr.String())
	}
}

func TestLoadFromConfigRejectsMissingSpecBody(t *testing.T) {
	t.Parallel()

	repoRoot := repoRoot(t)
	configPath := filepath.Join(repoRoot, "testdata", "invalid-spec-bundle", "pituitary.toml")
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	_, err = LoadFromConfig(cfg)
	if err == nil {
		t.Fatal("LoadFromConfig() error = nil, want missing-body failure")
	}
	if !strings.Contains(err.Error(), "missing-body/body.md") {
		t.Fatalf("LoadFromConfig() error = %q, want path-specific missing body message", err)
	}
}

func TestLoadFromConfigRejectsNestedBundles(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	mustWriteFile(t, filepath.Join(repo, "specs", "parent", "spec.toml"), `
id = "SPEC-100"
title = "Parent"
status = "draft"
domain = "api"
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(repo, "specs", "parent", "body.md"), "Parent body\n")
	mustWriteFile(t, filepath.Join(repo, "specs", "parent", "child", "spec.toml"), `
id = "SPEC-101"
title = "Child"
status = "draft"
domain = "api"
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(repo, "specs", "parent", "child", "body.md"), "Child body\n")

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	_, err = LoadFromConfig(cfg)
	if err == nil {
		t.Fatal("LoadFromConfig() error = nil, want nested-bundle failure")
	}
	if !strings.Contains(err.Error(), "nested spec bundle") {
		t.Fatalf("LoadFromConfig() error = %q, want nested-bundle message", err)
	}
}

func TestLoadAndPreviewAllowExcludedNestedBundle(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
exclude = ["parent/child/spec.toml"]
`)
	mustWriteFile(t, filepath.Join(repo, "specs", "parent", "spec.toml"), `
id = "SPEC-100"
title = "Parent"
status = "draft"
domain = "api"
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(repo, "specs", "parent", "body.md"), "Parent body\n")
	mustWriteFile(t, filepath.Join(repo, "specs", "parent", "child", "spec.toml"), `
id = "SPEC-101"
title = "Child"
status = "draft"
domain = "api"
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(repo, "specs", "parent", "child", "body.md"), "Child body\n")

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig() error = %v", err)
	}
	if got, want := len(result.Specs), 1; got != want {
		t.Fatalf("spec count = %d, want %d", got, want)
	}
	if got, want := result.Specs[0].Ref, "SPEC-100"; got != want {
		t.Fatalf("spec ref = %q, want %q", got, want)
	}

	preview, err := PreviewFromConfig(cfg)
	if err != nil {
		t.Fatalf("PreviewFromConfig() error = %v", err)
	}
	if got, want := preview.Sources[0].ItemCount, 1; got != want {
		t.Fatalf("preview item count = %d, want %d", got, want)
	}
	if got, want := preview.Sources[0].Items[0].Path, "specs/parent/spec.toml"; got != want {
		t.Fatalf("preview item path = %q, want %q", got, want)
	}
}

func TestParseSpecBundleRejectsDuplicateKeys(t *testing.T) {
	t.Parallel()

	t.Run("scalar", func(t *testing.T) {
		t.Parallel()

		_, err := parseSpecBundle([]byte(`
id = "SPEC-100"
title = "First"
title = "Second"
status = "draft"
domain = "api"
body = "body.md"
`))
		if err == nil {
			t.Fatal("parseSpecBundle() error = nil, want duplicate scalar field error")
		}
		if !strings.Contains(err.Error(), "duplicate title; first defined at line 3") {
			t.Fatalf("parseSpecBundle() error = %q, want duplicate title details", err)
		}
	})

	t.Run("array", func(t *testing.T) {
		t.Parallel()

		_, err := parseSpecBundle(bytes.TrimSpace([]byte(`
id = "SPEC-100"
title = "Example"
status = "draft"
domain = "api"
body = "body.md"
authors = ["one"]
authors = [
  "two",
]
`)))
		if err == nil {
			t.Fatal("parseSpecBundle() error = nil, want duplicate array field error")
		}
		if !strings.Contains(err.Error(), "duplicate authors; first defined at line 6") {
			t.Fatalf("parseSpecBundle() error = %q, want duplicate authors details", err)
		}
	})
}

func TestLoadFromConfigRejectsMalformedSpecArrays(t *testing.T) {
	t.Parallel()

	t.Run("unknown array field", func(t *testing.T) {
		t.Parallel()

		repo := t.TempDir()
		mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
		mustWriteFile(t, filepath.Join(repo, "specs", "spec.toml"), `
id = "SPEC-100"
title = "Unknown Array Field"
status = "draft"
domain = "test"
body = "body.md"
widgets = ["unexpected"]
`)
		mustWriteFile(t, filepath.Join(repo, "specs", "body.md"), "Spec body\n")

		cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
		if err != nil {
			t.Fatalf("config.Load() error = %v", err)
		}

		_, err = LoadFromConfig(cfg)
		if err == nil {
			t.Fatal("LoadFromConfig() error = nil, want unsupported array field failure")
		}
		if !strings.Contains(err.Error(), `unsupported array field "widgets"`) {
			t.Fatalf("LoadFromConfig() error = %q, want unsupported array field message", err)
		}
	})

	t.Run("malformed array value", func(t *testing.T) {
		t.Parallel()

		repo := t.TempDir()
		mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
		mustWriteFile(t, filepath.Join(repo, "specs", "spec.toml"), `
id = "SPEC-101"
title = "Malformed Array Value"
status = "draft"
domain = "test"
body = "body.md"
tags = ["valid", bad]
`)
		mustWriteFile(t, filepath.Join(repo, "specs", "body.md"), "Spec body\n")

		cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
		if err != nil {
			t.Fatalf("config.Load() error = %v", err)
		}

		_, err = LoadFromConfig(cfg)
		if err == nil {
			t.Fatal("LoadFromConfig() error = nil, want malformed array failure")
		}
		if !strings.Contains(err.Error(), "expected quoted string") {
			t.Fatalf("LoadFromConfig() error = %q, want quoted-string failure", err)
		}
	})

	t.Run("unterminated multiline array", func(t *testing.T) {
		t.Parallel()

		repo := t.TempDir()
		mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
		mustWriteFile(t, filepath.Join(repo, "specs", "spec.toml"), `
id = "SPEC-102"
title = "Unterminated Multiline Array"
status = "draft"
domain = "test"
body = "body.md"
tags = [
"valid"
`)
		mustWriteFile(t, filepath.Join(repo, "specs", "body.md"), "Spec body\n")

		cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
		if err != nil {
			t.Fatalf("config.Load() error = %v", err)
		}

		_, err = LoadFromConfig(cfg)
		if err == nil {
			t.Fatal("LoadFromConfig() error = nil, want unterminated array failure")
		}
		if !strings.Contains(err.Error(), `unterminated array for "tags"`) {
			t.Fatalf("LoadFromConfig() error = %q, want unterminated array message", err)
		}
	})
}

func TestLoadFromConfigRejectsDuplicateSpecRefsAcrossSources(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs-a"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs-a"

[[sources]]
name = "specs-b"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs-b"
`)
	mustWriteFile(t, filepath.Join(repo, "specs-a", "spec.toml"), `
id = "SPEC-200"
title = "First Duplicate Spec"
status = "draft"
domain = "test"
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(repo, "specs-a", "body.md"), "Spec body A\n")
	mustWriteFile(t, filepath.Join(repo, "specs-b", "spec.toml"), `
id = "SPEC-200"
title = "Second Duplicate Spec"
status = "draft"
domain = "test"
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(repo, "specs-b", "body.md"), "Spec body B\n")

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	_, err = LoadFromConfig(cfg)
	if err == nil {
		t.Fatal("LoadFromConfig() error = nil, want duplicate spec failure")
	}
	if !strings.Contains(err.Error(), `duplicate spec ref "SPEC-200"`) {
		t.Fatalf("LoadFromConfig() error = %q, want duplicate spec ref message", err)
	}
	if !strings.Contains(err.Error(), `source "specs-a"`) || !strings.Contains(err.Error(), `source "specs-b"`) {
		t.Fatalf("LoadFromConfig() error = %q, want both source names", err)
	}
	if !strings.Contains(err.Error(), `path "specs-a"`) || !strings.Contains(err.Error(), `path "specs-b"`) {
		t.Fatalf("LoadFromConfig() error = %q, want both source paths", err)
	}
}

func TestLoadFromConfigRejectsDuplicateDocRefsAcrossSources(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs-a"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs-a"

[[sources]]
name = "docs-b"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs-b"
`)
	mustWriteFile(t, filepath.Join(repo, "docs-a", "guide.md"), "# Shared Guide\n")
	mustWriteFile(t, filepath.Join(repo, "docs-b", "guide.md"), "# Shared Guide\n")

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	_, err = LoadFromConfig(cfg)
	if err == nil {
		t.Fatal("LoadFromConfig() error = nil, want duplicate doc failure")
	}
	if !strings.Contains(err.Error(), `duplicate doc ref "doc://guide"`) {
		t.Fatalf("LoadFromConfig() error = %q, want duplicate doc ref message", err)
	}
	if !strings.Contains(err.Error(), `source "docs-a"`) || !strings.Contains(err.Error(), `source "docs-b"`) {
		t.Fatalf("LoadFromConfig() error = %q, want both source names", err)
	}
	if !strings.Contains(err.Error(), `path "docs-a"`) || !strings.Contains(err.Error(), `path "docs-b"`) {
		t.Fatalf("LoadFromConfig() error = %q, want both source paths", err)
	}
}

func TestLoadFromConfigFiltersMarkdownDocsBySelectors(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md", "runbooks/*.md"]
exclude = ["runbooks/draft-*.md"]
`)
	mustWriteFile(t, filepath.Join(repo, "docs", "guides", "api-rate-limits.md"), "# API Rate Limits\n")
	mustWriteFile(t, filepath.Join(repo, "docs", "runbooks", "rate-limit-rollout.md"), "# Rollout\n")
	mustWriteFile(t, filepath.Join(repo, "docs", "runbooks", "draft-rollout.md"), "# Draft\n")
	mustWriteFile(t, filepath.Join(repo, "docs", "development", "testing-guide.md"), "# Testing Guide\n")

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig() error = %v", err)
	}
	if got, want := len(result.Docs), 2; got != want {
		t.Fatalf("doc count = %d, want %d", got, want)
	}

	refs := []string{result.Docs[0].Ref, result.Docs[1].Ref}
	sort.Strings(refs)
	wantRefs := []string{"doc://guides/api-rate-limits", "doc://runbooks/rate-limit-rollout"}
	if !equalStrings(refs, wantRefs) {
		t.Fatalf("doc refs = %#v, want %#v", refs, wantRefs)
	}
}

func TestLoadFromConfigFiltersMarkdownDocsByExplicitFilesAndKeepsStableRefs(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "."
files = ["docs/guides/api-rate-limits.md", "docs/runbooks/rate-limit-rollout.md"]
`)
	mustWriteFile(t, filepath.Join(repo, "docs", "guides", "api-rate-limits.md"), "# API Rate Limits\n")
	mustWriteFile(t, filepath.Join(repo, "docs", "runbooks", "rate-limit-rollout.md"), "# Rollout\n")
	mustWriteFile(t, filepath.Join(repo, "docs", "development", "testing-guide.md"), "# Testing Guide\n")

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig() error = %v", err)
	}
	if got, want := len(result.Docs), 2; got != want {
		t.Fatalf("doc count = %d, want %d", got, want)
	}

	refs := []string{result.Docs[0].Ref, result.Docs[1].Ref}
	sort.Strings(refs)
	wantRefs := []string{"doc://docs/guides/api-rate-limits", "doc://docs/runbooks/rate-limit-rollout"}
	if !equalStrings(refs, wantRefs) {
		t.Fatalf("doc refs = %#v, want %#v", refs, wantRefs)
	}
}

func TestLoadFromConfigFiltersSpecBundlesByExplicitFiles(t *testing.T) {
	t.Parallel()

	repoRoot := repoRoot(t)
	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "`+filepath.ToSlash(repoRoot)+`"
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
files = ["rate-limit-v2/spec.toml", "burst-handling/spec.toml"]
`)

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig() error = %v", err)
	}
	if got, want := len(result.Specs), 2; got != want {
		t.Fatalf("spec count = %d, want %d", got, want)
	}

	refs := []string{result.Specs[0].Ref, result.Specs[1].Ref}
	sort.Strings(refs)
	wantRefs := []string{"SPEC-042", "SPEC-055"}
	if !equalStrings(refs, wantRefs) {
		t.Fatalf("spec refs = %#v, want %#v", refs, wantRefs)
	}
}

func TestLoadFromConfigLoadsMarkdownContractsWithExplicitMetadata(t *testing.T) {
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
---
id: RFC-AUTH-001
status: accepted
domain: identity
supersedes: SPEC-008
depends_on:
  - SPEC-042
applies_to:
  - code://src/auth/session_policy.go
  - config://config/auth/session.yaml
---

# Session Policy

All interactive sessions must use tenant-scoped policy evaluation.
`)

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig() error = %v", err)
	}
	if got, want := len(result.Specs), 1; got != want {
		t.Fatalf("spec count = %d, want %d", got, want)
	}

	spec := result.Specs[0]
	if got, want := spec.Ref, "RFC-AUTH-001"; got != want {
		t.Fatalf("ref = %q, want %q", got, want)
	}
	if got, want := spec.Status, model.StatusAccepted; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if got, want := spec.Domain, "identity"; got != want {
		t.Fatalf("domain = %q, want %q", got, want)
	}
	if got, want := spec.Title, "Session Policy"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
	if !hasRelation(spec.Relations, model.RelationSupersedes, "SPEC-008") {
		t.Fatalf("relations = %+v, want supersedes SPEC-008", spec.Relations)
	}
	if !hasRelation(spec.Relations, model.RelationDependsOn, "SPEC-042") {
		t.Fatalf("relations = %+v, want depends_on SPEC-042", spec.Relations)
	}
	if got, want := spec.AppliesTo, []string{"code://src/auth/session_policy.go", "config://config/auth/session.yaml"}; !equalStrings(got, want) {
		t.Fatalf("applies_to = %#v, want %#v", got, want)
	}
	if got, want := spec.SourceRef, "file://contracts/auth/session-policy.md"; got != want {
		t.Fatalf("source_ref = %q, want %q", got, want)
	}
	if got, want := spec.Metadata["ref_source"], "explicit"; got != want {
		t.Fatalf("metadata.ref_source = %q, want %q", got, want)
	}
	if spec.Inference == nil || spec.Inference.Level != "high" {
		t.Fatalf("inference = %+v, want high confidence", spec.Inference)
	}

	preview, err := PreviewFromConfig(cfg)
	if err != nil {
		t.Fatalf("PreviewFromConfig() error = %v", err)
	}
	if got, want := preview.Sources[0].Items[0].ArtifactKind, "spec"; got != want {
		t.Fatalf("preview artifact kind = %q, want %q", got, want)
	}
	if got, want := preview.Sources[0].Items[0].Path, "contracts/auth/session-policy.md"; got != want {
		t.Fatalf("preview path = %q, want %q", got, want)
	}
}

func TestLoadFromConfigMarkdownContractsDegradeGracefullyWhenMetadataMissing(t *testing.T) {
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

Use tenant-scoped limits and preserve burst budgets.
`)

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig() error = %v", err)
	}
	if got, want := len(result.Specs), 1; got != want {
		t.Fatalf("spec count = %d, want %d", got, want)
	}

	spec := result.Specs[0]
	if got, want := spec.Ref, "contract://contracts/platform/tenant-rate-limits"; got != want {
		t.Fatalf("fallback ref = %q, want %q", got, want)
	}
	if got, want := spec.Status, model.StatusDraft; got != want {
		t.Fatalf("fallback status = %q, want %q", got, want)
	}
	if got, want := spec.Title, "Tenant Rate Limits"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
	if spec.Domain != "" {
		t.Fatalf("domain = %q, want empty fallback", spec.Domain)
	}
	if got, want := spec.Metadata["ref_source"], "path"; got != want {
		t.Fatalf("metadata.ref_source = %q, want %q", got, want)
	}
	if got, want := spec.Metadata["status_source"], "default"; got != want {
		t.Fatalf("metadata.status_source = %q, want %q", got, want)
	}
	if spec.Inference == nil || spec.Inference.Level != "low" {
		t.Fatalf("inference = %+v, want low confidence", spec.Inference)
	}
	if len(spec.Inference.Reasons) == 0 {
		t.Fatalf("inference reasons = %+v, want explicit fallback reasons", spec.Inference)
	}
}

func TestPreviewFromConfigUsesSelectors(t *testing.T) {
	t.Parallel()

	repoRoot := repoRoot(t)
	cfg, err := config.Load(filepath.Join(repoRoot, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := PreviewFromConfig(cfg)
	if err != nil {
		t.Fatalf("PreviewFromConfig() error = %v", err)
	}
	if got, want := len(result.Sources), 2; got != want {
		t.Fatalf("source count = %d, want %d", got, want)
	}

	specs := result.Sources[0]
	if specs.Name != "specs" || specs.ItemCount != 3 {
		t.Fatalf("spec preview = %+v, want 3 spec items", specs)
	}

	docs := result.Sources[1]
	if docs.Name != "docs" || docs.ItemCount != 2 {
		t.Fatalf("docs preview = %+v, want 2 doc items", docs)
	}

	paths := []string{docs.Items[0].Path, docs.Items[1].Path}
	sort.Strings(paths)
	wantPaths := []string{"docs/guides/api-rate-limits.md", "docs/runbooks/rate-limit-rollout.md"}
	if !equalStrings(paths, wantPaths) {
		t.Fatalf("doc preview paths = %#v, want %#v", paths, wantPaths)
	}
}

func TestPreviewFromConfigUsesExplicitFiles(t *testing.T) {
	t.Parallel()

	repoRoot := repoRoot(t)
	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "`+filepath.ToSlash(repoRoot)+`"
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
files = ["guides/api-rate-limits.md"]
`)

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := PreviewFromConfig(cfg)
	if err != nil {
		t.Fatalf("PreviewFromConfig() error = %v", err)
	}
	if got, want := len(result.Sources), 1; got != want {
		t.Fatalf("source count = %d, want %d", got, want)
	}
	if got, want := result.Sources[0].Files, []string{"guides/api-rate-limits.md"}; !equalStrings(got, want) {
		t.Fatalf("preview files = %#v, want %#v", got, want)
	}
	if got, want := result.Sources[0].ItemCount, 1; got != want {
		t.Fatalf("item count = %d, want %d", got, want)
	}
	if got, want := result.Sources[0].Items[0].Path, "docs/guides/api-rate-limits.md"; got != want {
		t.Fatalf("preview path = %q, want %q", got, want)
	}
}

func TestLoadFromConfigReadsOversizedSpecArrayValues(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	longTag := strings.Repeat("spec-tag-", 8*1024)
	mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	mustWriteFile(t, filepath.Join(repo, "specs", "spec.toml"), `
id = "SPEC-300"
title = "Oversized Array"
status = "draft"
domain = "test"
body = "body.md"
tags = ["`+longTag+`"]
`)
	mustWriteFile(t, filepath.Join(repo, "specs", "body.md"), "Spec body\n")

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig() error = %v", err)
	}
	if got, want := len(result.Specs), 1; got != want {
		t.Fatalf("spec count = %d, want %d", got, want)
	}
	if got, want := result.Specs[0].Tags, []string{longTag}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("spec tags = %#v, want %#v", got, want)
	}
}

func TestLoadFromConfigFallsBackToFilenameForDocTitle(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
`)
	mustWriteFile(t, filepath.Join(repo, "docs", "operational-notes.md"), "No title heading here.\n")

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig() error = %v", err)
	}
	if got, want := len(result.Docs), 1; got != want {
		t.Fatalf("doc count = %d, want %d", got, want)
	}
	if got, want := result.Docs[0].Ref, "doc://operational-notes"; got != want {
		t.Fatalf("doc ref = %q, want %q", got, want)
	}
	if got, want := result.Docs[0].Title, "operational-notes"; got != want {
		t.Fatalf("doc title = %q, want %q", got, want)
	}
}

func TestLoadFromConfigReadsOversizedDocTitle(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	longTitle := strings.Repeat("doc-title-", 8*1024)
	mustWriteFile(t, filepath.Join(repo, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
`)
	mustWriteFile(t, filepath.Join(repo, "docs", "huge-title.md"), "# "+longTitle+"\n\nBody\n")

	cfg, err := config.Load(filepath.Join(repo, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}

	result, err := LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig() error = %v", err)
	}
	if got, want := len(result.Docs), 1; got != want {
		t.Fatalf("doc count = %d, want %d", got, want)
	}
	if got, want := result.Docs[0].Title, longTitle; got != want {
		t.Fatalf("doc title length = %d, want %d", len(got), len(want))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func hasRelation(relations []model.Relation, typ model.RelationType, ref string) bool {
	for _, relation := range relations {
		if relation.Type == typ && relation.Ref == ref {
			return true
		}
	}
	return false
}

func equalStrings(got, want []string) bool {
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
