package index

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"sync"

	sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

var sqliteDriverOnce sync.Once
var sqliteReadyOnce sync.Once
var sqliteReadyErr error

const sqliteReadinessDSN = "file:pituitary-sqlite-ready?mode=memory&cache=shared"

func openReadWrite(path string) (*sql.DB, error) {
	return openReadWriteContext(context.Background(), path)
}

func openReadWriteContext(ctx context.Context, path string) (*sql.DB, error) {
	if err := CheckSQLiteReadyContext(ctx); err != nil {
		return nil, err
	}
	dsn := sqliteURI(path, "")
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	if err := configureDB(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// OpenReadOnly opens a fresh read-only SQLite handle for query paths.
func OpenReadOnly(path string) (*sql.DB, error) {
	return OpenReadOnlyContext(context.Background(), path)
}

// OpenReadOnlyContext opens a fresh read-only SQLite handle for query paths.
func OpenReadOnlyContext(ctx context.Context, path string) (*sql.DB, error) {
	if err := CheckSQLiteReadyContext(ctx); err != nil {
		return nil, err
	}
	dsn := sqliteURI(path, "ro")
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	if err := configureDB(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `PRAGMA query_only = ON`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func sqliteURI(path, mode string) string {
	u := url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(path),
	}
	query := url.Values{}
	if mode != "" {
		query.Set("mode", mode)
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func ensureSQLiteDriver() {
	sqliteDriverOnce.Do(sqlitevec.Auto)
}

// CheckSQLiteReady validates that the SQLite driver and sqlite-vec extension are usable.
func CheckSQLiteReady() error {
	return CheckSQLiteReadyContext(context.Background())
}

// CheckSQLiteReadyContext validates that the SQLite driver and sqlite-vec extension are usable.
func CheckSQLiteReadyContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	sqliteReadyOnce.Do(func() {
		sqliteReadyErr = probeSQLiteReady()
	})
	if sqliteReadyErr != nil {
		return sqliteReadyErr
	}
	return ctx.Err()
}

func configureDB(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	return nil
}

func probeSQLiteReady() error {
	ensureSQLiteDriver()

	db, err := sql.Open("sqlite3", sqliteReadinessDSN)
	if err != nil {
		return fmt.Errorf("open sqlite readiness probe: %w", err)
	}
	defer db.Close()

	if err := configureDB(context.Background(), db); err != nil {
		return fmt.Errorf("configure sqlite readiness probe: %w", err)
	}
	if _, err := db.Exec(`CREATE VIRTUAL TABLE pituitary_vec_probe USING vec0(
chunk_id integer primary key,
embedding float[8] distance_metric=cosine
)`); err != nil {
		return fmt.Errorf("initialize sqlite-vec extension: %w", err)
	}
	if _, err := db.Exec(`DROP TABLE pituitary_vec_probe`); err != nil {
		return fmt.Errorf("drop sqlite-vec readiness probe: %w", err)
	}
	return nil
}
