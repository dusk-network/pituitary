package index

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/source"
	ststore "github.com/dusk-network/stroma/v2/store"
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
	if result.FullRebuild {
		t.Fatalf("result = %+v, want incremental update path", result)
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
	originalSnapshotPath, err := currentStromaSnapshotPathContext(context.Background(), indexPath)
	if err != nil {
		t.Fatalf("currentStromaSnapshotPathContext() error = %v", err)
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
	if result.FullRebuild {
		t.Fatalf("result = %+v, want incremental update path", result)
	}
	if result.ArtifactCount != 2 {
		t.Fatalf("artifact_count = %d, want 2", result.ArtifactCount)
	}
	updatedSnapshotPath, err := currentStromaSnapshotPathContext(context.Background(), indexPath)
	if err != nil {
		t.Fatalf("currentStromaSnapshotPathContext() after update error = %v", err)
	}
	if updatedSnapshotPath == originalSnapshotPath {
		t.Fatalf("updated snapshot path = %q, want new content-addressed snapshot", updatedSnapshotPath)
	}
	if _, err := os.Stat(originalSnapshotPath); err != nil {
		t.Fatalf("stat original snapshot %s: %v", originalSnapshotPath, err)
	}
	if _, err := os.Stat(updatedSnapshotPath); err != nil {
		t.Fatalf("stat updated snapshot %s: %v", updatedSnapshotPath, err)
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
	if result.FullRebuild {
		t.Fatalf("result = %+v, want incremental update path", result)
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
	if result.FullRebuild {
		t.Fatalf("result = %+v, want incremental update path", result)
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
	originalSnapshotPath, err := currentStromaSnapshotPathContext(context.Background(), indexPath)
	if err != nil {
		t.Fatalf("currentStromaSnapshotPathContext() error = %v", err)
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
	if result.FullRebuild {
		t.Fatalf("result = %+v, want incremental update path", result)
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
	currentSnapshotPath, err := currentStromaSnapshotPathContext(context.Background(), indexPath)
	if err != nil {
		t.Fatalf("currentStromaSnapshotPathContext() after update error = %v", err)
	}
	if currentSnapshotPath != originalSnapshotPath {
		t.Fatalf("snapshot path = %q, want unchanged %q when only source fingerprint changed", currentSnapshotPath, originalSnapshotPath)
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

	// Verify the staged rebuild never replaced the live DB.
	hashAfter := fileHash(t, cfg.Workspace.ResolvedIndexPath)
	if hashBefore != hashAfter {
		t.Fatal("DB file hash changed after failed update; live DB should remain unchanged")
	}

	// Verify no legacy .bak file was left behind.
	if _, err := os.Stat(cfg.Workspace.ResolvedIndexPath + ".bak"); !os.IsNotExist(err) {
		t.Fatal(".bak file should not exist after cleanup")
	}
}

func TestNormalizeStromaUpdateErrorTreatsMissingSnapshotAsPrecondition(t *testing.T) {
	t.Parallel()

	err := normalizeStromaUpdateError("snapshot.db", &ststore.MissingIndexError{Path: "other.db"})
	if !IsUpdatePrecondition(err) {
		t.Fatalf("expected UpdatePreconditionError, got %v", err)
	}
	if !strings.Contains(err.Error(), "snapshot.db") {
		t.Fatalf("normalized error = %q, want snapshot path", err)
	}
	if !strings.Contains(err.Error(), "pituitary index --rebuild") {
		t.Fatalf("normalized error = %q, want rebuild guidance", err)
	}
}

func TestNormalizeStromaUpdateErrorTreatsCompatibilityErrorsAsPreconditions(t *testing.T) {
	t.Parallel()

	markers := []string{
		"schema version mismatch",
		"embedder fingerprint mismatch",
		"embedder dimension mismatch",
		"quantization mismatch",
		"update embedder is required when adding records",
	}
	for _, marker := range markers {
		marker := marker
		t.Run(marker, func(t *testing.T) {
			t.Parallel()

			err := normalizeStromaUpdateError("snapshot.db", fmt.Errorf("stroma update failed: %s", marker))
			if !IsUpdatePrecondition(err) {
				t.Fatalf("expected UpdatePreconditionError, got %v", err)
			}
			if !strings.Contains(err.Error(), marker) {
				t.Fatalf("normalized error = %q, want marker %q", err, marker)
			}
			if !strings.Contains(err.Error(), "pituitary index --rebuild") {
				t.Fatalf("normalized error = %q, want rebuild guidance", err)
			}
		})
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

// TestUpdateContextualizerPrefixMatchesFreshRebuild is the regression
// guard for #348: before this fix, stindex.Update was called without
// Contextualizer set, so changed records re-embedded with empty
// context_prefix while unchanged records kept the prefix stroma had
// persisted on the initial rebuild. The snapshot ended up with mixed
// semantics — old records with prefixes, new records without. This
// test proves that an incremental update now produces byte-identical
// context_prefix values to what a fresh rebuild at the updated state
// would produce.
func TestUpdateContextualizerPrefixMatchesFreshRebuild(t *testing.T) {
	t.Parallel()

	// Workspace A: rebuild → modify one doc → update.
	incrementalCfg, changedPrefix, unchangedPrefix := prefixesAfterUpdate(t)

	// Workspace B: rebuild directly against the modified content.
	freshChanged, freshUnchanged := prefixesAfterFreshRebuild(t)

	if changedPrefix == "" {
		t.Fatalf("workspace A post-update context_prefix (changed doc) is empty; contextualizer did not run on update (cfg.IndexPath=%s)", incrementalCfg.Workspace.ResolvedIndexPath)
	}
	if changedPrefix != freshChanged {
		t.Fatalf("post-update context_prefix (changed) = %q, want %q (fresh rebuild)", changedPrefix, freshChanged)
	}
	// Regression: the unchanged record must keep its original prefix
	// too — reuse logic in stroma can only preserve the prefix if
	// rebuild persisted a prefix and update didn't wipe it. A bug in
	// the threading that "mostly works" might re-chunk the changed
	// record correctly but invalidate reuse for the unchanged one.
	if unchangedPrefix == "" {
		t.Fatalf("workspace A post-update context_prefix (unchanged doc) is empty; reuse dropped the stored prefix")
	}
	if unchangedPrefix != freshUnchanged {
		t.Fatalf("post-update context_prefix (unchanged) = %q, want %q (fresh rebuild)", unchangedPrefix, freshUnchanged)
	}
}

// TestUpdateChunkingConfigSkewTriggersFullRebuild is the regression
// guard for the config-skew hazard flagged in the adversarial review
// of #348: after an operator changes runtime.chunking config between
// rebuild and update, the incremental path would otherwise re-chunk
// changed records under the new config while leaving unchanged
// records on the old chunk shapes, silently producing a mixed-
// generation snapshot. The rebuild-time chunking_config_fingerprint
// plus the precondition check at
// validateStoredChunkingConfigContext route the update through the
// existing precondition→fallback-to-full-rebuild path, producing a
// coherent snapshot without operator intervention.
func TestUpdateChunkingConfigSkewTriggersFullRebuild(t *testing.T) {
	t.Parallel()

	repoDir, indexPath := t.TempDir(), filepath.Join(t.TempDir(), "pituitary.db")
	configPath := filepath.Join(t.TempDir(), "pituitary.toml")
	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "mutable.md"), "# Mutable\n\n## Policy\n\ninitial body.\n")
	writeContextualizerConfig(t, configPath, repoDir, indexPath, "")
	if _, err := loadAndRebuild(t, configPath); err != nil {
		t.Fatalf("initial rebuild: %v", err)
	}

	// Operator enables the contextualizer and runs --update.
	writeContextualizerConfig(t, configPath, repoDir, indexPath, "title_ancestry")
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig: %v", err)
	}
	result, err := UpdateContextWithOptions(context.Background(), cfg, records)
	if err != nil {
		t.Fatalf("UpdateContextWithOptions: %v", err)
	}
	if !result.FullRebuild {
		t.Fatalf("result.FullRebuild = false, want true (config skew should force a full rebuild fallback); result=%+v", result)
	}
	// With the rebuild fallback, every record gets the new prefix
	// applied — no mixed-generation snapshot.
	if got := firstChunkContextPrefix(t, cfg.Workspace.ResolvedIndexPath, "doc://guides/mutable"); got == "" {
		t.Fatalf("expected contextualizer prefix on mutable doc after fallback rebuild; got empty")
	}
}

func writeContextualizerConfig(t *testing.T, configPath, repoDir, indexPath, format string) {
	t.Helper()
	block := ""
	if format != "" {
		block = fmt.Sprintf("\n[runtime.chunking.contextualizer]\nformat = %q\n", format)
	}
	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(repoDir)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
`+block+`
[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
`)
}

// TestUpdateChunkPolicyAppliedToChangedRecord is the regression guard
// for the chunk-policy half of #348: before this fix, the per-kind
// ChunkPolicy was not threaded into stindex.Update, so changed records
// re-chunked through stroma's default MarkdownPolicy instead of the
// configured policy. We prove parity here by comparing section count
// for a changed record against what a fresh rebuild produces.
func TestUpdateChunkPolicyAppliedToChangedRecord(t *testing.T) {
	t.Parallel()

	incrementalCount, _ := sectionCountAndHeadingsAfterUpdate(t)
	freshCount, _ := sectionCountAndHeadingsAfterFreshRebuild(t)

	if incrementalCount == 0 {
		t.Fatal("workspace A post-update section count is 0; update did not persist the changed record")
	}
	if incrementalCount != freshCount {
		t.Fatalf("post-update section count = %d, want %d (fresh rebuild); update path is not applying configured ChunkPolicy (fell back to default MarkdownPolicy heading-split)", incrementalCount, freshCount)
	}
}

// prefixesAfterUpdate walks a rebuild → modify one doc → update
// sequence with contextualizer enabled and returns the persisted
// context_prefix for both a changed doc and an unchanged sibling.
// The unchanged sibling is critical: it catches bugs where the
// update path correctly re-chunks the changed doc but drops or
// mutates reused prefixes for records that shouldn't have touched.
func prefixesAfterUpdate(t *testing.T) (cfg *config.Config, changed, unchanged string) {
	t.Helper()
	repoDir, configPath, _ := contextualizerFixture(t, "initial")
	cfg, err := loadAndRebuild(t, configPath)
	if err != nil {
		t.Fatalf("initial rebuild: %v", err)
	}

	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "mutable.md"),
		"# Mutable\n\n## Policy\n\nupdated body for parity test.\n")
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig (post-edit): %v", err)
	}
	result, err := UpdateContextWithOptions(context.Background(), cfg, records)
	if err != nil {
		t.Fatalf("UpdateContextWithOptions: %v", err)
	}
	if result.FullRebuild {
		t.Fatalf("update fell through to full rebuild; cannot exercise the incremental path")
	}
	return cfg,
		firstChunkContextPrefix(t, cfg.Workspace.ResolvedIndexPath, "doc://guides/mutable"),
		firstChunkContextPrefix(t, cfg.Workspace.ResolvedIndexPath, "doc://guides/stable")
}

// prefixesAfterFreshRebuild does a single rebuild against the
// already-updated content and returns both the changed and unchanged
// records' first-chunk prefixes.
func prefixesAfterFreshRebuild(t *testing.T) (changed, unchanged string) {
	t.Helper()
	_, configPath, _ := contextualizerFixture(t, "updated")
	cfg, err := loadAndRebuild(t, configPath)
	if err != nil {
		t.Fatalf("fresh rebuild: %v", err)
	}
	return firstChunkContextPrefix(t, cfg.Workspace.ResolvedIndexPath, "doc://guides/mutable"),
		firstChunkContextPrefix(t, cfg.Workspace.ResolvedIndexPath, "doc://guides/stable")
}

// sectionCountAndHeadingsAfterUpdate runs rebuild → modify → update
// with a non-default ChunkPolicy and returns the section count +
// headings for the changed record.
func sectionCountAndHeadingsAfterUpdate(t *testing.T) (int, []string) {
	t.Helper()
	repoDir, configPath, _ := chunkPolicyFixture(t, "initial")
	cfg, err := loadAndRebuild(t, configPath)
	if err != nil {
		t.Fatalf("initial rebuild: %v", err)
	}
	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "mutable.md"),
		"# Mutable\n\n## Policy\n\nSection one body with several descriptive words for token-budget.\n\n## Retention\n\nSection two body with several additional descriptive words for token-budget.\n\n## Expiry\n\nSection three body with several more descriptive words for token-budget.\n")
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig (post-edit): %v", err)
	}
	result, err := UpdateContextWithOptions(context.Background(), cfg, records)
	if err != nil {
		t.Fatalf("UpdateContextWithOptions: %v", err)
	}
	if result.FullRebuild {
		t.Fatalf("update fell through to full rebuild; cannot exercise the incremental path")
	}
	return chunkDetailsForRecord(t, cfg.Workspace.ResolvedIndexPath, "doc://guides/mutable")
}

// sectionCountAndHeadingsAfterFreshRebuild rebuilds directly against
// the post-edit content with the same ChunkPolicy and returns the
// changed record's section count + headings.
func sectionCountAndHeadingsAfterFreshRebuild(t *testing.T) (int, []string) {
	t.Helper()
	_, configPath, _ := chunkPolicyFixture(t, "updated")
	cfg, err := loadAndRebuild(t, configPath)
	if err != nil {
		t.Fatalf("fresh rebuild: %v", err)
	}
	return chunkDetailsForRecord(t, cfg.Workspace.ResolvedIndexPath, "doc://guides/mutable")
}

// contextualizerFixture writes a workspace with the contextualizer
// enabled. The content stage ("initial" or "updated") selects whether
// the mutable doc has its pre- or post-edit body so the same function
// covers both halves of the rebuild/update parity comparison.
func contextualizerFixture(t *testing.T, stage string) (repoDir, configPath, indexPath string) {
	t.Helper()
	repoDir = t.TempDir()
	indexPath = filepath.Join(t.TempDir(), "pituitary.db")
	configPath = filepath.Join(t.TempDir(), "pituitary.toml")

	body := "# Mutable\n\n## Policy\n\ninitial body.\n"
	if stage == "updated" {
		body = "# Mutable\n\n## Policy\n\nupdated body for parity test.\n"
	}
	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "mutable.md"), body)
	// Second doc — never edited across stages. Its prefix must
	// survive an incremental update unchanged so the test can prove
	// the update path preserved the reused chunk's context_prefix
	// column rather than wiping it.
	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "stable.md"),
		"# Stable\n\n## Reference\n\nbody that never changes.\n")

	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(repoDir)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[runtime.chunking.contextualizer]
format = "title_ancestry"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
`)
	return repoDir, configPath, indexPath
}

// chunkPolicyFixture writes a workspace configured with late_chunk
// for docs. Late-chunking emits parent + leaf chunks, so the stored
// chunk count for a multi-section record differs structurally from
// what stroma's default MarkdownPolicy would produce — that
// divergence is the signal the test pivots on to prove the policy is
// honored on the update path and not silently reverted to the
// default.
func chunkPolicyFixture(t *testing.T, stage string) (repoDir, configPath, indexPath string) {
	t.Helper()
	repoDir = t.TempDir()
	indexPath = filepath.Join(t.TempDir(), "pituitary.db")
	configPath = filepath.Join(t.TempDir(), "pituitary.toml")

	body := "# Mutable\n\n## Policy\n\ninitial body paragraph.\n"
	if stage == "updated" {
		body = "# Mutable\n\n## Policy\n\nSection one body with several descriptive words for token-budget.\n\n## Retention\n\nSection two body with several additional descriptive words for token-budget.\n\n## Expiry\n\nSection three body with several more descriptive words for token-budget.\n"
	}
	mustWriteFile(t, filepath.Join(repoDir, "docs", "guides", "mutable.md"), body)

	mustWriteFile(t, configPath, `
[workspace]
root = "`+filepath.ToSlash(repoDir)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[runtime.chunking.doc]
policy = "late_chunk"
max_tokens = 64
child_max_tokens = 4
child_overlap_tokens = 1

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md"]
`)
	return repoDir, configPath, indexPath
}

// firstChunkContextPrefix reads the persisted context_prefix for the
// first chunk of the named record from the stroma snapshot DB.
func firstChunkContextPrefix(t *testing.T, indexPath, recordRef string) string {
	t.Helper()
	db := mustOpenCorpusReadOnly(t, indexPath)
	defer db.Close()
	var prefix string
	if err := db.QueryRow(
		`SELECT context_prefix FROM chunks WHERE record_ref = ? ORDER BY id LIMIT 1`,
		recordRef,
	).Scan(&prefix); err != nil {
		t.Fatalf("query context_prefix for %s: %v", recordRef, err)
	}
	return prefix
}

// chunkDetailsForRecord returns the chunk count and headings stored
// for the named record in the snapshot, preserving id-order.
func chunkDetailsForRecord(t *testing.T, indexPath, recordRef string) (int, []string) {
	t.Helper()
	db := mustOpenCorpusReadOnly(t, indexPath)
	defer db.Close()
	rows, err := db.Query(
		`SELECT heading FROM chunks WHERE record_ref = ? ORDER BY id`,
		recordRef,
	)
	if err != nil {
		t.Fatalf("query chunks for %s: %v", recordRef, err)
	}
	defer rows.Close()
	var headings []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			t.Fatalf("scan chunk for %s: %v", recordRef, err)
		}
		headings = append(headings, h)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate chunks for %s: %v", recordRef, err)
	}
	return len(headings), headings
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
