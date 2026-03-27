package index

import (
	"context"
	"testing"

	"github.com/dusk-network/pituitary/internal/source"
)

func TestResolveIndexedSpecRefsWithConfigContextResolvesBundleAndBodyPaths(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	refs, err := ResolveIndexedSpecRefsWithConfigContext(context.Background(), cfg, []string{
		"specs/rate-limit-legacy/spec.toml",
		"specs/rate-limit-v2/body.md",
	})
	if err != nil {
		t.Fatalf("ResolveIndexedSpecRefsWithConfigContext() error = %v", err)
	}

	want := []string{"SPEC-008", "SPEC-042"}
	if len(refs) != len(want) {
		t.Fatalf("resolved refs = %v, want %v", refs, want)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("resolved refs = %v, want %v", refs, want)
		}
	}
}

func TestResolveIndexedSpecRefWithConfigContextClassifiesMissingPath(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	_, err = ResolveIndexedSpecRefWithConfigContext(context.Background(), cfg, "specs/missing/spec.toml")
	if err == nil {
		t.Fatal("ResolveIndexedSpecRefWithConfigContext() error = nil, want not-found error")
	}
	if !IsSpecPathNotFound(err) {
		t.Fatalf("ResolveIndexedSpecRefWithConfigContext() error = %v, want classified not-found error", err)
	}
}

func TestResolveIndexedSpecRefsWithConfigContextMatchesCaseInsensitivelyOnWindows(t *testing.T) {
	t.Parallel()

	previous := indexedSpecPathCaseInsensitive
	indexedSpecPathCaseInsensitive = true
	t.Cleanup(func() {
		indexedSpecPathCaseInsensitive = previous
	})

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	ref, err := ResolveIndexedSpecRefWithConfigContext(context.Background(), cfg, "SPECS/RATE-LIMIT-V2/BODY.MD")
	if err != nil {
		t.Fatalf("ResolveIndexedSpecRefWithConfigContext() error = %v", err)
	}
	if got, want := ref, "SPEC-042"; got != want {
		t.Fatalf("resolved ref = %q, want %q", got, want)
	}
}
