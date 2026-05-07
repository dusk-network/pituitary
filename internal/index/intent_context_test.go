package index

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
	stindex "github.com/dusk-network/stroma/v3/index"
)

func TestGetIntentOutlineReturnsBoundedOutline(t *testing.T) {
	t.Parallel()

	cfg := rebuildIntentFixtureIndex(t)
	result, err := GetIntentOutline(cfg, IntentOutlineRequest{
		Ref:            "doc://guides/api-rate-limits",
		Kind:           model.ArtifactKindDoc,
		MaxOutlineRows: 1,
	})
	if err != nil {
		t.Fatalf("GetIntentOutline() error = %v", err)
	}
	if result.SnapshotFingerprint == "" {
		t.Fatal("SnapshotFingerprint is empty")
	}
	if result.Record.Ref != "doc://guides/api-rate-limits" || result.Record.Kind != model.ArtifactKindDoc {
		t.Fatalf("record = %+v, want API guide doc", result.Record)
	}
	if result.Record.SourceRef == "" {
		t.Fatal("record source_ref is empty")
	}
	if len(result.Outline) != 1 {
		t.Fatalf("outline length = %d, want bounded single row", len(result.Outline))
	}
	if result.Outline[0].ChunkID == 0 || result.Outline[0].Heading == "" {
		t.Fatalf("outline row = %+v, want chunk id and heading", result.Outline[0])
	}
	if !result.OutlineTruncated {
		t.Fatal("OutlineTruncated = false, want true for one-row cap")
	}
}

func TestGetIntentOutlineAppliesActiveStatusDefaults(t *testing.T) {
	t.Parallel()

	cfg := rebuildIntentFixtureIndex(t)
	_, err := GetIntentOutline(cfg, IntentOutlineRequest{
		Ref:  "SPEC-008",
		Kind: model.ArtifactKindSpec,
	})
	if err == nil {
		t.Fatal("GetIntentOutline() error = nil, want superseded spec filtered by active defaults")
	}
	var filtered *RecordFilteredError
	if !errors.As(err, &filtered) {
		t.Fatalf("GetIntentOutline() error = %T (%v), want RecordFilteredError", err, err)
	}

	result, err := GetIntentOutline(cfg, IntentOutlineRequest{
		Ref:  "SPEC-008",
		Kind: model.ArtifactKindSpec,
		Filters: SearchSpecFilters{
			Statuses: []string{model.StatusSuperseded},
		},
	})
	if err != nil {
		t.Fatalf("GetIntentOutline(historical) error = %v", err)
	}
	if result.Status != model.StatusSuperseded {
		t.Fatalf("Status = %q, want superseded", result.Status)
	}
}

func TestExpandIntentContextReturnsSelectedSection(t *testing.T) {
	t.Parallel()

	cfg := rebuildIntentFixtureIndex(t)
	outline, err := GetIntentOutline(cfg, IntentOutlineRequest{
		Ref:  "SPEC-042",
		Kind: model.ArtifactKindSpec,
	})
	if err != nil {
		t.Fatalf("GetIntentOutline() error = %v", err)
	}
	if len(outline.Outline) == 0 {
		t.Fatal("outline is empty")
	}
	chunkID := outline.Outline[0].ChunkID

	result, err := ExpandIntentContext(cfg, ExpandIntentContextRequest{
		ChunkID:             chunkID,
		SnapshotFingerprint: outline.SnapshotFingerprint,
		IncludeParent:       true,
		NeighborWindow:      1,
		MaxSectionBytes:     80,
	})
	if err != nil {
		t.Fatalf("ExpandIntentContext() error = %v", err)
	}
	if result.SnapshotFingerprint != outline.SnapshotFingerprint {
		t.Fatalf("SnapshotFingerprint = %q, want %q", result.SnapshotFingerprint, outline.SnapshotFingerprint)
	}
	if len(result.Sections) == 0 {
		t.Fatal("sections is empty")
	}
	var foundSelected bool
	for _, section := range result.Sections {
		if section.ChunkID == 0 || section.Ref == "" || section.Kind == "" || section.SourceRef == "" {
			t.Fatalf("section missing provenance: %+v", section)
		}
		if section.Role == "selected" && section.ChunkID == chunkID {
			foundSelected = section.Content != "" && section.Heading != ""
		}
		if len(section.Content) > 83 {
			t.Fatalf("section content length = %d, want bounded content: %q", len(section.Content), section.Content)
		}
	}
	if !foundSelected {
		t.Fatalf("sections = %+v, want selected section with content", result.Sections)
	}
}

func TestExpandIntentContextIncludeParentReturnsParentSection(t *testing.T) {
	t.Parallel()

	repoDir := t.TempDir()
	indexPath := filepath.Join(t.TempDir(), "pituitary.db")
	configPath := filepath.Join(t.TempDir(), "pituitary.toml")
	body := "# Long Guide\n\n" + strings.Repeat("parent lineage context marker ", 260) + "\n"
	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "long.md"), body)
	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(repoDir)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[runtime.chunking.doc]
policy = "late_chunk"
max_tokens = 512
child_max_tokens = 48
child_overlap_tokens = 8

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	outline, err := GetIntentOutline(cfg, IntentOutlineRequest{
		Ref:  "doc://guides/long",
		Kind: model.ArtifactKindDoc,
	})
	if err != nil {
		t.Fatalf("GetIntentOutline() error = %v", err)
	}
	leafID, parentID, ok := firstOutlineLeafWithParent(outline.Outline)
	if !ok {
		t.Fatalf("outline = %+v, want at least one leaf with parent_chunk_id", outline.Outline)
	}

	withoutParent, err := ExpandIntentContext(cfg, ExpandIntentContextRequest{
		ChunkID:             leafID,
		SnapshotFingerprint: outline.SnapshotFingerprint,
	})
	if err != nil {
		t.Fatalf("ExpandIntentContext(include_parent=false) error = %v", err)
	}
	if containsSectionRole(withoutParent.Sections, parentID, "parent") {
		t.Fatalf("sections = %+v, want no parent section when include_parent=false", withoutParent.Sections)
	}

	withParent, err := ExpandIntentContext(cfg, ExpandIntentContextRequest{
		ChunkID:             leafID,
		SnapshotFingerprint: outline.SnapshotFingerprint,
		IncludeParent:       true,
	})
	if err != nil {
		t.Fatalf("ExpandIntentContext(include_parent=true) error = %v", err)
	}
	if !containsSectionRole(withParent.Sections, parentID, "parent") {
		t.Fatalf("sections = %+v, want parent chunk %d returned with role parent", withParent.Sections, parentID)
	}
}

func TestExpandIntentContextDistinguishesStaleAndMissingHandles(t *testing.T) {
	t.Parallel()

	cfg := rebuildIntentFixtureIndex(t)
	outline, err := GetIntentOutline(cfg, IntentOutlineRequest{
		Ref:  "SPEC-042",
		Kind: model.ArtifactKindSpec,
	})
	if err != nil {
		t.Fatalf("GetIntentOutline() error = %v", err)
	}
	_, err = ExpandIntentContext(cfg, ExpandIntentContextRequest{
		ChunkID:             outline.Outline[0].ChunkID,
		SnapshotFingerprint: "old-snapshot",
	})
	if !IsStaleSnapshot(err) {
		t.Fatalf("ExpandIntentContext(stale snapshot) error = %T (%v), want StaleSnapshotError", err, err)
	}

	_, err = ExpandIntentContext(cfg, ExpandIntentContextRequest{
		ChunkID:             99999999,
		SnapshotFingerprint: outline.SnapshotFingerprint,
	})
	if !IsMissingChunk(err) || !strings.Contains(err.Error(), "handle may be stale") {
		t.Fatalf("ExpandIntentContext(missing chunk) error = %T (%v), want MissingChunkError", err, err)
	}
}

func firstOutlineLeafWithParent(rows []OutlineContextOutlineRow) (int64, int64, bool) {
	for _, row := range rows {
		if row.ParentChunkID != nil {
			return row.ChunkID, *row.ParentChunkID, true
		}
	}
	return 0, 0, false
}

func containsSectionRole(sections []OutlineContextSection, chunkID int64, role string) bool {
	for _, section := range sections {
		if section.ChunkID == chunkID && section.Role == role {
			return true
		}
	}
	return false
}

func TestExpandIntentContextRequiresSnapshotFingerprint(t *testing.T) {
	t.Parallel()

	cfg := rebuildIntentFixtureIndex(t)
	_, err := ExpandIntentContext(cfg, ExpandIntentContextRequest{ChunkID: 1})
	if err == nil || !strings.Contains(err.Error(), "snapshot_fingerprint is required") {
		t.Fatalf("ExpandIntentContext() error = %v, want snapshot fingerprint requirement", err)
	}
}

func TestValidateExpandedIntentShapeRejectsCrossRecordSections(t *testing.T) {
	t.Parallel()

	_, err := validateExpandedIntentShape([]stindex.Section{
		{ChunkID: 10, Ref: "SPEC-042", Kind: model.ArtifactKindSpec},
		{ChunkID: 11, Ref: "SPEC-008", Kind: model.ArtifactKindSpec},
	}, 10)
	if !IsIntentExpansionInvariant(err) {
		t.Fatalf("validateExpandedIntentShape() error = %T (%v), want invariant error", err, err)
	}

	_, err = validateExpandedIntentShape([]stindex.Section{
		{ChunkID: 10, Ref: "shared-ref", Kind: model.ArtifactKindSpec},
		{ChunkID: 11, Ref: "shared-ref", Kind: model.ArtifactKindDoc},
	}, 10)
	if !IsIntentExpansionInvariant(err) {
		t.Fatalf("validateExpandedIntentShape(kind mismatch) error = %T (%v), want invariant error", err, err)
	}
}

func rebuildIntentFixtureIndex(t *testing.T) *config.Config {
	t.Helper()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	return cfg
}
