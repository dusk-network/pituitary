package index

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestRebuildCreatesSQLiteIndexFromFixtures(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}

	result, err := Rebuild(cfg, records)
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if result.ArtifactCount != 5 || result.SpecCount != 3 || result.DocCount != 2 {
		t.Fatalf("artifact counts = %+v", result)
	}
	if result.ChunkCount != 17 {
		t.Fatalf("chunk count = %d, want 17", result.ChunkCount)
	}
	if result.EdgeCount != 9 {
		t.Fatalf("edge count = %d, want 9", result.EdgeCount)
	}
	if result.EmbedderDimension != 8 {
		t.Fatalf("embedder dimension = %d, want 8", result.EmbedderDimension)
	}
	if len(result.Sources) != 2 {
		t.Fatalf("result.Sources = %+v, want 2 source summaries", result.Sources)
	}
	if _, err := os.Stat(cfg.Workspace.ResolvedIndexPath + ".new"); !os.IsNotExist(err) {
		t.Fatalf("staging database still exists: %v", err)
	}

	db := mustOpenReadOnly(t, cfg.Workspace.ResolvedIndexPath)
	defer db.Close()

	assertCount(t, db, `SELECT COUNT(*) FROM artifacts`, 5)
	assertCount(t, db, `SELECT COUNT(*) FROM chunks`, 17)
	assertCount(t, db, `SELECT COUNT(*) FROM edges`, 9)
	assertCount(t, db, `SELECT COUNT(*) FROM chunks_vec`, 17)
	assertCount(t, db, `SELECT COUNT(*) FROM metadata`, 5)
	assertMetadataValue(t, db, "embedder_fingerprint", "fixture|fixture-8d|plain_v1")
	assertMetadataValue(t, db, "source_fingerprint", sourceFingerprint(cfg))
	assertSections(t, db, "SPEC-042", []string{
		"Overview",
		"Requirements",
		"Design Decisions",
	})
	assertSections(t, db, "doc://guides/api-rate-limits", []string{
		"Public API Rate Limits",
		"Public API Rate Limits / Default Limit",
		"Public API Rate Limits / Configuration",
		"Public API Rate Limits / Operational Notes",
	})

	assertSchemaObject(t, db, "table", "artifacts")
	assertSchemaObject(t, db, "table", "chunks")
	assertSchemaObject(t, db, "table", "chunks_vec")
	assertSchemaObject(t, db, "table", "edges")
	assertSchemaObject(t, db, "index", "idx_artifacts_kind_status_domain")
	assertSchemaObject(t, db, "index", "idx_edges_from_ref_type")
	assertAllAdapters(t, db, config.AdapterFilesystem)
	assertSourceAdapterMetadata(t, db, config.AdapterFilesystem, 5)
	assertSchemaSQLContains(t, db, "chunks_vec", "CREATE VIRTUAL TABLE chunks_vec USING vec0")
	assertSchemaSQLContains(t, db, "chunks_vec", "embedding float[8] distance_metric=cosine")
	assertColumnType(t, db, `SELECT typeof(embedding) FROM chunks_vec LIMIT 1`, "blob")

	_, err = db.Exec(`INSERT INTO metadata (key, value) VALUES ('x', 'y')`)
	if err == nil {
		t.Fatal("read-only handle allowed write")
	}
}

func TestRebuildPreservesLastGoodDatabaseOnFailure(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}

	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("initial Rebuild() error = %v", err)
	}
	before := fileHash(t, cfg.Workspace.ResolvedIndexPath)

	broken := &source.LoadResult{
		Specs: append([]model.SpecRecord{}, records.Specs...),
		Docs:  append([]model.DocRecord{}, records.Docs...),
	}
	broken.Specs = append(broken.Specs, broken.Specs[0])

	if _, err := Rebuild(cfg, broken); err == nil {
		t.Fatal("Rebuild() error = nil, want duplicate-artifact failure")
	}

	after := fileHash(t, cfg.Workspace.ResolvedIndexPath)
	if before != after {
		t.Fatalf("active database changed after failed rebuild")
	}
	if _, err := os.Stat(cfg.Workspace.ResolvedIndexPath + ".new"); !os.IsNotExist(err) {
		t.Fatalf("staging database still exists after failure: %v", err)
	}
}

func TestPrepareRebuildSummarizesFixturesWithoutWritingDatabase(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}

	result, err := PrepareRebuild(cfg, records)
	if err != nil {
		t.Fatalf("PrepareRebuild() error = %v", err)
	}
	if !result.DryRun {
		t.Fatalf("result = %+v, want dry_run=true", result)
	}
	if result.ArtifactCount != 5 || result.SpecCount != 3 || result.DocCount != 2 {
		t.Fatalf("artifact counts = %+v", result)
	}
	if result.ChunkCount != 17 {
		t.Fatalf("chunk count = %d, want 17", result.ChunkCount)
	}
	if result.EdgeCount != 9 {
		t.Fatalf("edge count = %d, want 9", result.EdgeCount)
	}
	if result.EmbedderDimension != 8 {
		t.Fatalf("embedder dimension = %d, want 8", result.EmbedderDimension)
	}
	if len(result.Sources) != 2 {
		t.Fatalf("result.Sources = %+v, want 2 source summaries", result.Sources)
	}
	if _, err := os.Stat(cfg.Workspace.ResolvedIndexPath); !os.IsNotExist(err) {
		t.Fatalf("PrepareRebuild() created database: %v", err)
	}
	if _, err := os.Stat(cfg.Workspace.ResolvedIndexPath + ".new"); !os.IsNotExist(err) {
		t.Fatalf("PrepareRebuild() created staging database: %v", err)
	}
}

func TestPrepareRebuildDoesNotLeaveCreatedIndexDirectoriesBehind(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	indexDir := filepath.Join(root, ".pituitary")
	cfg := loadFixtureConfigWithIndexPath(t, filepath.Join(indexDir, "pituitary.db"))
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := os.Stat(indexDir); !os.IsNotExist(err) {
		t.Fatalf("index directory exists before dry run: %v", err)
	}

	result, err := PrepareRebuild(cfg, records)
	if err != nil {
		t.Fatalf("PrepareRebuild() error = %v", err)
	}
	if !result.DryRun {
		t.Fatalf("result = %+v, want dry_run=true", result)
	}
	if _, err := os.Stat(indexDir); !os.IsNotExist(err) {
		t.Fatalf("PrepareRebuild() left index directory behind: %v", err)
	}
	if _, err := os.Stat(cfg.Workspace.ResolvedIndexPath); !os.IsNotExist(err) {
		t.Fatalf("PrepareRebuild() created database: %v", err)
	}
	if _, err := os.Stat(cfg.Workspace.ResolvedIndexPath + ".new"); !os.IsNotExist(err) {
		t.Fatalf("PrepareRebuild() created staging database: %v", err)
	}
}

func TestPrepareRebuildDoesNotReuseWhenSourceFingerprintChanges(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	cfg.Sources[1].Include = []string{"guides/*.md"}
	records, err = source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() after source change error = %v", err)
	}

	result, err := PrepareRebuildContextWithOptions(t.Context(), cfg, records, RebuildOptions{})
	if err != nil {
		t.Fatalf("PrepareRebuildContextWithOptions() error = %v", err)
	}
	if result.ReusedArtifactCount != 0 || result.ReusedChunkCount != 0 {
		t.Fatalf("result = %+v, want reuse disabled when source fingerprint changes", result)
	}
	if result.EmbeddedChunkCount != result.ChunkCount {
		t.Fatalf("result = %+v, want all chunks re-embedded when source fingerprint changes", result)
	}
}

func TestPrepareRebuildDoesNotReuseWhenStoredEmbedderDimensionDiffers(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	db, err := sql.Open("sqlite3", cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := db.Exec(`UPDATE metadata SET value = '999' WHERE key = 'embedder_dimension'`); err != nil {
		t.Fatalf("update embedder_dimension: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	result, err := PrepareRebuildContextWithOptions(t.Context(), cfg, records, RebuildOptions{})
	if err != nil {
		t.Fatalf("PrepareRebuildContextWithOptions() error = %v", err)
	}
	if result.ReusedArtifactCount != 0 || result.ReusedChunkCount != 0 {
		t.Fatalf("result = %+v, want reuse disabled when stored embedder dimension changes", result)
	}
	if result.EmbeddedChunkCount != result.ChunkCount {
		t.Fatalf("result = %+v, want all chunks re-embedded when stored embedder dimension changes", result)
	}
}

func TestPrepareRebuildValidatesIndexPathFilesystemPreconditions(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	lockedPath := filepath.Join(root, "locked")
	mustWriteFile(t, lockedPath, "not a directory")

	cfg := loadFixtureConfigWithIndexPath(t, filepath.Join(lockedPath, "pituitary.db"))
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}

	_, err = PrepareRebuild(cfg, records)
	if err == nil {
		t.Fatal("PrepareRebuild() error = nil, want filesystem preflight failure")
	}
	if !strings.Contains(err.Error(), "create index directory") {
		t.Fatalf("PrepareRebuild() error = %q, want create index directory failure", err)
	}
	if _, err := os.Stat(cfg.Workspace.ResolvedIndexPath + ".new"); err == nil || (!os.IsNotExist(err) && !strings.Contains(err.Error(), "not a directory")) {
		t.Fatalf("PrepareRebuild() left staging database probe behind: %v", err)
	}
}

func TestPrepareRebuildAcceptsSymlinkedStaleStagePathLikeRebuild(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	indexDir := filepath.Join(root, ".pituitary")
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", indexDir, err)
	}

	cfg := loadFixtureConfigWithIndexPath(t, filepath.Join(indexDir, "pituitary.db"))
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}

	staleTarget := filepath.Join(root, "stale-stage-target")
	if err := os.MkdirAll(staleTarget, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", staleTarget, err)
	}
	mustWriteFile(t, filepath.Join(staleTarget, "keep.txt"), "keep")

	stagePath := cfg.Workspace.ResolvedIndexPath + ".new"
	if err := os.Symlink(staleTarget, stagePath); err != nil {
		t.Fatalf("symlink %s -> %s: %v", stagePath, staleTarget, err)
	}

	result, err := PrepareRebuild(cfg, records)
	if err != nil {
		t.Fatalf("PrepareRebuild() error = %v, want symlinked stale stage path to validate", err)
	}
	if !result.DryRun {
		t.Fatalf("result = %+v, want dry_run=true", result)
	}
	info, err := os.Lstat(stagePath)
	if err != nil {
		t.Fatalf("lstat %s after PrepareRebuild(): %v", stagePath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("stage path mode = %v, want symlink after PrepareRebuild()", info.Mode())
	}

	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v, want parity with PrepareRebuild()", err)
	}
	if _, err := os.Lstat(stagePath); !os.IsNotExist(err) {
		t.Fatalf("Rebuild() left stale stage path behind: %v", err)
	}
	if info, err := os.Stat(cfg.Workspace.ResolvedIndexPath); err != nil {
		t.Fatalf("stat %s after Rebuild(): %v", cfg.Workspace.ResolvedIndexPath, err)
	} else if info.IsDir() {
		t.Fatalf("index path %s is a directory after Rebuild()", cfg.Workspace.ResolvedIndexPath)
	}
	if _, err := os.Stat(filepath.Join(staleTarget, "keep.txt")); err != nil {
		t.Fatalf("stale target directory was modified: %v", err)
	}
}

func TestRebuildInfersASTEdges(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	indexPath := filepath.Join(dir, ".pituitary", "pituitary.db")

	// Create pituitary.toml pointing at the temp workspace.
	configContent := `
[workspace]
root = "` + filepath.ToSlash(dir) + `"
index_path = "` + filepath.ToSlash(indexPath) + `"
infer_applies_to = true

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`
	configPath := filepath.Join(dir, "pituitary.toml")
	mustWriteFile(t, configPath, configContent)

	// Create a spec whose body mentions "SlidingWindowLimiter".
	mustWriteFile(t, filepath.Join(dir, "specs", "rate-limit", "spec.toml"), `id = "SPEC-042"
title = "Rate Limiting"
status = "accepted"
domain = "api"
authors = ["test"]
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(dir, "specs", "rate-limit", "body.md"), `## Overview

This spec governs the SlidingWindowLimiter implementation.
`)

	// Create a Go source file that defines SlidingWindowLimiter.
	mustWriteFile(t, filepath.Join(dir, "src", "limiter.go"), `package limiter

type SlidingWindowLimiter struct {
	window int
}

func NewSlidingWindowLimiter(w int) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{window: w}
}
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("LoadFromConfig: %v", err)
	}

	result, err := Rebuild(cfg, records)
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	if result.InferredEdgeCount == 0 {
		t.Fatal("expected at least one inferred edge")
	}

	// Verify governed-by returns the inferred edge.
	govResult, err := GovernedByContext(context.Background(), cfg.Workspace.ResolvedIndexPath, "src/limiter.go", "", "")
	if err != nil {
		t.Fatalf("GovernedBy: %v", err)
	}
	if len(govResult.Specs) == 0 {
		t.Fatal("expected governed-by to find SPEC-042 for src/limiter.go")
	}
	if govResult.Specs[0].Ref != "SPEC-042" {
		t.Errorf("expected SPEC-042, got %s", govResult.Specs[0].Ref)
	}
	if govResult.Specs[0].Source != "inferred" {
		t.Errorf("expected source=inferred, got %s", govResult.Specs[0].Source)
	}

	// Verify status reports governance coverage.
	status, err := ReadStatusContext(context.Background(), cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("ReadStatus: %v", err)
	}
	if status.GovernanceCoverage == nil {
		t.Fatal("expected governance coverage in status")
	}
	if status.GovernanceCoverage.InferredEdges == 0 {
		t.Error("expected InferredEdges > 0")
	}
	if status.GovernanceCoverage.TotalFiles == 0 {
		t.Error("expected TotalFiles > 0")
	}

	// Verify the schema has both edge_source column and ast_cache table.
	db := mustOpenReadOnly(t, cfg.Workspace.ResolvedIndexPath)
	defer db.Close()
	assertSchemaObject(t, db, "table", "ast_cache")

	var edgeSource string
	if err := db.QueryRow(`SELECT edge_source FROM edges WHERE edge_type = 'applies_to' AND edge_source = 'inferred' LIMIT 1`).Scan(&edgeSource); err != nil {
		t.Fatalf("query inferred edge: %v", err)
	}
	if edgeSource != "inferred" {
		t.Errorf("expected edge_source=inferred, got %s", edgeSource)
	}
}

func loadFixtureConfig(tb testing.TB) *config.Config {
	tb.Helper()

	return loadFixtureConfigWithIndexPath(tb, filepath.Join(tb.TempDir(), "pituitary.db"))
}

func loadFixtureConfigWithIndexPath(tb testing.TB, indexPath string) *config.Config {
	tb.Helper()

	repoRoot := repoRoot(tb)
	configPath := filepath.Join(tb.TempDir(), "pituitary.toml")
	mustWriteFile(tb, configPath, `
[workspace]
root = "`+filepath.ToSlash(repoRoot)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md", "runbooks/*.md"]
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}

func mustOpenReadOnly(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("OpenReadOnly(%s) error = %v", path, err)
	}
	return db
}

func assertCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("query %q = %d, want %d", query, got, want)
	}
}

func assertSchemaObject(t *testing.T, db *sql.DB, objectType, name string) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = ? AND name = ?`, objectType, name).Scan(&count); err != nil {
		t.Fatalf("lookup sqlite_master %s %s: %v", objectType, name, err)
	}
	if count != 1 {
		t.Fatalf("missing schema object %s %s", objectType, name)
	}
}

func assertSchemaSQLContains(t *testing.T, db *sql.DB, name, wantSubstring string) {
	t.Helper()
	var sqlText string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE name = ?`, name).Scan(&sqlText); err != nil {
		t.Fatalf("lookup sqlite_master sql for %s: %v", name, err)
	}
	if !strings.Contains(sqlText, wantSubstring) {
		t.Fatalf("schema for %s = %q, want substring %q", name, sqlText, wantSubstring)
	}
}

func assertColumnType(t *testing.T, db *sql.DB, query, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	if got != want {
		t.Fatalf("query %q = %q, want %q", query, got, want)
	}
}

func assertMetadataValue(t *testing.T, db *sql.DB, key, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(`SELECT value FROM metadata WHERE key = ?`, key).Scan(&got); err != nil {
		t.Fatalf("lookup metadata %s: %v", key, err)
	}
	if got != want {
		t.Fatalf("metadata %s = %q, want %q", key, got, want)
	}
}

func assertSections(t *testing.T, db *sql.DB, artifactRef string, want []string) {
	t.Helper()

	rows, err := db.Query(`SELECT section FROM chunks WHERE artifact_ref = ? ORDER BY id`, artifactRef)
	if err != nil {
		t.Fatalf("query sections for %s: %v", artifactRef, err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var section string
		if err := rows.Scan(&section); err != nil {
			t.Fatalf("scan section for %s: %v", artifactRef, err)
		}
		got = append(got, section)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sections for %s: %v", artifactRef, err)
	}
	if len(got) != len(want) {
		t.Fatalf("sections for %s = %v, want %v", artifactRef, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sections for %s = %v, want %v", artifactRef, got, want)
		}
	}
}

func fileHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func repoRoot(tb testing.TB) string {
	tb.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		tb.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func mustWriteFile(tb testing.TB, path, content string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		tb.Fatalf("write %s: %v", path, err)
	}
}

func assertSourceAdapterMetadata(t *testing.T, db *sql.DB, want string, wantCount int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM artifacts WHERE json_extract(metadata_json, '$.source_adapter') = ?`, want).Scan(&got); err != nil {
		t.Fatalf("query source_adapter metadata: %v", err)
	}
	if got != wantCount {
		t.Fatalf("source_adapter metadata count = %d, want %d with value %q", got, wantCount, want)
	}
}

func assertAllAdapters(t *testing.T, db *sql.DB, want string) {
	t.Helper()
	var bad int
	if err := db.QueryRow(`SELECT COUNT(*) FROM artifacts WHERE adapter != ?`, want).Scan(&bad); err != nil {
		t.Fatalf("query adapter mismatch: %v", err)
	}
	if bad != 0 {
		t.Fatalf("adapter mismatch: %d artifact(s) do not have adapter=%q", bad, want)
	}
}

func TestAdapterFromMetadata(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		metadata map[string]string
		want     string
	}{
		{
			name:     "nil metadata falls back to filesystem",
			metadata: nil,
			want:     config.AdapterFilesystem,
		},
		{
			name:     "empty metadata falls back to filesystem",
			metadata: map[string]string{},
			want:     config.AdapterFilesystem,
		},
		{
			name:     "source_adapter present",
			metadata: map[string]string{"source_adapter": "github"},
			want:     "github",
		},
		{
			name:     "source_adapter empty string falls back",
			metadata: map[string]string{"source_adapter": ""},
			want:     config.AdapterFilesystem,
		},
		{
			name:     "source_adapter whitespace-only falls back",
			metadata: map[string]string{"source_adapter": "  "},
			want:     config.AdapterFilesystem,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := adapterFromMetadata(tt.metadata)
			if got != tt.want {
				t.Errorf("adapterFromMetadata() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRebuildSetsTemporalValidityOnEdges(t *testing.T) {
	t.Parallel()
	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig: %v", err)
	}

	today := time.Now().UTC().Format("2006-01-02")
	_, err = Rebuild(cfg, records)
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	db, err := OpenReadOnlyContext(context.Background(), cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer db.Close()

	// Verify schema version is 5.
	var version string
	if err := db.QueryRow(`SELECT value FROM metadata WHERE key = 'schema_version'`).Scan(&version); err != nil {
		t.Fatalf("read schema_version: %v", err)
	}
	if version != "6" {
		t.Errorf("schema_version = %q, want 6", version)
	}

	// Verify that manual edges have valid_from set to today (YYYY-MM-DD).
	rows, err := db.Query(`SELECT from_ref, to_ref, edge_type, edge_source, valid_from, valid_to FROM edges WHERE edge_source = 'manual'`)
	if err != nil {
		t.Fatalf("query edges: %v", err)
	}
	defer rows.Close()

	manualCount := 0
	for rows.Next() {
		var fromRef, toRef, edgeType, edgeSource string
		var validFrom, validTo sql.NullString
		if err := rows.Scan(&fromRef, &toRef, &edgeType, &edgeSource, &validFrom, &validTo); err != nil {
			t.Fatalf("scan edge: %v", err)
		}
		manualCount++
		if !validFrom.Valid {
			t.Errorf("edge %s->%s (%s) has NULL valid_from, expected a date", fromRef, toRef, edgeType)
			continue
		}
		if validFrom.String != today {
			t.Errorf("edge %s->%s valid_from = %q, want %q", fromRef, toRef, validFrom.String, today)
		}
	}
	if manualCount == 0 {
		t.Fatal("expected at least one manual edge")
	}
}

func TestGovernedByTemporalFilter(t *testing.T) {
	t.Parallel()
	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig: %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	// The fixture has applies_to edges from SPEC-042 and SPEC-055 to
	// code://src/api/middleware/ratelimiter.go. SPEC-008 is superseded so
	// governed-by only returns accepted specs.
	result, err := GovernedByContext(context.Background(), cfg.Workspace.ResolvedIndexPath, "src/api/middleware/ratelimiter.go", "", "")
	if err != nil {
		t.Fatalf("GovernedBy (no filter): %v", err)
	}
	if len(result.Specs) == 0 {
		t.Skip("no governing specs found; skipping temporal filter test")
	}

	// With a future date, should still return results (edges are currently valid).
	futureDate := time.Now().UTC().Add(24 * time.Hour).Format("2006-01-02")
	futureResult, err := GovernedByContext(context.Background(), cfg.Workspace.ResolvedIndexPath, "src/api/middleware/ratelimiter.go", futureDate, "")
	if err != nil {
		t.Fatalf("GovernedBy (future): %v", err)
	}
	if len(futureResult.Specs) != len(result.Specs) {
		t.Errorf("future query returned %d specs, expected %d", len(futureResult.Specs), len(result.Specs))
	}

	// With a very old date (before edges existed), should return no results
	// because valid_from was set to the rebuild timestamp.
	pastDate := "1970-01-01"
	pastResult, err := GovernedByContext(context.Background(), cfg.Workspace.ResolvedIndexPath, "src/api/middleware/ratelimiter.go", pastDate, "")
	if err != nil {
		t.Fatalf("GovernedBy (past): %v", err)
	}
	if len(pastResult.Specs) != 0 {
		t.Errorf("past query returned %d specs, expected 0", len(pastResult.Specs))
	}
}

func TestRebuildSetsValidToForSupersededSpecs(t *testing.T) {
	t.Parallel()
	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig: %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	db, err := OpenReadOnlyContext(context.Background(), cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer db.Close()

	// SPEC-008 is superseded — its edges should have valid_to set.
	rows, err := db.Query(`SELECT to_ref, valid_from, valid_to FROM edges WHERE from_ref = 'SPEC-008' AND edge_source = 'manual'`)
	if err != nil {
		t.Fatalf("query SPEC-008 edges: %v", err)
	}
	defer rows.Close()

	edgeCount := 0
	for rows.Next() {
		var toRef string
		var validFrom, validTo sql.NullString
		if err := rows.Scan(&toRef, &validFrom, &validTo); err != nil {
			t.Fatalf("scan: %v", err)
		}
		edgeCount++
		if !validTo.Valid {
			t.Errorf("superseded spec SPEC-008 edge to %s has NULL valid_to", toRef)
		}
	}
	if edgeCount == 0 {
		t.Fatal("expected edges from SPEC-008")
	}

	// SPEC-042 is accepted — its edges should NOT have valid_to set.
	rows2, err := db.Query(`SELECT to_ref, valid_to FROM edges WHERE from_ref = 'SPEC-042' AND edge_source = 'manual'`)
	if err != nil {
		t.Fatalf("query SPEC-042 edges: %v", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var toRef string
		var validTo sql.NullString
		if err := rows2.Scan(&toRef, &validTo); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if validTo.Valid {
			t.Errorf("active spec SPEC-042 edge to %s has non-NULL valid_to = %s", toRef, validTo.String)
		}
	}
}

func TestRebuildSetsConfidenceTiersOnEdges(t *testing.T) {
	t.Parallel()
	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig: %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	db, err := OpenReadOnlyContext(context.Background(), cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	defer db.Close()

	// All manual edges should have confidence='extracted' and score=1.0.
	rows, err := db.Query(`SELECT from_ref, to_ref, confidence, confidence_score FROM edges WHERE edge_source = 'manual'`)
	if err != nil {
		t.Fatalf("query manual edges: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var fromRef, toRef, confidence string
		var score float64
		if err := rows.Scan(&fromRef, &toRef, &confidence, &score); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if confidence != "extracted" {
			t.Errorf("manual edge %s->%s has confidence=%q, want extracted", fromRef, toRef, confidence)
		}
		if score != 1.0 {
			t.Errorf("manual edge %s->%s has confidence_score=%f, want 1.0", fromRef, toRef, score)
		}
	}
}

func TestGovernedByConfidenceFilter(t *testing.T) {
	t.Parallel()
	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig: %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	// Without confidence filter, should return all governing specs.
	allResult, err := GovernedByContext(context.Background(), cfg.Workspace.ResolvedIndexPath, "src/api/middleware/ratelimiter.go", "", "")
	if err != nil {
		t.Fatalf("GovernedBy (no filter): %v", err)
	}
	if len(allResult.Specs) == 0 {
		t.Skip("no governing specs found; skipping confidence filter test")
	}

	// With min-confidence=extracted, should return only extracted edges (all manual edges).
	extractedResult, err := GovernedByContext(context.Background(), cfg.Workspace.ResolvedIndexPath, "src/api/middleware/ratelimiter.go", "", "extracted")
	if err != nil {
		t.Fatalf("GovernedBy (extracted): %v", err)
	}
	// All fixture edges are manual (extracted), so counts should match.
	if len(extractedResult.Specs) != len(allResult.Specs) {
		t.Errorf("extracted filter returned %d specs, expected %d", len(extractedResult.Specs), len(allResult.Specs))
	}

	// Verify confidence fields are populated.
	for _, spec := range allResult.Specs {
		if spec.Confidence == "" {
			t.Errorf("spec %s has empty confidence", spec.Ref)
		}
		if spec.ConfidenceScore == 0 {
			t.Errorf("spec %s has zero confidence_score", spec.Ref)
		}
	}
}
