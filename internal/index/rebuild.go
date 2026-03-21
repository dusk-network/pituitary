package index

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dusk-network/pituitary/internal/chunk"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

const schemaVersion = 2

// RebuildResult reports the staged rebuild outcome.
type RebuildResult struct {
	DryRun             bool   `json:"dry_run,omitempty"`
	IndexPath          string `json:"index_path"`
	ArtifactCount      int    `json:"artifact_count"`
	SpecCount          int    `json:"spec_count"`
	DocCount           int    `json:"doc_count"`
	ChunkCount         int    `json:"chunk_count"`
	EdgeCount          int    `json:"edge_count"`
	EmbedderDimension  int    `json:"embedder_dimension"`
	ContentFingerprint string `json:"content_fingerprint"`
}

// Rebuild writes a fresh staging database and atomically swaps it into place.
func Rebuild(cfg *config.Config, records *source.LoadResult) (*RebuildResult, error) {
	return RebuildContext(context.Background(), cfg, records)
}

// PrepareRebuild validates rebuild prerequisites and summarizes the pending index contents without writing a database.
func PrepareRebuild(cfg *config.Config, records *source.LoadResult) (*RebuildResult, error) {
	return PrepareRebuildContext(context.Background(), cfg, records)
}

// PrepareRebuildContext validates rebuild prerequisites and summarizes the pending index contents without writing a database.
func PrepareRebuildContext(ctx context.Context, cfg *config.Config, records *source.LoadResult) (*RebuildResult, error) {
	embedder, err := prepareRebuildContext(ctx, cfg, records)
	if err != nil {
		return nil, err
	}

	result := summarizeRebuild(records, embedder.Dimension())
	result.IndexPath = cfg.Workspace.ResolvedIndexPath
	result.DryRun = true
	return result, nil
}

// RebuildContext writes a fresh staging database and atomically swaps it into place.
func RebuildContext(ctx context.Context, cfg *config.Config, records *source.LoadResult) (*RebuildResult, error) {
	embedder, err := prepareRebuildContext(ctx, cfg, records)
	if err != nil {
		return nil, err
	}
	dimension := embedder.Dimension()

	indexPath := cfg.Workspace.ResolvedIndexPath
	stagePath := indexPath + ".new"
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		return nil, fmt.Errorf("create index directory: %w", err)
	}
	_ = os.Remove(stagePath)

	db, err := openReadWriteContext(ctx, stagePath)
	if err != nil {
		return nil, fmt.Errorf("open staging database: %w", err)
	}
	success := false
	defer func() {
		_ = db.Close()
		if !success {
			_ = os.Remove(stagePath)
		}
	}()

	result, err := buildStagingContext(ctx, db, dimension, embedder, records)
	if err != nil {
		return nil, err
	}
	result.IndexPath = indexPath

	if err := db.Close(); err != nil {
		return nil, fmt.Errorf("close staging database: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.Rename(stagePath, indexPath); err != nil {
		return nil, fmt.Errorf("swap staging database into place: %w", err)
	}
	success = true
	return result, nil
}

func prepareRebuildContext(ctx context.Context, cfg *config.Config, records *source.LoadResult) (Embedder, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if records == nil {
		return nil, fmt.Errorf("records are required")
	}

	embedder, err := newEmbedder(cfg.Runtime.Embedder)
	if err != nil {
		return nil, err
	}
	if err := CheckSQLiteReadyContext(ctx); err != nil {
		return nil, err
	}
	return embedder, nil
}

func summarizeRebuild(records *source.LoadResult, dimension int) *RebuildResult {
	result := &RebuildResult{
		ArtifactCount:     len(records.Specs) + len(records.Docs),
		SpecCount:         len(records.Specs),
		DocCount:          len(records.Docs),
		EmbedderDimension: dimension,
	}

	fingerprintParts := make([]string, 0, len(records.Specs)+len(records.Docs))
	for _, spec := range records.Specs {
		result.ChunkCount += len(chunk.Markdown(spec.Title, spec.BodyText))
		result.EdgeCount += len(spec.Relations) + len(spec.AppliesTo)
		fingerprintParts = append(fingerprintParts, spec.Ref+":"+spec.ContentHash)
	}
	for _, doc := range records.Docs {
		result.ChunkCount += len(chunk.Markdown(doc.Title, doc.BodyText))
		fingerprintParts = append(fingerprintParts, doc.Ref+":"+doc.ContentHash)
	}
	result.ContentFingerprint = fingerprint(fingerprintParts)
	return result
}

func buildStaging(db *sql.DB, dimension int, embedder Embedder, records *source.LoadResult) (*RebuildResult, error) {
	return buildStagingContext(context.Background(), db, dimension, embedder, records)
}

func buildStagingContext(ctx context.Context, db *sql.DB, dimension int, embedder Embedder, records *source.LoadResult) (*RebuildResult, error) {
	if err := createSchemaContext(ctx, db, dimension); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin rebuild transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	result := &RebuildResult{EmbedderDimension: dimension}
	var fingerprintParts []string

	if err := insertMetadataContext(ctx, tx, "schema_version", strconv.Itoa(schemaVersion)); err != nil {
		return nil, err
	}
	if err := insertMetadataContext(ctx, tx, "embedder_dimension", strconv.Itoa(dimension)); err != nil {
		return nil, err
	}

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

	edgeStmt, err := tx.PrepareContext(ctx, `INSERT INTO edges (from_ref, to_ref, edge_type) VALUES (?, ?, ?)`)
	if err != nil {
		return nil, fmt.Errorf("prepare edge insert: %w", err)
	}
	defer edgeStmt.Close()

	for _, spec := range records.Specs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := insertSpecArtifactContext(ctx, tx, spec); err != nil {
			return nil, err
		}
		result.ArtifactCount++
		result.SpecCount++
		fingerprintParts = append(fingerprintParts, spec.Ref+":"+spec.ContentHash)

		if err := insertArtifactChunksContext(ctx, chunkStmt, vectorStmt, embedder, spec.Ref, spec.Title, spec.BodyText); err != nil {
			return nil, err
		}
		result.ChunkCount += len(chunk.Markdown(spec.Title, spec.BodyText))

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

	for _, doc := range records.Docs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := insertDocArtifactContext(ctx, tx, doc); err != nil {
			return nil, err
		}
		result.ArtifactCount++
		result.DocCount++
		fingerprintParts = append(fingerprintParts, doc.Ref+":"+doc.ContentHash)

		if err := insertArtifactChunksContext(ctx, chunkStmt, vectorStmt, embedder, doc.Ref, doc.Title, doc.BodyText); err != nil {
			return nil, err
		}
		result.ChunkCount += len(chunk.Markdown(doc.Title, doc.BodyText))
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit rebuild transaction: %w", err)
	}
	tx = nil

	if err := runIntegrityChecksContext(ctx, db); err != nil {
		return nil, err
	}

	result.ContentFingerprint = fingerprint(fingerprintParts)
	if err := insertContentFingerprintContext(ctx, db, result.ContentFingerprint); err != nil {
		return nil, err
	}
	return result, nil
}

func createSchema(db *sql.DB, dimension int) error {
	return createSchemaContext(context.Background(), db, dimension)
}

func createSchemaContext(ctx context.Context, db *sql.DB, dimension int) error {
	statements := []string{
		`CREATE TABLE artifacts (
			ref           TEXT PRIMARY KEY,
			kind          TEXT NOT NULL,
			title         TEXT,
			status        TEXT,
			domain        TEXT,
			source_ref    TEXT NOT NULL,
			adapter       TEXT NOT NULL,
			body_format   TEXT NOT NULL,
			content_hash  TEXT NOT NULL,
			metadata_json TEXT NOT NULL
		)`,
		`CREATE TABLE chunks (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			artifact_ref  TEXT NOT NULL,
			section       TEXT,
			content       TEXT NOT NULL,
			FOREIGN KEY (artifact_ref) REFERENCES artifacts(ref)
		)`,
		fmt.Sprintf(`CREATE VIRTUAL TABLE chunks_vec USING vec0(
			chunk_id INTEGER PRIMARY KEY,
			embedding float[%d] distance_metric=cosine
		)`, dimension),
		`CREATE TABLE edges (
			from_ref      TEXT NOT NULL,
			to_ref        TEXT NOT NULL,
			edge_type     TEXT NOT NULL,
			PRIMARY KEY (from_ref, to_ref, edge_type)
		)`,
		`CREATE TABLE metadata (
			key           TEXT PRIMARY KEY,
			value         TEXT NOT NULL
		)`,
		`CREATE INDEX idx_artifacts_kind_status_domain
			ON artifacts(kind, status, domain)`,
		`CREATE INDEX idx_chunks_artifact_ref
			ON chunks(artifact_ref)`,
		`CREATE INDEX idx_edges_from_ref_type
			ON edges(from_ref, edge_type)`,
		`CREATE INDEX idx_edges_to_ref_type
			ON edges(to_ref, edge_type)`,
	}

	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func insertSpecArtifact(tx *sql.Tx, spec model.SpecRecord) error {
	return insertSpecArtifactContext(context.Background(), tx, spec)
}

func insertSpecArtifactContext(ctx context.Context, tx *sql.Tx, spec model.SpecRecord) error {
	metadataJSON, err := json.Marshal(spec.Metadata)
	if err != nil {
		return fmt.Errorf("marshal spec %s metadata: %w", spec.Ref, err)
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO artifacts (ref, kind, title, status, domain, source_ref, adapter, body_format, content_hash, metadata_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		spec.Ref,
		spec.Kind,
		spec.Title,
		spec.Status,
		spec.Domain,
		spec.SourceRef,
		config.AdapterFilesystem,
		spec.BodyFormat,
		spec.ContentHash,
		string(metadataJSON),
	)
	if err != nil {
		return fmt.Errorf("insert spec artifact %s: %w", spec.Ref, err)
	}
	return nil
}

func insertDocArtifact(tx *sql.Tx, doc model.DocRecord) error {
	return insertDocArtifactContext(context.Background(), tx, doc)
}

func insertDocArtifactContext(ctx context.Context, tx *sql.Tx, doc model.DocRecord) error {
	metadataJSON, err := json.Marshal(doc.Metadata)
	if err != nil {
		return fmt.Errorf("marshal doc %s metadata: %w", doc.Ref, err)
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO artifacts (ref, kind, title, status, domain, source_ref, adapter, body_format, content_hash, metadata_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		doc.Ref,
		doc.Kind,
		doc.Title,
		nil,
		nil,
		doc.SourceRef,
		config.AdapterFilesystem,
		doc.BodyFormat,
		doc.ContentHash,
		string(metadataJSON),
	)
	if err != nil {
		return fmt.Errorf("insert doc artifact %s: %w", doc.Ref, err)
	}
	return nil
}

func insertArtifactChunks(chunkStmt, vectorStmt *sql.Stmt, embedder Embedder, artifactRef, title, body string) error {
	return insertArtifactChunksContext(context.Background(), chunkStmt, vectorStmt, embedder, artifactRef, title, body)
}

func insertArtifactChunksContext(ctx context.Context, chunkStmt, vectorStmt *sql.Stmt, embedder Embedder, artifactRef, title, body string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sections := chunk.Markdown(title, body)
	if len(sections) == 0 {
		return nil
	}

	texts := make([]string, 0, len(sections))
	for _, section := range sections {
		if err := ctx.Err(); err != nil {
			return err
		}
		texts = append(texts, textForEmbedding(title, section))
	}

	vectors, err := embedder.EmbedTexts(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed chunks for %s: %w", artifactRef, err)
	}
	if len(vectors) != len(sections) {
		return fmt.Errorf("embed chunks for %s: returned %d vector(s) for %d section(s)", artifactRef, len(vectors), len(sections))
	}

	for i, section := range sections {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunkID, err := insertChunkContext(ctx, chunkStmt, artifactRef, section.Heading, section.Body)
		if err != nil {
			return err
		}
		if err := insertChunkVectorContext(ctx, vectorStmt, chunkID, embedder.Dimension(), vectors[i]); err != nil {
			return err
		}
	}

	return nil
}

func insertChunk(stmt *sql.Stmt, artifactRef, section, content string) (int64, error) {
	return insertChunkContext(context.Background(), stmt, artifactRef, section, content)
}

func insertChunkContext(ctx context.Context, stmt *sql.Stmt, artifactRef, section, content string) (int64, error) {
	result, err := stmt.ExecContext(ctx, artifactRef, section, content)
	if err != nil {
		return 0, fmt.Errorf("insert chunk for %s: %w", artifactRef, err)
	}
	chunkID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read chunk id for %s: %w", artifactRef, err)
	}
	return chunkID, nil
}

func insertChunkVector(stmt *sql.Stmt, chunkID int64, _ int, vector []float64) error {
	return insertChunkVectorContext(context.Background(), stmt, chunkID, 0, vector)
}

func insertChunkVectorContext(ctx context.Context, stmt *sql.Stmt, chunkID int64, _ int, vector []float64) error {
	embeddingBlob, err := encodeVectorBlob(vector)
	if err != nil {
		return fmt.Errorf("encode chunk vector %d: %w", chunkID, err)
	}
	if _, err := stmt.ExecContext(ctx, chunkID, embeddingBlob); err != nil {
		return fmt.Errorf("insert chunk vector %d: %w", chunkID, err)
	}
	return nil
}

func textForEmbedding(title string, section chunk.Section) string {
	parts := make([]string, 0, 3)
	if trimmed := strings.TrimSpace(title); trimmed != "" {
		parts = append(parts, trimmed)
	}
	if trimmed := strings.TrimSpace(section.Heading); trimmed != "" && trimmed != strings.TrimSpace(title) {
		parts = append(parts, trimmed)
	}
	if trimmed := strings.TrimSpace(section.Body); trimmed != "" {
		parts = append(parts, trimmed)
	}
	return strings.Join(parts, "\n\n")
}

func insertEdge(stmt *sql.Stmt, fromRef, toRef, edgeType string) error {
	return insertEdgeContext(context.Background(), stmt, fromRef, toRef, edgeType)
}

func insertEdgeContext(ctx context.Context, stmt *sql.Stmt, fromRef, toRef, edgeType string) error {
	if _, err := stmt.ExecContext(ctx, fromRef, toRef, edgeType); err != nil {
		return fmt.Errorf("insert edge %s -> %s (%s): %w", fromRef, toRef, edgeType, err)
	}
	return nil
}

func insertMetadata(tx *sql.Tx, key, value string) error {
	return insertMetadataContext(context.Background(), tx, key, value)
}

func insertMetadataContext(ctx context.Context, tx *sql.Tx, key, value string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO metadata (key, value) VALUES (?, ?)`, key, value); err != nil {
		return fmt.Errorf("insert metadata %s: %w", key, err)
	}
	return nil
}

func insertContentFingerprint(db *sql.DB, value string) error {
	return insertContentFingerprintContext(context.Background(), db, value)
}

func insertContentFingerprintContext(ctx context.Context, db *sql.DB, value string) error {
	if _, err := db.ExecContext(ctx, `INSERT INTO metadata (key, value) VALUES (?, ?)`, "content_fingerprint", value); err != nil {
		return fmt.Errorf("insert content fingerprint: %w", err)
	}
	return nil
}

func runIntegrityChecks(db *sql.DB) error {
	return runIntegrityChecksContext(context.Background(), db)
}

func runIntegrityChecksContext(ctx context.Context, db *sql.DB) error {
	row := db.QueryRowContext(ctx, `PRAGMA integrity_check`)
	var result string
	if err := row.Scan(&result); err != nil {
		return fmt.Errorf("run integrity_check: %w", err)
	}
	if strings.ToLower(result) != "ok" {
		return fmt.Errorf("integrity_check failed: %s", result)
	}

	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
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

func fixtureDimension(modelName string) (int, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		modelName = "fixture-8d"
	}
	if strings.HasPrefix(modelName, "fixture-") && strings.HasSuffix(modelName, "d") {
		raw := strings.TrimSuffix(strings.TrimPrefix(modelName, "fixture-"), "d")
		dimension, err := strconv.Atoi(raw)
		if err != nil || dimension <= 0 {
			return 0, fmt.Errorf("runtime.embedder.model %q does not encode a valid fixture dimension", modelName)
		}
		return dimension, nil
	}
	return 0, fmt.Errorf("runtime.embedder.model %q is not a supported fixture embedder", modelName)
}

func fingerprint(parts []string) string {
	sort.Strings(parts)
	hash := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(hash[:])
}
