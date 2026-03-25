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

**SpecRecord/DocRecord placement:** These types currently live in `internal/model` and are imported throughout the kernel. Phase 1 uses type aliases (`type SpecRecord = sdk.SpecRecord`) in `internal/model` to maintain backward compatibility for all internal consumers while making the types available to extensions via `sdk/`. This is a smaller delta than moving the types and rewriting all internal imports.

**sdk/ stability:** The `sdk/` package is the stability boundary. Adding a method to the `Adapter` interface breaks all extensions. Changes to `sdk/` types or interfaces require a deprecation path, not free refactoring. The `Adapter` interface is intentionally one method (`Load`) to minimize this surface.

**CI enforcement:** The `internal/` → `extensions/` import ban is enforced by adding a grep check to `make vet`: `! grep -r '"github.com/dusk-network/pituitary/extensions' internal/`. This fails the build if any kernel file imports an extension package.

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
        srcCfg := sdk.SourceConfig{
            Name: src.Name, Adapter: src.Adapter, Kind: src.Kind,
            Path: src.Path, Files: src.Files, Options: src.Options,
            WorkspaceRoot: cfg.Workspace.RootPath,
        }
        adapterResult, err := adapter.Load(ctx, srcCfg)
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

**Config schema changes required:** The current config parser (`internal/config/config.go`) is a hand-rolled line-by-line TOML parser that hard-codes allowed adapter names (`filesystem`) and kind values (`spec_bundle`, `markdown_docs`, `markdown_contract`). It rejects unknown keys and does not support nested tables under `[[sources]]`. Phase 1 must:

- Add an `Options map[string]any` field to `config.Source`
- Recognize `[sources.options]` as a valid nested section inside `[[sources]]` blocks
- Parse typed TOML values (int, bool, string, arrays) into `map[string]any` for options
- Relax the adapter validation to accept any registered adapter name (not just `filesystem`)
- Relax the kind validation to accept adapter-defined kinds (each adapter declares which kinds it supports)
- Keep the "reject unknown keys" behavior for top-level `[[sources]]` fields — only the `options` nested table is exempt from kernel validation
- Bump `schema_version` to 3 to signal the extended config format

**Parser strategy:** The hand-rolled parser requires non-trivial extension to support nested tables and typed values. Phase 1 should evaluate switching to `BurntSushi/toml` (or `pelletier/go-toml`) which handles this for free, versus extending the hand-rolled parser. The choice should be made at implementation time based on the blast radius of each approach.

### Custom builds for third-party adapters

Third-party adapters live in separate Go modules. They import `sdk/` for the interface and types, and `internal/source` for registration is NOT available to them (Go's `internal/` convention). Instead, `sdk/` provides a public `Register` function with a deferred queue to handle `init()` ordering safely:

```go
// sdk/register.go
package sdk

var (
    registerFunc func(name string, factory AdapterFactory)
    pendingQueue []pendingRegistration
)

type pendingRegistration struct {
    name    string
    factory AdapterFactory
}

// Register registers an adapter factory. Safe to call from init() regardless
// of import order — if the kernel hasn't initialized yet, the registration
// is queued and drained when the kernel calls SetRegisterFunc.
func Register(name string, factory AdapterFactory) {
    if registerFunc != nil {
        registerFunc(name, factory)
        return
    }
    pendingQueue = append(pendingQueue, pendingRegistration{name, factory})
}

// SetRegisterFunc is called by the kernel to wire the registry.
// Drains any queued registrations from init() calls that ran before
// the kernel initialized.
func SetRegisterFunc(f func(string, AdapterFactory)) {
    if registerFunc != nil {
        panic("sdk.SetRegisterFunc called twice")
    }
    registerFunc = f
    for _, p := range pendingQueue {
        f(p.name, p.factory)
    }
    pendingQueue = nil
}
```

The kernel wires this at program start:

```go
// internal/source/registry.go
func init() {
    sdk.SetRegisterFunc(Register)
}
```

This deferred queue pattern avoids a latent init-ordering bug: Go makes no ordering guarantee between `init()` functions in packages that don't import each other. Without the queue, a third-party adapter's `init()` could run before the kernel's `init()`, panicking on a nil function pointer. The queue ensures registrations are safe regardless of order.

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

7. **Credentials use env var references, never literal tokens.** Extension adapters that need API tokens (GitHub, GitLab, Jira) follow the existing `api_key_env` pattern from `runtime.embedder`: config names the env var, never the secret. No secrets in tracked files.

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
- **Credential management:** Promoted to guardrail 7. Extensions use env var references (`api_key_env`), never literal tokens.
