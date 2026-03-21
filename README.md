# Pituitary

Pituitary is a spec-management tool for keeping specifications, docs, and eventually code behavior aligned.

The current repository is the bootstrap for the first shipping slice defined in `ARCHITECTURE.md`. The initial scope is intentionally narrow:

- local filesystem only
- `spec.toml` + `body.md` spec bundles
- Markdown docs
- SQLite + `sqlite-vec`
- CLI-first required transport
- the repo also ships an optional MCP wrapper through `pituitary serve`
- the repo also ships CI validation, but not GitHub-specific review or reporting integrations
- spec/doc analysis before code compliance

## Repository Layout

- `ARCHITECTURE.md`: architecture and first-ship scope
- `CONTRIBUTING.md`: developer workflow and repository guardrails
- `IMPLEMENTATION_BACKLOG.md`: milestone backlog derived from the architecture
- `Makefile`: local format, test, vet, and CI entrypoints
- `cmd/`: reserved CLI transport layer
- `internal/`: reserved core package boundaries
- `pituitary.toml`: repo-local workspace config
- `specs/`: fixture spec bundles for bootstrap and testing
- `docs/`: fixture docs, including one intentional drift case
- `testdata/`: reserved test-only fixtures
- `main.go`: minimal CLI bootstrap

## Bootstrap CLI

The bootstrap now has seven working end-to-end commands. Every shipped command supports `--format json` with the shared response envelope described below:

- `index --rebuild`: rebuild the local SQLite index from configured sources
- `search-specs --query "...":` search indexed spec sections
- `check-overlap --spec-ref SPEC-042`: detect overlapping indexed specs, or use `--spec-record-file` for a draft canonical `spec_record`
- `compare-specs --spec-ref SPEC-008 --spec-ref SPEC-042`: compare exactly two indexed specs and return structured tradeoffs
- `analyze-impact --spec-ref SPEC-042`: traverse dependent specs, affected refs, and semantically related docs
- `check-doc-drift --scope all`: detect drifting docs, or target one or more docs with repeated `--doc-ref`
- `review-spec --spec-ref SPEC-042`: compose overlap, comparison, impact, and targeted doc-drift in one report, or use `--spec-record-file` for a draft canonical `spec_record`

Example:

```sh
# Clone and build
git clone https://github.com/dusk-network/pituitary.git
cd pituitary
go build -o pituitary .

# Build the index from the included example specs
export ANTHROPIC_API_KEY="your-key"   # or configure another provider
./pituitary index --rebuild

# Try some queries
./pituitary search-specs --query "rate limiting"
./pituitary check-overlap --spec-ref SPEC-042
./pituitary review-spec --spec-ref SPEC-042
```

The repo ships with a small example workspace under `specs/` and curated fixture docs under `docs/guides/` and `docs/runbooks/` — three spec bundles with intentional overlaps and a guide with intentional drift — so you can try every command out of the box.

## How It Works

Pituitary manages specs written as **spec bundles**: a `spec.toml` metadata file paired with a `body.md` Markdown file.

```
specs/
├── rate-limit-v2/
│   ├── spec.toml      # id, status, dependencies, applies_to
│   └── body.md        # the actual spec content
├── burst-handling/
│   ├── spec.toml
│   └── body.md
└── rate-limit-legacy/
    ├── spec.toml
    └── body.md
```

A `spec.toml` looks like this:

```toml
id = "SPEC-042"
title = "Per-Tenant Rate Limiting for Public API Endpoints"
status = "accepted"
domain = "api"
authors = ["emanuele"]
tags = ["rate-limiting", "api", "multi-tenant", "security"]
body = "body.md"

supersedes = ["SPEC-008"]
applies_to = [
  "code://src/api/middleware/ratelimiter.go",
  "config://src/api/config/limits.yaml",
]
```

When you run `pituitary index --rebuild`, Pituitary:

1. Discovers all spec bundles and Markdown docs in your configured sources.
2. Chunks the content by heading-aware sections.
3. Generates embeddings for each chunk.
4. Stores everything — metadata, embeddings, and dependency graph — in a single SQLite database.
5. Writes to a staging DB first and atomically swaps it in, so a failed rebuild never corrupts your index.

The workspace is configured with a `pituitary.toml` at your project root:

```toml
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md", "runbooks/*.md"]
```

## Commands

Every command supports `--format json` for machine-readable output.

| Command | What it does |
|---|---|
| `index --rebuild` | Rebuild the SQLite index from all configured sources |
| `search-specs --query "..."` | Semantic search across indexed spec sections |
| `check-overlap --spec-ref SPEC-042` | Detect specs that cover overlapping ground |
| `compare-specs --spec-ref SPEC-008 --spec-ref SPEC-042` | Side-by-side tradeoff analysis of two specs |
| `analyze-impact --spec-ref SPEC-042` | Trace which specs, code refs, and docs are affected by a change |
| `check-doc-drift --scope all` | Find docs that have gone stale relative to accepted specs |
| `review-spec --spec-ref SPEC-042` | Full review: overlap + comparison + impact + drift in one report |

### Example: full spec review

```sh
$ ./pituitary review-spec --spec-ref SPEC-042

# Returns a composed report covering:
#   - Overlapping specs (SPEC-008 detected as significant overlap)
#   - Comparison (SPEC-042 supersedes SPEC-008, adds per-tenant support)
#   - Impact (SPEC-055 depends on SPEC-042, 1 doc affected)
#   - Doc drift (docs/guides/api-rate-limits.md has stale rate values)
```

### JSON output

All commands share a consistent JSON envelope:

```json
{
  "request": { ... },
  "result": { ... },
  "warnings": [],
  "errors": []
}
```

## Optional MCP

Pituitary also exposes an optional stdio MCP server through:

```sh
go run . serve --config pituitary.toml
```

The MCP transport is intentionally thin:

- it exposes only `search_specs`, `check_overlap`, `compare_specs`, `analyze_impact`, `check_doc_drift`, and `review_spec`
- it reuses the same shared analysis and retrieval packages as the CLI
- it does not replace `pituitary index --rebuild`, which remains a CLI-only operation
- it ships in this repo as an optional wrapper rather than a separate product path

## Frozen V1 Contracts

Before feature work continues, the repository treats these v1 rules as fixed:

- Canonical `ref` values use declared spec IDs such as `SPEC-042` for specs, `doc://...` refs derived from workspace-relative Markdown paths for docs, and logical `code://...` / `config://...` refs for governed artifacts.
- `source_ref` is provenance-only and uses workspace-rooted `file://...` URIs.
- Persisted spec statuses are `draft`, `review`, `accepted`, `superseded`, and `deprecated`. Default search covers active specs (`draft`, `review`, `accepted`), while overlap analysis may still surface `superseded` specs as historical context.
- `spec_bundle` sources are discovered recursively by directories containing `spec.toml`; nested bundles are invalid. Markdown docs are discovered recursively as `*.md`.
- `search-specs` normalizes an optional `limit` request field; it defaults to `10` and must stay within `1..50`.
- JSON CLI responses share one envelope with normalized `request`, tool-specific `result`, and structured `warnings` / `errors`.
- `check_doc_drift` accepts exactly one of `doc_ref`, `doc_refs`, or `scope = "all"`. `review_spec` reuses `check_doc_drift` with targeted `doc_refs` from impact analysis rather than widening to the whole workspace by default.

## Frozen V1 AI Runtime

- Embeddings and qualitative analysis are configured independently under `[runtime.embedder]` and `[runtime.analysis]`.
- Each provider contract includes `provider`, `model`, `endpoint`, `api_key_env`, `timeout_ms`, and `max_retries`. Secrets come only from environment variables named by `api_key_env`, never from tracked config.
- CI and local tests must use the deterministic `fixture` provider mode instead of live model credentials or network calls.
- The embedder owns vector dimension discovery, and storage must size vector tables from that reported dimension rather than a hardcoded constant.
- If the qualitative provider is disabled or unavailable, deterministic commands can still run, but AI-backed analysis commands must fail with `dependency_unavailable`. If the embedder is unavailable, embedding-dependent commands must fail the same way.

## Development

Use the repo-local task targets to keep the bootstrap predictable:

```sh
make fmt
make smoke-sqlite-vec
make test
make vet
make bench
make ci
```

The `Makefile` sets `GOCACHE` to a repo-local `.cache/` directory so build and test commands do not depend on a user-global cache path.

Local builds also require a CGO-capable C toolchain because the current `sqlite-vec` integration is wired through `github.com/mattn/go-sqlite3` plus `github.com/asg017/sqlite-vec-go-bindings/cgo`. `make smoke-sqlite-vec` is the explicit readiness probe for that runtime path, and CI runs with `CGO_ENABLED=1`.

The repository also ships a GitHub Actions CI job that runs the same fmt, readiness, test, and vet workflow defined in `Makefile`.
