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
	Ref             string  `json:"ref"`
	Title           string  `json:"title"`
	Source          string  `json:"source"`           // "manual" or "inferred"
	Confidence      string  `json:"confidence"`       // "extracted", "inferred", or "ambiguous"
	ConfidenceScore float64 `json:"confidence_score"` // 0.0–1.0
}

// GoverningSpecsResult is the structured result of a governed-by query.
type GoverningSpecsResult struct {
	Path  string          `json:"path"`
	Refs  []string        `json:"refs"`
	Specs []GoverningSpec `json:"specs"`
}

// GovernedByContext queries the index for accepted specs whose applies_to edges
// match any of the candidate refs derived from the given workspace-relative path.
// When atDate is non-empty, only edges active at that date are considered
// (valid_from <= atDate AND (valid_to IS NULL OR valid_to >= atDate)).
func GovernedByContext(ctx context.Context, dbPath string, path string, atDate string, minConfidence string) (*GoverningSpecsResult, error) {
	normalized := normalizePath(path)
	if normalized == "" || normalized == "." {
		return nil, fmt.Errorf("governed_by requires a non-empty file path")
	}

	db, err := OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	refs, err := ResolveGovernedRefsForPathContext(ctx, db, normalized)
	if err != nil {
		return nil, err
	}

	hasSource := hasEdgeSourceColumn(ctx, db)
	hasConfidence := hasEdgeConfidenceColumn(ctx, db)

	var builder strings.Builder
	args := make([]any, 0, len(refs)+4)
	switch {
	case hasConfidence:
		builder.WriteString(`
SELECT DISTINCT a.ref, COALESCE(a.title, ''), e.edge_source, e.confidence, e.confidence_score
FROM edges e
JOIN artifacts a ON a.ref = e.from_ref
WHERE a.kind = 'spec'
  AND a.status = 'accepted'
  AND e.edge_type = 'applies_to'
  AND e.to_ref IN (`)
	case hasSource:
		builder.WriteString(`
SELECT DISTINCT a.ref, COALESCE(a.title, ''), e.edge_source
FROM edges e
JOIN artifacts a ON a.ref = e.from_ref
WHERE a.kind = 'spec'
  AND a.status = 'accepted'
  AND e.edge_type = 'applies_to'
  AND e.to_ref IN (`)
	default:
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
	builder.WriteString(")")
	appendMinConfidenceClause(&builder, &args, hasConfidence, minConfidence)
	appendTemporalClause(&builder, &args, atDate)
	if hasSource {
		builder.WriteString("\nORDER BY e.edge_source, a.ref")
	} else {
		builder.WriteString("\nORDER BY a.ref")
	}

	rows, err := db.QueryContext(ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query governing specs: %w", err)
	}
	defer rows.Close()

	var specs []GoverningSpec
	for rows.Next() {
		var spec GoverningSpec
		switch {
		case hasConfidence:
			if err := rows.Scan(&spec.Ref, &spec.Title, &spec.Source, &spec.Confidence, &spec.ConfidenceScore); err != nil {
				return nil, fmt.Errorf("scan governing spec: %w", err)
			}
		case hasSource:
			if err := rows.Scan(&spec.Ref, &spec.Title, &spec.Source); err != nil {
				return nil, fmt.Errorf("scan governing spec: %w", err)
			}
			spec.Confidence = confidenceFromSource(spec.Source)
			spec.ConfidenceScore = confidenceScoreFromSource(spec.Source)
		default:
			if err := rows.Scan(&spec.Ref, &spec.Title); err != nil {
				return nil, fmt.Errorf("scan governing spec: %w", err)
			}
			spec.Source = "manual"
			spec.Confidence = "extracted"
			spec.ConfidenceScore = 1.0
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

// ResolveGovernedRefsForPathContext expands a workspace-relative path into the
// applies_to refs that may govern it. This preserves the historical code/config
// candidates and adds any indexed artifact refs normalized from the same
// workspace path, such as doc:// refs for markdown docs.
func ResolveGovernedRefsForPathContext(ctx context.Context, db *sql.DB, path string) ([]string, error) {
	path = normalizePath(path)
	refs := governedRefsForPath(path)
	artifactRefs, err := indexedArtifactRefsForPathContext(ctx, db, path)
	if err != nil {
		return nil, err
	}
	return uniqueGovernedRefs(append(refs, artifactRefs...)), nil
}

// appendTemporalClause adds a temporal validity filter to the SQL query when
// atDate is non-empty. It filters for edges active at the given date using:
// valid_from <= atDate AND (valid_to IS NULL OR valid_to >= atDate).
// The date is normalized to YYYY-MM-DD for consistent TEXT comparison with
// stored values.
func appendTemporalClause(builder *strings.Builder, args *[]any, atDate string) {
	atDate = strings.TrimSpace(atDate)
	if atDate == "" {
		return
	}
	// Normalize: truncate to date-only (YYYY-MM-DD) for consistent comparison.
	if len(atDate) > 10 {
		atDate = atDate[:10]
	}
	builder.WriteString(` AND (e.valid_from IS NULL OR e.valid_from <= ?)`)
	*args = append(*args, atDate)
	builder.WriteString(` AND (e.valid_to IS NULL OR e.valid_to >= ?)`)
	*args = append(*args, atDate)
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

func indexedArtifactRefsForPathContext(ctx context.Context, db *sql.DB, path string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT ref
FROM artifacts
WHERE json_extract(metadata_json, '$.path') = ?
ORDER BY ref`, path)
	if err != nil {
		return nil, fmt.Errorf("query indexed artifact refs: %w", err)
	}
	defer rows.Close()

	var refs []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, fmt.Errorf("scan indexed artifact ref: %w", err)
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexed artifact refs: %w", err)
	}
	return refs, nil
}

func uniqueGovernedRefs(refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	result := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		result = append(result, ref)
	}
	return result
}

func hasEdgeSourceColumn(ctx context.Context, db *sql.DB) bool {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('edges') WHERE name = 'edge_source'`).Scan(&count)
	return err == nil && count > 0
}

func hasEdgeConfidenceColumn(ctx context.Context, db *sql.DB) bool {
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('edges') WHERE name = 'confidence'`).Scan(&count)
	return err == nil && count > 0
}

// confidenceFromSource derives a confidence tier from an edge_source value for
// backward compatibility with pre-v6 indexes.
func confidenceFromSource(source string) string {
	if source == "inferred" {
		return "inferred"
	}
	return "extracted"
}

func confidenceScoreFromSource(source string) float64 {
	if source == "inferred" {
		return 0.7
	}
	return 1.0
}

// appendMinConfidenceClause adds a WHERE filter on confidence tier when
// minConfidence is non-empty and the index has the confidence column.
func appendMinConfidenceClause(builder *strings.Builder, args *[]any, hasColumn bool, minConfidence string) {
	if minConfidence == "" || !hasColumn {
		return
	}
	switch minConfidence {
	case "extracted":
		builder.WriteString(` AND e.confidence = 'extracted'`)
	case "inferred":
		builder.WriteString(` AND e.confidence IN ('extracted', 'inferred')`)
		// "ambiguous" includes all tiers — no filter needed.
	}
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "code://")
	path = strings.TrimPrefix(path, "config://")
	path = strings.TrimPrefix(path, "doc://")
	return filepath.ToSlash(filepath.Clean(path))
}
