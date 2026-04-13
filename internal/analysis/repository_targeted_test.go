package analysis

import (
	"context"
	"testing"

	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestLoadIndexedSpecsContextSelectedRefsLoadsOnlyRequestedSpec(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	db, err := index.OpenReadOnlyContext(context.Background(), cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("index.OpenReadOnlyContext() error = %v", err)
	}
	defer db.Close()

	snapshot, err := index.OpenStromaSnapshotContext(context.Background(), db, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("index.OpenStromaSnapshotContext() error = %v", err)
	}
	defer snapshot.Close()

	specs, err := loadIndexedSpecsContext(context.Background(), db, snapshot, []string{"SPEC-042"})
	if err != nil {
		t.Fatalf("loadIndexedSpecsContext() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("loadIndexedSpecsContext() returned %d specs, want 1", len(specs))
	}

	spec, ok := specs["SPEC-042"]
	if !ok {
		t.Fatalf("loaded specs = %+v, want SPEC-042", specs)
	}
	if len(spec.Record.AppliesTo) == 0 {
		t.Fatalf("SPEC-042 applies_to = %v, want hydrated edges", spec.Record.AppliesTo)
	}
	if len(spec.Sections) == 0 {
		t.Fatalf("SPEC-042 sections = %v, want hydrated sections", spec.Sections)
	}
	if _, ok := specs["SPEC-055"]; ok {
		t.Fatalf("loaded specs = %+v, did not expect unrelated SPEC-055", specs)
	}
}

func TestLoadIndexedDocsContextSelectedRefsLoadsOnlyRequestedDoc(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	db, err := index.OpenReadOnlyContext(context.Background(), cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("index.OpenReadOnlyContext() error = %v", err)
	}
	defer db.Close()

	snapshot, err := index.OpenStromaSnapshotContext(context.Background(), db, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("index.OpenStromaSnapshotContext() error = %v", err)
	}
	defer snapshot.Close()

	docs, err := loadIndexedDocsContext(context.Background(), db, snapshot, []string{"doc://guides/api-rate-limits"})
	if err != nil {
		t.Fatalf("loadIndexedDocsContext() error = %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("loadIndexedDocsContext() returned %d docs, want 1", len(docs))
	}

	doc, ok := docs["doc://guides/api-rate-limits"]
	if !ok {
		t.Fatalf("loaded docs = %+v, want guides/api-rate-limits", docs)
	}
	if len(doc.Sections) == 0 {
		t.Fatalf("doc sections = %v, want hydrated sections", doc.Sections)
	}
	if _, ok := docs["doc://runbooks/rate-limit-rollout"]; ok {
		t.Fatalf("loaded docs = %+v, did not expect unrelated runbook", docs)
	}
}
