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

	// DefaultDocLateChunkParentMaxTokens is the default parent section
	// budget used when docs fall through to the #344 LateChunkPolicy
	// default. Sized for long-form docs while staying well under typical
	// context windows.
	DefaultDocLateChunkParentMaxTokens = 2048
	// DefaultDocLateChunkChildMaxTokens caps leaf chunks at a size that
	// still embeds cleanly with mainstream 512-token embedders.
	DefaultDocLateChunkChildMaxTokens = 384
	// DefaultDocLateChunkChildOverlapTokens adds small leaf overlap so
	// heading-straddling queries do not lose recall at chunk boundaries.
	DefaultDocLateChunkChildOverlapTokens = 48
)

// ResolverDefaultsVersion tags the behavior of Resolve when a kind is
// left unconfigured. It is persisted in the rebuild-time chunking
// fingerprint so that flipping a default here forces `--update` to fall
// back to a full rebuild instead of silently producing a mixed-
// generation snapshot with old records on the old default and new
// records on the new default.
//
//   - "1" (pre-#344): zero-config returned a nil policy, so both kinds
//     fell through to stroma's MarkdownPolicy default.
//   - "2" (#344): zero-config for docs now defaults to LateChunkPolicy
//     with the tuned DefaultDocLateChunk* knobs; spec defaults are
//     unchanged.
//
// Bump this string whenever Resolve's zero-config behavior changes so
// pre-existing snapshots fail the fingerprint check and get rebuilt.
const ResolverDefaultsVersion = "2"

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
// Resolve always returns a non-nil KindRouterPolicy. Its Default mirrors
// stroma's bounded nil-policy path (MarkdownPolicy with the
// DefaultMaxChunkSections cap in place) so unknown kinds keep the
// pre-#338 DoS guard.
//
// Per-kind resolution:
//
//   - Spec: keeps stroma's MarkdownPolicy default via router.Default when
//     unconfigured; explicit `[runtime.chunking.spec]` builds a concrete
//     policy and pins it in ByKind.
//   - Doc: when unconfigured, defaults to LateChunkPolicy with the
//     DefaultDocLateChunk* knobs (#344 product lever — long-form docs
//     get parent+leaf hierarchy feeding ExpandContext). Operators who
//     don't want the storage overhead opt back by setting
//     `[runtime.chunking.doc]` policy = "markdown"`, which overrides the
//     default via ByKind.
//
// The tuning knobs users set without an explicit Policy field still
// resolve through MarkdownPolicy (buildPolicy treats empty Policy as
// PolicyMarkdown), so "I only tuned max_tokens" keeps the DoS guard on
// without silently inheriting the late-chunk default.
func Resolve(cfg Config) (stchunk.Policy, error) {
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
	} else {
		router.ByKind[sdk.ArtifactKindDoc] = defaultDocPolicy()
	}
	return router, nil
}

// defaultDocPolicy returns the #344 LateChunkPolicy default for docs.
// Kept as a helper so callers and tests agree on the exact defaults
// without scattering magic numbers.
func defaultDocPolicy() stchunk.LateChunkPolicy {
	return stchunk.LateChunkPolicy{
		ParentMaxTokens:    DefaultDocLateChunkParentMaxTokens,
		ChildMaxTokens:     DefaultDocLateChunkChildMaxTokens,
		ChildOverlapTokens: DefaultDocLateChunkChildOverlapTokens,
		MaxSections:        stindex.DefaultMaxChunkSections,
	}
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
