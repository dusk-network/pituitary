package analysis

import (
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
)

func TestIsSemanticEmbedderConfigured(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		provider string
		want     bool
	}{
		{"empty provider", "", false},
		{"fixture", config.RuntimeProviderFixture, false},
		{"openai_compatible", config.RuntimeProviderOpenAI, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &config.Config{
				Runtime: config.Runtime{
					Embedder: config.RuntimeProvider{
						Provider: tc.provider,
					},
				},
			}
			got := isSemanticEmbedderConfigured(cfg)
			if got != tc.want {
				t.Errorf("isSemanticEmbedderConfigured(%q) = %v, want %v", tc.provider, got, tc.want)
			}
		})
	}

	t.Run("nil config", func(t *testing.T) {
		t.Parallel()
		if isSemanticEmbedderConfigured(nil) {
			t.Error("expected false for nil config")
		}
	})
}

func TestDeduplicateSemanticMatches(t *testing.T) {
	t.Parallel()

	matches := []semanticTerminologyMatch{
		{Term: "agent", ArtifactRef: "doc-1", Section: "Overview", Similarity: 0.7},
		{Term: "agent", ArtifactRef: "doc-1", Section: "Overview", Similarity: 0.8},
		{Term: "agent", ArtifactRef: "doc-2", Section: "Setup", Similarity: 0.6},
		{Term: "worker", ArtifactRef: "doc-1", Section: "Overview", Similarity: 0.5},
	}

	result := deduplicateSemanticMatches(matches)
	if len(result) != 3 {
		t.Fatalf("got %d matches, want 3", len(result))
	}

	// The doc-1/Overview/agent entry should keep the higher similarity.
	for _, m := range result {
		if m.ArtifactRef == "doc-1" && m.Section == "Overview" && m.Term == "agent" {
			if m.Similarity != 0.8 {
				t.Errorf("kept similarity = %f, want 0.8", m.Similarity)
			}
		}
	}
}

func TestConvertSemanticMatchesToFindings(t *testing.T) {
	t.Parallel()

	matches := []semanticTerminologyMatch{
		{
			Term:        "openclaw",
			Preferred:   "pituitary",
			ArtifactRef: "docs/guide",
			Kind:        "doc",
			Title:       "Getting Started",
			SourceRef:   "docs/guide.md",
			Section:     "Overview",
			Excerpt:     "the spec management pipeline handles...",
			Similarity:  0.72,
		},
		{
			Term:        "openclaw",
			Preferred:   "pituitary",
			ArtifactRef: "docs/guide",
			Kind:        "doc",
			Title:       "Getting Started",
			SourceRef:   "docs/guide.md",
			Section:     "Architecture",
			Excerpt:     "the governance layer enforces...",
			Similarity:  0.65,
		},
	}

	governed := map[string]terminologyGovernedTerm{
		"openclaw": {
			Term:           "openclaw",
			PreferredTerm:  "pituitary",
			Classification: terminologyClassificationHistoricalAlias,
		},
	}

	findings := convertSemanticMatchesToFindings(matches, governed)
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1 (grouped by artifact)", len(findings))
	}

	finding := findings[0]
	if finding.Ref != "docs/guide" {
		t.Errorf("ref = %q, want docs/guide", finding.Ref)
	}
	if len(finding.Sections) != 2 {
		t.Errorf("got %d sections, want 2", len(finding.Sections))
	}
	if finding.Score != 0.72 {
		t.Errorf("score = %f, want 0.72", finding.Score)
	}

	// Check provenance on matches.
	for _, section := range finding.Sections {
		for _, match := range section.Matches {
			if match.Provenance != ProvenanceEmbeddingSimilarity {
				t.Errorf("provenance = %q, want %q", match.Provenance, ProvenanceEmbeddingSimilarity)
			}
			if match.Classification != terminologyClassificationHistoricalAlias {
				t.Errorf("classification = %q, want %q", match.Classification, terminologyClassificationHistoricalAlias)
			}
		}
	}
}

func TestTerminologyChunkHasLiteralMatch(t *testing.T) {
	t.Parallel()

	matchers := compileTerminologyMatchers([]string{"openclaw", "old name"})

	cases := []struct {
		content string
		want    bool
	}{
		{"the openclaw system handles specs", true},
		{"Old Name is no longer used", true},
		{"the spec management system handles governance", false},
		{"", false},
	}

	for _, tc := range cases {
		got := terminologyChunkHasLiteralMatch(tc.content, matchers)
		if got != tc.want {
			t.Errorf("terminologyChunkHasLiteralMatch(%q) = %v, want %v", tc.content, got, tc.want)
		}
	}
}

func TestExtractSemanticExcerpt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		content string
		want    string
	}{
		{"first line\nsecond line", "first line"},
		{"- bullet point\nnext", "bullet point"},
		{"", ""},
		{"\n\n  ", ""},
	}

	for _, tc := range cases {
		got := extractSemanticExcerpt(tc.content)
		if got != tc.want {
			t.Errorf("extractSemanticExcerpt(%q) = %q, want %q", tc.content, got, tc.want)
		}
	}
}
