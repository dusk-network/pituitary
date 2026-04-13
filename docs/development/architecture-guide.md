# Architecture Guide

This guide is the contributor-friendly version of [ARCHITECTURE.md](../../ARCHITECTURE.md). It focuses on how the current codebase is actually organized and how data moves through it.

For canonical product boundaries and full contract details, read `ARCHITECTURE.md`. This guide keeps a short summary of the current shipped slice because contributors still need that context when deciding where code should go.

## What Pituitary Does

Pituitary indexes a local workspace of:

- spec bundles: `spec.toml` plus `body.md`
- inferred Markdown contracts
- Markdown docs

It then answers questions over that indexed corpus:

- semantic search
- overlap detection
- spec comparison
- impact analysis
- doc drift detection
- composed review workflows

The current implementation is:

- local filesystem only
- SQLite plus `sqlite-vec`
- CLI-first
- optional MCP wrapper over the same shared logic

## High-Level Data Flow

The end-to-end flow is:

1. `pituitary.toml` is parsed and validated.
2. configured sources are loaded from disk into canonical records
3. Markdown bodies are chunked into heading-aware sections
4. embeddings and graph edges are written into the SQLite index
5. analysis code queries that index and returns structured results
6. CLI and MCP expose those results without duplicating core logic

In package terms:

```text
filesystem -> internal/source -> internal/chunk -> internal/index -> internal/analysis -> internal/app -> cmd or internal/mcp
```

## Package Map

### `cmd/`

The CLI transport layer.

Responsibilities:

- parse flags with the standard library `flag` package
- validate CLI-specific arguments
- call shared operations in `internal/app`
- render text or JSON output
- convert operation failures into exit codes

Most command files follow the same shape:

- build a request struct from flags
- normalize or validate request details
- call `app.<Operation>()`
- render success or error through shared output helpers

`cmd/index.go` is the main exception because rebuild is intentionally CLI-only and goes directly through config, source loading, and index rebuild.

### `internal/app/`

The shared transport-agnostic entrypoint layer.

Responsibilities:

- load config
- call the relevant core package
- normalize transport-facing issues
- classify failures into stable error codes and exit behavior

If a capability is exposed from both CLI and MCP, `internal/app` is the place to keep request execution and error classification aligned.

### `internal/config/`

Parses and validates `pituitary.toml`.

Responsibilities:

- resolve the workspace root relative to the config file
- validate source definitions
- validate index path rules
- validate runtime provider configuration

The current parser is intentionally narrow and internal. It does not use a general TOML dependency.

### `internal/source/`

Loads configured sources from disk and normalizes them into canonical records.

Responsibilities:

- discover `spec.toml` bundles
- discover Markdown contracts
- discover Markdown docs
- validate malformed bundles
- derive canonical refs and `source_ref` values
- hash content and carry normalized metadata forward

### `internal/model/`

Shared core types.

Responsibilities:

- canonical spec and doc record types
- relation and response-adjacent shared structures

If multiple packages need the same shape, it likely belongs here.

### `internal/chunk/`

Turns Markdown into heading-aware sections.

Responsibilities:

- split Markdown on ATX headings
- preserve heading paths in section titles
- fall back to a title-scoped chunk when no headings are present

### `internal/index/`

Owns the SQLite-backed index and semantic retrieval path.

Responsibilities:

- manage SQLite handles and readiness checks
- build and atomically swap the index
- own embeddings and vector encoding
- run similarity search against `sqlite-vec`

If work touches the DB schema, rebuild flow, vector storage, or search execution, it belongs here.

### `internal/analysis/`

Owns the core analysis capabilities.

Responsibilities:

- overlap detection
- comparison
- impact analysis
- doc drift detection
- composed review workflows

This package reads from the index and returns structured analysis results. It should not know about CLI flags or MCP tool wiring.

### `internal/mcp/`

The optional MCP transport.

Responsibilities:

- register MCP tools
- map MCP request shapes into shared app operations
- fail fast on startup problems

This layer should stay thin. New business logic should not start here.

## Workspace And Fixture Layout

The repo includes a small fixture workspace:

- `specs/`
- `docs/`
- `pituitary.toml`
- `testdata/bootstrap_expectations.json`

The repo also includes a self-dogfood contract slice under `dogfood/contracts/` that governs the README and contributor docs.

That workspace is used across tests and benchmarks. When changing behavior, prefer extending the existing fixtures and expectations instead of inventing parallel test-only worlds unless the case is truly isolated.

## Where To Add New Features

### Add a new analysis capability

Typical path:

1. add core logic in `internal/analysis`
2. add a shared operation in `internal/app` if the feature is transport-exposed
3. add a CLI command in `cmd/`
4. optionally add an MCP tool in `internal/mcp/`
5. add tests at each layer you touched
6. update docs

### Add a new index or retrieval behavior

Typical path:

1. change `internal/index`
2. update dependent analysis or app code if request/response behavior changes
3. extend rebuild/search tests and benchmarks

### Add a new source format

Typical path:

1. extend `internal/source`
2. update `internal/config` validation if config shape changes
3. update canonical model and docs only if the feature changes stored semantics

## Design Rules To Preserve

- Keep transports thin.
- Keep deterministic retrieval separate from qualitative analysis.
- Keep rebuild atomic.
- Keep request and error contracts stable.
- Prefer extending fixtures and targeted tests over adding hidden assumptions.

If you are unsure where something belongs, start from the data flow and place the change in the earliest layer that can own it cleanly.
