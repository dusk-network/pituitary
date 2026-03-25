# RFC 0002: Kernel/Extension Adapter Architecture

## Status

Proposed

## Date

2026-03-25

## Related

- RFC 0001: Spec-Centric Compliance Direction
- Positioning design: `docs/superpowers/specs/2026-03-25-positioning-design.md`

## Summary

Pituitary should separate its core (kernel) from external source adapters (extensions) using Go's registry pattern with blank imports. The kernel stays pure — local filesystem, SQLite, deterministic, no vendor dependencies. Extensions are separate Go packages that implement a source adapter interface, register themselves via `init()`, and compile into the same single binary. GitHub issues is the first extension adapter.

## Context

Pituitary currently supports one source adapter: `filesystem`. It reads `spec.toml` bundles, markdown docs, and markdown contracts from the local filesystem. This is the kernel — and it should stay that way.

But teams produce records of intent across many systems: GitHub issues (where specs get proposed and modified, as in the BIP/EIP pattern), GitLab work items, Jira tickets, Notion pages, JSON configs, databases. The positioning design identifies GitHub issues as a high-priority source because it shifts Pituitary from end-of-workflow (CI catches drift at merge time) to beginning-of-workflow (Pituitary catches contradictions when an issue is created).

The risk is coupling: if Pituitary's core imports `go-github`, it takes on a vendor dependency that doesn't belong in the kernel. If extensions are separate binaries, distribution becomes a coordination problem. The architecture must keep the kernel pure while giving extensions first-class status in a single binary.

## Options Considered

### Option A: Compiled-in adapters, extract later

Add new adapters directly in `internal/source/`. Keep everything in one package. Promise to extract later if it gets messy.

**Problem:** "Extract later" never happens. Vendor SDKs leak into the kernel. The boundary erodes.

### Option B: Separate binaries (exec protocol)

Extensions are standalone CLIs that output canonical JSON. Pituitary ingests via an `exec` adapter.

**Problem:** Multiple binaries to distribute, version, and coordinate. Users must install `pituitary` + `pituitary-source-github` + `pituitary-source-jira`. Discovery and error messages become complex.

### Option C: Registry pattern with blank imports

Extensions are separate Go packages that implement an adapter interface. They register themselves via `init()`. The main binary imports them with blank imports. Everything compiles into one binary. The kernel never imports extension packages — only the main binary's import list wires them together.

This is the pattern used by Caddy (plugins), Telegraf (input/output plugins), Hugo (modules), and many other Go projects.

## Decision

Adopt Option C.

### The boundary

```
pituitary/
├── internal/              # KERNEL — never imports extensions/
│   ├── source/
│   │   ├── registry.go    # Adapter interface + registry
│   │   ├── filesystem.go  # built-in filesystem adapter (kernel)
│   │   ├── discover.go
│   │   └── ...
│   ├── config/            # config parsing, validation
│   ├── model/             # SpecRecord, DocRecord (the contract)
│   ├── analysis/          # overlap, drift, impact, compliance
│   ├── index/             # SQLite, embeddings, rebuild
│   ├── mcp/               # MCP server
│   └── ...
├── extensions/            # EXTENSIONS — import only kernel interfaces
│   └── github/
│       ├── adapter.go     # implements source.Adapter
│       ├── client.go      # GitHub API client (imports go-github)
│       └── ...
├── cmd/                   # CLI commands
└── main.go                # blank-imports: import _ "extensions/github"
```

**Rule:** `internal/` never imports `extensions/`. The dependency arrow is strictly one-way: extensions depend on kernel interfaces, never the reverse.

### The adapter interface

```go
// internal/source/registry.go

// Adapter loads canonical records from one configured source.
type Adapter interface {
    // Load returns specs and docs from this source.
    Load(ctx context.Context, cfg config.Source, workspace config.Workspace) (*AdapterResult, error)
}

// AdapterResult is what an adapter returns.
type AdapterResult struct {
    Specs []model.SpecRecord
    Docs  []model.DocRecord
}

// AdapterFactory creates an adapter instance.
type AdapterFactory func() Adapter

// registry holds registered adapter factories, keyed by adapter name.
var registry = map[string]AdapterFactory{}

// Register adds an adapter factory. Called from extension init() functions.
func Register(name string, factory AdapterFactory) {
    if _, exists := registry[name]; exists {
        panic(fmt.Sprintf("source adapter %q already registered", name))
    }
    registry[name] = factory
}

// LookupAdapter returns the factory for a named adapter, or nil.
func LookupAdapter(name string) AdapterFactory {
    return registry[name]
}
```

### The filesystem adapter becomes a registered adapter

The existing `filesystem` adapter registers itself like any extension — it just happens to live in the kernel:

```go
// internal/source/filesystem.go
func init() {
    Register("filesystem", func() Adapter { return &filesystemAdapter{} })
}
```

### Extensions register via init()

```go
// extensions/github/adapter.go
package github

import "github.com/dusk-network/pituitary/internal/source"

func init() {
    source.Register("github", func() source.Adapter { return &githubAdapter{} })
}
```

### Main binary wires extensions via blank imports

```go
// main.go
package main

import (
    _ "github.com/dusk-network/pituitary/extensions/github"
    "github.com/dusk-network/pituitary/cmd"
)

func main() { cmd.Run() }
```

### LoadFromConfig dispatches through the registry

```go
// internal/source/loader.go (refactored from filesystem.go)
func LoadFromConfig(cfg *config.Config) (*LoadResult, error) {
    result := &LoadResult{...}

    for _, src := range cfg.Sources {
        factory := LookupAdapter(src.Adapter)
        if factory == nil {
            return nil, fmt.Errorf("source %q: unknown adapter %q", src.Name, src.Adapter)
        }
        adapter := factory()
        adapterResult, err := adapter.Load(ctx, src, cfg.Workspace)
        if err != nil {
            return nil, fmt.Errorf("source %q: %w", src.Name, err)
        }
        // merge adapterResult into result...
    }

    return result, nil
}
```

### Config for extension adapters

Extension adapters use adapter-specific options via a generic map:

```toml
[[sources]]
name = "github-issues"
adapter = "github"
kind = "issue"

[sources.options]
repo = "dusk-network/pituitary"
labels = ["spec", "rfc"]
state = "open"
```

The `options` table is opaque to the kernel — each adapter parses its own options from the config. The kernel validates only the fields it owns (`name`, `adapter`, `kind`, `path`, `files`, `include`, `exclude`). Extension-specific fields live in `options`.

### Custom builds for third-party adapters

Anyone can write an adapter in a separate Go module:

```go
// In github.com/someone/pituitary-jira-adapter
package jira

import "github.com/dusk-network/pituitary/internal/source"

func init() {
    source.Register("jira", func() source.Adapter { return &jiraAdapter{} })
}
```

Users include it in a custom build:

```go
// custom/main.go
package main

import (
    _ "github.com/dusk-network/pituitary/extensions/github"
    _ "github.com/someone/pituitary-jira-adapter"
    "github.com/dusk-network/pituitary/cmd"
)

func main() { cmd.Run() }
```

One `go build`, one binary, all adapters included.

## Guardrails

1. **`internal/` never imports `extensions/`.** This is the load-bearing invariant. CI should enforce it (e.g., a linter check or import-path grep in the test suite).

2. **The adapter interface is the contract.** Extensions produce `[]model.SpecRecord` and `[]model.DocRecord`. The analysis engine never knows or cares where records came from.

3. **Extension-specific config lives in `options`.** The kernel config schema stays stable. Extension authors own their options parsing and validation.

4. **The filesystem adapter stays in the kernel.** Local-first, no-network, deterministic indexing is the product foundation. It is not an extension — it's the kernel.

5. **Official extensions ship in the default binary.** The release binary includes all maintained extensions via blank imports in `main.go`. Users don't need custom builds for common adapters.

6. **Extensions must not widen the analysis contract.** An extension that returns records with new fields in `Metadata` is fine. An extension that requires changes to the analysis engine is a kernel change and needs its own RFC.

## Implementation Sequence

### Phase 1: Extract the adapter interface (kernel change)

- Create `internal/source/registry.go` with the `Adapter` interface and registry
- Refactor `LoadFromConfig` to dispatch through the registry
- Register the filesystem adapter via `init()`
- Add CI check: `internal/` must not import `extensions/`
- Zero behavior change — all existing tests pass

### Phase 2: GitHub issues adapter (first extension)

- Create `extensions/github/` with the adapter implementation
- GitHub issues → `SpecRecord` (for issues labeled as specs/RFCs) or `DocRecord` (for discussion issues)
- Config: `adapter = "github"` with options for repo, labels, state
- Add to `main.go` blank imports
- Add integration tests (mock GitHub API or fixture responses)

### Phase 3: JSON source adapter

- Create `extensions/json/` for indexing structured JSON files as intent artifacts
- Config: `adapter = "json"` with options for schema mapping

## What This Does Not Change

- The analysis engine (`internal/analysis/`) is untouched. It consumes `SpecRecord`/`DocRecord` — it doesn't care about the source.
- The index layer (`internal/index/`) is untouched. It stores canonical records.
- The CLI and MCP surfaces are untouched. They invoke `LoadFromConfig` which dispatches to adapters.
- The existing filesystem adapter behavior is identical. It just registers through the new interface instead of being hard-wired.

## Open Questions

- **Adapter discovery/listing:** Should `pituitary status` list registered adapters so users can see what's available? Useful for debugging "unknown adapter" errors.
- **Adapter versioning:** If an extension's record schema evolves, how does the kernel handle version skew? Likely answer: the canonical model is the contract; extensions must produce valid records or fail.
- **Extension testing:** Should the kernel provide test helpers (fake config, assertions) for extension authors? Likely useful once third-party adapters emerge.
- **Credential management:** Extension adapters (GitHub, GitLab, Jira) need API tokens. These should follow the existing `api_key_env` pattern from `runtime.embedder` — env var name in config, never the token itself.
