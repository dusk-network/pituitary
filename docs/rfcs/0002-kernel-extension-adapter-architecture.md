# RFC 0002: Kernel/Extension Adapter Architecture

## Status

Proposed

## Date

2026-03-25

## Related

- RFC 0001: Spec-Centric Compliance Direction

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

Go's `internal/` package convention prevents cross-module imports. Third-party adapters (in separate Go modules) cannot import `internal/source` or `internal/model`. The adapter interface and canonical types must therefore live in a **public package** that both the kernel and extensions import.

```
pituitary/
├── sdk/                   # PUBLIC — the extension contract
│   ├── adapter.go         # Adapter interface, AdapterResult, AdapterFactory
│   ├── model.go           # SpecRecord, DocRecord (re-exported or moved here)
│   └── config.go          # Source config types extensions need
├── internal/              # KERNEL — never imports extensions/
│   ├── source/
│   │   ├── registry.go    # registry implementation (imports sdk/)
│   │   ├── filesystem.go  # built-in filesystem adapter (kernel)
│   │   ├── discover.go
│   │   └── ...
│   ├── config/            # full config parsing, validation
│   ├── model/             # may re-export or alias sdk/model types
│   ├── analysis/          # overlap, drift, impact, compliance
│   ├── index/             # SQLite, embeddings, rebuild
│   ├── mcp/               # MCP server
│   └── ...
├── extensions/            # EXTENSIONS — import only sdk/
│   └── github/
│       ├── adapter.go     # implements sdk.Adapter
│       ├── client.go      # GitHub API client (imports go-github)
│       └── ...
├── cmd/                   # CLI commands
└── main.go                # blank-imports: import _ "extensions/github"
```

**Rules:**
- `internal/` never imports `extensions/`. The dependency arrow is strictly one-way.
- Both `internal/` and `extensions/` import `sdk/`. The `sdk/` package is the shared contract.
- `sdk/` is minimal: only types and interfaces that extensions need. No business logic.
- Third-party adapters in separate Go modules import `github.com/dusk-network/pituitary/sdk`.

### The adapter interface

The interface lives in `sdk/` so both kernel and third-party extensions can import it:

```go
// sdk/adapter.go
package sdk

import "context"

// Adapter loads canonical records from one configured source.
type Adapter interface {
    // Load returns specs and docs from this source.
    Load(ctx context.Context, cfg SourceConfig) (*AdapterResult, error)
}

// AdapterResult is what an adapter returns.
type AdapterResult struct {
    Specs []SpecRecord
    Docs  []DocRecord
}

// AdapterFactory creates an adapter instance.
type AdapterFactory func() Adapter

// SourceConfig is the subset of source configuration that adapters receive.
type SourceConfig struct {
    Name    string            `json:"name"`
    Adapter string            `json:"adapter"`
    Kind    string            `json:"kind"`
    Path    string            `json:"path"`
    Files   []string          `json:"files,omitempty"`
    Options map[string]any    `json:"options,omitempty"`

    // WorkspaceRoot is the absolute path to the workspace root.
    WorkspaceRoot string      `json:"-"`
}
```

The registry lives in the kernel, since it wires adapters at startup:

```go
// internal/source/registry.go
package source

import "github.com/dusk-network/pituitary/sdk"

var registry = map[string]sdk.AdapterFactory{}

func Register(name string, factory sdk.AdapterFactory) {
    if _, exists := registry[name]; exists {
        panic(fmt.Sprintf("source adapter %q already registered", name))
    }
    registry[name] = factory
}

func LookupAdapter(name string) sdk.AdapterFactory {
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

import (
    "github.com/dusk-network/pituitary/internal/source"
    "github.com/dusk-network/pituitary/sdk"
)

func init() {
    source.Register("github", func() sdk.Adapter { return &githubAdapter{} })
}
```

Note: in-repo extensions under `extensions/` can import `internal/source` for registration because they are part of the same Go module. Third-party extensions in separate modules cannot — see the third-party section below.

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

**Config schema changes required:** The current config parser (`internal/config/config.go`) hard-codes the allowed adapter names (`filesystem`) and kind values (`spec_bundle`, `markdown_docs`, `markdown_contract`). Phase 1 must:

- Add an `Options map[string]any` field to `config.Source`
- Relax the adapter validation to accept any registered adapter name (not just `filesystem`)
- Relax the kind validation to accept adapter-defined kinds (each adapter declares which kinds it supports)
- Bump `schema_version` to 3 to signal the extended config format

### Custom builds for third-party adapters

Third-party adapters live in separate Go modules. They import `sdk/` for the interface and types, and `internal/source` for registration is NOT available to them (Go's `internal/` convention). Instead, the kernel exposes a public registration function via `sdk/`:

```go
// sdk/register.go
package sdk

// RegisterFunc is set by the kernel at startup. Extensions call it to register.
var RegisterFunc func(name string, factory AdapterFactory)

// Register registers an adapter factory. Safe to call from init().
func Register(name string, factory AdapterFactory) {
    if RegisterFunc == nil {
        panic("sdk.Register called before kernel initialized RegisterFunc")
    }
    RegisterFunc(name, factory)
}
```

The kernel wires this at program start:

```go
// internal/source/registry.go init()
func init() {
    sdk.RegisterFunc = Register
}
```

Third-party adapters use `sdk.Register`:

```go
// In github.com/someone/pituitary-jira-adapter
package jira

import "github.com/dusk-network/pituitary/sdk"

func init() {
    sdk.Register("jira", func() sdk.Adapter { return &jiraAdapter{} })
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

- Create `sdk/` package with `Adapter` interface, `AdapterResult`, `SourceConfig`, canonical model types, and public `Register` function
- Create `internal/source/registry.go` with registry implementation wired to `sdk.RegisterFunc`
- Extend `config.Source` with `Options map[string]any` field; relax adapter/kind validation to accept registered adapters
- Refactor `LoadFromConfig` to dispatch through the registry
- Register the filesystem adapter via `init()`
- Add CI check: `internal/` must not import `extensions/`
- Bump `schema_version` to 3
- Zero behavior change for existing configs — all existing tests pass

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
