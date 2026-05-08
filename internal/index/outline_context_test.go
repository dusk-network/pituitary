package index

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
	stindex "github.com/dusk-network/stroma/v3/index"
)

func TestRetrieveOutlineContextReturnsOutlineAndExpandedContext(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	result, err := RetrieveOutlineContext(cfg, OutlineContextQuery{
		Query:          "tenant-specific API rate-limit configuration",
		Refs:           []string{"doc://guides/api-rate-limits"},
		Kinds:          []string{model.ArtifactKindDoc},
		Limit:          1,
		IncludeParent:  true,
		NeighborWindow: 1,
	})
	if err != nil {
		t.Fatalf("RetrieveOutlineContext() error = %v", err)
	}
	if result.SnapshotFingerprint == "" {
		t.Fatal("SnapshotFingerprint is empty")
	}
	if len(result.Records) != 1 {
		t.Fatalf("records = %+v, want one record", result.Records)
	}
	record := result.Records[0]
	if record.Ref != "doc://guides/api-rate-limits" || record.Kind != model.ArtifactKindDoc {
		t.Fatalf("record = %+v, want API guide doc", record)
	}
	if len(record.Outline) == 0 {
		t.Fatalf("record.Outline is empty")
	}
	if len(record.Selections) != 1 {
		t.Fatalf("selections = %+v, want one selection", record.Selections)
	}
	selection := record.Selections[0]
	if selection.ChunkID == 0 || selection.Heading == "" || selection.SelectionSource != "deterministic" {
		t.Fatalf("selection = %+v, want deterministic selected chunk", selection)
	}
	if len(selection.Expanded) == 0 {
		t.Fatalf("selection.Expanded is empty")
	}
	var foundSelected bool
	for _, section := range selection.Expanded {
		if section.Role == "selected" && section.ChunkID == selection.ChunkID && section.Content != "" {
			foundSelected = true
		}
	}
	if !foundSelected {
		t.Fatalf("expanded = %+v, want selected section with content", selection.Expanded)
	}
}

// #341: outline-context retrieval is the second hybrid call site that
// already consumes runtime.search; PrefilterDimension must reach
// SearchRecords there too. A truncated value (<= stored embedder dim)
// has to keep returning results, and an oversized value has to be
// rejected by the local preflight before stroma embeds the query.
func TestRetrieveOutlineContextHonorsMatryoshkaPrefilterDimension(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	cfg.Runtime.Search.PrefilterDimension = 4 // fixture-8d stores 8 dims
	result, err := RetrieveOutlineContext(cfg, OutlineContextQuery{
		Query:          "tenant-specific API rate-limit configuration",
		Refs:           []string{"doc://guides/api-rate-limits"},
		Kinds:          []string{model.ArtifactKindDoc},
		Limit:          1,
		IncludeParent:  true,
		NeighborWindow: 1,
	})
	if err != nil {
		t.Fatalf("RetrieveOutlineContext(prefilter=4) error = %v", err)
	}
	if len(result.Records) == 0 {
		t.Fatal("RetrieveOutlineContext(prefilter=4) returned no records; expected truncated path to succeed")
	}
}

func TestRetrieveOutlineContextRejectsPrefilterDimensionAboveStored(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	cfg.Runtime.Search.PrefilterDimension = 9 // fixture-8d stores 8 dims
	_, err = RetrieveOutlineContext(cfg, OutlineContextQuery{
		Query:          "tenant-specific API rate-limit configuration",
		Refs:           []string{"doc://guides/api-rate-limits"},
		Kinds:          []string{model.ArtifactKindDoc},
		Limit:          1,
		IncludeParent:  true,
		NeighborWindow: 1,
	})
	if err == nil {
		t.Fatal("RetrieveOutlineContext(prefilter > stored) error = nil, want preflight rejection")
	}
	if !strings.Contains(err.Error(), "runtime.search.matryoshka_prefilter_dimension") {
		t.Fatalf("RetrieveOutlineContext() error = %q, want preflight to name the config key", err.Error())
	}
	if !strings.Contains(err.Error(), "exceeds stored embedder_dimension") {
		t.Fatalf("RetrieveOutlineContext() error = %q, want preflight to name the stored dimension", err.Error())
	}
}

func TestRetrieveOutlineContextFallsBackWhenSelectorFails(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	result, err := RetrieveOutlineContext(cfg, OutlineContextQuery{
		Query: "rate limiting",
		Refs:  []string{"SPEC-042"},
		Kinds: []string{model.ArtifactKindSpec},
		Limit: 1,
		Selector: func(context.Context, OutlineContextSelectionInput) ([]int64, error) {
			return nil, errors.New("selector unavailable")
		},
	})
	if err != nil {
		t.Fatalf("RetrieveOutlineContext() error = %v", err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "deterministic selection used") {
		t.Fatalf("warnings = %v, want deterministic fallback warning", result.Warnings)
	}
	if len(result.Records) != 1 || len(result.Records[0].Selections) != 1 {
		t.Fatalf("records = %+v, want one fallback selection", result.Records)
	}
	if got, want := result.Records[0].Selections[0].SelectionSource, "deterministic_fallback"; got != want {
		t.Fatalf("SelectionSource = %q, want %q", got, want)
	}
}

func TestRetrieveOutlineContextRejectsUnboundedInputs(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	_, err := RetrieveOutlineContext(cfg, OutlineContextQuery{
		Query: "rate limiting",
		Limit: maxOutlineContextLimit + 1,
	})
	if err == nil || !strings.Contains(err.Error(), "limit must be less than or equal") {
		t.Fatalf("RetrieveOutlineContext() error = %v, want limit guard", err)
	}
}

func TestTruncateOutlineContentPreservesUTF8(t *testing.T) {
	t.Parallel()

	content, truncated := truncateOutlineContent("alpha café 東京 omega", 11)
	if !truncated {
		t.Fatal("truncateOutlineContent() truncated = false, want true")
	}
	if !utf8.ValidString(content) {
		t.Fatalf("truncateOutlineContent() returned invalid UTF-8: %q", content)
	}
	if strings.ContainsRune(content, utf8.RuneError) {
		t.Fatalf("truncateOutlineContent() content = %q, want no replacement rune", content)
	}

	content, truncated = truncateOutlineContent("🙂 wide", 1)
	if !truncated || content != "..." || !utf8.ValidString(content) {
		t.Fatalf("truncateOutlineContent() = (%q, %v), want UTF-8 ellipsis-only truncation", content, truncated)
	}
}

func TestSectionFromStromaOmitsUnknownDepth(t *testing.T) {
	t.Parallel()

	section := sectionFromStroma(stindex.Section{ChunkID: 42, Heading: "Leaf", Content: "body"}, nil, OutlineContextOutlineRow{}, 42, 100)
	if section.Depth != nil {
		t.Fatalf("Depth = %v, want nil for section missing from outline", *section.Depth)
	}

	depth := 2
	section = sectionFromStroma(stindex.Section{ChunkID: 42, Heading: "Leaf", Content: "body"}, map[int64]OutlineContextOutlineRow{
		42: {ChunkID: 42, Depth: depth},
	}, OutlineContextOutlineRow{}, 42, 100)
	if section.Depth == nil || *section.Depth != depth {
		t.Fatalf("Depth = %v, want %d", section.Depth, depth)
	}
}
