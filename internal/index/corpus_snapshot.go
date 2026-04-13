package index

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	stindex "github.com/dusk-network/stroma/index"
)

const stromaSnapshotPathKey = "stroma_snapshot_path"

func stromaSnapshotPathForContent(indexPath, contentFingerprint string) string {
	dir := filepath.Dir(indexPath)
	ext := filepath.Ext(indexPath)
	base := strings.TrimSuffix(filepath.Base(indexPath), ext)
	fingerprint := strings.TrimSpace(contentFingerprint)
	if fingerprint == "" {
		fingerprint = "snapshot"
	}
	return filepath.Join(dir, base+".stroma."+fingerprint+".db")
}

func currentStromaSnapshotPathContext(ctx context.Context, indexPath string) (string, error) {
	info, err := os.Stat(indexPath)
	switch {
	case os.IsNotExist(err):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("stat index %s: %w", indexPath, err)
	case info.IsDir():
		return "", fmt.Errorf("index path %s is a directory", indexPath)
	}

	db, err := OpenReadOnlyContext(ctx, indexPath)
	if err != nil {
		return "", err
	}
	defer db.Close()

	path, err := readOptionalMetadataValueContext(ctx, db, stromaSnapshotPathKey)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(path) != "" {
		return path, nil
	}

	hasRecords, err := tableExistsContext(ctx, db, "records")
	if err == nil && hasRecords {
		return indexPath, nil
	}

	// Unknown legacy layouts should simply disable reuse for rebuild paths.
	return "", nil
}

func stromaSnapshotPathFromDBContext(ctx context.Context, db *sql.DB, indexPath string) (string, error) {
	if db == nil {
		return "", fmt.Errorf("index database is required")
	}

	path, err := readOptionalMetadataValueContext(ctx, db, stromaSnapshotPathKey)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(path) != "" {
		return path, nil
	}

	// Legacy mixed indexes stored the corpus tables in the same SQLite file.
	hasRecords, err := tableExistsContext(ctx, db, "records")
	if err == nil && hasRecords {
		return indexPath, nil
	}

	return "", fmt.Errorf("index metadata is missing %s; run `pituitary index --rebuild`", stromaSnapshotPathKey)
}

func OpenStromaSnapshotContext(ctx context.Context, db *sql.DB, indexPath string) (*stindex.Snapshot, error) {
	path, err := stromaSnapshotPathFromDBContext(ctx, db, indexPath)
	if err != nil {
		return nil, err
	}
	snapshot, err := stindex.OpenSnapshot(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("open stroma snapshot %s: %w", path, err)
	}
	return snapshot, nil
}

func readOptionalMetadataValueContext(ctx context.Context, db *sql.DB, key string) (string, error) {
	var value string
	err := db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read metadata %s: %w", key, err)
	}
	return value, nil
}

func tableExistsContext(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, name).Scan(&count); err != nil {
		return false, fmt.Errorf("check table %s: %w", name, err)
	}
	return count > 0, nil
}
