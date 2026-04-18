package config

import (
	"strings"
	"testing"
)

func TestLoadRuntimeSearchDefaultsToZero(t *testing.T) {
	t.Parallel()

	cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"
`)

	if !cfg.Runtime.Search.IsZero() {
		t.Fatalf("runtime.search should be zero by default; got %+v", cfg.Runtime.Search)
	}
}

func TestLoadRuntimeSearchParsesFusion(t *testing.T) {
	t.Parallel()

	cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.search.fusion]
strategy = "rrf"
k = 80
`)

	fusion := cfg.Runtime.Search.Fusion
	if fusion.Strategy != SearchFusionStrategyRRF {
		t.Fatalf("fusion.strategy = %q, want %q", fusion.Strategy, SearchFusionStrategyRRF)
	}
	if fusion.K != 80 {
		t.Fatalf("fusion.k = %d, want 80", fusion.K)
	}
}

func TestLoadRuntimeSearchParsesReranker(t *testing.T) {
	t.Parallel()

	cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.search]
reranker = "arm_aware_historical"
`)

	if got := cfg.Runtime.Search.Reranker; got != SearchRerankerArmAwareHistorical {
		t.Fatalf("search.reranker = %q, want %q", got, SearchRerankerArmAwareHistorical)
	}
}

func TestLoadRuntimeSearchRejectsRRFWithoutK(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.search.fusion]
strategy = "rrf"
`)
	if err == nil {
		t.Fatal("expected error when strategy = rrf has no k")
	}
	if !strings.Contains(err.Error(), "fusion.k") {
		t.Fatalf("error should mention fusion.k; got %v", err)
	}
}

func TestLoadRuntimeSearchRejectsDefaultStrategyWithK(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.search.fusion]
strategy = "default_rrf"
k = 60
`)
	if err == nil {
		t.Fatal("expected error when k is set alongside default_rrf strategy")
	}
	if !strings.Contains(err.Error(), SearchFusionStrategyRRF) {
		t.Fatalf("error should steer users to the rrf strategy; got %v", err)
	}
}

// Regression for the Copilot finding that strategy="rrf" with k<0 used
// to emit two overlapping errors ("must be > 0" and "must be >= 0").
// The validator should now emit a single, clear error.
func TestLoadRuntimeSearchRejectsNegativeKOnce(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.search.fusion]
strategy = "rrf"
k = -1
`)
	if err == nil {
		t.Fatal("expected error for negative k on strategy rrf")
	}
	msg := err.Error()
	if strings.Count(msg, "fusion.k") > 1 {
		t.Fatalf("validator emitted overlapping fusion.k errors; got:\n%s", msg)
	}
	if !strings.Contains(msg, "must be > 0") {
		t.Fatalf("error should call out must-be-positive requirement; got %v", err)
	}
}

func TestLoadRuntimeSearchRejectsUnknownStrategy(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.search.fusion]
strategy = "weighted_sum"
k = 60
`)
	if err == nil {
		t.Fatal("expected error for unknown fusion strategy")
	}
	if !strings.Contains(err.Error(), "unsupported strategy") {
		t.Fatalf("error should call out unsupported strategy; got %v", err)
	}
}

func TestLoadRuntimeSearchRejectsUnknownReranker(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.search]
reranker = "no_such_reranker"
`)
	if err == nil {
		t.Fatal("expected error for unknown reranker policy")
	}
	if !strings.Contains(err.Error(), "reranker") {
		t.Fatalf("error should mention reranker; got %v", err)
	}
}

// Regression for the #346 ghost-field bug: the strict-field parser must
// reject unknown keys under [runtime.search.fusion] so a typo does not
// silently fall through to stroma's DefaultFusion() and appear to work.
func TestLoadRuntimeSearchRejectsGhostFusionField(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.search.fusion]
strategy = "rrf"
k = 60
unknown_knob = true
`)
	if err == nil {
		t.Fatal("expected ghost-field error for runtime.search.fusion.unknown_knob")
	}
	if !strings.Contains(err.Error(), "unknown_knob") {
		t.Fatalf("error should name the unknown field; got %v", err)
	}
}

func TestLoadRuntimeSearchRejectsGhostSearchSubtree(t *testing.T) {
	t.Parallel()

	_, err := loadChunkingFixtureErr(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.search.gadget]
any = "thing"
`)
	if err == nil {
		t.Fatal("expected error for unknown subtree under runtime.search")
	}
	if !strings.Contains(err.Error(), `"fusion"`) {
		t.Fatalf("error should suggest fusion as the supported subtree; got %v", err)
	}
}

func TestRenderRoundTripPreservesSearchFusion(t *testing.T) {
	t.Parallel()

	cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.search.fusion]
strategy = "rrf"
k = 80
`)

	rendered, err := Render(cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(rendered, "[runtime.search.fusion]") {
		t.Fatalf("rendered output missing search fusion block:\n%s", rendered)
	}
	if !strings.Contains(rendered, `strategy = "rrf"`) {
		t.Fatalf("rendered output missing strategy value:\n%s", rendered)
	}
	if !strings.Contains(rendered, "k = 80") {
		t.Fatalf("rendered output missing k value:\n%s", rendered)
	}

	round := loadRenderedConfig(t, rendered)
	if got := round.Runtime.Search.Fusion.Strategy; got != SearchFusionStrategyRRF {
		t.Fatalf("round-tripped fusion.strategy = %q, want %q", got, SearchFusionStrategyRRF)
	}
	if got := round.Runtime.Search.Fusion.K; got != 80 {
		t.Fatalf("round-tripped fusion.k = %d, want 80", got)
	}
}

func TestRenderRoundTripPreservesReranker(t *testing.T) {
	t.Parallel()

	cfg := loadChunkingFixture(t, `
[workspace]
root = "workspace"
index_path = ".pituitary/pituitary.db"

[runtime.search]
reranker = "arm_aware_historical"
`)

	rendered, err := Render(cfg)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(rendered, "[runtime.search]") {
		t.Fatalf("rendered output missing search block:\n%s", rendered)
	}
	if !strings.Contains(rendered, `reranker = "arm_aware_historical"`) {
		t.Fatalf("rendered output missing reranker value:\n%s", rendered)
	}

	round := loadRenderedConfig(t, rendered)
	if got := round.Runtime.Search.Reranker; got != SearchRerankerArmAwareHistorical {
		t.Fatalf("round-tripped search.reranker = %q, want %q", got, SearchRerankerArmAwareHistorical)
	}
}
