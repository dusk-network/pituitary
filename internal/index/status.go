package index

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/dusk-network/pituitary/internal/model"
)

// Status reports whether the configured index exists and its basic counts.
type Status struct {
	IndexPath          string              `json:"index_path"`
	Exists             bool                `json:"index_exists"`
	SpecCount          int                 `json:"spec_count"`
	DocCount           int                 `json:"doc_count"`
	ChunkCount         int                 `json:"chunk_count"`
	Repos              []RepoCoverage      `json:"repo_coverage,omitempty"`
	GovernanceCoverage *GovernanceCoverage `json:"governance_coverage,omitempty"`
}

// GovernanceCoverage reports the percentage of indexed source files that have
// at least one governance link (manual or inferred applies_to edge).
type GovernanceCoverage struct {
	TotalFiles    int     `json:"total_files"`
	GovernedFiles int     `json:"governed_files"`
	Percentage    float64 `json:"percentage"`
	ManualEdges   int     `json:"manual_edges"`
	InferredEdges int     `json:"inferred_edges"`
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

	// Governance coverage (schema v4+).
	if hasEdgeSourceColumn(ctx, db) {
		coverage, coverageErr := queryGovernanceCoverageContext(ctx, db)
		if coverageErr == nil {
			status.GovernanceCoverage = coverage
		}
	}

	return status, nil
}

func queryGovernanceCoverageContext(ctx context.Context, db *sql.DB) (*GovernanceCoverage, error) {
	var coverage GovernanceCoverage

	// Count distinct governed code file paths (scoped to code:// refs to avoid
	// inflating coverage with config:// refs that aren't in ast_cache).
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT to_ref) FROM edges WHERE edge_type = 'applies_to' AND to_ref LIKE 'code://%'`).
		Scan(&coverage.GovernedFiles); err != nil {
		return nil, err
	}

	// Count total source files tracked in ast_cache.
	var tableCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='ast_cache'`).Scan(&tableCount); err == nil && tableCount > 0 {
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ast_cache`).Scan(&coverage.TotalFiles); err != nil {
			return nil, err
		}
	}
	if coverage.TotalFiles > 0 {
		coverage.Percentage = float64(coverage.GovernedFiles) / float64(coverage.TotalFiles) * 100
	}

	// Count edges by source.
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM edges WHERE edge_type = 'applies_to' AND edge_source = 'manual'`).
		Scan(&coverage.ManualEdges); err != nil {
		return nil, err
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM edges WHERE edge_type = 'applies_to' AND edge_source = 'inferred'`).
		Scan(&coverage.InferredEdges); err != nil {
		return nil, err
	}

	return &coverage, nil
}
