// Package fusion bridges Pituitary's search fusion configuration to
// stroma's FusionStrategy interface. A zero config resolves to nil so
// stroma's DefaultFusion() governs the byte-identical pre-#342 path.
package fusion

import (
	"fmt"
	"strings"

	stindex "github.com/dusk-network/stroma/v2/index"
)

const (
	// StrategyDefault selects stroma's DefaultFusion() exactly. Resolve
	// returns nil on this path so SearchParams.Fusion stays unset and
	// the snapshot keeps the pre-#342 arm-native single-arm score
	// preservation contract documented on stindex.DefaultFusion.
	StrategyDefault = "default_rrf"

	// StrategyRRF selects an explicitly parameterised stindex.RRFFusion.
	// K must be > 0 under this strategy; PreserveSingleArmScore is held
	// true so the single-arm degenerate case keeps stroma's default
	// behavior unless a future knob explicitly opts out.
	StrategyRRF = "rrf"
)

// Config captures pituitary-side fusion overrides. A zero value means
// "no override" and Resolve returns nil.
type Config struct {
	// Strategy names the fusion implementation. Empty or StrategyDefault
	// both resolve to nil so stroma's DefaultFusion() governs the path.
	Strategy string

	// K is the RRF constant. Only consulted under StrategyRRF; must be
	// > 0 there.
	K int
}

// IsZero reports whether no override is configured.
func (c Config) IsZero() bool {
	return c == Config{}
}

// Resolve builds a stroma FusionStrategy from a pituitary Config.
//
// The zero config and StrategyDefault both resolve to nil. Callers are
// expected to pass nil straight through to stindex.SearchParams.Fusion
// so stroma's DefaultFusion() governs the snapshot — byte-identical to
// pre-#342 retrieval output on that path.
func Resolve(cfg Config) (stindex.FusionStrategy, error) {
	strategy := strings.TrimSpace(cfg.Strategy)
	switch strategy {
	case "", StrategyDefault:
		if cfg.K != 0 {
			return nil, fmt.Errorf("fusion.k only applies to strategy %q", StrategyRRF)
		}
		return nil, nil
	case StrategyRRF:
		if cfg.K <= 0 {
			return nil, fmt.Errorf("fusion.k must be > 0 for strategy %q", StrategyRRF)
		}
		return stindex.RRFFusion{K: cfg.K, PreserveSingleArmScore: true}, nil
	default:
		return nil, fmt.Errorf(
			"unsupported fusion.strategy %q (supported: %q, %q)",
			strategy, StrategyDefault, StrategyRRF,
		)
	}
}
