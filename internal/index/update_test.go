package index

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestUpdateNoOp(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	result, err := UpdateContextWithOptions(context.Background(), cfg, records)
	if err != nil {
		t.Fatalf("UpdateContextWithOptions() error = %v", err)
	}

	if !result.Update {
		t.Fatal("result.Update should be true")
	}
	if result.AddedCount != 0 {
		t.Fatalf("added_count = %d, want 0", result.AddedCount)
	}
	if result.UpdatedCount != 0 {
		t.Fatalf("updated_count = %d, want 0", result.UpdatedCount)
	}
	if result.RemovedCount != 0 {
		t.Fatalf("removed_count = %d, want 0", result.RemovedCount)
	}
	if result.ArtifactCount != result.UnchangedCount {
		t.Fatalf("unchanged_count = %d, want %d", result.UnchangedCount, result.ArtifactCount)
	}
	if result.ContentFingerprint == "" {
		t.Fatal("content_fingerprint should not be empty")
	}

	db := mustOpenReadOnly(t, cfg.Workspace.ResolvedIndexPath)
	defer db.Close()
	assertCount(t, db, `SELECT COUNT(*) FROM artifacts`, result.ArtifactCount)
}

func TestUpdateAddsNewArtifact(t *testing.T) {
	t.Parallel()

	indexPath := filepath.Join(t.TempDir(), "pituitary.db")
	repoDir := t.TempDir()

	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "existing.md"), "# Existing\n\nAlready here.\n")

	configPath := filepath.Join(t.TempDir(), "pituitary.toml")
	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(repoDir)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
`)

	cfg, err := loadAndRebuild(t, configPath)
	if err != nil {
		t.Fatal(err)
	}

	// Add a new doc.
	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "new-doc.md"), "# New Doc\n\nJust added.\n")

	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() reload error = %v", err)
	}

	result, err := UpdateContextWithOptions(context.Background(), cfg, records)
	if err != nil {
		t.Fatalf("UpdateContextWithOptions() error = %v", err)
	}

	if result.AddedCount != 1 {
		t.Fatalf("added_count = %d, want 1", result.AddedCount)
	}
	if result.ArtifactCount != 2 {
		t.Fatalf("artifact_count = %d, want 2", result.ArtifactCount)
	}

	db := mustOpenReadOnly(t, indexPath)
	defer db.Close()
	assertCount(t, db, `SELECT COUNT(*) FROM artifacts`, 2)
}

func TestUpdateRemovesDeletedArtifact(t *testing.T) {
	t.Parallel()

	indexPath := filepath.Join(t.TempDir(), "pituitary.db")
	repoDir := t.TempDir()

	// Set up a mini workspace with two docs.
	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "keep.md"), "# Keep\n\nThis doc stays.\n")
	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "remove.md"), "# Remove\n\nThis doc will be removed.\n")

	configPath := filepath.Join(t.TempDir(), "pituitary.toml")
	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(repoDir)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
`)

	cfg, err := loadAndRebuild(t, configPath)
	if err != nil {
		t.Fatal(err)
	}

	// Remove one doc.
	if err := os.Remove(filepath.Join(repoDir, "docs", "guides", "remove.md")); err != nil {
		t.Fatalf("remove fixture: %v", err)
	}

	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}

	result, err := UpdateContextWithOptions(context.Background(), cfg, records)
	if err != nil {
		t.Fatalf("UpdateContextWithOptions() error = %v", err)
	}

	if result.RemovedCount != 1 {
		t.Fatalf("removed_count = %d, want 1", result.RemovedCount)
	}
	if result.ArtifactCount != 1 {
		t.Fatalf("artifact_count = %d, want 1", result.ArtifactCount)
	}

	db := mustOpenReadOnly(t, indexPath)
	defer db.Close()
	assertCount(t, db, `SELECT COUNT(*) FROM artifacts`, 1)
	corpusDB := mustOpenCorpusReadOnly(t, indexPath)
	defer corpusDB.Close()
	assertCount(t, corpusDB, `SELECT COUNT(*) FROM records`, 1)
}

func TestUpdateChangedArtifact(t *testing.T) {
	t.Parallel()

	indexPath := filepath.Join(t.TempDir(), "pituitary.db")
	repoDir := t.TempDir()

	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "mutable.md"), "# Mutable\n\nOriginal content.\n")

	configPath := filepath.Join(t.TempDir(), "pituitary.toml")
	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(repoDir)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
`)

	cfg, err := loadAndRebuild(t, configPath)
	if err != nil {
		t.Fatal(err)
	}

	// Modify the doc.
	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "mutable.md"), "# Mutable\n\nUpdated content with changes.\n")

	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}

	result, err := UpdateContextWithOptions(context.Background(), cfg, records)
	if err != nil {
		t.Fatalf("UpdateContextWithOptions() error = %v", err)
	}

	if result.UpdatedCount != 1 {
		t.Fatalf("updated_count = %d, want 1", result.UpdatedCount)
	}
	if result.AddedCount != 0 {
		t.Fatalf("added_count = %d, want 0", result.AddedCount)
	}
}

func TestUpdatePreconditionSchemaVersion(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	// Tamper with schema_version.
	db, err := sql.Open("sqlite3", cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := db.Exec(`UPDATE metadata SET value = '999' WHERE key = 'schema_version'`); err != nil {
		t.Fatalf("tamper schema_version: %v", err)
	}
	db.Close()

	result, err := UpdateContextWithOptions(context.Background(), cfg, records)
	if err != nil {
		t.Fatalf("UpdateContextWithOptions() error = %v", err)
	}
	if !result.FullRebuild {
		t.Fatalf("result = %+v, want update to recover via full rebuild", result)
	}
	db = mustOpenReadOnly(t, cfg.Workspace.ResolvedIndexPath)
	defer db.Close()
	assertMetadataValue(t, db, "schema_version", "10")
}

func TestUpdatePreconditionEmbedder(t *testing.T) {
	t.Parallel()

	indexPath := filepath.Join(t.TempDir(), "pituitary.db")
	cfg := loadFixtureConfigWithIndexPath(t, indexPath)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	// Tamper with embedder fingerprint.
	db, err := sql.Open("sqlite3", indexPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := db.Exec(`UPDATE metadata SET value = 'wrong|fingerprint' WHERE key = 'embedder_fingerprint'`); err != nil {
		t.Fatalf("tamper embedder_fingerprint: %v", err)
	}
	db.Close()

	result, err := UpdateContextWithOptions(context.Background(), cfg, records)
	if err != nil {
		t.Fatalf("UpdateContextWithOptions() error = %v", err)
	}
	if !result.FullRebuild {
		t.Fatalf("result = %+v, want update to recover via full rebuild", result)
	}
	db = mustOpenReadOnly(t, indexPath)
	defer db.Close()
	assertMetadataValue(t, db, "embedder_fingerprint", "fixture|fixture-8d|plain_v1")
}

func TestUpdateMissingIndex(t *testing.T) {
	t.Parallel()

	indexPath := filepath.Join(t.TempDir(), "does-not-exist.db")
	cfg := loadFixtureConfigWithIndexPath(t, indexPath)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}

	_, err = UpdateContextWithOptions(context.Background(), cfg, records)
	if !IsMissingIndex(err) {
		t.Fatalf("expected MissingIndexError, got %v", err)
	}
}

func TestUpdateEdgesRebuilt(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	rebuildResult, err := Rebuild(cfg, records)
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	result, err := UpdateContextWithOptions(context.Background(), cfg, records)
	if err != nil {
		t.Fatalf("UpdateContextWithOptions() error = %v", err)
	}

	if result.EdgeCount != rebuildResult.EdgeCount {
		t.Fatalf("edge_count = %d, want %d (same as rebuild)", result.EdgeCount, rebuildResult.EdgeCount)
	}

	db := mustOpenReadOnly(t, cfg.Workspace.ResolvedIndexPath)
	defer db.Close()
	assertCount(t, db, `SELECT COUNT(*) FROM edges`, result.EdgeCount)
}

func TestUpdateContentFingerprint(t *testing.T) {
	t.Parallel()

	indexPath := filepath.Join(t.TempDir(), "pituitary.db")
	repoDir := t.TempDir()

	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "doc.md"), "# Doc\n\nOriginal.\n")

	configPath := filepath.Join(t.TempDir(), "pituitary.toml")
	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(repoDir)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
`)

	cfg, err := loadAndRebuild(t, configPath)
	if err != nil {
		t.Fatal(err)
	}

	db := mustOpenReadOnly(t, indexPath)
	var originalFP string
	db.QueryRow(`SELECT value FROM metadata WHERE key = 'content_fingerprint'`).Scan(&originalFP)
	db.Close()

	// Modify content.
	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "doc.md"), "# Doc\n\nChanged content.\n")

	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}

	result, err := UpdateContextWithOptions(context.Background(), cfg, records)
	if err != nil {
		t.Fatalf("UpdateContextWithOptions() error = %v", err)
	}

	if result.ContentFingerprint == originalFP {
		t.Fatal("content_fingerprint should have changed after update")
	}

	db = mustOpenReadOnly(t, indexPath)
	defer db.Close()
	assertMetadataValue(t, db, "content_fingerprint", result.ContentFingerprint)
}

func TestUpdateSourceFingerprintUpdated(t *testing.T) {
	t.Parallel()

	indexPath := filepath.Join(t.TempDir(), "pituitary.db")
	repoDir := t.TempDir()

	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "doc.md"), "# Doc\n\nContent.\n")

	configPath := filepath.Join(t.TempDir(), "pituitary.toml")
	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(repoDir)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
`)

	cfg, err := loadAndRebuild(t, configPath)
	if err != nil {
		t.Fatal(err)
	}

	// Change the source config without changing actual content.
	cfg.Sources[0].Include = []string{"guides/*.md", "runbooks/*.md"}

	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}

	result, err := UpdateContextWithOptions(context.Background(), cfg, records)
	if err != nil {
		t.Fatalf("UpdateContextWithOptions() error = %v", err)
	}

	// Even though no artifacts changed, source_fingerprint should be updated.
	if result.UnchangedCount != 1 {
		t.Fatalf("unchanged_count = %d, want 1", result.UnchangedCount)
	}

	db := mustOpenReadOnly(t, indexPath)
	defer db.Close()

	var storedSourceFP string
	db.QueryRow(`SELECT value FROM metadata WHERE key = 'source_fingerprint'`).Scan(&storedSourceFP)

	// Verify freshness is now clean.
	status, err := InspectFreshnessContext(context.Background(), cfg)
	if err != nil {
		t.Fatalf("InspectFreshnessContext() error = %v", err)
	}
	if status.State != "fresh" {
		t.Fatalf("freshness state = %q after update, want fresh", status.State)
	}
}

func TestUpdatePreservesDBOnFailure(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	// Record the DB file hash before the failed update.
	hashBefore := fileHash(t, cfg.Workspace.ResolvedIndexPath)

	// Use a cancelled context to trigger failure mid-update.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err = UpdateContextWithOptions(ctx, cfg, records)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}

	// Verify the DB was restored from backup.
	hashAfter := fileHash(t, cfg.Workspace.ResolvedIndexPath)
	if hashBefore != hashAfter {
		t.Fatal("DB file hash changed after failed update; backup should have been restored")
	}

	// Verify no .bak file left behind.
	if _, err := os.Stat(cfg.Workspace.ResolvedIndexPath + ".bak"); !os.IsNotExist(err) {
		t.Fatal(".bak file should not exist after cleanup")
	}
}

func TestUpdateDeltaReportsAddedSpec(t *testing.T) {
	t.Parallel()

	indexPath := filepath.Join(t.TempDir(), "pituitary.db")
	repoDir := t.TempDir()

	// Set up a spec workspace with one spec.
	mustWriteFile(t, filepath.Join(repoDir, "specs", "auth", "spec.toml"), `id = "SPEC-001"
title = "Authentication"
status = "accepted"
domain = "auth"
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(repoDir, "specs", "auth", "body.md"), "# Authentication\n\nAuth spec body.\n")

	configPath := filepath.Join(t.TempDir(), "pituitary.toml")
	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(repoDir)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)

	cfg, err := loadAndRebuild(t, configPath)
	if err != nil {
		t.Fatal(err)
	}

	// Add a second spec.
	mustWriteFile(t, filepath.Join(repoDir, "specs", "rate-limit", "spec.toml"), `id = "SPEC-002"
title = "Rate Limiting"
status = "draft"
domain = "api"
body = "body.md"
`)
	mustWriteFile(t, filepath.Join(repoDir, "specs", "rate-limit", "body.md"), "# Rate Limiting\n\nRate limit spec body.\n")

	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}

	result, err := UpdateWithDeltaContextAndOptions(context.Background(), cfg, records, UpdateOptions{ComputeDelta: true}, nil)
	if err != nil {
		t.Fatalf("UpdateWithDeltaContextAndOptions() error = %v", err)
	}

	if result.Delta == nil {
		t.Fatal("expected non-nil delta")
	}
	if len(result.Delta.AddedSpecs) != 1 {
		t.Fatalf("expected 1 added spec, got %d", len(result.Delta.AddedSpecs))
	}
	if result.Delta.AddedSpecs[0].Ref != "SPEC-002" {
		t.Fatalf("expected added spec SPEC-002, got %s", result.Delta.AddedSpecs[0].Ref)
	}
	if !strings.Contains(result.Delta.Summary, "1 spec(s) added") {
		t.Fatalf("expected summary to mention added spec, got %q", result.Delta.Summary)
	}
}

func TestUpdateDeltaNoOpHasNoChanges(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	result, err := UpdateWithDeltaContextAndOptions(context.Background(), cfg, records, UpdateOptions{ComputeDelta: true}, nil)
	if err != nil {
		t.Fatalf("UpdateWithDeltaContextAndOptions() error = %v", err)
	}

	if result.Delta == nil {
		t.Fatal("expected non-nil delta")
	}
	if result.Delta.Summary != "no governance changes" {
		t.Fatalf("expected no governance changes, got %q", result.Delta.Summary)
	}
}

// loadAndRebuild loads config and performs an initial rebuild.
func loadAndRebuild(t *testing.T, configPath string) (*config.Config, error) {
	t.Helper()
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	if _, err := Rebuild(cfg, records); err != nil {
		return nil, err
	}
	return cfg, nil
}
