package index

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCheckSQLiteReadyPasses(t *testing.T) {
	t.Parallel()

	if err := CheckSQLiteReady(); err != nil {
		t.Fatalf("CheckSQLiteReady() error = %v", err)
	}
}

func TestOpenReadOnlyContextWrapsMissingIndexErrors(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.db")
	_, err := OpenReadOnlyContext(context.Background(), path)
	if err == nil {
		t.Fatal("OpenReadOnlyContext() error = nil, want missing index failure")
	}
	if !IsMissingIndex(err) {
		t.Fatalf("OpenReadOnlyContext() error = %T (%v), want MissingIndexError", err, err)
	}
	if got, want := MissingIndexPath(err), path; got != want {
		t.Fatalf("MissingIndexPath() = %q, want %q", got, want)
	}
	if got, want := err.Error(), "index "+path+" does not exist; run `pituitary index --rebuild`"; got != want {
		t.Fatalf("OpenReadOnlyContext() error = %q, want %q", got, want)
	}
}

func TestOpenReadOnlyContextOpensExistingIndex(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "pituitary.db")
	writer, err := openReadWriteContext(context.Background(), path)
	if err != nil {
		t.Fatalf("openReadWriteContext() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	db, err := OpenReadOnlyContext(context.Background(), path)
	if err != nil {
		t.Fatalf("OpenReadOnlyContext() error = %v", err)
	}
	defer db.Close()
}
