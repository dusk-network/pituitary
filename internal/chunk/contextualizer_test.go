package chunk

import (
	"context"
	"strings"
	"testing"

	stchunk "github.com/dusk-network/stroma/v2/chunk"
	stcorpus "github.com/dusk-network/stroma/v2/corpus"
	stindex "github.com/dusk-network/stroma/v2/index"
)

func TestBuildPrefix_TitleAncestry(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		record stcorpus.Record
		sec    stchunk.Section
		want   string
	}{
		{
			name:   "full ancestry from stroma joined heading",
			record: stcorpus.Record{Title: "SPEC-043: Rate limiting"},
			sec:    stchunk.Section{Heading: "Policy / Rate limits"},
			want:   "SPEC-043: Rate limiting > Policy / Rate limits",
		},
		{
			name:   "single-level heading",
			record: stcorpus.Record{Title: "SPEC-043"},
			sec:    stchunk.Section{Heading: "Overview"},
			want:   "SPEC-043 > Overview",
		},
		{
			name:   "heading equals title collapses to title",
			record: stcorpus.Record{Title: "SPEC-043"},
			sec:    stchunk.Section{Heading: "SPEC-043"},
			want:   "SPEC-043",
		},
		{
			name:   "empty heading yields title alone",
			record: stcorpus.Record{Title: "SPEC-043"},
			sec:    stchunk.Section{},
			want:   "SPEC-043",
		},
		{
			name:   "empty title keeps heading verbatim",
			record: stcorpus.Record{},
			sec:    stchunk.Section{Heading: "Top / Sub"},
			want:   "Top / Sub",
		},
		{
			name:   "whitespace inputs collapse to empty",
			record: stcorpus.Record{Title: "  "},
			sec:    stchunk.Section{Heading: "  "},
			want:   "",
		},
		{
			// Regression: a single-section heading whose literal text
			// contains the " / " separator sequence must not be
			// reparsed as multi-level ancestry. BuildPrefix passes the
			// heading opaquely, so the output preserves the original
			// text exactly.
			name:   "heading containing literal slash is preserved",
			record: stcorpus.Record{Title: "SPEC-043"},
			sec:    stchunk.Section{Heading: "Latency / Throughput trade-offs"},
			want:   "SPEC-043 > Latency / Throughput trade-offs",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := BuildPrefix(PrefixFormatTitleAncestry, tc.record, tc.sec)
			if got != tc.want {
				t.Fatalf("BuildPrefix() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildPrefix_RefTitle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		record stcorpus.Record
		want   string
	}{
		{"ref and title", stcorpus.Record{Ref: "spec:rate-limiting", Title: "Rate limiting"}, "[spec:rate-limiting] Rate limiting"},
		{"title only", stcorpus.Record{Title: "Rate limiting"}, "Rate limiting"},
		{"ref only", stcorpus.Record{Ref: "spec:rate-limiting"}, "[spec:rate-limiting]"},
		{"both empty", stcorpus.Record{}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := BuildPrefix(PrefixFormatRefTitle, tc.record, stchunk.Section{Heading: "ignored"})
			if got != tc.want {
				t.Fatalf("BuildPrefix() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildPrefix_UnknownFormatReturnsEmpty(t *testing.T) {
	t.Parallel()

	got := BuildPrefix("semantic", stcorpus.Record{Title: "X"}, stchunk.Section{Heading: "Y"})
	if got != "" {
		t.Fatalf("BuildPrefix(unknown) = %q, want empty", got)
	}
}

func TestBuildPrefix_Deterministic(t *testing.T) {
	t.Parallel()

	// Reuse-cache invalidation in stroma pivots on stable prefix output
	// across rebuilds; prove determinism here so a future non-stable
	// change gets caught at the unit layer rather than silently
	// triggering full rebuilds in prod.
	record := stcorpus.Record{Ref: "spec:rate-limiting", Title: "SPEC-043: Rate limiting"}
	section := stchunk.Section{Heading: "Policy / Rate limits"}

	for _, format := range []PrefixFormat{
		PrefixFormatTitleAncestry, PrefixFormatRefTitle,
	} {
		first := BuildPrefix(format, record, section)
		for i := 0; i < 32; i++ {
			if got := BuildPrefix(format, record, section); got != first {
				t.Fatalf("BuildPrefix(%q) iteration %d = %q, want stable %q",
					format, i, got, first)
			}
		}
	}
}

func TestResolveContextualizer_ZeroConfigReturnsNil(t *testing.T) {
	t.Parallel()

	c, err := ResolveContextualizer(ContextualizerConfig{})
	if err != nil {
		t.Fatalf("ResolveContextualizer(zero): %v", err)
	}
	if c != nil {
		t.Fatalf("ResolveContextualizer(zero) = %T, want nil (preserves pre-#343 rebuild contract)", c)
	}
}

func TestResolveContextualizer_KnownFormat(t *testing.T) {
	t.Parallel()

	c, err := ResolveContextualizer(ContextualizerConfig{Format: PrefixFormatTitleAncestry})
	if err != nil {
		t.Fatalf("ResolveContextualizer: %v", err)
	}
	if c == nil {
		t.Fatal("expected a non-nil contextualizer")
	}
}

func TestResolveContextualizer_UnknownFormat(t *testing.T) {
	t.Parallel()

	_, err := ResolveContextualizer(ContextualizerConfig{Format: "semantic"})
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
	if !strings.Contains(err.Error(), "unknown contextualizer format") {
		t.Fatalf("error should mention unknown format; got: %v", err)
	}
}

func TestContextualizer_ContextualizeChunksLenMatchesSections(t *testing.T) {
	t.Parallel()

	c, err := NewContextualizer(PrefixFormatTitleAncestry)
	if err != nil {
		t.Fatalf("NewContextualizer: %v", err)
	}
	record := stcorpus.Record{Title: "SPEC-043: Rate limiting"}
	sections := []stchunk.Section{
		{Heading: "Policy / Rate limits"},
		{Heading: "Policy / Burst"},
		{Heading: ""},
	}
	prefixes, err := c.ContextualizeChunks(context.Background(), record, sections)
	if err != nil {
		t.Fatalf("ContextualizeChunks: %v", err)
	}
	if got, want := len(prefixes), len(sections); got != want {
		t.Fatalf("len(prefixes) = %d, want %d (stroma rejects mismatched lengths)", got, want)
	}
	if !strings.Contains(prefixes[0], "Rate limits") {
		t.Fatalf("prefix[0] = %q, expected ancestry leaf", prefixes[0])
	}
}

func TestContextualizer_EmptySectionsReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	c, err := NewContextualizer(PrefixFormatTitleAncestry)
	if err != nil {
		t.Fatalf("NewContextualizer: %v", err)
	}
	prefixes, err := c.ContextualizeChunks(context.Background(), stcorpus.Record{Title: "X"}, nil)
	if err != nil {
		t.Fatalf("ContextualizeChunks: %v", err)
	}
	if len(prefixes) != 0 {
		t.Fatalf("len(prefixes) = %d, want 0 for empty sections", len(prefixes))
	}
}

func TestContextualizer_SatisfiesStromaInterface(t *testing.T) {
	t.Parallel()

	c, err := NewContextualizer(PrefixFormatRefTitle)
	if err != nil {
		t.Fatalf("NewContextualizer: %v", err)
	}
	var iface stindex.ChunkContextualizer = c
	_ = iface
}

func TestContextualizerConfig_IsZero(t *testing.T) {
	t.Parallel()

	if !(ContextualizerConfig{}).IsZero() {
		t.Fatal("zero ContextualizerConfig should be IsZero")
	}
	if (ContextualizerConfig{Format: PrefixFormatRefTitle}).IsZero() {
		t.Fatal("populated ContextualizerConfig should not be IsZero")
	}
}
