package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixDocDriftPlansDeterministicEdits(t *testing.T) {
	t.Parallel()

	configPath := writeOperationWorkspace(t, false)
	operation := FixDocDrift(context.Background(), configPath, FixRequest{
		Path: "docs/guides/api-rate-limits.md",
	})
	if operation.Issue != nil {
		t.Fatalf("FixDocDrift() issue = %+v, want nil", operation.Issue)
	}
	if operation.Result == nil {
		t.Fatal("FixDocDrift() result = nil, want structured result")
	}
	if operation.Result.Applied {
		t.Fatal("FixDocDrift() result.applied = true, want false")
	}
	if got, want := operation.Result.PlannedFileCount, 1; got != want {
		t.Fatalf("planned_file_count = %d, want %d", got, want)
	}
	if operation.Result.PlannedEditCount < 3 {
		t.Fatalf("planned_edit_count = %d, want at least 3", operation.Result.PlannedEditCount)
	}
	if len(operation.Result.Files) != 1 {
		t.Fatalf("files = %+v, want one file", operation.Result.Files)
	}
	file := operation.Result.Files[0]
	if got, want := file.DocRef, "doc://guides/api-rate-limits"; got != want {
		t.Fatalf("file.doc_ref = %q, want %q", got, want)
	}
	if file.Status != "planned" {
		t.Fatalf("file.status = %q, want planned", file.Status)
	}
	if len(file.Edits) < 3 {
		t.Fatalf("file.edits = %+v, want multiple edits", file.Edits)
	}
	if len(operation.Result.Guidance) == 0 || !strings.Contains(operation.Result.Guidance[0], "--yes") {
		t.Fatalf("guidance = %v, want apply guidance", operation.Result.Guidance)
	}
}

func TestFixDocDriftAppliesEditsAndMarksIndexStale(t *testing.T) {
	t.Parallel()

	configPath := writeOperationWorkspace(t, false)
	operation := FixDocDrift(context.Background(), configPath, FixRequest{
		Path:  "docs/guides/api-rate-limits.md",
		Apply: true,
	})
	if operation.Issue != nil {
		t.Fatalf("FixDocDrift() issue = %+v, want nil", operation.Issue)
	}
	if operation.Result == nil {
		t.Fatal("FixDocDrift() result = nil, want structured result")
	}
	if !operation.Result.Applied {
		t.Fatal("FixDocDrift() result.applied = false, want true")
	}
	if got, want := operation.Result.AppliedFileCount, 1; got != want {
		t.Fatalf("applied_file_count = %d, want %d", got, want)
	}
	if operation.Result.AppliedEditCount < 3 {
		t.Fatalf("applied_edit_count = %d, want at least 3", operation.Result.AppliedEditCount)
	}
	if len(operation.Result.Guidance) == 0 || !strings.Contains(operation.Result.Guidance[0], "pituitary index --rebuild") {
		t.Fatalf("guidance = %v, want rebuild guidance", operation.Result.Guidance)
	}

	guidePath := filepath.Join(filepath.Dir(configPath), "docs", "guides", "api-rate-limits.md")
	updatedBytes, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	updated := string(updatedBytes)
	for _, want := range []string{
		"sliding-window rate limiter",
		"200 requests per minute",
		"tenant-specific overrides are supported through configuration",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated guide %q does not contain %q", updated, want)
		}
	}
}

func TestFixDocDriftMatchesPathCaseInsensitivelyOnWindows(t *testing.T) {
	previous := fixPathCaseInsensitive
	fixPathCaseInsensitive = true
	t.Cleanup(func() {
		fixPathCaseInsensitive = previous
	})

	configPath := writeOperationWorkspace(t, false)
	operation := FixDocDrift(context.Background(), configPath, FixRequest{
		Path: "DOCS/GUIDES/API-RATE-LIMITS.MD",
	})
	if operation.Issue != nil {
		t.Fatalf("FixDocDrift() issue = %+v, want nil", operation.Issue)
	}
	if operation.Result == nil {
		t.Fatal("FixDocDrift() result = nil, want structured result")
	}
	if got, want := operation.Result.PlannedFileCount, 1; got != want {
		t.Fatalf("planned_file_count = %d, want %d", got, want)
	}
}
