package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/source"
	stindex "github.com/dusk-network/stroma/index"
	ststore "github.com/dusk-network/stroma/store"
)

// UpdatePreconditionError reports that the existing index is structurally
// incompatible with the current configuration and requires a full rebuild.
type UpdatePreconditionError struct {
	Reason string
	Action string
}

func (e *UpdatePreconditionError) Error() string {
	if strings.TrimSpace(e.Action) != "" {
		return fmt.Sprintf("%s; %s", e.Reason, e.Action)
	}
	return e.Reason
}

// IsUpdatePrecondition reports whether err wraps an update-precondition failure.
func IsUpdatePrecondition(err error) bool {
	var target *UpdatePreconditionError
	return errors.As(err, &target)
}

// UpdateOptions controls optional update behavior.
type UpdateOptions struct {
	ComputeDelta bool
}

// UpdateContextWithOptions performs an index update, using Stroma incremental
// snapshot updates when the existing index is structurally compatible and
// falling back to a full rebuild otherwise.
func UpdateContextWithOptions(ctx context.Context, cfg *config.Config, records *source.LoadResult) (*RebuildResult, error) {
	return updateContext(ctx, cfg, records, UpdateOptions{}, nil)
}

// UpdateWithProgressContextAndOptions performs an update with progress
// reporting.
func UpdateWithProgressContextAndOptions(ctx context.Context, cfg *config.Config, records *source.LoadResult, reporter RebuildProgressReporter) (*RebuildResult, error) {
	return updateContext(ctx, cfg, records, UpdateOptions{}, reporter)
}

// UpdateWithDeltaContextAndOptions performs an update with progress reporting
// and governance delta computation.
func UpdateWithDeltaContextAndOptions(ctx context.Context, cfg *config.Config, records *source.LoadResult, options UpdateOptions, reporter RebuildProgressReporter) (*RebuildResult, error) {
	return updateContext(ctx, cfg, records, options, reporter)
}

func updateContext(ctx context.Context, cfg *config.Config, records *source.LoadResult, options UpdateOptions, reporter RebuildProgressReporter) (*RebuildResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if records == nil {
		return nil, fmt.Errorf("records are required")
	}
	indexPath := cfg.Workspace.ResolvedIndexPath

	info, err := os.Stat(indexPath)
	switch {
	case os.IsNotExist(err):
		return nil, &MissingIndexError{Path: indexPath}
	case err != nil:
		return nil, fmt.Errorf("stat index %s: %w", indexPath, err)
	case info.IsDir():
		return nil, fmt.Errorf("index path %s is a directory", indexPath)
	}

	db, err := OpenReadOnlyContext(ctx, indexPath)
	if err != nil {
		return nil, fmt.Errorf("open index %s: %w", indexPath, err)
	}
	closeDB := true
	defer func() {
		if closeDB && db != nil {
			_ = db.Close()
		}
	}()

	storedRefs, err := loadStoredArtifactRefsContext(ctx, db)
	if err != nil {
		return nil, err
	}
	diff := computeArtifactDiff(storedRefs, records)

	embedder, err := prepareRebuildContext(ctx, cfg, records)
	if err != nil {
		return nil, err
	}
	dimension, err := embedder.Dimension(ctx)
	if err != nil {
		return nil, err
	}
	currentSnapshotPath, err := validateIncrementalUpdateEligibilityContext(ctx, db, cfg, embedder, dimension)
	if err != nil && !IsUpdatePrecondition(err) {
		return nil, err
	}
	shouldFallback := IsUpdatePrecondition(err)

	var oldEdges []snapshotEdge
	var oldArtifacts []snapshotArtifact
	if options.ComputeDelta {
		oldEdges, err = snapshotEdgesContext(ctx, db)
		if err != nil {
			return nil, fmt.Errorf("snapshot old edges: %w", err)
		}
		oldArtifacts, err = snapshotSpecArtifactsContext(ctx, db)
		if err != nil {
			return nil, fmt.Errorf("snapshot old artifacts: %w", err)
		}
	}
	if err := db.Close(); err != nil {
		return nil, fmt.Errorf("close index %s before rebuild: %w", indexPath, err)
	}
	closeDB = false
	db = nil

	if shouldFallback {
		return rebuildUpdateContext(ctx, cfg, records, diff, options, reporter, oldArtifacts, oldEdges)
	}

	reuseState, err := loadReuseStateContext(ctx, currentSnapshotPath, embedder.Fingerprint(), dimension, RebuildOptions{})
	if err != nil {
		return nil, err
	}
	result := summarizeRebuild(records, dimension, reuseState, RebuildOptions{})
	result.IndexPath = indexPath
	result.Update = true
	result.FullRebuild = false
	result.AddedCount = len(diff.added)
	result.UpdatedCount = len(diff.updated)
	result.RemovedCount = len(diff.removed)
	result.UnchangedCount = len(diff.unchanged)

	snapshotPath, cleanupSnapshot, err := updateStromaSnapshotContext(ctx, cfg, records, diff, currentSnapshotPath, embedder, reuseState, reporter)
	if err != nil {
		if IsUpdatePrecondition(err) {
			return rebuildUpdateContext(ctx, cfg, records, diff, options, reporter, oldArtifacts, oldEdges)
		}
		return nil, err
	}
	snapshotPublished := false
	defer func() {
		if !snapshotPublished && cleanupSnapshot != nil {
			cleanupSnapshot()
		}
	}()

	result.ContentFingerprint = contentFingerprint(records)
	if err := publishBusinessIndexContext(ctx, cfg, records, result, snapshotPath); err != nil {
		return nil, err
	}
	snapshotPublished = true
	if err := loadUpdateDeltaContext(ctx, indexPath, result, options, oldArtifacts, oldEdges); err != nil {
		return nil, err
	}

	return result, nil
}

// storedArtifactRef holds the minimal stored artifact data needed for diffing.
type storedArtifactRef struct {
	ref         string
	contentHash string
}

func loadStoredArtifactRefsContext(ctx context.Context, db *sql.DB) ([]storedArtifactRef, error) {
	rows, err := db.QueryContext(ctx, `SELECT ref, content_hash FROM artifacts`)
	if err != nil {
		return nil, fmt.Errorf("query stored artifact refs: %w", err)
	}
	defer rows.Close()

	var refs []storedArtifactRef
	for rows.Next() {
		var r storedArtifactRef
		if err := rows.Scan(&r.ref, &r.contentHash); err != nil {
			return nil, fmt.Errorf("scan stored artifact ref: %w", err)
		}
		refs = append(refs, r)
	}
	return refs, rows.Err()
}

// artifactDiff classifies artifacts into add/update/remove/unchanged buckets.
type artifactDiff struct {
	added     []string // refs present in records but not in DB
	updated   []string // refs present in both but content_hash differs
	removed   []string // refs present in DB but not in records
	unchanged []string // refs present in both with matching content_hash
}

func computeArtifactDiff(stored []storedArtifactRef, records *source.LoadResult) artifactDiff {
	storedMap := make(map[string]string, len(stored))
	for _, s := range stored {
		storedMap[s.ref] = s.contentHash
	}

	desiredMap := make(map[string]string, len(records.Specs)+len(records.Docs))
	for _, spec := range records.Specs {
		desiredMap[spec.Ref] = spec.ContentHash
	}
	for _, doc := range records.Docs {
		desiredMap[doc.Ref] = doc.ContentHash
	}

	var diff artifactDiff

	// Classify desired refs against stored.
	for ref, desiredHash := range desiredMap {
		storedHash, exists := storedMap[ref]
		switch {
		case !exists:
			diff.added = append(diff.added, ref)
		case storedHash != desiredHash:
			diff.updated = append(diff.updated, ref)
		default:
			diff.unchanged = append(diff.unchanged, ref)
		}
	}

	// Find removed refs.
	for ref := range storedMap {
		if _, exists := desiredMap[ref]; !exists {
			diff.removed = append(diff.removed, ref)
		}
	}

	sort.Strings(diff.added)
	sort.Strings(diff.updated)
	sort.Strings(diff.removed)
	sort.Strings(diff.unchanged)

	return diff
}

func rebuildUpdateContext(
	ctx context.Context,
	cfg *config.Config,
	records *source.LoadResult,
	diff artifactDiff,
	options UpdateOptions,
	reporter RebuildProgressReporter,
	oldArtifacts []snapshotArtifact,
	oldEdges []snapshotEdge,
) (*RebuildResult, error) {
	result, err := rebuildContext(ctx, cfg, records, RebuildOptions{}, reporter)
	if err != nil {
		return nil, err
	}
	result.Update = true
	result.FullRebuild = true
	result.AddedCount = len(diff.added)
	result.UpdatedCount = len(diff.updated)
	result.RemovedCount = len(diff.removed)
	result.UnchangedCount = len(diff.unchanged)
	if err := loadUpdateDeltaContext(ctx, cfg.Workspace.ResolvedIndexPath, result, options, oldArtifacts, oldEdges); err != nil {
		return nil, err
	}
	return result, nil
}

func loadUpdateDeltaContext(ctx context.Context, indexPath string, result *RebuildResult, options UpdateOptions, oldArtifacts []snapshotArtifact, oldEdges []snapshotEdge) error {
	if !options.ComputeDelta {
		return nil
	}

	newDB, err := OpenReadOnlyContext(ctx, indexPath)
	if err != nil {
		return fmt.Errorf("reopen updated index %s: %w", indexPath, err)
	}
	defer newDB.Close()

	newEdges, err := snapshotEdgesContext(ctx, newDB)
	if err != nil {
		return fmt.Errorf("snapshot new edges: %w", err)
	}
	newArtifacts, err := snapshotSpecArtifactsContext(ctx, newDB)
	if err != nil {
		return fmt.Errorf("snapshot new artifacts: %w", err)
	}
	result.Delta = computeGovernanceDelta(oldArtifacts, newArtifacts, oldEdges, newEdges)
	return nil
}

func validateIncrementalUpdateEligibilityContext(ctx context.Context, db *sql.DB, cfg *config.Config, embedder Embedder, dimension int) (string, error) {
	storedSchemaVersion, err := readRequiredMetadataValueContext(ctx, db, "schema_version")
	if err != nil {
		return "", &UpdatePreconditionError{Reason: err.Error()}
	}
	if strings.TrimSpace(storedSchemaVersion) != fmt.Sprintf("%d", schemaVersion) {
		return "", &UpdatePreconditionError{
			Reason: fmt.Sprintf("index schema_version %q does not match expected schema_version %d", strings.TrimSpace(storedSchemaVersion), schemaVersion),
			Action: "run `pituitary index --rebuild`",
		}
	}
	if err := validateStoredEmbedderContext(ctx, db, embedder.Fingerprint(), dimension); err != nil {
		return "", &UpdatePreconditionError{Reason: err.Error()}
	}

	snapshotPath, err := stromaSnapshotPathFromDBContext(ctx, db, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return "", &UpdatePreconditionError{Reason: err.Error()}
	}
	info, err := os.Stat(snapshotPath)
	switch {
	case os.IsNotExist(err):
		return "", &UpdatePreconditionError{
			Reason: fmt.Sprintf("stroma snapshot %s does not exist", snapshotPath),
			Action: "run `pituitary index --rebuild`",
		}
	case err != nil:
		return "", fmt.Errorf("stat stroma snapshot %s: %w", snapshotPath, err)
	case info.IsDir():
		return "", &UpdatePreconditionError{
			Reason: fmt.Sprintf("stroma snapshot path %s is a directory", snapshotPath),
			Action: "run `pituitary index --rebuild`",
		}
	default:
		return snapshotPath, nil
	}
}

func readRequiredMetadataValueContext(ctx context.Context, db *sql.DB, key string) (string, error) {
	value, err := readOptionalMetadataValueContext(ctx, db, key)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("index metadata is missing %s; run `pituitary index --rebuild`", key)
	}
	return value, nil
}

func updateStromaSnapshotContext(
	ctx context.Context,
	cfg *config.Config,
	records *source.LoadResult,
	diff artifactDiff,
	currentSnapshotPath string,
	embedder Embedder,
	reuseState *reuseState,
	reporter RebuildProgressReporter,
) (string, func(), error) {
	if len(diff.added) == 0 && len(diff.updated) == 0 && len(diff.removed) == 0 {
		return currentSnapshotPath, nil, nil
	}

	targetSnapshotPath := stromaSnapshotPathForContent(cfg.Workspace.ResolvedIndexPath, contentFingerprint(records))
	cleanupSnapshot, err := prepareSnapshotUpdateTarget(currentSnapshotPath, targetSnapshotPath)
	if err != nil {
		return "", nil, err
	}

	selected := selectedLoadResultForRefs(records, append(append([]string{}, diff.added...), diff.updated...))
	emitPlannedRebuildProgress(selected, reuseState, reporter)
	selectedRecords, err := corpusRecordsFromLoadResult(selected)
	if err != nil {
		if cleanupSnapshot != nil {
			cleanupSnapshot()
		}
		return "", nil, err
	}
	if _, err := stindex.Update(ctx, selectedRecords, diff.removed, stindex.UpdateOptions{
		Path:     targetSnapshotPath,
		Embedder: embedder,
	}); err != nil {
		if cleanupSnapshot != nil {
			cleanupSnapshot()
		}
		return "", nil, normalizeStromaUpdateError(targetSnapshotPath, err)
	}
	return targetSnapshotPath, cleanupSnapshot, nil
}

func selectedLoadResultForRefs(records *source.LoadResult, refs []string) *source.LoadResult {
	selected := &source.LoadResult{}
	if records == nil || len(refs) == 0 {
		return selected
	}

	selected.Sources = append(selected.Sources, records.Sources...)
	refSet := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		refSet[ref] = struct{}{}
	}
	for _, spec := range records.Specs {
		if _, ok := refSet[spec.Ref]; ok {
			selected.Specs = append(selected.Specs, spec)
		}
	}
	for _, doc := range records.Docs {
		if _, ok := refSet[doc.Ref]; ok {
			selected.Docs = append(selected.Docs, doc)
		}
	}
	return selected
}

func prepareSnapshotUpdateTarget(currentSnapshotPath, targetSnapshotPath string) (func(), error) {
	if strings.TrimSpace(currentSnapshotPath) == "" {
		return nil, &UpdatePreconditionError{
			Reason: "stroma snapshot metadata is missing",
			Action: "run `pituitary index --rebuild`",
		}
	}
	if currentSnapshotPath == targetSnapshotPath {
		return nil, nil
	}
	if err := os.Remove(targetSnapshotPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale stroma snapshot %s: %w", targetSnapshotPath, err)
	}
	if err := copyFile(currentSnapshotPath, targetSnapshotPath); err != nil {
		return nil, err
	}
	return func() {
		_ = os.Remove(targetSnapshotPath)
	}, nil
}

func copyFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open stroma snapshot %s: %w", srcPath, err)
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return fmt.Errorf("stat stroma snapshot %s: %w", srcPath, err)
	}

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode())
	if err != nil {
		return fmt.Errorf("create stroma snapshot %s: %w", dstPath, err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		_ = os.Remove(dstPath)
		return fmt.Errorf("copy stroma snapshot to %s: %w", dstPath, err)
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(dstPath)
		return fmt.Errorf("close copied stroma snapshot %s: %w", dstPath, err)
	}
	return nil
}

func normalizeStromaUpdateError(snapshotPath string, err error) error {
	if err == nil {
		return nil
	}
	if ststore.IsMissingIndex(err) {
		return &UpdatePreconditionError{
			Reason: fmt.Sprintf("stroma snapshot %s does not exist", snapshotPath),
			Action: "run `pituitary index --rebuild`",
		}
	}
	message := strings.TrimSpace(err.Error())
	for _, marker := range []string{
		"schema version mismatch",
		"embedder fingerprint mismatch",
		"embedder dimension mismatch",
		"quantization mismatch",
		"update embedder is required when adding records",
	} {
		if strings.Contains(message, marker) {
			return &UpdatePreconditionError{
				Reason: message,
				Action: "run `pituitary index --rebuild`",
			}
		}
	}
	return err
}

// upsertMetadataContext inserts or replaces a metadata key-value pair.
func upsertMetadataContext(ctx context.Context, tx *sql.Tx, key, value string) error {
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO metadata (key, value) VALUES (?, ?)`, key, value); err != nil {
		return fmt.Errorf("upsert metadata %s: %w", key, err)
	}
	return nil
}

// runTransactionIntegrityChecks runs FK and integrity checks within the
// current transaction. These must run before commit so that failures trigger
// a rollback rather than leaving a corrupted live database.
func runTransactionIntegrityChecks(ctx context.Context, tx *sql.Tx) error {
	row := tx.QueryRowContext(ctx, `PRAGMA integrity_check`)
	var result string
	if err := row.Scan(&result); err != nil {
		return fmt.Errorf("run integrity_check: %w", err)
	}
	if strings.ToLower(result) != "ok" {
		return fmt.Errorf("integrity_check failed: %s", result)
	}

	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("run foreign_key_check: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		var table string
		var rowID int
		var parent string
		var fkid int
		if err := rows.Scan(&table, &rowID, &parent, &fkid); err != nil {
			return fmt.Errorf("scan foreign_key_check result: %w", err)
		}
		return fmt.Errorf("foreign_key_check failed for table %s row %d parent %s fk %d", table, rowID, parent, fkid)
	}

	return nil
}

// migrateToSchemaV4 adds the edge_source column and ast_cache table to an
// existing v3 index. Only runs when the stored schema version is exactly "3".
func migrateToSchemaV4(ctx context.Context, db *sql.DB) error {
	var storedVersion string
	if err := db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'schema_version'`).Scan(&storedVersion); err != nil {
		return nil // no metadata row — let validateUpdatePreconditions handle it
	}
	if strings.TrimSpace(storedVersion) != "3" {
		return nil // not a v3 index — nothing to migrate
	}

	// Add edge_source column if missing.
	var colCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('edges') WHERE name = 'edge_source'`).Scan(&colCount); err != nil {
		return err
	}
	if colCount == 0 {
		if _, err := db.ExecContext(ctx, `ALTER TABLE edges ADD COLUMN edge_source TEXT NOT NULL DEFAULT 'manual'`); err != nil {
			return fmt.Errorf("add edge_source column: %w", err)
		}
	}

	// Create ast_cache table if not present.
	var tableCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='ast_cache'`).Scan(&tableCount); err != nil {
		return err
	}
	if tableCount == 0 {
		if _, err := db.ExecContext(ctx, `CREATE TABLE ast_cache (content_hash TEXT PRIMARY KEY, path TEXT NOT NULL, symbols_json TEXT NOT NULL)`); err != nil {
			return fmt.Errorf("create ast_cache table: %w", err)
		}
	}

	// Bump schema version to 4.
	if _, err := db.ExecContext(ctx, `UPDATE metadata SET value = '4' WHERE key = 'schema_version'`); err != nil {
		return fmt.Errorf("update schema_version: %w", err)
	}
	return nil
}

// migrateToSchemaV5 adds temporal validity columns (valid_from, valid_to) to
// the edges and artifacts tables. Only runs when the stored schema version is "4".
func migrateToSchemaV5(ctx context.Context, db *sql.DB) error {
	var storedVersion string
	if err := db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'schema_version'`).Scan(&storedVersion); err != nil {
		return nil
	}
	if strings.TrimSpace(storedVersion) != "4" {
		return nil
	}

	addColumn := func(table, column string) error {
		var count int
		if err := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = '%s'`, table, column)).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			if _, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s TEXT`, table, column)); err != nil {
				return fmt.Errorf("add %s.%s column: %w", table, column, err)
			}
		}
		return nil
	}

	for _, col := range []string{"valid_from", "valid_to"} {
		if err := addColumn("edges", col); err != nil {
			return err
		}
		if err := addColumn("artifacts", col); err != nil {
			return err
		}
	}

	if _, err := db.ExecContext(ctx, `UPDATE metadata SET value = '5' WHERE key = 'schema_version'`); err != nil {
		return fmt.Errorf("update schema_version to 5: %w", err)
	}
	return nil
}

// migrateToSchemaV7 adds the rationale_json column to ast_cache for storing
// extracted rationale comments alongside code symbols. Only runs when schema is "6".
func migrateToSchemaV7(ctx context.Context, db *sql.DB) error {
	var storedVersion string
	if err := db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'schema_version'`).Scan(&storedVersion); err != nil {
		return nil
	}
	if strings.TrimSpace(storedVersion) != "6" {
		return nil
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('ast_cache') WHERE name = 'rationale_json'`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.ExecContext(ctx, `ALTER TABLE ast_cache ADD COLUMN rationale_json TEXT NOT NULL DEFAULT '[]'`); err != nil {
			return fmt.Errorf("add ast_cache.rationale_json column: %w", err)
		}
	}

	if _, err := db.ExecContext(ctx, `UPDATE metadata SET value = '7' WHERE key = 'schema_version'`); err != nil {
		return fmt.Errorf("update schema_version to 7: %w", err)
	}
	return nil
}

// migrateToSchemaV6 adds confidence tier columns to the edges table and
// backfills existing edges based on edge_source. Only runs when schema is "5".
func migrateToSchemaV6(ctx context.Context, db *sql.DB) error {
	var storedVersion string
	if err := db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = 'schema_version'`).Scan(&storedVersion); err != nil {
		return nil
	}
	if strings.TrimSpace(storedVersion) != "5" {
		return nil
	}

	addColumn := func(table, column, ddl string) error {
		var count int
		if err := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = '%s'`, table, column)).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			if _, err := db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s`, table, ddl)); err != nil {
				return fmt.Errorf("add %s.%s: %w", table, column, err)
			}
		}
		return nil
	}

	if err := addColumn("edges", "confidence", "confidence TEXT NOT NULL DEFAULT 'extracted'"); err != nil {
		return err
	}
	if err := addColumn("edges", "confidence_score", "confidence_score REAL NOT NULL DEFAULT 1.0"); err != nil {
		return err
	}

	// Backfill: inferred edges get confidence='inferred', score=0.7.
	if _, err := db.ExecContext(ctx, `UPDATE edges SET confidence = 'inferred', confidence_score = 0.7 WHERE edge_source = 'inferred' AND confidence = 'extracted'`); err != nil {
		return fmt.Errorf("backfill inferred confidence: %w", err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE metadata SET value = '6' WHERE key = 'schema_version'`); err != nil {
		return fmt.Errorf("update schema_version to 6: %w", err)
	}
	return nil
}
