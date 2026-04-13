package index

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"

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
	GovernanceHotspots *GovernanceHotspots `json:"governance_hotspots,omitempty"`
}

// GovernanceCoverage reports the percentage of indexed source files that have
// at least one governance link (manual or inferred applies_to edge).
type GovernanceCoverage struct {
	TotalFiles     int     `json:"total_files"`
	GovernedFiles  int     `json:"governed_files"`
	Percentage     float64 `json:"percentage"`
	ManualEdges    int     `json:"manual_edges"`
	InferredEdges  int     `json:"inferred_edges"`
	ExtractedEdges int     `json:"extracted_edges,omitempty"`
	AmbiguousEdges int     `json:"ambiguous_edges,omitempty"`
}

// GovernanceHotspots surfaces governance-maintenance hotspots from the indexed
// applies_to graph so operators can distinguish weak traceability from direct
// contradiction-oriented findings.
type GovernanceHotspots struct {
	HighFanOutSpecs        []GovernanceSpecHotspot     `json:"high_fan_out_specs,omitempty"`
	WeakLinkArtifacts      []GovernanceArtifactHotspot `json:"weak_link_artifacts,omitempty"`
	MultiGovernedArtifacts []GovernanceArtifactHotspot `json:"multi_governed_artifacts,omitempty"`
}

type GovernanceSpecHotspot struct {
	Ref                string `json:"ref"`
	Title              string `json:"title,omitempty"`
	AppliesToCount     int    `json:"applies_to_count"`
	ExtractedEdgeCount int    `json:"extracted_edge_count,omitempty"`
	InferredEdgeCount  int    `json:"inferred_edge_count,omitempty"`
	AmbiguousEdgeCount int    `json:"ambiguous_edge_count,omitempty"`
}

type GovernanceArtifactHotspot struct {
	Ref                string   `json:"ref"`
	Title              string   `json:"title,omitempty"`
	SourceRef          string   `json:"source_ref,omitempty"`
	GoverningSpecCount int      `json:"governing_spec_count"`
	ExtractedEdgeCount int      `json:"extracted_edge_count,omitempty"`
	InferredEdgeCount  int      `json:"inferred_edge_count,omitempty"`
	AmbiguousEdgeCount int      `json:"ambiguous_edge_count,omitempty"`
	GoverningSpecs     []string `json:"governing_specs,omitempty"`
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

	snapshot, err := OpenStromaSnapshotContext(ctx, db, path)
	if err != nil {
		return nil, err
	}
	defer snapshot.Close()

	stats, err := snapshot.Stats(ctx)
	if err != nil {
		return nil, fmt.Errorf("read stroma stats: %w", err)
	}

	status.Exists = true
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifacts WHERE kind = ?`, model.ArtifactKindSpec).Scan(&status.SpecCount); err != nil {
		return nil, fmt.Errorf("count indexed specs: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifacts WHERE kind = ?`, model.ArtifactKindDoc).Scan(&status.DocCount); err != nil {
		return nil, fmt.Errorf("count indexed docs: %w", err)
	}
	status.ChunkCount = stats.ChunkCount
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
		hotspots, hotspotErr := queryGovernanceHotspotsContext(ctx, db)
		if hotspotErr == nil {
			status.GovernanceHotspots = hotspots
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

	// Count edges by confidence tier (schema v6+).
	if hasEdgeConfidenceColumn(ctx, db) {
		_ = db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM edges WHERE edge_type = 'applies_to' AND confidence = 'extracted'`).
			Scan(&coverage.ExtractedEdges)
		_ = db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM edges WHERE edge_type = 'applies_to' AND confidence = 'ambiguous'`).
			Scan(&coverage.AmbiguousEdges)
	}

	return &coverage, nil
}

func queryGovernanceHotspotsContext(ctx context.Context, db *sql.DB) (*GovernanceHotspots, error) {
	hasConfidence := hasEdgeConfidenceColumn(ctx, db)

	highFanOutSpecs, err := queryGovernanceSpecHotspotsContext(ctx, db, hasConfidence)
	if err != nil {
		return nil, err
	}
	weakLinkArtifacts, err := queryGovernanceArtifactHotspotsContext(
		ctx,
		db,
		hasConfidence,
		`extracted_edge_count = 0 AND (inferred_edge_count > 0 OR ambiguous_edge_count > 0)`,
		`ambiguous_edge_count DESC, inferred_edge_count DESC, governing_spec_count DESC, ref`,
	)
	if err != nil {
		return nil, err
	}
	multiGovernedArtifacts, err := queryGovernanceArtifactHotspotsContext(
		ctx,
		db,
		hasConfidence,
		`governing_spec_count > 1`,
		`governing_spec_count DESC, ambiguous_edge_count DESC, inferred_edge_count DESC, ref`,
	)
	if err != nil {
		return nil, err
	}

	if len(highFanOutSpecs) == 0 && len(weakLinkArtifacts) == 0 && len(multiGovernedArtifacts) == 0 {
		return nil, nil
	}
	return &GovernanceHotspots{
		HighFanOutSpecs:        highFanOutSpecs,
		WeakLinkArtifacts:      weakLinkArtifacts,
		MultiGovernedArtifacts: multiGovernedArtifacts,
	}, nil
}

func queryGovernanceSpecHotspotsContext(ctx context.Context, db *sql.DB, hasConfidence bool) ([]GovernanceSpecHotspot, error) {
	query := fmt.Sprintf(`
SELECT
  s.ref,
  COALESCE(s.title, ''),
  COUNT(*) AS applies_to_count,
  %s AS extracted_edge_count,
  %s AS inferred_edge_count,
  %s AS ambiguous_edge_count
FROM edges e
JOIN artifacts s ON s.ref = e.from_ref
WHERE e.edge_type = 'applies_to'
  AND s.kind = ?
  AND s.status = ?
GROUP BY s.ref, s.title
ORDER BY applies_to_count DESC, s.ref
LIMIT 5`,
		governanceExtractedEdgeExpr(hasConfidence),
		governanceInferredEdgeExpr(hasConfidence),
		governanceAmbiguousEdgeExpr(hasConfidence),
	)

	rows, err := db.QueryContext(ctx, query, model.ArtifactKindSpec, model.StatusAccepted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hotspots := []GovernanceSpecHotspot{}
	for rows.Next() {
		var hotspot GovernanceSpecHotspot
		if err := rows.Scan(
			&hotspot.Ref,
			&hotspot.Title,
			&hotspot.AppliesToCount,
			&hotspot.ExtractedEdgeCount,
			&hotspot.InferredEdgeCount,
			&hotspot.AmbiguousEdgeCount,
		); err != nil {
			return nil, err
		}
		hotspots = append(hotspots, hotspot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return hotspots, nil
}

func queryGovernanceArtifactHotspotsContext(ctx context.Context, db *sql.DB, hasConfidence bool, filter string, orderBy string) ([]GovernanceArtifactHotspot, error) {
	aggregateQuery := fmt.Sprintf(`
SELECT
  e.to_ref AS ref,
  COALESCE(a.title, '') AS title,
  COALESCE(a.source_ref, '') AS source_ref,
  COUNT(DISTINCT e.from_ref) AS governing_spec_count,
  %s AS extracted_edge_count,
  %s AS inferred_edge_count,
  %s AS ambiguous_edge_count
FROM edges e
JOIN artifacts s ON s.ref = e.from_ref
LEFT JOIN artifacts a ON a.ref = e.to_ref
WHERE e.edge_type = 'applies_to'
  AND s.kind = ?
  AND s.status = ?
GROUP BY e.to_ref, a.title, a.source_ref`,
		governanceExtractedEdgeExpr(hasConfidence),
		governanceInferredEdgeExpr(hasConfidence),
		governanceAmbiguousEdgeExpr(hasConfidence),
	)
	query := fmt.Sprintf(`
SELECT
  ref,
  title,
  source_ref,
  governing_spec_count,
  extracted_edge_count,
  inferred_edge_count,
  ambiguous_edge_count
FROM (%s)
WHERE %s
ORDER BY %s
LIMIT 5`, aggregateQuery, filter, orderBy)

	rows, err := db.QueryContext(ctx, query, model.ArtifactKindSpec, model.StatusAccepted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hotspots := []GovernanceArtifactHotspot{}
	for rows.Next() {
		var hotspot GovernanceArtifactHotspot
		if err := rows.Scan(
			&hotspot.Ref,
			&hotspot.Title,
			&hotspot.SourceRef,
			&hotspot.GoverningSpecCount,
			&hotspot.ExtractedEdgeCount,
			&hotspot.InferredEdgeCount,
			&hotspot.AmbiguousEdgeCount,
		); err != nil {
			return nil, err
		}
		hotspots = append(hotspots, hotspot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	refs := make([]string, 0, len(hotspots))
	for _, hotspot := range hotspots {
		refs = append(refs, hotspot.Ref)
	}
	specRefsByArtifact, err := loadGovernanceSpecRefsByArtifactContext(ctx, db, refs)
	if err != nil {
		return nil, err
	}
	for i := range hotspots {
		hotspots[i].GoverningSpecs = specRefsByArtifact[hotspots[i].Ref]
	}
	return hotspots, nil
}

func loadGovernanceSpecRefsByArtifactContext(ctx context.Context, db *sql.DB, refs []string) (map[string][]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(refs)), ",")
	query := fmt.Sprintf(`
SELECT
  e.to_ref,
  e.from_ref
FROM edges e
JOIN artifacts s ON s.ref = e.from_ref
WHERE e.edge_type = 'applies_to'
  AND s.kind = ?
  AND s.status = ?
  AND e.to_ref IN (%s)
GROUP BY e.to_ref, e.from_ref
ORDER BY e.to_ref, e.from_ref`, placeholders)

	args := make([]any, 0, 2+len(refs))
	args = append(args, model.ArtifactKindSpec, model.StatusAccepted)
	for _, ref := range refs {
		args = append(args, ref)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	specRefsByArtifact := make(map[string][]string, len(refs))
	for rows.Next() {
		var artifactRef string
		var specRef string
		if err := rows.Scan(&artifactRef, &specRef); err != nil {
			return nil, err
		}
		specRefsByArtifact[artifactRef] = append(specRefsByArtifact[artifactRef], specRef)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return specRefsByArtifact, nil
}

func governanceExtractedEdgeExpr(hasConfidence bool) string {
	if hasConfidence {
		return `SUM(CASE WHEN e.confidence = 'extracted' THEN 1 ELSE 0 END)`
	}
	return `SUM(CASE WHEN e.edge_source = 'manual' THEN 1 ELSE 0 END)`
}

func governanceInferredEdgeExpr(hasConfidence bool) string {
	if hasConfidence {
		return `SUM(CASE WHEN e.confidence = 'inferred' THEN 1 ELSE 0 END)`
	}
	return `SUM(CASE WHEN e.edge_source = 'inferred' THEN 1 ELSE 0 END)`
}

func governanceAmbiguousEdgeExpr(hasConfidence bool) string {
	if hasConfidence {
		return `SUM(CASE WHEN e.confidence = 'ambiguous' THEN 1 ELSE 0 END)`
	}
	return `0`
}

func splitGovernanceSpecRefs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	values := strings.Split(raw, ",")
	refs := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		ref := strings.TrimSpace(value)
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}
