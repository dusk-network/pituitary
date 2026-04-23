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
	"time"

	"github.com/dusk-network/pituitary/internal/ast"
	pchunk "github.com/dusk-network/pituitary/internal/chunk"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
	stcorpus "github.com/dusk-network/stroma/v2/corpus"
	stindex "github.com/dusk-network/stroma/v2/index"
)

const schemaVersion = 10

// RebuildResult reports the staged rebuild outcome.
// When Update is true, the result describes an incremental update instead of a full rebuild.
type RebuildResult struct {
	DryRun                bool                       `json:"dry_run,omitempty"`
	Update                bool                       `json:"update,omitempty"`
	IndexPath             string                     `json:"index_path"`
	FullRebuild           bool                       `json:"full_rebuild,omitempty"`
	ArtifactCount         int                        `json:"artifact_count"`
	SpecCount             int                        `json:"spec_count"`
	DocCount              int                        `json:"doc_count"`
	ChunkCount            int                        `json:"chunk_count"`
	EdgeCount             int                        `json:"edge_count"`
	EmbedderDimension     int                        `json:"embedder_dimension"`
	ReusedArtifactCount   int                        `json:"reused_artifact_count,omitempty"`
	ReusedChunkCount      int                        `json:"reused_chunk_count,omitempty"`
	EmbeddedChunkCount    int                        `json:"embedded_chunk_count,omitempty"`
	AddedCount            int                        `json:"added_count,omitempty"`
	UpdatedCount          int                        `json:"updated_count,omitempty"`
	RemovedCount          int                        `json:"removed_count,omitempty"`
	UnchangedCount        int                        `json:"unchanged_count,omitempty"`
	InferredEdgeCount     int                        `json:"inferred_edge_count,omitempty"`
	InferAppliesToEnabled bool                       `json:"infer_applies_to_enabled"`
	ContentFingerprint    string                     `json:"content_fingerprint"`
	Repos                 []RepoCoverage             `json:"repo_coverage,omitempty"`
	Sources               []source.LoadSourceSummary `json:"sources,omitempty"`
	Delta                 *GovernanceDelta           `json:"delta,omitempty"`
}

// RebuildOptions controls optional rebuild behavior.
type RebuildOptions struct {
	Full bool
}

// RebuildProgressEvent reports one text-mode rebuild progress update.
type RebuildProgressEvent struct {
	Phase        string `json:"phase"`
	ArtifactKind string `json:"artifact_kind"`
	ArtifactRef  string `json:"artifact_ref"`
	Current      int    `json:"current"`
	Total        int    `json:"total"`
	ChunkCount   int    `json:"chunk_count,omitempty"`
}

// RebuildProgressReporter receives rebuild progress events.
type RebuildProgressReporter func(RebuildProgressEvent)

// Rebuild writes a fresh staging database and atomically swaps it into place.
func Rebuild(cfg *config.Config, records *source.LoadResult) (*RebuildResult, error) {
	return RebuildContextWithOptions(context.Background(), cfg, records, RebuildOptions{})
}

// RebuildWithProgressContext writes a fresh staging database, atomically swaps it into place,
// and reports chunking/embedding progress through the provided callback.
func RebuildWithProgressContext(ctx context.Context, cfg *config.Config, records *source.LoadResult, reporter RebuildProgressReporter) (*RebuildResult, error) {
	return RebuildWithProgressContextAndOptions(ctx, cfg, records, RebuildOptions{}, reporter)
}

// PrepareRebuild validates rebuild prerequisites and summarizes the pending index contents without writing a database.
func PrepareRebuild(cfg *config.Config, records *source.LoadResult) (*RebuildResult, error) {
	return PrepareRebuildContextWithOptions(context.Background(), cfg, records, RebuildOptions{})
}

// PrepareRebuildContext validates rebuild prerequisites and summarizes the pending index contents without writing a database.
func PrepareRebuildContext(ctx context.Context, cfg *config.Config, records *source.LoadResult) (*RebuildResult, error) {
	return PrepareRebuildContextWithOptions(ctx, cfg, records, RebuildOptions{})
}

// PrepareRebuildContextWithOptions validates rebuild prerequisites and summarizes the pending index contents without writing a database.
func PrepareRebuildContextWithOptions(ctx context.Context, cfg *config.Config, records *source.LoadResult, options RebuildOptions) (*RebuildResult, error) {
	embedder, err := prepareRebuildContext(ctx, cfg, records)
	if err != nil {
		return nil, err
	}
	dimension, err := embedder.Dimension(ctx)
	if err != nil {
		return nil, err
	}
	if err := prepareDryRunPreflightContext(ctx, cfg.Workspace.ResolvedIndexPath, dimension); err != nil {
		return nil, err
	}
	currentSnapshotPath, err := currentStromaSnapshotPathContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return nil, err
	}
	reuseState, err := loadReuseStateContext(ctx, currentSnapshotPath, embedder.Fingerprint(), dimension, options)
	if err != nil {
		return nil, err
	}

	result := summarizeRebuild(records, dimension, reuseState, options)
	result.IndexPath = cfg.Workspace.ResolvedIndexPath
	result.DryRun = true
	result.InferAppliesToEnabled = resolveInferAppliesTo(cfg, records.Specs)
	return result, nil
}

func prepareDryRunPreflightContext(ctx context.Context, indexPath string, dimension int) error {
	createdDirs, err := ensureIndexDirectory(indexPath)
	if err != nil {
		return err
	}
	defer cleanupCreatedDirectories(createdDirs)

	if err := validateIndexTargetPath(indexPath); err != nil {
		return err
	}
	stagePath := indexPath + ".new"
	if err := validateStaleStagePath(stagePath); err != nil {
		return err
	}
	probePath, err := allocateDryRunProbePath(filepath.Dir(indexPath))
	if err != nil {
		return err
	}
	return probeStagingDatabaseContext(ctx, probePath, dimension)
}

func validateBusinessIndexPublishPreflight(indexPath string) error {
	if _, err := ensureIndexDirectory(indexPath); err != nil {
		return err
	}
	if err := validateIndexTargetPath(indexPath); err != nil {
		return err
	}
	return validateStaleStagePath(indexPath + ".new")
}

// RebuildContext writes a fresh staging database and atomically swaps it into place.
func RebuildContext(ctx context.Context, cfg *config.Config, records *source.LoadResult) (*RebuildResult, error) {
	return RebuildContextWithOptions(ctx, cfg, records, RebuildOptions{})
}

// RebuildContextWithOptions writes a fresh staging database with explicit rebuild options.
func RebuildContextWithOptions(ctx context.Context, cfg *config.Config, records *source.LoadResult, options RebuildOptions) (*RebuildResult, error) {
	return rebuildContext(ctx, cfg, records, options, nil)
}

// RebuildWithProgressContextAndOptions writes a fresh staging database with progress reporting and explicit rebuild options.
func RebuildWithProgressContextAndOptions(ctx context.Context, cfg *config.Config, records *source.LoadResult, options RebuildOptions, reporter RebuildProgressReporter) (*RebuildResult, error) {
	return rebuildContext(ctx, cfg, records, options, reporter)
}

func rebuildContext(ctx context.Context, cfg *config.Config, records *source.LoadResult, options RebuildOptions, reporter RebuildProgressReporter) (*RebuildResult, error) {
	embedder, err := prepareRebuildContext(ctx, cfg, records)
	if err != nil {
		return nil, err
	}
	dimension, err := embedder.Dimension(ctx)
	if err != nil {
		return nil, err
	}
	indexPath := cfg.Workspace.ResolvedIndexPath
	currentSnapshotPath, err := currentStromaSnapshotPathContext(ctx, indexPath)
	if err != nil {
		return nil, err
	}
	reuseState, err := loadReuseStateContext(ctx, currentSnapshotPath, embedder.Fingerprint(), dimension, options)
	if err != nil {
		return nil, err
	}

	contentFP := contentFingerprint(records)
	snapshotPath := stromaSnapshotPathForContent(indexPath, contentFP)
	if err := validateBusinessIndexPublishPreflight(indexPath); err != nil {
		return nil, err
	}

	corpusRecords, err := corpusRecordsFromLoadResult(records)
	if err != nil {
		return nil, err
	}
	emitPlannedRebuildProgress(records, reuseState, reporter)

	chunkPolicy, err := pchunk.Resolve(chunkConfigFromRuntime(cfg.Runtime.Chunking))
	if err != nil {
		return nil, fmt.Errorf("resolve chunk policy: %w", err)
	}
	contextualizer, err := pchunk.ResolveContextualizer(chunkContextualizerFromRuntime(cfg.Runtime.Chunking))
	if err != nil {
		return nil, fmt.Errorf("resolve chunk contextualizer: %w", err)
	}

	stromaOptions := stindex.BuildOptions{
		Path:           snapshotPath,
		Embedder:       embedder,
		ChunkPolicy:    chunkPolicy,
		Contextualizer: contextualizer,
	}
	if !options.Full {
		// Stroma rebuilds through Path+".new", so reusing the currently published
		// snapshot remains safe even when the content-addressed path is unchanged.
		stromaOptions.ReuseFromPath = currentSnapshotPath
	}
	if _, err := stindex.Rebuild(ctx, corpusRecords, stromaOptions); err != nil {
		return nil, fmt.Errorf("build stroma snapshot: %w", err)
	}

	// #344 AC: under LateChunkPolicy we must actually see parent/leaf
	// linkage land in the snapshot; validate loudly here so a
	// regression in stroma or in the resolver cannot ship quietly.
	if docLateChunkActive(chunkPolicy) {
		if err := validateDocParentChainContext(ctx, snapshotPath); err != nil {
			return nil, err
		}
	}

	result := summarizeRebuild(records, dimension, reuseState, options)
	result.ContentFingerprint = contentFP
	result.IndexPath = indexPath

	// summarizeRebuild computes ChunkCount predictively via the
	// pre-router Markdown sectioner, which systematically undercounts
	// doc chunks once LateChunkPolicy emits parent+leaf hierarchies.
	// Refresh the count from the freshly-published snapshot so the
	// reported chunk growth reflects reality — operators need to see
	// the real index-size / embedding-cost footprint of the #344
	// default to evaluate it.
	if err := refreshRebuildChunkCountFromSnapshotContext(ctx, snapshotPath, result); err != nil {
		return nil, err
	}

	if err := publishBusinessIndexContext(ctx, cfg, records, result, snapshotPath); err != nil {
		return nil, err
	}
	return result, nil
}

// refreshRebuildChunkCountFromSnapshotContext overwrites result.ChunkCount
// with the ground-truth count from the published stroma snapshot. This
// replaces the Markdown-based predictive count that summarizeRebuild
// produces, which is wrong for docs under LateChunkPolicy where each
// record can emit a parent plus multiple leaves.
func refreshRebuildChunkCountFromSnapshotContext(ctx context.Context, snapshotPath string, result *RebuildResult) error {
	if result == nil || snapshotPath == "" {
		return nil
	}
	snapshot, err := stindex.OpenSnapshot(ctx, snapshotPath)
	if err != nil {
		return fmt.Errorf("open stroma snapshot for chunk-count refresh: %w", err)
	}
	defer snapshot.Close()
	stats, err := snapshot.Stats(ctx)
	if err != nil {
		return fmt.Errorf("read stroma snapshot stats: %w", err)
	}
	result.ChunkCount = stats.ChunkCount
	return nil
}

func corpusRecordsFromLoadResult(records *source.LoadResult) ([]stcorpus.Record, error) {
	if records == nil {
		return nil, fmt.Errorf("records are required")
	}

	corpusRecords := make([]stcorpus.Record, 0, len(records.Specs)+len(records.Docs))
	for _, spec := range records.Specs {
		record, err := corpusRecordFromSpec(spec)
		if err != nil {
			return nil, err
		}
		corpusRecords = append(corpusRecords, record)
	}
	for _, doc := range records.Docs {
		record, err := corpusRecordFromDoc(doc)
		if err != nil {
			return nil, err
		}
		corpusRecords = append(corpusRecords, record)
	}
	return corpusRecords, nil
}

// chunkConfigFromRuntime adapts the config-layer chunking shape to the
// chunk package's Config. It is a pure data translation: config holds
// raw TOML-validated values, chunk.Resolve turns them into a
// stroma chunk.Policy.
func chunkConfigFromRuntime(cfg config.ChunkingConfig) pchunk.Config {
	return pchunk.Config{
		Spec: chunkKindFromConfig(cfg.Spec),
		Doc:  chunkKindFromConfig(cfg.Doc),
	}
}

func chunkKindFromConfig(kind config.ChunkingKindConfig) pchunk.KindConfig {
	return pchunk.KindConfig{
		Policy:             kind.Policy,
		MaxTokens:          kind.MaxTokens,
		OverlapTokens:      kind.OverlapTokens,
		MaxSections:        kind.MaxSections,
		ChildMaxTokens:     kind.ChildMaxTokens,
		ChildOverlapTokens: kind.ChildOverlapTokens,
	}
}

// chunkContextualizerFromRuntime adapts the config-layer contextualizer
// shape to the chunk package's ContextualizerConfig. Format names are
// duplicated in the config package so this is a pure string copy; the
// chunk package validates the format at resolve time.
func chunkContextualizerFromRuntime(cfg config.ChunkingConfig) pchunk.ContextualizerConfig {
	return pchunk.ContextualizerConfig{
		Format: pchunk.PrefixFormat(cfg.Contextualizer.Format),
	}
}

func corpusRecordFromSpec(spec model.SpecRecord) (stcorpus.Record, error) {
	metadata := cloneMetadata(spec.Metadata)

	return stcorpus.Record{
		Ref:         spec.Ref,
		Kind:        spec.Kind,
		Title:       spec.Title,
		SourceRef:   spec.SourceRef,
		BodyFormat:  spec.BodyFormat,
		BodyText:    spec.BodyText,
		ContentHash: spec.ContentHash,
		Metadata:    metadata,
	}.Normalize()
}

func corpusRecordFromDoc(doc model.DocRecord) (stcorpus.Record, error) {
	return stcorpus.Record{
		Ref:         doc.Ref,
		Kind:        doc.Kind,
		Title:       doc.Title,
		SourceRef:   doc.SourceRef,
		BodyFormat:  doc.BodyFormat,
		BodyText:    doc.BodyText,
		ContentHash: doc.ContentHash,
		Metadata:    cloneMetadata(doc.Metadata),
	}.Normalize()
}

func publishBusinessIndexContext(ctx context.Context, cfg *config.Config, records *source.LoadResult, result *RebuildResult, snapshotPath string) error {
	indexPath := cfg.Workspace.ResolvedIndexPath
	stagePath, err := prepareStagingPath(indexPath)
	if err != nil {
		return err
	}

	success := false
	defer func() {
		if !success {
			_ = os.Remove(stagePath)
		}
	}()

	db, err := openReadWriteContext(ctx, stagePath)
	if err != nil {
		return fmt.Errorf("open staging database: %w", err)
	}
	if err := createSchemaContext(ctx, db, result.EmbedderDimension); err != nil {
		_ = db.Close()
		return fmt.Errorf("create staging schema: %w", err)
	}
	if err := finalizeBusinessIndexContext(ctx, db, cfg, records, result, snapshotPath); err != nil {
		_ = db.Close()
		return err
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("close staging database: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(stagePath, indexPath); err != nil {
		return fmt.Errorf("swap staging database into place: %w", err)
	}
	success = true
	return nil
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func finalizeBusinessIndexContext(ctx context.Context, db *sql.DB, cfg *config.Config, records *source.LoadResult, result *RebuildResult, snapshotPath string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin business index transaction: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	for _, spec := range records.Specs {
		if err := insertSpecArtifactContext(ctx, tx, spec); err != nil {
			return err
		}
	}
	for _, doc := range records.Docs {
		if err := insertDocArtifactContext(ctx, tx, doc); err != nil {
			return err
		}
	}

	edgeStmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO edges (from_ref, to_ref, edge_type, edge_source, valid_from, valid_to, confidence, confidence_score) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare edge insert: %w", err)
	}
	defer edgeStmt.Close()

	rebuildTime := time.Now().UTC().Format("2006-01-02")
	rebuildTimePtr := &rebuildTime
	edgeCount := 0

	for _, spec := range records.Specs {
		var edgeValidTo *string
		if spec.Status == model.StatusSuperseded || spec.Status == model.StatusDeprecated {
			edgeValidTo = rebuildTimePtr
		}
		for _, relation := range spec.Relations {
			if err := insertEdgeContext(ctx, edgeStmt, spec.Ref, relation.Ref, string(relation.Type), "manual", rebuildTimePtr, edgeValidTo, ConfidenceExtracted); err != nil {
				return err
			}
			edgeCount++
		}
		for _, appliesTo := range spec.AppliesTo {
			if err := insertEdgeContext(ctx, edgeStmt, spec.Ref, appliesTo, "applies_to", "manual", rebuildTimePtr, edgeValidTo, ConfidenceExtracted); err != nil {
				return err
			}
			edgeCount++
		}
	}

	inferredCount, err := inferASTEdgesContext(ctx, tx, edgeStmt, cfg, records.Specs, rebuildTimePtr)
	if err != nil {
		return fmt.Errorf("infer AST edges: %w", err)
	}
	edgeCount += inferredCount

	if err := upsertMetadataContext(ctx, tx, "schema_version", strconv.Itoa(schemaVersion)); err != nil {
		return err
	}
	if err := upsertMetadataContext(ctx, tx, "stroma_snapshot_path", snapshotPath); err != nil {
		return err
	}
	if err := upsertMetadataContext(ctx, tx, "embedder_dimension", strconv.Itoa(result.EmbedderDimension)); err != nil {
		return err
	}
	embedderFingerprint, err := ConfiguredEmbedderFingerprint(cfg.Runtime.Embedder)
	if err != nil {
		return err
	}
	if err := upsertMetadataContext(ctx, tx, "embedder_fingerprint", embedderFingerprint); err != nil {
		return err
	}
	if err := upsertMetadataContext(ctx, tx, "source_fingerprint", sourceFingerprint(cfg)); err != nil {
		return err
	}
	if err := upsertMetadataContext(ctx, tx, "chunking_config_fingerprint", chunkingConfigFingerprint(cfg.Runtime.Chunking)); err != nil {
		return err
	}
	if err := upsertMetadataContext(ctx, tx, "infer_applies_to_enabled", strconv.FormatBool(resolveInferAppliesTo(cfg, records.Specs))); err != nil {
		return err
	}
	if manifest := sourceManifestJSON(cfg); manifest != "" {
		if err := upsertMetadataContext(ctx, tx, "source_manifest", manifest); err != nil {
			return err
		}
	}

	result.ContentFingerprint = contentFingerprint(records)
	if err := upsertMetadataContext(ctx, tx, "content_fingerprint", result.ContentFingerprint); err != nil {
		return err
	}

	if err := runTransactionIntegrityChecks(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit business index transaction: %w", err)
	}
	tx = nil

	if err := runIntegrityChecksContext(ctx, db); err != nil {
		return err
	}

	result.EdgeCount = edgeCount
	result.InferredEdgeCount = inferredCount
	result.InferAppliesToEnabled = resolveInferAppliesTo(cfg, records.Specs)
	return nil
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func emitPlannedRebuildProgress(records *source.LoadResult, state *reuseState, reporter RebuildProgressReporter) {
	if reporter == nil || records == nil {
		return
	}

	totalArtifacts := len(records.Specs) + len(records.Docs)
	currentArtifact := 0

	for _, spec := range records.Specs {
		currentArtifact++
		plan := planArtifactReuse(spec.Title, spec.ContentHash, spec.BodyText, storedArtifactForRecord(state, spec.Ref))
		reportRebuildProgress(reporter, RebuildProgressEvent{
			Phase:        "chunking",
			ArtifactKind: model.ArtifactKindSpec,
			ArtifactRef:  spec.Ref,
			Current:      currentArtifact,
			Total:        totalArtifacts,
			ChunkCount:   len(plan.sections),
		})
		if plan.embeddedChunkCount > 0 {
			reportRebuildProgress(reporter, RebuildProgressEvent{
				Phase:        "embedding",
				ArtifactKind: model.ArtifactKindSpec,
				ArtifactRef:  spec.Ref,
				Current:      currentArtifact,
				Total:        totalArtifacts,
				ChunkCount:   plan.embeddedChunkCount,
			})
		}
	}

	for _, doc := range records.Docs {
		currentArtifact++
		plan := planArtifactReuse(doc.Title, doc.ContentHash, doc.BodyText, storedArtifactForRecord(state, doc.Ref))
		reportRebuildProgress(reporter, RebuildProgressEvent{
			Phase:        "chunking",
			ArtifactKind: model.ArtifactKindDoc,
			ArtifactRef:  doc.Ref,
			Current:      currentArtifact,
			Total:        totalArtifacts,
			ChunkCount:   len(plan.sections),
		})
		if plan.embeddedChunkCount > 0 {
			reportRebuildProgress(reporter, RebuildProgressEvent{
				Phase:        "embedding",
				ArtifactKind: model.ArtifactKindDoc,
				ArtifactRef:  doc.Ref,
				Current:      currentArtifact,
				Total:        totalArtifacts,
				ChunkCount:   plan.embeddedChunkCount,
			})
		}
	}
}

func prepareRebuildContext(ctx context.Context, cfg *config.Config, records *source.LoadResult) (Embedder, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if records == nil {
		return nil, fmt.Errorf("records are required")
	}
	if err := ValidateRelationGraph(records.Specs); err != nil {
		return nil, err
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

func prepareStagingPath(indexPath string) (string, error) {
	if _, err := ensureIndexDirectory(indexPath); err != nil {
		return "", err
	}
	if err := validateIndexTargetPath(indexPath); err != nil {
		return "", err
	}

	stagePath := indexPath + ".new"
	if err := removeStaleStagePath(stagePath); err != nil {
		return "", err
	}
	return stagePath, nil
}

func ensureIndexDirectory(indexPath string) ([]string, error) {
	createdDirs, err := ensureDirectoryPath(filepath.Dir(indexPath))
	if err != nil {
		return nil, fmt.Errorf("create index directory: %w", err)
	}
	return createdDirs, nil
}

func ensureDirectoryPath(dirPath string) ([]string, error) {
	info, err := os.Stat(dirPath)
	switch {
	case err == nil:
		if !info.IsDir() {
			return nil, fmt.Errorf("mkdir %s: not a directory", dirPath)
		}
		return nil, nil
	case !os.IsNotExist(err):
		return nil, err
	}

	missing := make([]string, 0, 4)
	for current := dirPath; ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		switch {
		case err == nil:
			if !info.IsDir() {
				return nil, fmt.Errorf("mkdir %s: not a directory", current)
			}
			created := make([]string, 0, len(missing))
			for i := len(missing) - 1; i >= 0; i-- {
				// #nosec G301 -- staging directories are local workspace state, not secret material.
				if err := os.Mkdir(missing[i], 0o755); err != nil {
					cleanupCreatedDirectories(created)
					if os.IsExist(err) {
						if info, statErr := os.Stat(missing[i]); statErr == nil && info.IsDir() {
							continue
						}
					}
					return nil, err
				}
				created = append(created, missing[i])
			}
			return created, nil
		case os.IsNotExist(err):
			missing = append(missing, current)
			parent := filepath.Dir(current)
			if parent == current {
				return nil, fmt.Errorf("mkdir %s: path does not exist", current)
			}
		default:
			return nil, err
		}
	}
}

func cleanupCreatedDirectories(paths []string) {
	for i := len(paths) - 1; i >= 0; i-- {
		_ = os.Remove(paths[i])
	}
}

func validateIndexTargetPath(indexPath string) error {
	info, err := os.Stat(indexPath)
	switch {
	case err == nil && info.IsDir():
		return fmt.Errorf("index path %s is a directory", indexPath)
	case err == nil:
		return nil
	case os.IsNotExist(err):
		return nil
	default:
		return fmt.Errorf("stat index path %s: %w", indexPath, err)
	}
}

func validateStaleStagePath(stagePath string) error {
	info, err := os.Lstat(stagePath)
	switch {
	case os.IsNotExist(err):
		return nil
	case err != nil:
		return fmt.Errorf("stat staging database %s: %w", stagePath, err)
	case !info.IsDir():
		return nil
	}

	entries, err := os.ReadDir(stagePath)
	if err != nil {
		return fmt.Errorf("remove stale staging database: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("remove stale staging database: remove %s: directory not empty", stagePath)
	}
	return nil
}

func removeStaleStagePath(stagePath string) error {
	if err := os.Remove(stagePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale staging database: %w", err)
	}
	return nil
}

func allocateDryRunProbePath(dirPath string) (string, error) {
	file, err := os.CreateTemp(dirPath, ".pituitary-dry-run-*.db")
	if err != nil {
		return "", fmt.Errorf("open staging database: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close staging database probe: %w", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove staging database probe seed: %w", err)
	}
	return path, nil
}

func probeStagingDatabaseContext(ctx context.Context, stagePath string, dimension int) error {
	db, err := openReadWriteContext(ctx, stagePath)
	if err != nil {
		return fmt.Errorf("open staging database: %w", err)
	}
	if err := createSchemaContext(ctx, db, dimension); err != nil {
		_ = db.Close()
		_ = os.Remove(stagePath)
		return fmt.Errorf("create schema: %w", err)
	}
	if err := db.Close(); err != nil {
		_ = os.Remove(stagePath)
		return fmt.Errorf("close staging database: %w", err)
	}
	if err := os.Remove(stagePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove staging database probe: %w", err)
	}
	return nil
}

func createSchemaContext(ctx context.Context, db *sql.DB, dimension int) error {
	_ = dimension
	statements := []string{
		`CREATE TABLE artifacts (
			ref           TEXT PRIMARY KEY,
			kind          TEXT NOT NULL,
			title         TEXT,
			status        TEXT,
			domain        TEXT,
			source_ref    TEXT NOT NULL,
			adapter       TEXT NOT NULL DEFAULT 'filesystem',
			content_hash  TEXT NOT NULL,
			metadata_json TEXT NOT NULL,
			valid_from    TEXT,
			valid_to      TEXT
		)`,
		`CREATE TABLE edges (
			from_ref         TEXT NOT NULL,
			to_ref           TEXT NOT NULL,
			edge_type        TEXT NOT NULL,
			edge_source      TEXT NOT NULL DEFAULT 'manual',
			valid_from       TEXT,
			valid_to         TEXT,
			confidence       TEXT NOT NULL DEFAULT 'extracted',
			confidence_score REAL NOT NULL DEFAULT 1.0,
			PRIMARY KEY (from_ref, to_ref, edge_type)
		)`,
		`CREATE TABLE ast_cache (
			content_hash   TEXT PRIMARY KEY,
			path           TEXT NOT NULL,
			symbols_json   TEXT NOT NULL,
			rationale_json TEXT NOT NULL DEFAULT '[]'
		)`,
		`CREATE TABLE metadata (
			key           TEXT PRIMARY KEY,
			value         TEXT NOT NULL
		)`,
		`CREATE INDEX idx_artifacts_kind_status_domain
			ON artifacts(kind, status, domain)`,
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

func insertSpecArtifactContext(ctx context.Context, tx *sql.Tx, spec model.SpecRecord) error {
	metadataJSON, err := json.Marshal(spec.Metadata)
	if err != nil {
		return fmt.Errorf("marshal spec %s metadata: %w", spec.Ref, err)
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO artifacts (ref, kind, title, status, domain, source_ref, adapter, content_hash, metadata_json, valid_from, valid_to)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)`,
		spec.Ref,
		spec.Kind,
		spec.Title,
		nullableString(strings.TrimSpace(spec.Status)),
		nullableString(strings.TrimSpace(spec.Domain)),
		spec.SourceRef,
		adapterFromMetadata(spec.Metadata),
		spec.ContentHash,
		string(metadataJSON),
	)
	if err != nil {
		return fmt.Errorf("insert spec artifact %s: %w", spec.Ref, err)
	}
	return nil
}

func insertDocArtifactContext(ctx context.Context, tx *sql.Tx, doc model.DocRecord) error {
	metadataJSON, err := json.Marshal(doc.Metadata)
	if err != nil {
		return fmt.Errorf("marshal doc %s metadata: %w", doc.Ref, err)
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO artifacts (ref, kind, title, status, domain, source_ref, adapter, content_hash, metadata_json, valid_from, valid_to)
		 VALUES (?, ?, ?, NULL, NULL, ?, ?, ?, ?, NULL, NULL)`,
		doc.Ref,
		doc.Kind,
		doc.Title,
		doc.SourceRef,
		adapterFromMetadata(doc.Metadata),
		doc.ContentHash,
		string(metadataJSON),
	)
	if err != nil {
		return fmt.Errorf("insert doc artifact %s: %w", doc.Ref, err)
	}
	return nil
}

// adapterFromMetadata returns the source adapter recorded in metadata, falling
// back to the filesystem adapter when the key is absent or blank.
func adapterFromMetadata(metadata map[string]string) string {
	if v := strings.TrimSpace(metadata["source_adapter"]); v != "" {
		return v
	}
	return config.AdapterFilesystem
}

func reportRebuildProgress(reporter RebuildProgressReporter, event RebuildProgressEvent) {
	if reporter == nil {
		return
	}
	reporter(event)
}

// EdgeConfidence describes the confidence tier and score for an edge.
type EdgeConfidence struct {
	Tier  string  // "extracted", "inferred", or "ambiguous"
	Score float64 // 0.0–1.0
}

// Standard confidence values for edge insertion.
var (
	ConfidenceExtracted = EdgeConfidence{Tier: "extracted", Score: 1.0}
	ConfidenceInferred  = EdgeConfidence{Tier: "inferred", Score: 0.7}
)

func insertEdgeContext(ctx context.Context, stmt *sql.Stmt, fromRef, toRef, edgeType, edgeSource string, validFrom, validTo *string, conf EdgeConfidence) error {
	if _, err := stmt.ExecContext(ctx, fromRef, toRef, edgeType, edgeSource, validFrom, validTo, conf.Tier, conf.Score); err != nil {
		return fmt.Errorf("insert edge %s -> %s (%s, %s): %w", fromRef, toRef, edgeType, edgeSource, err)
	}
	return nil
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
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate foreign_key_check result: %w", err)
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

// chunkingConfigFingerprint is a stable hash of every field that
// affects how a record is chunked or contextualized. It is persisted
// at rebuild time and validated on --update so a config change
// between rebuild and update can't silently produce a snapshot with
// mixed chunk shapes (old records re-chunked under one policy, new
// records under another). Any mismatch forces an explicit rebuild.
//
// The pchunk.ResolverDefaultsVersion component captures Resolve's
// zero-config defaults. Without it, flipping a default in Resolve
// (e.g. #344: doc default flips from MarkdownPolicy to LateChunkPolicy)
// would leave existing snapshots' raw-field fingerprint unchanged and
// silently mix chunk shapes across an incremental update. Including
// the version forces rebuild on default flips even when no config
// field changed on disk.
func chunkingConfigFingerprint(cfg config.ChunkingConfig) string {
	parts := []string{
		"resolver_defaults_version=" + pchunk.ResolverDefaultsVersion,
		"contextualizer_format=" + cfg.Contextualizer.Format,
		"spec_policy=" + cfg.Spec.Policy,
		fmt.Sprintf("spec_max_tokens=%d", cfg.Spec.MaxTokens),
		fmt.Sprintf("spec_overlap_tokens=%d", cfg.Spec.OverlapTokens),
		fmt.Sprintf("spec_max_sections=%d", cfg.Spec.MaxSections),
		fmt.Sprintf("spec_child_max_tokens=%d", cfg.Spec.ChildMaxTokens),
		fmt.Sprintf("spec_child_overlap_tokens=%d", cfg.Spec.ChildOverlapTokens),
		"doc_policy=" + cfg.Doc.Policy,
		fmt.Sprintf("doc_max_tokens=%d", cfg.Doc.MaxTokens),
		fmt.Sprintf("doc_overlap_tokens=%d", cfg.Doc.OverlapTokens),
		fmt.Sprintf("doc_max_sections=%d", cfg.Doc.MaxSections),
		fmt.Sprintf("doc_child_max_tokens=%d", cfg.Doc.ChildMaxTokens),
		fmt.Sprintf("doc_child_overlap_tokens=%d", cfg.Doc.ChildOverlapTokens),
	}
	return fingerprint(parts)
}

// resolveInferAppliesTo is the single source of truth for whether AST
// inference runs during a rebuild. Explicit workspace.infer_applies_to wins;
// otherwise the default turns on for schema_version >= 3 corpora whose specs
// declare at least one code:// applies_to target, since that is the signal the
// feature was designed to serve.
func resolveInferAppliesTo(cfg *config.Config, specs []model.SpecRecord) bool {
	if cfg.Workspace.InferAppliesToSet {
		return cfg.Workspace.InferAppliesTo
	}
	if cfg.SchemaVersion < 3 {
		return false
	}
	for _, spec := range specs {
		for _, target := range spec.AppliesTo {
			if strings.HasPrefix(target, "code://") {
				return true
			}
		}
	}
	return false
}

// inferASTEdgesContext walks the workspace for code files, extracts symbols via
// tree-sitter, matches them against spec body text, and inserts inferred
// applies_to edges into the staging database.
func inferASTEdgesContext(ctx context.Context, tx *sql.Tx, edgeStmt *sql.Stmt, cfg *config.Config, specs []model.SpecRecord, validFrom *string) (int, error) {
	if !resolveInferAppliesTo(cfg, specs) {
		return 0, nil
	}
	workspaceRoot := cfg.Workspace.RootPath
	if workspaceRoot == "" {
		return 0, nil
	}

	// Quick check: if no spec body contains matchable identifiers, skip the
	// expensive filesystem walk and tree-sitter parsing entirely.
	hasMatchable := false
	for _, spec := range specs {
		if len(ast.ScanSpecIdentifiers(spec.BodyText)) > 0 {
			hasMatchable = true
			break
		}
	}
	if !hasMatchable {
		return 0, nil
	}

	codePaths, err := ast.WalkWorkspace(workspaceRoot)
	if err != nil {
		return 0, fmt.Errorf("walk workspace for AST extraction: %w", err)
	}
	if len(codePaths) == 0 {
		return 0, nil
	}

	// Prepare cache insert statement.
	cacheStmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO ast_cache (content_hash, path, symbols_json, rationale_json) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare ast_cache insert: %w", err)
	}
	defer cacheStmt.Close()

	// Load cached symbols from the previous index if available.
	cachedData := loadCachedASTData(ctx, cfg.Workspace.ResolvedIndexPath)

	// Extract symbols and rationale from each code file.
	const maxFileSize = 1 << 20 // 1 MB — skip minified bundles and generated files
	fileSymbols := make(map[string][]ast.Symbol, len(codePaths))
	for _, relPath := range codePaths {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		fullPath := filepath.Join(workspaceRoot, relPath)
		info, err := os.Stat(fullPath)
		if err != nil || info.Size() > maxFileSize {
			continue
		}
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		hash := ast.ContentHash(content, relPath)

		// Check cache first.
		if cached, ok := cachedData[hash]; ok {
			fileSymbols[relPath] = cached.Symbols
			if err := insertASTCacheWithRationale(ctx, cacheStmt, hash, relPath, cached.Symbols, cached.Rationale); err != nil {
				return 0, err
			}
			continue
		}

		lang := ast.DetectLanguage(relPath)
		if lang == "" {
			continue
		}
		symbols, err := ast.ExtractSymbols(content, lang)
		if err != nil {
			continue
		}
		rationale := ast.ExtractRationale(content, symbols, lang)
		fileSymbols[relPath] = symbols
		if err := insertASTCacheWithRationale(ctx, cacheStmt, hash, relPath, symbols, rationale); err != nil {
			return 0, err
		}
	}

	// Build spec summaries for matching.
	specSummaries := make([]ast.SpecSummary, len(specs))
	for i, spec := range specs {
		specSummaries[i] = ast.SpecSummary{
			Ref:             spec.Ref,
			Body:            spec.BodyText,
			ManualAppliesTo: spec.AppliesTo,
		}
	}

	// Run inference and insert edges.
	inferred := ast.InferEdges(fileSymbols, specSummaries)
	count := 0
	for _, edge := range inferred {
		ref := "code://" + edge.FilePath
		if err := insertEdgeContext(ctx, edgeStmt, edge.SpecRef, ref, "applies_to", "inferred", validFrom, nil, ConfidenceInferred); err != nil {
			// INSERT OR IGNORE means duplicate-key errors won't happen,
			// but handle unexpected errors.
			return count, err
		}
		count++
	}
	return count, nil
}

// cachedASTEntry holds both symbols and rationale from the ast_cache.
type cachedASTEntry struct {
	Symbols   []ast.Symbol
	Rationale []ast.Rationale
}

func loadCachedASTData(ctx context.Context, indexPath string) map[string]cachedASTEntry {
	result := make(map[string]cachedASTEntry)
	if indexPath == "" {
		return result
	}
	info, err := os.Stat(indexPath)
	if err != nil || info.IsDir() {
		return result
	}
	db, err := OpenReadOnlyContext(ctx, indexPath)
	if err != nil {
		return result
	}
	defer db.Close()

	// Check if ast_cache table exists (schema v4+).
	var tableCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='ast_cache'`).Scan(&tableCount); err != nil || tableCount == 0 {
		return result
	}

	// Check if rationale_json column exists (schema v7+).
	hasRationale := false
	var colCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('ast_cache') WHERE name = 'rationale_json'`).Scan(&colCount); err == nil && colCount > 0 {
		hasRationale = true
	}

	query := `SELECT content_hash, symbols_json FROM ast_cache`
	if hasRationale {
		query = `SELECT content_hash, symbols_json, rationale_json FROM ast_cache`
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var hash, symbolsJSON string
		var rationaleJSON string
		var scanErr error
		if hasRationale {
			scanErr = rows.Scan(&hash, &symbolsJSON, &rationaleJSON)
		} else {
			scanErr = rows.Scan(&hash, &symbolsJSON)
		}
		if scanErr != nil {
			continue
		}
		var symbols []ast.Symbol
		if err := json.Unmarshal([]byte(symbolsJSON), &symbols); err != nil {
			continue
		}
		var rationale []ast.Rationale
		if rationaleJSON != "" {
			if err := json.Unmarshal([]byte(rationaleJSON), &rationale); err != nil {
				continue
			}
		}
		result[hash] = cachedASTEntry{Symbols: symbols, Rationale: rationale}
	}
	return result
}

func insertASTCacheWithRationale(ctx context.Context, stmt *sql.Stmt, hash, path string, symbols []ast.Symbol, rationale []ast.Rationale) error {
	symbolData, err := json.Marshal(symbols)
	if err != nil {
		return fmt.Errorf("marshal AST symbols for cache: %w", err)
	}
	if rationale == nil {
		rationale = []ast.Rationale{}
	}
	rationaleData, err := json.Marshal(rationale)
	if err != nil {
		return fmt.Errorf("marshal AST rationale for cache: %w", err)
	}
	if _, err := stmt.ExecContext(ctx, hash, path, string(symbolData), string(rationaleData)); err != nil {
		return fmt.Errorf("insert AST cache %s: %w", path, err)
	}
	return nil
}
