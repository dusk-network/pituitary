package fusion

import (
	"testing"

	stindex "github.com/dusk-network/stroma/v2/index"
)

func TestResolveZeroConfigReturnsNil(t *testing.T) {
	got, err := Resolve(Config{})
	if err != nil {
		t.Fatalf("Resolve(zero) err = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("Resolve(zero) = %v, want nil so stroma DefaultFusion() governs", got)
	}
}

func TestResolveDefaultStrategyReturnsNil(t *testing.T) {
	got, err := Resolve(Config{Strategy: StrategyDefault})
	if err != nil {
		t.Fatalf("Resolve(default) err = %v, want nil", err)
	}
	if got != nil {
		t.Fatalf("Resolve(default) = %v, want nil for byte-identical default path", got)
	}
}

func TestResolveDefaultStrategyRejectsK(t *testing.T) {
	_, err := Resolve(Config{Strategy: StrategyDefault, K: 60})
	if err == nil {
		t.Fatal("Resolve(default,K=60) err = nil, want strategy-mismatch error")
	}
}

func TestResolveRRFBuildsFusion(t *testing.T) {
	got, err := Resolve(Config{Strategy: StrategyRRF, K: 80})
	if err != nil {
		t.Fatalf("Resolve(rrf,K=80) err = %v", err)
	}
	rrf, ok := got.(stindex.RRFFusion)
	if !ok {
		t.Fatalf("Resolve(rrf) = %T, want stindex.RRFFusion", got)
	}
	if rrf.K != 80 {
		t.Fatalf("RRFFusion.K = %d, want 80", rrf.K)
	}
	if !rrf.PreserveSingleArmScore {
		t.Fatal("RRFFusion.PreserveSingleArmScore = false, want true to preserve stroma default single-arm contract")
	}
}

func TestResolveRRFRequiresPositiveK(t *testing.T) {
	for _, k := range []int{0, -1} {
		if _, err := Resolve(Config{Strategy: StrategyRRF, K: k}); err == nil {
			t.Fatalf("Resolve(rrf,K=%d) err = nil, want positive-K error", k)
		}
	}
}

func TestResolveUnknownStrategyErrors(t *testing.T) {
	_, err := Resolve(Config{Strategy: "weighted_sum"})
	if err == nil {
		t.Fatal("Resolve(unknown) err = nil, want unsupported-strategy error")
	}
}

func TestConfigIsZero(t *testing.T) {
	if !(Config{}).IsZero() {
		t.Fatal("Config{}.IsZero() = false, want true")
	}
	if (Config{Strategy: StrategyRRF, K: 60}).IsZero() {
		t.Fatal("non-default Config reported as zero")
	}
}
