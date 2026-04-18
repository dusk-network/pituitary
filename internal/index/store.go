package index

import (
	"context"
	"database/sql"

	ststore "github.com/dusk-network/stroma/v2/store"
)

func openReadWriteContext(ctx context.Context, path string) (*sql.DB, error) {
	return ststore.OpenReadWriteContext(ctx, path)
}

// OpenReadOnly opens a fresh read-only SQLite handle for query paths.
func OpenReadOnly(path string) (*sql.DB, error) {
	return OpenReadOnlyContext(context.Background(), path)
}

// OpenReadOnlyContext opens a fresh read-only SQLite handle for query paths.
func OpenReadOnlyContext(ctx context.Context, path string) (*sql.DB, error) {
	db, err := ststore.OpenReadOnlyContext(ctx, path)
	if err != nil {
		return nil, normalizeStoreError(err)
	}
	return db, nil
}

// CheckSQLiteReady validates that the SQLite driver and sqlite-vec extension are usable.
func CheckSQLiteReady() error {
	return CheckSQLiteReadyContext(context.Background())
}

// CheckSQLiteReadyContext validates that the SQLite driver and sqlite-vec extension are usable.
func CheckSQLiteReadyContext(ctx context.Context) error {
	return ststore.CheckSQLiteReadyContext(ctx)
}

func normalizeStoreError(err error) error {
	if !ststore.IsMissingIndex(err) {
		return err
	}
	return &MissingIndexError{Path: ststore.MissingIndexPath(err)}
}
