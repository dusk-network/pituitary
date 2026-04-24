package astinfer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dusk-network/pituitary/internal/codeinfer"
)

func TestInfererInfersEdgesAndRationale(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	srcPath := filepath.Join(root, "src", "limiter.go")
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.WriteFile(srcPath, []byte(`package limiter

// WHY: preserve burst behavior while enforcing sustained rate limits.
type SlidingWindowLimiter struct {
	window int
}
`), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	result, err := (Inferer{}).InferAppliesTo(context.Background(), codeinfer.Request{
		WorkspaceRoot: root,
		Specs: []codeinfer.SpecInput{
			{
				Ref:      "SPEC-042",
				BodyText: "This spec governs SlidingWindowLimiter.",
			},
		},
	})
	if err != nil {
		t.Fatalf("InferAppliesTo() error = %v", err)
	}
	if result == nil {
		t.Fatal("InferAppliesTo() result = nil")
	}
	if len(result.Edges) != 1 {
		t.Fatalf("edges = %+v, want one inferred edge", result.Edges)
	}
	if result.Edges[0].SpecRef != "SPEC-042" || result.Edges[0].FilePath != "src/limiter.go" {
		t.Fatalf("edge = %+v, want SPEC-042 -> src/limiter.go", result.Edges[0])
	}
	if len(result.CacheEntries) != 1 {
		t.Fatalf("cache entries = %+v, want one entry", result.CacheEntries)
	}
	if len(result.CacheEntries[0].Symbols) == 0 {
		t.Fatalf("cache symbols = empty, want extracted symbols")
	}
	if len(result.CacheEntries[0].Rationale) == 0 {
		t.Fatalf("cache rationale = empty, want WHY rationale")
	}
}
