package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dusk-network/pituitary/internal/ast"
)

// FileRationale holds rationale entries for one code file.
type FileRationale struct {
	Path      string          `json:"path"`
	Rationale []ast.Rationale `json:"rationale"`
}

// QueryRationaleContext returns rationale entries for the given file paths
// from the ast_cache table.
func QueryRationaleContext(ctx context.Context, dbPath string, paths []string) ([]FileRationale, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	db, err := OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("open index for rationale query: %w", err)
	}
	defer db.Close()

	// Check if rationale_json column exists (schema v7+).
	var colCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('ast_cache') WHERE name = 'rationale_json'`).Scan(&colCount); err != nil || colCount == 0 {
		return nil, nil
	}

	// Normalize paths: strip code:// or config:// prefixes.
	normalized := make([]string, len(paths))
	for i, p := range paths {
		normalized[i] = strings.TrimPrefix(strings.TrimPrefix(p, "code://"), "config://")
	}

	query := `SELECT path, rationale_json FROM ast_cache WHERE path IN (` + placeholders(len(normalized)) + `)`
	args := make([]any, len(normalized))
	for i, p := range normalized {
		args[i] = p
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query rationale: %w", err)
	}
	defer rows.Close()

	var results []FileRationale
	for rows.Next() {
		var path, rationaleJSON string
		if err := rows.Scan(&path, &rationaleJSON); err != nil {
			return nil, fmt.Errorf("scan rationale: %w", err)
		}
		var rationale []ast.Rationale
		if err := json.Unmarshal([]byte(rationaleJSON), &rationale); err != nil {
			return nil, fmt.Errorf("decode rationale_json for path %q: %w", path, err)
		}
		if len(rationale) > 0 {
			results = append(results, FileRationale{Path: path, Rationale: rationale})
		}
	}
	return results, rows.Err()
}

// queryRationaleDBContext queries rationale from an already-open database connection.
func queryRationaleDBContext(ctx context.Context, db *sql.DB, paths []string) (map[string][]ast.Rationale, error) {
	result := make(map[string][]ast.Rationale)
	if len(paths) == 0 {
		return result, nil
	}

	// Check if rationale_json column exists.
	var colCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('ast_cache') WHERE name = 'rationale_json'`).Scan(&colCount); err != nil || colCount == 0 {
		return result, nil
	}

	// Normalize paths.
	normalized := make([]string, len(paths))
	for i, p := range paths {
		normalized[i] = strings.TrimPrefix(strings.TrimPrefix(p, "code://"), "config://")
	}

	query := `SELECT path, rationale_json FROM ast_cache WHERE path IN (` + placeholders(len(normalized)) + `)`
	args := make([]any, len(normalized))
	for i, p := range normalized {
		args[i] = p
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return result, fmt.Errorf("query rationale: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var path, rationaleJSON string
		if err := rows.Scan(&path, &rationaleJSON); err != nil {
			continue
		}
		var rationale []ast.Rationale
		if err := json.Unmarshal([]byte(rationaleJSON), &rationale); err != nil {
			continue
		}
		if len(rationale) > 0 {
			result[path] = rationale
		}
	}
	return result, rows.Err()
}
