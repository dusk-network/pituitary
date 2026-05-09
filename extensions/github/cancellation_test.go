package github

import (
	"context"
	"errors"
	"testing"

	"github.com/dusk-network/pituitary/sdk"
)

// uncooperativeIssueClient mimics a client that ignores ctx and
// returns the full issue list regardless of cancellation. This lets us
// prove the adapter itself enforces ctx, not just the client.
type uncooperativeIssueClient struct {
	issues []githubIssue
}

func (u uncooperativeIssueClient) ListIssues(_ context.Context, _ string, _ string, _ issueListOptions) ([]githubIssue, error) {
	return append([]githubIssue(nil), u.issues...), nil
}

// TestAdapterLoadHonorsCanceledContextAtEntry verifies the adapter
// rejects an already-canceled ctx before it ever reaches the client.
func TestAdapterLoadHonorsCanceledContextAtEntry(t *testing.T) {
	t.Parallel()

	a := &adapter{clientFactory: func(_ sourceOptions) (issueClient, error) {
		return uncooperativeIssueClient{issues: nil}, nil
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := a.Load(ctx, sdk.SourceConfig{
		Name:    "issues",
		Adapter: adapterName,
		Kind:    kindIssue,
		Options: map[string]any{"repo": "owner/repo"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Load(canceled) error = %v, want context.Canceled", err)
	}
}

// TestAdapterLoadHonorsCancelDuringRecordLoop proves the adapter's
// per-record ctx.Err() guard catches cancellation even when the client
// ignores ctx and returns all records. Without that guard the adapter
// would happily build every spec/doc after the caller had gone away.
func TestAdapterLoadHonorsCancelDuringRecordLoop(t *testing.T) {
	t.Parallel()

	issues := make([]githubIssue, 0, 32)
	for i := 0; i < 32; i++ {
		issues = append(issues, githubIssue{
			Number:  i + 1,
			Title:   "issue",
			Body:    "body",
			State:   "open",
			HTMLURL: "https://example.invalid/owner/repo/issues/x",
			Labels:  []string{"bug"},
		})
	}

	a := &adapter{clientFactory: func(_ sourceOptions) (issueClient, error) {
		return uncooperativeIssueClient{issues: issues}, nil
	}}

	// Budget exactly covers the entry guard. Inside the per-record
	// loop the very next ctx.Err() returns Canceled.
	ctx := newDeferredCancelContext(1)

	_, err := a.Load(ctx, sdk.SourceConfig{
		Name:    "issues",
		Adapter: adapterName,
		Kind:    kindIssue,
		Options: map[string]any{"repo": "owner/repo"},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Load(deferred cancel) error = %v, want context.Canceled", err)
	}
	if !ctx.canceledOnce {
		t.Fatalf("deferred-cancel context never reached the canceled phase")
	}
}

// TestAdapterLoadNilContextDoesNotPanic guards the nil-ctx defensive
// shim added in the cancellation propagation work.
func TestAdapterLoadNilContextDoesNotPanic(t *testing.T) {
	t.Parallel()

	a := &adapter{clientFactory: func(_ sourceOptions) (issueClient, error) {
		return uncooperativeIssueClient{issues: nil}, nil
	}}

	//nolint:staticcheck // exercising the nil-context guard
	if _, err := a.Load(nil, sdk.SourceConfig{
		Name:    "issues",
		Adapter: adapterName,
		Kind:    kindIssue,
		Options: map[string]any{"repo": "owner/repo"},
	}); err != nil {
		t.Fatalf("Load(nil ctx) error = %v, want nil", err)
	}
}

type deferredCancelContext struct {
	context.Context
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
