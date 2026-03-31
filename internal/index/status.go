package index

import (
	"context"
	"fmt"
	"os"

	"github.com/dusk-network/pituitary/internal/model"
)

// Status reports whether the configured index exists and its basic counts.
type Status struct {
	IndexPath  string         `json:"index_path"`
	Exists     bool           `json:"index_exists"`
	SpecCount  int            `json:"spec_count"`
	DocCount   int            `json:"doc_count"`
	ChunkCount int            `json:"chunk_count"`
	Repos      []RepoCoverage `json:"repo_coverage,omitempty"`
}

// ReadStatus inspects the configured index path and returns basic counts.
func ReadStatus(path string) (*Status, error) {
	return ReadStatusContext(context.Background(), path)
}

// ReadStatusContext inspects the configured index path and returns basic counts.
func ReadStatusContext(ctx context.Context, path string) (*Status, error) {
	status := &Status{IndexPath: path}

	info, err := os.Stat(path)
	switch {
	case os.IsNotExist(err):
		return status, nil
	case err != nil:
		return nil, fmt.Errorf("stat index %s: %w", path, err)
	case info.IsDir():
		return nil, fmt.Errorf("index path %s is a directory", path)
	}

	db, err := OpenReadOnlyContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open index %s: %w", path, err)
	}
	defer db.Close()

	status.Exists = true
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifacts WHERE kind = ?`, model.ArtifactKindSpec).Scan(&status.SpecCount); err != nil {
		return nil, fmt.Errorf("count indexed specs: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifacts WHERE kind = ?`, model.ArtifactKindDoc).Scan(&status.DocCount); err != nil {
		return nil, fmt.Errorf("count indexed docs: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chunks`).Scan(&status.ChunkCount); err != nil {
		return nil, fmt.Errorf("count indexed chunks: %w", err)
	}
	status.Repos, err = repoCoverageFromDBContext(ctx, db)
	if err != nil {
		return nil, err
	}

	return status, nil
}
