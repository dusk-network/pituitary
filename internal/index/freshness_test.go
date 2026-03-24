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

func TestInspectFreshnessReportsFreshAfterRebuild(t *testing.T) {
	t.Parallel()

	cfg := loadFreshnessFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	status, err := InspectFreshnessContext(context.Background(), cfg)
	if err != nil {
		t.Fatalf("InspectFreshnessContext() error = %v", err)
	}
	if got, want := status.State, freshnessStateFresh; got != want {
		t.Fatalf("freshness.state = %q, want %q", got, want)
	}
	if len(status.Issues) != 0 {
		t.Fatalf("freshness.issues = %+v, want none", status.Issues)
	}
	if status.Action != "" {
		t.Fatalf("freshness.action = %q, want empty", status.Action)
	}
}

func TestInspectFreshnessReportsStaleWhenWorkspaceContentChanges(t *testing.T) {
	t.Parallel()

	cfg := loadFreshnessFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	mustWriteFile(t, filepath.Join(cfg.Workspace.RootPath, "docs", "guides", "api-rate-limits.md"), `
# API Rate Limits

Updated guide content for freshness testing.
`)

	status, err := InspectFreshnessContext(context.Background(), cfg)
	if err != nil {
		t.Fatalf("InspectFreshnessContext() error = %v", err)
	}
	if got, want := status.State, freshnessStateStale; got != want {
		t.Fatalf("freshness.state = %q, want %q", got, want)
	}
	if len(status.Issues) != 1 || status.Issues[0].Kind != "content_fingerprint_mismatch" {
		t.Fatalf("freshness.issues = %+v, want content_fingerprint_mismatch", status.Issues)
	}
	if !strings.Contains(status.Action, "pituitary index --rebuild") {
		t.Fatalf("freshness.action = %q, want rebuild guidance", status.Action)
	}
}

func TestInspectFreshnessReportsIncompatibleWhenSourceConfigChanges(t *testing.T) {
	t.Parallel()

	cfg := loadFreshnessFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	cfg.Sources[1].Include = []string{"guides/*.md"}

	status, err := InspectFreshnessContext(context.Background(), cfg)
	if err != nil {
		t.Fatalf("InspectFreshnessContext() error = %v", err)
	}
	if got, want := status.State, freshnessStateIncompatible; got != want {
		t.Fatalf("freshness.state = %q, want %q", got, want)
	}
	if len(status.Issues) == 0 || status.Issues[0].Kind != "source_fingerprint_mismatch" {
		t.Fatalf("freshness.issues = %+v, want source_fingerprint_mismatch", status.Issues)
	}
}

func TestInspectFreshnessReportsIncompatibleWhenMetadataIsMissing(t *testing.T) {
	t.Parallel()

	cfg := loadFreshnessFixtureConfig(t)
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
	if _, err := db.Exec(`DELETE FROM metadata WHERE key = 'source_fingerprint'`); err != nil {
		t.Fatalf("delete source_fingerprint: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close writable db: %v", err)
	}

	status, err := InspectFreshnessContext(context.Background(), cfg)
	if err != nil {
		t.Fatalf("InspectFreshnessContext() error = %v", err)
	}
	if got, want := status.State, freshnessStateIncompatible; got != want {
		t.Fatalf("freshness.state = %q, want %q", got, want)
	}
	if len(status.Issues) == 0 || status.Issues[0].Kind != "missing_source_fingerprint" {
		t.Fatalf("freshness.issues = %+v, want missing_source_fingerprint", status.Issues)
	}
}

func loadFreshnessFixtureConfig(tb testing.TB) *config.Config {
	tb.Helper()

	repoRoot := repoRoot(tb)
	root := tb.TempDir()
	copyTree(tb, filepath.Join(repoRoot, "specs"), filepath.Join(root, "specs"))
	copyTree(tb, filepath.Join(repoRoot, "docs"), filepath.Join(root, "docs"))

	configPath := filepath.Join(root, "pituitary.toml")
	mustWriteFile(tb, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

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

func copyTree(tb testing.TB, src, dst string) {
	tb.Helper()

	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		tb.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
}
