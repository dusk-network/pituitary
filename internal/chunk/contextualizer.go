package chunk

import (
	"context"
	"fmt"
	"strings"

	stchunk "github.com/dusk-network/stroma/v2/chunk"
	stcorpus "github.com/dusk-network/stroma/v2/corpus"
	stindex "github.com/dusk-network/stroma/v2/index"
)

// PrefixFormat selects the layout of the per-chunk context prefix.
//
// Heading content is always passed through verbatim. Stroma's
// chunk.Section.Heading is a " / "-joined display string produced by
// stroma/chunk.joinHeadings; reparsing it on this side would be wrong
// whenever a heading's literal text contains " / " (e.g. "Latency /
// Throughput trade-offs"). We treat the heading as opaque and only
// compose it with record-level identifiers.
type PrefixFormat string

const (
	// PrefixFormatTitleAncestry is "<Title> > <Heading>" with the
	// heading string verbatim from stroma. Strongest signal for
	// cross-spec heading collisions (e.g. "Rate limits" in SPEC-043
	// vs SPEC-018).
	PrefixFormatTitleAncestry PrefixFormat = "title_ancestry"

	// PrefixFormatRefTitle is "[<Ref>] <Title>". Machine-identifier-
	// forward variant; stable across Title edits. Does not include
	// heading, which trades heading-collision disambiguation for
	// Ref-stability.
	PrefixFormatRefTitle PrefixFormat = "ref_title"
)

// ContextualizerConfig is the chunk-package-facing config for the
// contextualizer. A zero value means "no contextualizer" — the rebuild
// path stays byte-identical to the pre-#343 contract (no prefix, no
// chunks.context_prefix column writes, no reuse-cache invalidation).
type ContextualizerConfig struct {
	// Format selects the prefix layout. Empty means disabled.
	Format PrefixFormat
}

// IsZero reports whether no contextualizer is configured.
func (c ContextualizerConfig) IsZero() bool {
	return c.Format == ""
}

// BuildPrefix is the pure per-section prefix function. Deterministic
// and I/O-free: stroma persists the prefix in chunks.context_prefix
// and pivots reuse-cache invalidation on its bytes, so any non-
// determinism here would silently force full rebuilds.
//
// Returns "" when the inputs yield no meaningful signal; stroma's
// runContextualizer normalizes whitespace-only entries to "" upstream
// so "no prefix" flows uniformly to embedding, FTS, and the reuse key.
//
// The heading string is never reparsed — it is passed through opaquely
// from stroma. This keeps the contract orthogonal to stroma's current
// heading display format and avoids silently corrupting prefixes when
// a heading's literal text contains the " / " separator sequence.
func BuildPrefix(format PrefixFormat, record stcorpus.Record, section stchunk.Section) string {
	title := strings.TrimSpace(record.Title)
	heading := strings.TrimSpace(section.Heading)
	ref := strings.TrimSpace(record.Ref)

	switch format {
	case PrefixFormatTitleAncestry:
		parts := make([]string, 0, 2)
		if title != "" {
			parts = append(parts, title)
		}
		if heading != "" && heading != title {
			parts = append(parts, heading)
		}
		return strings.Join(parts, " > ")

	case PrefixFormatRefTitle:
		switch {
		case ref != "" && title != "":
			return "[" + ref + "] " + title
		case title != "":
			return title
		case ref != "":
			return "[" + ref + "]"
		default:
			return ""
		}

	default:
		return ""
	}
}

// Contextualizer is pituitary's stindex.ChunkContextualizer adapter.
// It is a thin wrapper around BuildPrefix: all policy lives in the
// pure function so the adapter stays trivially correct w.r.t. stroma's
// "return exactly len(sections) prefixes" contract.
type Contextualizer struct {
	format PrefixFormat
}

// NewContextualizer returns a Contextualizer for the given format.
// Unknown formats are rejected at construction time so misconfiguration
// surfaces before the rebuild begins, not as a silent empty-prefix run.
func NewContextualizer(format PrefixFormat) (*Contextualizer, error) {
	if !knownPrefixFormat(format) {
		return nil, fmt.Errorf("unknown contextualizer format %q (expected %q or %q)",
			format, PrefixFormatTitleAncestry, PrefixFormatRefTitle)
	}
	return &Contextualizer{format: format}, nil
}

// ContextualizeChunks satisfies stindex.ChunkContextualizer.
func (c *Contextualizer) ContextualizeChunks(
	_ context.Context, record stcorpus.Record, sections []stchunk.Section,
) ([]string, error) {
	out := make([]string, len(sections))
	for i, s := range sections {
		out[i] = BuildPrefix(c.format, record, s)
	}
	return out, nil
}

// ResolveContextualizer is the sibling of Resolve: it builds the stroma
// ChunkContextualizer from pituitary's ContextualizerConfig. A zero
// config returns (nil, nil) so the pre-#343 rebuild contract stays
// intact until users opt in.
func ResolveContextualizer(cfg ContextualizerConfig) (stindex.ChunkContextualizer, error) {
	if cfg.IsZero() {
		return nil, nil
	}
	return NewContextualizer(cfg.Format)
}

func knownPrefixFormat(f PrefixFormat) bool {
	switch f {
	case PrefixFormatTitleAncestry, PrefixFormatRefTitle:
		return true
	default:
		return false
	}
}

// Compile-time interface check.
var _ stindex.ChunkContextualizer = (*Contextualizer)(nil)
