package index

import (
	"context"
	"errors"
	"testing"
)

func TestSQLiteURIUsesWindowsSafeDriveLetterForm(t *testing.T) {
	t.Parallel()

	got := sqliteURI("C:/Users/alice/pituitary.db", "")
	want := "file:///C:/Users/alice/pituitary.db"
	if got != want {
		t.Fatalf("sqliteURI() = %q, want %q", got, want)
	}
}

func TestSQLiteURIPreservesModeForWindowsDriveLetterPaths(t *testing.T) {
	t.Parallel()

	got := sqliteURI("C:/Users/alice/pituitary.db.new", "ro")
	want := "file:///C:/Users/alice/pituitary.db.new?mode=ro"
	if got != want {
		t.Fatalf("sqliteURI() = %q, want %q", got, want)
	}
}

func TestCheckSQLiteReadyPasses(t *testing.T) {
	resetSQLiteReadyForTest(t)

	if err := CheckSQLiteReady(); err != nil {
		t.Fatalf("CheckSQLiteReady() error = %v", err)
	}
}

func TestCheckSQLiteReadyRetriesAfterFailure(t *testing.T) {
	resetSQLiteReadyForTest(t)

	transient := errors.New("transient sqlite readiness failure")
	callCount := 0

	sqliteReadyMu.Lock()
	sqliteReadinessProbe = func(context.Context) error {
		callCount++
		if callCount == 1 {
			return transient
		}
		return nil
	}
	sqliteReadyMu.Unlock()

	if err := CheckSQLiteReady(); !errors.Is(err, transient) {
		t.Fatalf("first CheckSQLiteReady() error = %v, want %v", err, transient)
	}
	if err := CheckSQLiteReady(); err != nil {
		t.Fatalf("second CheckSQLiteReady() error = %v", err)
	}
	if callCount != 2 {
		t.Fatalf("probe call count = %d, want 2", callCount)
	}
	if !sqliteReady {
		t.Fatal("sqliteReady = false, want true after successful retry")
	}
}

func TestCheckSQLiteReadyCachesSuccess(t *testing.T) {
	resetSQLiteReadyForTest(t)

	callCount := 0

	sqliteReadyMu.Lock()
	sqliteReadinessProbe = func(context.Context) error {
		callCount++
		return nil
	}
	sqliteReadyMu.Unlock()

	if err := CheckSQLiteReady(); err != nil {
		t.Fatalf("first CheckSQLiteReady() error = %v", err)
	}
	if err := CheckSQLiteReady(); err != nil {
		t.Fatalf("second CheckSQLiteReady() error = %v", err)
	}
	if callCount != 1 {
		t.Fatalf("probe call count = %d, want 1", callCount)
	}
}

func resetSQLiteReadyForTest(t *testing.T) {
	t.Helper()

	sqliteReadyMu.Lock()
	sqliteReady = false
	sqliteReadinessProbe = probeSQLiteReadyContext
	sqliteReadyMu.Unlock()

	t.Cleanup(func() {
		sqliteReadyMu.Lock()
		sqliteReady = false
		sqliteReadinessProbe = probeSQLiteReadyContext
		sqliteReadyMu.Unlock()
	})
}
