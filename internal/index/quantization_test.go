package index

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/source"
)

// TestRebuildWritesConfiguredQuantizationToSnapshotMetadata is the
// rebuild-side end-to-end check that runtime.quantization survives all
// the way through stindex.BuildOptions and lands in the published stroma
// snapshot's metadata. Without this assertion, a regression in the
// rebuild.go plumbing would silently revert the snapshot to float32.
func TestRebuildWritesConfiguredQuantizationToSnapshotMetadata(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfigWithQuantization(t, config.QuantizationInt8)

	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	corpusDB := mustOpenCorpusReadOnly(t, cfg.Workspace.ResolvedIndexPath)
	defer corpusDB.Close()

	var got string
	if err := corpusDB.QueryRow(`SELECT value FROM metadata WHERE key = 'quantization'`).Scan(&got); err != nil {
		t.Fatalf("read snapshot metadata.quantization: %v", err)
	}
	if got != config.QuantizationInt8 {
		t.Fatalf("snapshot metadata.quantization = %q, want %q", got, config.QuantizationInt8)
	}
}

// TestUpdateAcrossQuantizationChangeForcesFullRebuild is the update-side
// end-to-end check that mirrors the rebuild test above. A quantization
// flip between rebuild and update must surface as
// "quantization mismatch" out of stroma's SyncFromSource, get normalized
// to an UpdatePreconditionError, and force the update path to fall back
// to a full rebuild — preserving the AC that quantization changes are
// not a reuse-compatible delta. This is the trip-wire that arms the
// runtime.quantization knob; without the UpdateOptions plumbing in
// update.go a configured int8 would silently keep reusing the stored
// float32 snapshot.
func TestUpdateAcrossQuantizationChangeForcesFullRebuild(t *testing.T) {
	t.Parallel()

	indexPath := filepath.Join(t.TempDir(), "pituitary.db")

	cfg := loadFixtureConfigWithQuantizationAtIndexPath(t, "", indexPath)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("baseline Rebuild() error = %v", err)
	}

	cfgInt8 := loadFixtureConfigWithQuantizationAtIndexPath(t, config.QuantizationInt8, indexPath)
	recordsInt8, err := source.LoadFromConfig(cfgInt8)
	if err != nil {
		t.Fatalf("second source.LoadFromConfig() error = %v", err)
	}
	result, err := UpdateContextWithOptions(context.Background(), cfgInt8, recordsInt8)
	if err != nil {
		t.Fatalf("UpdateContextWithOptions() error = %v", err)
	}
	if !result.Update {
		t.Fatalf("result.Update = false, want true (still an update operation)")
	}
	if !result.FullRebuild {
		t.Fatalf("result.FullRebuild = false, want true — quantization change must force rebuild rather than reuse")
	}

	corpusDB := mustOpenCorpusReadOnly(t, cfgInt8.Workspace.ResolvedIndexPath)
	defer corpusDB.Close()
	var got string
	if err := corpusDB.QueryRow(`SELECT value FROM metadata WHERE key = 'quantization'`).Scan(&got); err != nil {
		t.Fatalf("read snapshot metadata.quantization after rebuild: %v", err)
	}
	if got != config.QuantizationInt8 {
		t.Fatalf("snapshot metadata.quantization = %q, want %q after forced rebuild", got, config.QuantizationInt8)
	}
}

// TestRebuildAcrossQuantizationChangeReportsQuantizationReuseReason
// pins the reuse-reporting fix paired with the no-diff fast-path
// eligibility-gate fix. When a `--rebuild` runs after a quantization
// flip, pituitary's loadReuseStateContext must short-circuit to the
// "quantization" disabled reason — not silently keep the embedder-only
// view that inflates ReusedArtifactCount/ReusedChunkCount while stroma
// internally rejects reuse. Without this assertion the reporting layer
// can desync from stroma's actual reuse behavior and a regression in
// reuse.go is invisible.
func TestRebuildAcrossQuantizationChangeReportsQuantizationReuseReason(t *testing.T) {
	t.Parallel()

	indexPath := filepath.Join(t.TempDir(), "pituitary.db")

	cfg := loadFixtureConfigWithQuantizationAtIndexPath(t, "", indexPath)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("baseline Rebuild() error = %v", err)
	}

	cfgInt8 := loadFixtureConfigWithQuantizationAtIndexPath(t, config.QuantizationInt8, indexPath)
	recordsInt8, err := source.LoadFromConfig(cfgInt8)
	if err != nil {
		t.Fatalf("second source.LoadFromConfig() error = %v", err)
	}
	result, err := Rebuild(cfgInt8, recordsInt8)
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if result.ReuseDisabledReason == "" {
		t.Fatalf("expected non-empty ReuseDisabledReason after quantization flip; result = %+v", result)
	}
	if got := result.ReuseDisabledReason; !strings.Contains(got, "quantization") {
		t.Fatalf("ReuseDisabledReason = %q, want it to mention quantization", got)
	}
	if result.ReusedArtifactCount != 0 {
		t.Fatalf("ReusedArtifactCount = %d, want 0 (reuse must be disabled across quantization change)", result.ReusedArtifactCount)
	}
	if result.ReusedChunkCount != 0 {
		t.Fatalf("ReusedChunkCount = %d, want 0 (reuse must be disabled across quantization change)", result.ReusedChunkCount)
	}
}

func loadFixtureConfigWithQuantization(tb testing.TB, quantization string) *config.Config {
	tb.Helper()
	return loadFixtureConfigWithQuantizationAtIndexPath(tb, quantization, filepath.Join(tb.TempDir(), "pituitary.db"))
}

func loadFixtureConfigWithQuantizationAtIndexPath(tb testing.TB, quantization, indexPath string) *config.Config {
	tb.Helper()

	repoRoot := repoRoot(tb)
	configPath := filepath.Join(tb.TempDir(), "pituitary.toml")
	body := `
[workspace]
root = "` + filepath.ToSlash(repoRoot) + `"
index_path = "` + filepath.ToSlash(indexPath) + `"
infer_applies_to = false
`
	if quantization != "" {
		body += `
[runtime]
quantization = "` + quantization + `"
`
	}
	body += `
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
`
	mustWriteFile(tb, configPath, body)

	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}
