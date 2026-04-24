package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestCompareSpecsReturnsStructuredComparison(t *testing.T) {
	t.Parallel()

	cfg := loadCompareFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CompareSpecs(cfg, CompareRequest{SpecRefs: []string{"SPEC-008", "SPEC-042"}})
	if err != nil {
		t.Fatalf("CompareSpecs() error = %v", err)
	}
	if len(result.SpecRefs) != 2 {
		t.Fatalf("spec_refs = %v, want 2 refs", result.SpecRefs)
	}
	if len(result.Comparison.SharedScope) == 0 {
		t.Fatalf("shared_scope = %v, want shared governed scope", result.Comparison.SharedScope)
	}
	if result.Comparison.Compatibility.Level != "superseding" {
		t.Fatalf("compatibility = %+v, want superseding", result.Comparison.Compatibility)
	}
	if result.Comparison.Recommendation == "" || result.Comparison.Recommendation != "prefer SPEC-042 as the primary reference because it is the strongest accepted successor across the compared set" {
		t.Fatalf("recommendation = %q", result.Comparison.Recommendation)
	}
	if len(result.Comparison.Differences) != 2 {
		t.Fatalf("differences = %+v, want per-spec summaries", result.Comparison.Differences)
	}
	if len(result.Comparison.Tradeoffs) == 0 {
		t.Fatal("tradeoffs is empty")
	}
}

func TestCompareSpecsSupportsDraftSpecRecord(t *testing.T) {
	t.Parallel()

	cfg := loadCompareFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	var draft model.SpecRecord
	for _, spec := range records.Specs {
		if spec.Ref == "SPEC-042" {
			draft = spec
			break
		}
	}
	draft.Ref = "SPEC-900"
	draft.Title = "Draft Rate Limiting Update"
	draft.Status = model.StatusDraft

	result, err := CompareSpecs(cfg, CompareRequest{
		SpecRecord: &draft,
		SpecRefs:   []string{"SPEC-008"},
	})
	if err != nil {
		t.Fatalf("CompareSpecs() draft error = %v", err)
	}
	if len(result.SpecRefs) != 2 || result.SpecRefs[0] != "SPEC-900" || result.SpecRefs[1] != "SPEC-008" {
		t.Fatalf("spec_refs = %v, want [SPEC-900 SPEC-008]", result.SpecRefs)
	}
	if result.Comparison.Recommendation == "" {
		t.Fatal("draft comparison recommendation is empty")
	}
}

func TestCompareSpecsUsesAnalysisProviderWhenEnabled(t *testing.T) {
	t.Parallel()

	cfg := loadCompareFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	configureOpenAIAnalysisProvider(t, cfg, func(t *testing.T, request openAICompatibleChatRequest) string {
		t.Helper()
		if got, want := request.Model, "pituitary-analysis"; got != want {
			t.Fatalf("request.model = %q, want %q", got, want)
		}
		if got, want := request.MaxTokens, 1024; got != want {
			t.Fatalf("request.max_tokens = %d, want %d", got, want)
		}
		if len(request.Messages) != 2 {
			t.Fatalf("messages = %d, want 2", len(request.Messages))
		}

		var prompt compareAnalysisPrompt
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &prompt); err != nil {
			t.Fatalf("unmarshal prompt: %v", err)
		}
		if got, want := prompt.OrderedRefs, []string{"SPEC-008", "SPEC-042"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("ordered_refs = %v, want %v", got, want)
		}
		if len(prompt.Specs) != 2 {
			t.Fatalf("spec prompts = %+v, want two prompts", prompt.Specs)
		}
		if got, want := prompt.Specs[0].Sections[0].Heading, "Requirements"; got != want {
			t.Fatalf("first prompt section heading = %q, want %q", got, want)
		}
		if got, want := prompt.Specs[1].Sections[0].Heading, "Requirements"; got != want {
			t.Fatalf("second prompt section heading = %q, want %q", got, want)
		}

		return `{
			"shared_scope": ["domain:api", "code://src/api/middleware/ratelimiter.go"],
			"differences": [
				{"spec_ref": "SPEC-008", "title": "Per-API-Key Legacy Rate Limiting", "items": ["Uses a fixed window across all tenants"]},
				{"spec_ref": "SPEC-042", "title": "Per-Tenant Rate Limiting for Public API Endpoints", "items": ["Adds tenant-scoped policy overrides"]}
			],
			"tradeoffs": [
				{"topic": "migration", "summary": "SPEC-042 adds override flexibility but requires tenants to converge on the new policy surface."}
			],
			"compatibility": {
				"level": "superseding",
				"summary": "SPEC-042 is the accepted successor and should absorb any remaining SPEC-008 rollout work."
			},
			"recommendation": "prefer SPEC-042 after provider-backed adjudication because it preserves scope while updating the accepted control surface"
		}`
	})

	result, err := CompareSpecs(cfg, CompareRequest{SpecRefs: []string{"SPEC-008", "SPEC-042"}})
	if err != nil {
		t.Fatalf("CompareSpecs() error = %v", err)
	}
	if got, want := result.Comparison.Recommendation, "prefer SPEC-042 after provider-backed adjudication because it preserves scope while updating the accepted control surface"; got != want {
		t.Fatalf("recommendation = %q, want %q", got, want)
	}
	if len(result.Comparison.Tradeoffs) != 1 || result.Comparison.Tradeoffs[0].Topic != "migration" {
		t.Fatalf("tradeoffs = %+v, want provider-backed migration tradeoff", result.Comparison.Tradeoffs)
	}
}

func TestCompareSpecsRejectsMoreThanTwoRefs(t *testing.T) {
	t.Parallel()

	cfg := loadCompareFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	_, err = CompareSpecs(cfg, CompareRequest{SpecRefs: []string{"SPEC-008", "SPEC-042", "SPEC-055"}})
	if err == nil {
		t.Fatal("CompareSpecs() error = nil, want validation error")
	}
	if got := err.Error(); got != "exactly two spec_refs are required" {
		t.Fatalf("CompareSpecs() error = %q, want exactly two spec_refs are required", got)
	}
}

func TestCompareSpecsRejectsDuplicateRefs(t *testing.T) {
	t.Parallel()

	cfg := loadCompareFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	_, err = CompareSpecs(cfg, CompareRequest{SpecRefs: []string{"SPEC-008", "SPEC-008"}})
	if err == nil {
		t.Fatal("CompareSpecs() error = nil, want validation error")
	}
	if got := err.Error(); got != "spec_refs must refer to two distinct specs" {
		t.Fatalf("CompareSpecs() error = %q, want spec_refs must refer to two distinct specs", got)
	}
}

func loadCompareFixtureConfig(t *testing.T) *config.Config {
	t.Helper()

	repoRoot := repoRoot(t)
	root := t.TempDir()
	copyTree(t, filepath.Join(repoRoot, "specs", "rate-limit-legacy"), filepath.Join(root, "specs", "rate-limit-legacy"))
	copyTree(t, filepath.Join(repoRoot, "specs", "rate-limit-v2"), filepath.Join(root, "specs", "rate-limit-v2"))
	mustWriteFile(t, filepath.Join(root, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"
infer_applies_to = false

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	cfg, err := config.Load(filepath.Join(root, "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()

	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
}
