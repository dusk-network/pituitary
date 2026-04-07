package index

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
)

// GoverningSpec reports one accepted spec that governs a set of code/config refs.
type GoverningSpec struct {
	Ref    string `json:"ref"`
	Title  string `json:"title"`
	Source string `json:"source"` // "manual" or "inferred"
}

// GoverningSpecsResult is the structured result of a governed-by query.
type GoverningSpecsResult struct {
	Path  string          `json:"path"`
	Refs  []string        `json:"refs"`
	Specs []GoverningSpec `json:"specs"`
}

// GovernedByContext queries the index for accepted specs whose applies_to edges
// match any of the candidate refs derived from the given workspace-relative path.
func GovernedByContext(ctx context.Context, dbPath string, path string) (*GoverningSpecsResult, error) {
	normalized := normalizePath(path)
	if normalized == "" || normalized == "." {
		return nil, fmt.Errorf("governed_by requires a non-empty file path")
	}
	refs := governedRefsForPath(normalized)

	db, err := OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	hasSource := hasEdgeSourceColumn(ctx, db)

	var builder strings.Builder
	args := make([]any, 0, len(refs))
	if hasSource {
		builder.WriteString(`
SELECT DISTINCT a.ref, COALESCE(a.title, ''), e.edge_source
FROM edges e
JOIN artifacts a ON a.ref = e.from_ref
WHERE a.kind = 'spec'
  AND a.status = 'accepted'
  AND e.edge_type = 'applies_to'
  AND e.to_ref IN (`)
	} else {
		builder.WriteString(`
SELECT DISTINCT a.ref, COALESCE(a.title, '')
FROM edges e
JOIN artifacts a ON a.ref = e.from_ref
WHERE a.kind = 'spec'
  AND a.status = 'accepted'
  AND e.edge_type = 'applies_to'
  AND e.to_ref IN (`)
	}
	for i, ref := range refs {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString("?")
		args = append(args, ref)
	}
	if hasSource {
		builder.WriteString(")\nORDER BY e.edge_source, a.ref")
	} else {
		builder.WriteString(")\nORDER BY a.ref")
	}

	rows, err := db.QueryContext(ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query governing specs: %w", err)
	}
	defer rows.Close()

	var specs []GoverningSpec
	for rows.Next() {
		var spec GoverningSpec
		if hasSource {
			if err := rows.Scan(&spec.Ref, &spec.Title, &spec.Source); err != nil {
				return nil, fmt.Errorf("scan governing spec: %w", err)
			}
		} else {
			if err := rows.Scan(&spec.Ref, &spec.Title); err != nil {
				return nil, fmt.Errorf("scan governing spec: %w", err)
			}
			spec.Source = "manual"
		}
		specs = append(specs, spec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate governing specs: %w", err)
	}

	return &GoverningSpecsResult{
		Path:  normalized,
		Refs:  refs,
		Specs: specs,
	}, nil
}

// governedRefsForPath generates the candidate edge refs for a workspace path.
func governedRefsForPath(path string) []string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml", ".json", ".toml", ".ini", ".cfg", ".conf":
		return []string{"config://" + path, "code://" + path}
	default:
		return []string{"code://" + path, "config://" + path}
	}
}

func hasEdgeSourceColumn(ctx context.Context, db *sql.DB) bool {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('edges') WHERE name = 'edge_source'`).Scan(&count)
	return err == nil && count > 0
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "code://")
	path = strings.TrimPrefix(path, "config://")
	return filepath.ToSlash(filepath.Clean(path))
}
