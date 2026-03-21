package index

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if result.EdgeCount != 8 {
		t.Fatalf("edge count = %d, want 8", result.EdgeCount)
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
	assertCount(t, db, `SELECT COUNT(*) FROM edges`, 8)
	assertCount(t, db, `SELECT COUNT(*) FROM chunks_vec`, 17)
	assertCount(t, db, `SELECT COUNT(*) FROM metadata`, 3)
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
	if result.EdgeCount != 8 {
		t.Fatalf("edge count = %d, want 8", result.EdgeCount)
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

func loadFixtureConfig(tb testing.TB) *config.Config {
	tb.Helper()

	repoRoot := repoRoot(tb)
	indexPath := filepath.Join(tb.TempDir(), "pituitary.db")
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
