package index

import (
	"testing"

	"github.com/dusk-network/pituitary/internal/source"
)

func TestReadStatusReportsMissingIndex(t *testing.T) {
	t.Parallel()

	path := t.TempDir() + "/pituitary.db"
	status, err := ReadStatus(path)
	if err != nil {
		t.Fatalf("ReadStatus() error = %v", err)
	}
	if status.IndexPath != path {
		t.Fatalf("index path = %q, want %q", status.IndexPath, path)
	}
	if status.Exists {
		t.Fatal("status.Exists = true, want false")
	}
	if status.SpecCount != 0 || status.DocCount != 0 || status.ChunkCount != 0 {
		t.Fatalf("counts = %+v, want zero counts for missing index", status)
	}
}

func TestReadStatusReadsFixtureCounts(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	status, err := ReadStatus(cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("ReadStatus() error = %v", err)
	}
	if !status.Exists {
		t.Fatal("status.Exists = false, want true")
	}
	if status.SpecCount != 3 || status.DocCount != 2 || status.ChunkCount != 17 {
		t.Fatalf("status = %+v, want 3 specs, 2 docs, 17 chunks", status)
	}
}
