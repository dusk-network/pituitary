package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
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

// UpdateContextWithOptions performs an incremental index update, writing only
// changed artifacts to the existing database.
func UpdateContextWithOptions(ctx context.Context, cfg *config.Config, records *source.LoadResult) (*RebuildResult, error) {
	return updateContext(ctx, cfg, records, nil)
}

// UpdateWithProgressContextAndOptions performs an incremental index update with
// progress reporting.
func UpdateWithProgressContextAndOptions(ctx context.Context, cfg *config.Config, records *source.LoadResult, reporter RebuildProgressReporter) (*RebuildResult, error) {
	return updateContext(ctx, cfg, records, reporter)
}

func updateContext(ctx context.Context, cfg *config.Config, records *source.LoadResult, reporter RebuildProgressReporter) (*RebuildResult, error) {
	embedder, err := prepareRebuildContext(ctx, cfg, records)
	if err != nil {
		return nil, err
	}
	dimension, err := embedder.Dimension(ctx)
	if err != nil {
		return nil, err
	}

	indexPath := cfg.Workspace.ResolvedIndexPath

	// Step 3: Validate index file exists.
	info, err := os.Stat(indexPath)
	switch {
	case os.IsNotExist(err):
		return nil, &MissingIndexError{Path: indexPath}
	case err != nil:
		return nil, fmt.Errorf("stat index %s: %w", indexPath, err)
	case info.IsDir():
		return nil, fmt.Errorf("index path %s is a directory", indexPath)
	}

	// Step 4: Create backup.
	backupPath := indexPath + ".bak"
	if err := copyFile(indexPath, backupPath); err != nil {
		return nil, fmt.Errorf("create index backup: %w", err)
	}
	backupExists := true
	defer func() {
		if backupExists {
			_ = os.Remove(backupPath)
		}
	}()

	result, err := applyUpdateContext(ctx, indexPath, cfg, dimension, embedder, records, reporter)
	if err != nil {
		// Restore from backup on any failure.
		if restoreErr := os.Rename(backupPath, indexPath); restoreErr != nil {
			return nil, fmt.Errorf("update failed: %w; additionally, backup restore failed: %v", err, restoreErr)
		}
		backupExists = false
		return nil, err
	}
	return result, nil
}

func applyUpdateContext(ctx context.Context, indexPath string, cfg *config.Config, dimension int, embedder Embedder, records *source.LoadResult, reporter RebuildProgressReporter) (*RebuildResult, error) {
	db, err := openReadWriteContext(ctx, indexPath)
	if err != nil {
		return nil, fmt.Errorf("open index for update: %w", err)
	}
	defer db.Close()

	// Step 6: Validate preconditions.
	if err := validateUpdatePreconditions(ctx, db, embedder.Fingerprint(), dimension); err != nil {
		return nil, err
	}

	// Step 7: Load stored artifact refs and content hashes.
	storedRefs, err := loadStoredArtifactRefsContext(ctx, db)
	if err != nil {
		return nil, err
	}

	// Step 8: Compute diff.
	diff := computeArtifactDiff(storedRefs, records)

	// Step 9: Load stored chunks for changed artifacts (embedding reuse).
	var partialReuse *reuseState
	if len(diff.updated) > 0 {
		partialReuse, err = loadStoredChunksForRefsContext(ctx, db, diff.updated)
		if err != nil {
			return nil, fmt.Errorf("load stored chunks for reuse: %w", err)
		}
	} else {
		partialReuse = &reuseState{artifacts: map[string]storedArtifact{}}
	}

	// Build ref lookup maps for O(1) record access.
	specsByRef := make(map[string]model.SpecRecord, len(records.Specs))
	for _, spec := range records.Specs {
		specsByRef[spec.Ref] = spec
	}
	docsByRef := make(map[string]model.DocRecord, len(records.Docs))
	for _, doc := range records.Docs {
		docsByRef[doc.Ref] = doc
	}

	// Step 10: Begin transaction and apply changes.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin update transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	result := &RebuildResult{
		Update:            true,
		IndexPath:         indexPath,
		EmbedderDimension: dimension,
		AddedCount:        len(diff.added),
		UpdatedCount:      len(diff.updated),
		RemovedCount:      len(diff.removed),
		UnchangedCount:    len(diff.unchanged),
		Repos:             repoCoverageFromRecords(records),
		Sources:           append([]source.LoadSourceSummary(nil), records.Sources...),
	}

	totalWork := len(diff.removed) + len(diff.updated) + len(diff.added)
	currentWork := 0

	// Step 10a: Delete removed artifacts.
	for _, ref := range diff.removed {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		currentWork++
		reportRebuildProgress(reporter, RebuildProgressEvent{
			Phase:       "deleting",
			ArtifactRef: ref,
			Current:     currentWork,
			Total:       totalWork,
		})
		if err := deleteArtifactDataContext(ctx, tx, ref); err != nil {
			return nil, err
		}
	}

	// Step 10b: Delete changed artifacts' data.
	for _, ref := range diff.updated {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := deleteArtifactDataContext(ctx, tx, ref); err != nil {
			return nil, err
		}
	}

	// Prepare insert statements.
	chunkStmt, err := tx.PrepareContext(ctx, `INSERT INTO chunks (artifact_ref, section, content) VALUES (?, ?, ?)`)
	if err != nil {
		return nil, fmt.Errorf("prepare chunk insert: %w", err)
	}
	defer chunkStmt.Close()

	vectorStmt, err := tx.PrepareContext(ctx, `INSERT INTO chunks_vec (chunk_id, embedding) VALUES (?, ?)`)
	if err != nil {
		return nil, fmt.Errorf("prepare chunk vector insert: %w", err)
	}
	defer vectorStmt.Close()

	// Step 10c: Insert changed artifacts.
	for _, ref := range diff.updated {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		currentWork++
		chunkCount, reusedCount, embeddedCount, err := insertRecordContext(ctx, tx, chunkStmt, vectorStmt, embedder, specsByRef, docsByRef, ref, partialReuse, RebuildProgressEvent{
			Current: currentWork,
			Total:   totalWork,
		}, reporter)
		if err != nil {
			return nil, err
		}
		result.ChunkCount += chunkCount
		result.ReusedChunkCount += reusedCount
		result.EmbeddedChunkCount += embeddedCount
	}

	// Step 10d: Insert new artifacts.
	for _, ref := range diff.added {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		currentWork++
		chunkCount, reusedCount, embeddedCount, err := insertRecordContext(ctx, tx, chunkStmt, vectorStmt, embedder, specsByRef, docsByRef, ref, partialReuse, RebuildProgressEvent{
			Current: currentWork,
			Total:   totalWork,
		}, reporter)
		if err != nil {
			return nil, err
		}
		result.ChunkCount += chunkCount
		result.ReusedChunkCount += reusedCount
		result.EmbeddedChunkCount += embeddedCount
	}

	// Count chunks from unchanged artifacts in a single query.
	unchangedChunkCount, err := countChunksForRefsContext(ctx, db, diff.unchanged)
	if err != nil {
		return nil, err
	}
	result.ChunkCount += unchangedChunkCount

	// Step 10e: Full edge rebuild.
	if _, err := tx.ExecContext(ctx, `DELETE FROM edges`); err != nil {
		return nil, fmt.Errorf("delete edges: %w", err)
	}

	edgeStmt, err := tx.PrepareContext(ctx, `INSERT INTO edges (from_ref, to_ref, edge_type) VALUES (?, ?, ?)`)
	if err != nil {
		return nil, fmt.Errorf("prepare edge insert: %w", err)
	}
	defer edgeStmt.Close()

	for _, spec := range records.Specs {
		for _, relation := range spec.Relations {
			if err := insertEdgeContext(ctx, edgeStmt, spec.Ref, relation.Ref, string(relation.Type)); err != nil {
				return nil, err
			}
			result.EdgeCount++
		}
		for _, appliesTo := range spec.AppliesTo {
			if err := insertEdgeContext(ctx, edgeStmt, spec.Ref, appliesTo, "applies_to"); err != nil {
				return nil, err
			}
			result.EdgeCount++
		}
	}

	// Step 10f: Upsert metadata.
	if err := upsertMetadataContext(ctx, tx, "source_fingerprint", sourceFingerprint(cfg)); err != nil {
		return nil, err
	}
	result.ContentFingerprint = contentFingerprint(records)
	if err := upsertMetadataContext(ctx, tx, "content_fingerprint", result.ContentFingerprint); err != nil {
		return nil, err
	}

	// Step 10g + 11: Integrity checks inside transaction.
	if err := runTransactionIntegrityChecks(ctx, tx); err != nil {
		return nil, err
	}

	// Step 12: Commit.
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit update transaction: %w", err)
	}
	tx = nil

	// Populate final counts.
	result.ArtifactCount = len(records.Specs) + len(records.Docs)
	result.SpecCount = len(records.Specs)
	result.DocCount = len(records.Docs)

	return result, nil
}

// validateUpdatePreconditions checks that the existing index is structurally
// compatible with the current embedder configuration.
func validateUpdatePreconditions(ctx context.Context, db *sql.DB, embedderFP string, embedderDim int) error {
	metadata, err := readMetadataContext(ctx, db, "schema_version", "embedder_fingerprint", "embedder_dimension")
	if err != nil {
		return fmt.Errorf("read index metadata for update: %w", err)
	}

	if stored := strings.TrimSpace(metadata["schema_version"]); stored != fmt.Sprintf("%d", schemaVersion) {
		return &UpdatePreconditionError{
			Reason: fmt.Sprintf("index schema version %q does not match expected version %d", stored, schemaVersion),
			Action: "run `pituitary index --rebuild`",
		}
	}
	if stored := strings.TrimSpace(metadata["embedder_fingerprint"]); stored != embedderFP {
		return &UpdatePreconditionError{
			Reason: fmt.Sprintf("index embedder fingerprint %q does not match configured fingerprint %q", stored, embedderFP),
			Action: "run `pituitary index --rebuild`",
		}
	}
	if stored := strings.TrimSpace(metadata["embedder_dimension"]); stored != strconv.Itoa(embedderDim) {
		return &UpdatePreconditionError{
			Reason: fmt.Sprintf("index embedder dimension %q does not match configured dimension %d", stored, embedderDim),
			Action: "run `pituitary index --rebuild`",
		}
	}
	return nil
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

	return diff
}

// loadStoredChunksForRefsContext loads stored artifact + chunk data for a
// specific set of refs, for embedding reuse during update.
func loadStoredChunksForRefsContext(ctx context.Context, db *sql.DB, refs []string) (*reuseState, error) {
	if len(refs) == 0 {
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	}

	state := &reuseState{artifacts: make(map[string]storedArtifact, len(refs))}

	// Load artifact metadata.
	for _, ref := range refs {
		var (
			title       sql.NullString
			contentHash string
		)
		err := db.QueryRowContext(ctx, `SELECT title, content_hash FROM artifacts WHERE ref = ?`, ref).Scan(&title, &contentHash)
		if err != nil {
			return nil, fmt.Errorf("query stored artifact %s: %w", ref, err)
		}
		state.artifacts[ref] = storedArtifact{
			contentHash: contentHash,
			title:       title.String,
			chunks:      map[string]storedChunk{},
		}
	}

	// Load chunks with embeddings.
	for _, ref := range refs {
		rows, err := db.QueryContext(ctx, `
SELECT c.section, c.content, v.embedding
FROM chunks c
JOIN chunks_vec v ON v.chunk_id = c.id
WHERE c.artifact_ref = ?
ORDER BY c.id`, ref)
		if err != nil {
			return nil, fmt.Errorf("query stored chunks for %s: %w", ref, err)
		}

		artifact := state.artifacts[ref]
		for rows.Next() {
			var (
				section   sql.NullString
				content   string
				embedding []byte
			)
			if err := rows.Scan(&section, &content, &embedding); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan stored chunk for %s: %w", ref, err)
			}
			artifact.chunks[reuseChunkKey(artifact.title, section.String, content)] = storedChunk{
				embedding: append([]byte(nil), embedding...),
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate stored chunks for %s: %w", ref, err)
		}
		state.artifacts[ref] = artifact
	}

	return state, nil
}

// deleteArtifactDataContext removes an artifact and its associated chunks and
// vectors from the database within a transaction.
func deleteArtifactDataContext(ctx context.Context, tx *sql.Tx, ref string) error {
	// Delete vectors (vec0 virtual table) via subquery on chunks.
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks_vec WHERE chunk_id IN (SELECT id FROM chunks WHERE artifact_ref = ?)`, ref); err != nil {
		return fmt.Errorf("delete vectors for %s: %w", ref, err)
	}
	// Delete chunks.
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE artifact_ref = ?`, ref); err != nil {
		return fmt.Errorf("delete chunks for %s: %w", ref, err)
	}
	// Delete artifact.
	if _, err := tx.ExecContext(ctx, `DELETE FROM artifacts WHERE ref = ?`, ref); err != nil {
		return fmt.Errorf("delete artifact %s: %w", ref, err)
	}
	return nil
}

// insertRecordContext inserts a single artifact (spec or doc) with its chunks,
// embeddings, and edges into the transaction.
func insertRecordContext(ctx context.Context, tx *sql.Tx, chunkStmt, vectorStmt *sql.Stmt, embedder Embedder, specsByRef map[string]model.SpecRecord, docsByRef map[string]model.DocRecord, ref string, state *reuseState, baseEvent RebuildProgressEvent, reporter RebuildProgressReporter) (chunkCount, reusedCount, embeddedCount int, err error) {
	if spec, ok := specsByRef[ref]; ok {
		if err := insertSpecArtifactContext(ctx, tx, spec); err != nil {
			return 0, 0, 0, err
		}
		plan := planArtifactReuse(spec.Title, spec.ContentHash, spec.BodyText, storedArtifactForRecord(state, ref))
		baseEvent.Phase = "chunking"
		baseEvent.ArtifactKind = model.ArtifactKindSpec
		baseEvent.ArtifactRef = ref
		return insertArtifactChunksContext(ctx, chunkStmt, vectorStmt, embedder, ref, spec.Title, plan, baseEvent, reporter)
	}
	if doc, ok := docsByRef[ref]; ok {
		if err := insertDocArtifactContext(ctx, tx, doc); err != nil {
			return 0, 0, 0, err
		}
		plan := planArtifactReuse(doc.Title, doc.ContentHash, doc.BodyText, storedArtifactForRecord(state, ref))
		baseEvent.Phase = "chunking"
		baseEvent.ArtifactKind = model.ArtifactKindDoc
		baseEvent.ArtifactRef = ref
		return insertArtifactChunksContext(ctx, chunkStmt, vectorStmt, embedder, ref, doc.Title, plan, baseEvent, reporter)
	}
	return 0, 0, 0, fmt.Errorf("record %s not found in loaded sources", ref)
}

// countChunksForRefsContext returns the total chunk count for a set of artifact refs
// in a single query.
func countChunksForRefsContext(ctx context.Context, db *sql.DB, refs []string) (int, error) {
	if len(refs) == 0 {
		return 0, nil
	}
	query := `SELECT COUNT(*) FROM chunks WHERE artifact_ref IN (` + placeholders(len(refs)) + `)`
	args := make([]any, 0, len(refs))
	for _, ref := range refs {
		args = append(args, ref)
	}
	var count int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count chunks for unchanged artifacts: %w", err)
	}
	return count, nil
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

// copyFile copies src to dst using a temporary file and rename for atomicity.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}
