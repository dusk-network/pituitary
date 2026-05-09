package jsonadapter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dusk-network/pituitary/sdk"
)

// deferredCancelContext is a context whose Err() returns nil for the
// first nFreeChecks calls and context.Canceled afterwards. Used to
// prove cancellation guards inside the adapter walk callback (not just
// the entry guard) actually fire.
type deferredCancelContext struct {
	context.Context
	mu           sync.Mutex
	freeRemain   int
	canceledOnce bool
	doneCh       chan struct{}
}

func newDeferredCancelContext(nFreeChecks int) *deferredCancelContext {
	return &deferredCancelContext{
		Context:    context.Background(),
		freeRemain: nFreeChecks,
		doneCh:     make(chan struct{}),
	}
}

func (c *deferredCancelContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.freeRemain > 0 {
		c.freeRemain--
		return nil
	}
	if !c.canceledOnce {
		c.canceledOnce = true
		close(c.doneCh)
	}
	return context.Canceled
}

func (c *deferredCancelContext) Done() <-chan struct{} {
	return c.doneCh
}

// TestAdapterLoadHonorsCanceledContext checks the adapter rejects an
// already-canceled context before starting any I/O.
func TestAdapterLoadHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "specs", "a.json"), `{"ref":"X-1","title":"X","status":"draft","domain":"x","body":"b"}`)

	a := &adapter{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := a.Load(ctx, sdk.SourceConfig{
		Name:          "specs",
		Adapter:       adapterName,
		Kind:          kindSpec,
		Path:          "specs",
		WorkspaceRoot: root,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Load(canceled) error = %v, want context.Canceled", err)
	}
}

// TestAdapterPreviewHonorsCanceledContext mirrors the above for
// Preview, the path Pituitary's PreviewFromConfigContext uses.
func TestAdapterPreviewHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "specs", "a.json"), `{"ref":"X-1","title":"X","status":"draft","domain":"x","body":"b"}`)

	a := &adapter{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := a.Preview(ctx, sdk.SourceConfig{
		Name:          "specs",
		Adapter:       adapterName,
		Kind:          kindSpec,
		Path:          "specs",
		WorkspaceRoot: root,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Preview(canceled) error = %v, want context.Canceled", err)
	}
}

// TestAdapterLoadCancelDuringWalk proves cancellation observed inside
// the WalkDir callback (and only there) surfaces context.Canceled.
// Non-.json entries are walked but never selected, so removing the
// per-walk-entry guard would let the walk complete cleanly with zero
// matches and the Load would succeed.
func TestAdapterLoadCancelDuringWalk(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	specsDir := filepath.Join(root, "specs")
	if err := os.MkdirAll(specsDir, 0o755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	for i := 0; i < 64; i++ {
		path := filepath.Join(specsDir, fmt.Sprintf("noise_%03d.txt", i))
		if err := os.WriteFile(path, []byte("noise\n"), 0o644); err != nil {
			t.Fatalf("write noise: %v", err)
		}
	}

	a := &adapter{}
	// Budget covers Adapter.Load entry only; the very next ctx.Err()
	// (inside the WalkDir callback) returns Canceled.
	ctx := newDeferredCancelContext(1)

	_, err := a.Load(ctx, sdk.SourceConfig{
		Name:          "specs",
		Adapter:       adapterName,
		Kind:          kindSpec,
		Path:          "specs",
		WorkspaceRoot: root,
	})
	if err == nil {
		t.Fatalf("Load(deferred cancel) error = nil, want cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Load error = %v, want context.Canceled", err)
	}
	if !ctx.canceledOnce {
		t.Fatalf("deferred-cancel context never reached the canceled phase; cancellation was observed earlier than expected")
	}
}

// TestAdapterLoadNilContextDoesNotPanic guards the nil-ctx defensive
// check we added on the same shape as the filesystem adapter.
func TestAdapterLoadNilContextDoesNotPanic(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "specs", "a.json"), `{"ref":"X-1","title":"X","status":"draft","domain":"x","body":"b"}`)

	a := &adapter{}
	//nolint:staticcheck // exercising the nil-context guard
	if _, err := a.Load(nil, sdk.SourceConfig{
		Name:          "specs",
		Adapter:       adapterName,
		Kind:          kindSpec,
		Path:          "specs",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("Load(nil ctx) error = %v, want nil", err)
	}
}
