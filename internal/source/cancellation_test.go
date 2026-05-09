package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/sdk"
)

// TestLoadFromConfigContextHonorsCanceledContext verifies that an
// already-canceled context short-circuits the source-loading loop
// before any adapter is invoked.
func TestLoadFromConfigContextHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	workspaceRoot := t.TempDir()
	docsDir := filepath.Join(workspaceRoot, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatalf("write a.md: %v", err)
	}

	cfg := &config.Config{
		ConfigPath: filepath.Join(workspaceRoot, "pituitary.toml"),
		Workspace:  config.Workspace{RootPath: workspaceRoot},
		Sources: []config.Source{
			{
				Name:         "docs",
				Adapter:      config.AdapterFilesystem,
				Kind:         config.SourceKindMarkdownDocs,
				Path:         "docs",
				ResolvedPath: docsDir,
				RepoRootPath: workspaceRoot,
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := LoadFromConfigContext(ctx, cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("LoadFromConfigContext(canceled) error = %v, want context.Canceled", err)
	}
}

// deferredCancelContext is a context whose Err() returns nil for the
// first nFreeChecks calls and context.Canceled afterwards. It lets a
// test exercise cancellation that is only observed inside a filesystem
// walk callback, after the loader and adapter entry guards have
// already passed.
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

// TestLoadFromConfigContextCancelDuringWalk proves that cancellation
// observed inside the WalkDir callback (and only there) surfaces
// context.Canceled. The fixture uses zero selected .md files but many
// non-matching entries, so:
//   - the loader and adapter entry guards see no cancellation (budget=2),
//   - enumerateSelectedMarkdownPaths returns no matches with the walk
//     guard present (cancellation observed inside WalkDir),
//   - if the walk-callback ctx.Err() guard were removed, the walk would
//     complete cleanly (zero matches, no per-match loop iterations),
//     loadMarkdownDocs would return nil, and the loader would succeed.
//
// In other words, this test fails closed against regression of the
// per-walk-entry guard, not just the loop-entry short-circuit.
func TestLoadFromConfigContextCancelDuringWalk(t *testing.T) {
	t.Parallel()

	workspaceRoot := t.TempDir()
	docsDir := filepath.Join(workspaceRoot, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	// Many non-.md entries: walked but never selected for the per-match
	// loop. The only place ctx.Err() can return Canceled is the WalkDir
	// callback's guard.
	for i := 0; i < 64; i++ {
		path := filepath.Join(docsDir, fmt.Sprintf("noise_%03d.txt", i))
		if err := os.WriteFile(path, []byte("noise\n"), 0o644); err != nil {
			t.Fatalf("write noise: %v", err)
		}
	}

	cfg := &config.Config{
		ConfigPath: filepath.Join(workspaceRoot, "pituitary.toml"),
		Workspace:  config.Workspace{RootPath: workspaceRoot},
		Sources: []config.Source{
			{
				Name:         "docs",
				Adapter:      config.AdapterFilesystem,
				Kind:         config.SourceKindMarkdownDocs,
				Path:         "docs",
				ResolvedPath: docsDir,
				RepoRootPath: workspaceRoot,
			},
		},
	}

	// Free checks budget covers loader loop entry + filesystem adapter
	// Load entry. Subsequent ctx.Err() invocations (the walk callback)
	// return context.Canceled.
	ctx := newDeferredCancelContext(2)

	_, err := LoadFromConfigContext(ctx, cfg)
	if err == nil {
		t.Fatalf("LoadFromConfigContext(deferred cancel) error = nil, want cancellation")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("LoadFromConfigContext error = %v, want it to wrap context.Canceled", err)
	}
	if !ctx.canceledOnce {
		t.Fatalf("deferred-cancel context never reached the canceled phase; cancellation was observed earlier than expected")
	}
}

// TestFilesystemAdapterLoadHonorsCanceledContext verifies the adapter
// itself observes ctx.Err() before walking, regardless of caller.
func TestFilesystemAdapterLoadHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	workspaceRoot := t.TempDir()
	docsDir := filepath.Join(workspaceRoot, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatalf("write a.md: %v", err)
	}

	a := &filesystemAdapter{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := a.Load(ctx, sdk.SourceConfig{
		Name:          "docs",
		Adapter:       config.AdapterFilesystem,
		Kind:          config.SourceKindMarkdownDocs,
		Path:          "docs",
		WorkspaceRoot: workspaceRoot,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("filesystemAdapter.Load(canceled) error = %v, want context.Canceled", err)
	}
}

// TestDiscoverWorkspaceContextHonorsCanceledContext verifies that
// DiscoverWorkspaceContext short-circuits before walking when ctx is
// already canceled. This guards the same-shape gap that the load path
// closes: discovery scans whole workspaces just like source loading.
func TestDiscoverWorkspaceContextHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	workspaceRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspaceRoot, "specs"), 0o755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := DiscoverWorkspaceContext(ctx, DiscoverOptions{RootPath: workspaceRoot})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("DiscoverWorkspaceContext(canceled) error = %v, want context.Canceled", err)
	}
}

// TestDiscoverWorkspaceContextWriteRefusesAfterCancel verifies that
// DiscoverWorkspaceContext does not write a fresh config to disk when
// cancellation fires before the write boundary, so a canceled
// `init`/`discover` cannot leave behind a partial side effect.
func TestDiscoverWorkspaceContextWriteRefusesAfterCancel(t *testing.T) {
	t.Parallel()

	workspaceRoot := t.TempDir()
	bundleDir := filepath.Join(workspaceRoot, "specs", "demo")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatalf("mkdir bundle: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "spec.toml"), []byte(`id = "DEMO-1"
title = "Demo"
status = "draft"
domain = "demo"
body = "body.md"
`), 0o644); err != nil {
		t.Fatalf("write spec.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "body.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatalf("write body.md: %v", err)
	}

	configPath := filepath.Join(workspaceRoot, "pituitary.toml")
	// Pre-cancel: cancellation must surface before the write boundary
	// regardless of whether the scan, preview, render, or the
	// pre-write guard catches it. The invariant under test is that a
	// canceled DiscoverWorkspaceContext with Write=true never persists
	// a partial side effect.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := DiscoverWorkspaceContext(ctx, DiscoverOptions{
		RootPath:   workspaceRoot,
		ConfigPath: configPath,
		Write:      true,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("DiscoverWorkspaceContext(write,canceled) error = %v, want context.Canceled", err)
	}
	if _, statErr := os.Stat(configPath); !os.IsNotExist(statErr) {
		t.Fatalf("config file should not have been written; stat err = %v", statErr)
	}
}

// TestPreviewFromConfigContextHonorsCanceledContext verifies that the
// preview path observes cancellation between sources.
func TestPreviewFromConfigContextHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	workspaceRoot := t.TempDir()
	docsDir := filepath.Join(workspaceRoot, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatalf("write a.md: %v", err)
	}

	cfg := &config.Config{
		ConfigPath: filepath.Join(workspaceRoot, "pituitary.toml"),
		Workspace:  config.Workspace{RootPath: workspaceRoot},
		Sources: []config.Source{
			{
				Name:         "docs",
				Adapter:      config.AdapterFilesystem,
				Kind:         config.SourceKindMarkdownDocs,
				Path:         "docs",
				ResolvedPath: docsDir,
				RepoRootPath: workspaceRoot,
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := PreviewFromConfigContext(ctx, cfg)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PreviewFromConfigContext(canceled) error = %v, want context.Canceled", err)
	}
}

// TestFilesystemAdapterLoadHonorsNilContext defensively checks the
// adapter does not panic if a caller passes a nil context.
func TestFilesystemAdapterLoadHonorsNilContext(t *testing.T) {
	t.Parallel()

	workspaceRoot := t.TempDir()
	docsDir := filepath.Join(workspaceRoot, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "a.md"), []byte("# A\n"), 0o644); err != nil {
		t.Fatalf("write a.md: %v", err)
	}

	a := &filesystemAdapter{}
	//nolint:staticcheck // exercising the nil-context guard
	if _, err := a.Load(nil, sdk.SourceConfig{
		Name:          "docs",
		Adapter:       config.AdapterFilesystem,
		Kind:          config.SourceKindMarkdownDocs,
		Path:          "docs",
		WorkspaceRoot: workspaceRoot,
	}); err != nil {
		t.Fatalf("filesystemAdapter.Load(nil ctx) error = %v, want nil", err)
	}
}
