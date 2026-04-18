package chunk

import (
	"fmt"

	stchunk "github.com/dusk-network/stroma/v2/chunk"
	stindex "github.com/dusk-network/stroma/v2/index"

	"github.com/dusk-network/pituitary/sdk"
)

const (
	// PolicyMarkdown reproduces the pre-1.0 chunking pipeline exactly
	// (heading-aware sectioning + optional token-budget splitting).
	PolicyMarkdown = "markdown"

	// PolicyLateChunk emits parent + leaf chunks linked by parent_chunk_id.
	// Parents live as storage-only context that ExpandContext can surface
	// on demand; children are what participate in retrieval.
	PolicyLateChunk = "late_chunk"
)

// KindConfig describes the chunking policy for a single record kind
// (spec or doc). A zero value means "no override" — the resolver treats
// the kind as unset and defers to the router's Default.
type KindConfig struct {
	// Policy selects the chunking strategy. Empty means unset.
	Policy string

	// MaxTokens is the approximate max tokens per section for
	// PolicyMarkdown, and the ParentMaxTokens for PolicyLateChunk.
	// Zero disables the budget.
	MaxTokens int

	// OverlapTokens is the approximate overlap between adjacent
	// sub-sections for PolicyMarkdown. Ignored for PolicyLateChunk
	// (late-chunking uses ChildOverlapTokens).
	OverlapTokens int

	// MaxSections caps the total section count per record. Zero
	// applies stroma's DefaultMaxChunkSections so the per-record DoS
	// guard stays on by default (matches the pre-#338 nil-policy
	// rebuild path). Negative explicitly disables the cap for callers
	// that have upstream validation.
	MaxSections int

	// ChildMaxTokens is required for PolicyLateChunk and must be > 0;
	// stroma errors loud when a late-chunk policy has no child budget.
	ChildMaxTokens int

	// ChildOverlapTokens is the approximate overlap between adjacent
	// leaf chunks for PolicyLateChunk. Zero disables overlap.
	ChildOverlapTokens int
}

// IsZero reports whether the config has no overrides set.
func (c KindConfig) IsZero() bool {
	return c == KindConfig{}
}

// Config aggregates per-kind chunking overrides. A zero value means "no
// router, no overrides" — Resolve returns a nil Policy so stroma's
// default MarkdownPolicy applies exactly as it did pre-#338.
type Config struct {
	Spec KindConfig
	Doc  KindConfig
}

// IsZero reports whether no kind has an override configured.
func (c Config) IsZero() bool {
	return c.Spec.IsZero() && c.Doc.IsZero()
}

// Resolve builds a stroma chunk.Policy from pituitary's per-kind config.
//
// When no kind has an override configured (cfg.IsZero()), Resolve
// returns nil. Callers are expected to pass nil straight through to
// stroma, which preserves the default MarkdownPolicy behavior that
// shipped before the router was introduced — i.e., byte-identical to
// pre-change output.
//
// When at least one kind is configured, Resolve returns a
// KindRouterPolicy whose Default mirrors stroma's bounded nil-policy
// path (MarkdownPolicy with the DefaultMaxChunkSections cap in place)
// so a repo that only configures one kind does not silently disable
// the per-record section cap on the other kind. The ByKind map carries
// the configured kinds resolved to their concrete policies; those
// policies also receive the same cap substitution when the user left
// max_sections unset, so "I only tuned max_tokens" never implicitly
// disables the DoS guard.
func Resolve(cfg Config) (stchunk.Policy, error) {
	if cfg.IsZero() {
		return nil, nil
	}

	router := stchunk.KindRouterPolicy{
		Default: stchunk.MarkdownPolicy{
			Options: stchunk.Options{MaxSections: stindex.DefaultMaxChunkSections},
		},
		ByKind: map[string]stchunk.Policy{},
	}
	if !cfg.Spec.IsZero() {
		policy, err := buildPolicy(cfg.Spec)
		if err != nil {
			return nil, fmt.Errorf("spec chunking: %w", err)
		}
		router.ByKind[sdk.ArtifactKindSpec] = policy
	}
	if !cfg.Doc.IsZero() {
		policy, err := buildPolicy(cfg.Doc)
		if err != nil {
			return nil, fmt.Errorf("doc chunking: %w", err)
		}
		router.ByKind[sdk.ArtifactKindDoc] = policy
	}
	return router, nil
}

// resolveMaxSections mirrors stroma's index.resolveMaxChunkSections.
// Zero picks the bounded default so the per-record DoS guard stays on
// when users only set tuning knobs. Negative explicitly disables the
// cap (matches stroma's contract). Positive is passed through.
func resolveMaxSections(user int) int {
	switch {
	case user < 0:
		return 0 // stroma chunk.Options: 0 == no cap
	case user == 0:
		return stindex.DefaultMaxChunkSections
	default:
		return user
	}
}

func buildPolicy(kc KindConfig) (stchunk.Policy, error) {
	switch kc.Policy {
	case "", PolicyMarkdown:
		return stchunk.MarkdownPolicy{
			Options: stchunk.Options{
				MaxTokens:     kc.MaxTokens,
				OverlapTokens: kc.OverlapTokens,
				MaxSections:   resolveMaxSections(kc.MaxSections),
			},
		}, nil
	case PolicyLateChunk:
		if kc.ChildMaxTokens <= 0 {
			return nil, fmt.Errorf("policy %q requires child_max_tokens > 0", PolicyLateChunk)
		}
		return stchunk.LateChunkPolicy{
			ParentMaxTokens:    kc.MaxTokens,
			ChildMaxTokens:     kc.ChildMaxTokens,
			ChildOverlapTokens: kc.ChildOverlapTokens,
			MaxSections:        resolveMaxSections(kc.MaxSections),
		}, nil
	default:
		return nil, fmt.Errorf("unknown policy %q (expected %q or %q)", kc.Policy, PolicyMarkdown, PolicyLateChunk)
	}
}
