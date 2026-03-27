package index

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

var sqliteDriverOnce sync.Once
var sqliteReadyMu sync.Mutex
var sqliteReady bool
var sqliteReadinessProbe = probeSQLiteReadyContext

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
	info, err := os.Stat(path)
	switch {
	case os.IsNotExist(err):
		return nil, &MissingIndexError{Path: path}
	case err != nil:
		return nil, fmt.Errorf("stat index %s: %w", path, err)
	case info.IsDir():
		return nil, fmt.Errorf("index path %s is a directory", path)
	}

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
	normalizedPath := filepath.ToSlash(path)
	if hasWindowsDrivePrefix(normalizedPath) && !strings.HasPrefix(normalizedPath, "/") {
		normalizedPath = "/" + normalizedPath
	}
	u := url.URL{
		Scheme: "file",
		Path:   normalizedPath,
	}
	query := url.Values{}
	if mode != "" {
		query.Set("mode", mode)
	}
	u.RawQuery = query.Encode()
	return u.String()
}

func hasWindowsDrivePrefix(path string) bool {
	if len(path) < 2 || path[1] != ':' {
		return false
	}
	drive := path[0]
	return (drive >= 'a' && drive <= 'z') || (drive >= 'A' && drive <= 'Z')
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

	sqliteReadyMu.Lock()
	defer sqliteReadyMu.Unlock()

	if sqliteReady {
		return ctx.Err()
	}
	if err := sqliteReadinessProbe(ctx); err != nil {
		return err
	}
	sqliteReady = true
	return ctx.Err()
}

func configureDB(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	return nil
}

func probeSQLiteReadyContext(ctx context.Context) error {
	ensureSQLiteDriver()

	db, err := sql.Open("sqlite3", sqliteReadinessDSN)
	if err != nil {
		return fmt.Errorf("open sqlite readiness probe: %w", err)
	}
	defer db.Close()

	if err := configureDB(ctx, db); err != nil {
		return fmt.Errorf("configure sqlite readiness probe: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE VIRTUAL TABLE pituitary_vec_probe USING vec0(
chunk_id integer primary key,
embedding float[8] distance_metric=cosine
)`); err != nil {
		return fmt.Errorf("initialize sqlite-vec extension: %w", err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE pituitary_vec_probe`); err != nil {
		return fmt.Errorf("drop sqlite-vec readiness probe: %w", err)
	}
	return nil
}
