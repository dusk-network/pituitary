package index

import "testing"

func TestCheckSQLiteReadyPasses(t *testing.T) {
	t.Parallel()

	if err := CheckSQLiteReady(); err != nil {
		t.Fatalf("CheckSQLiteReady() error = %v", err)
	}
}
