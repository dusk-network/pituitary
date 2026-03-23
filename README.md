<p align="center">
  <strong>Pituitary</strong><br>
  <em>The master gland for your specification corpus.</em>
</p>

<p align="center">
  <a href="#quickstart">Quickstart</a> · <a href="#how-it-works">How It Works</a> · <a href="#commands">Commands</a> · <a href="#mcp-server">MCP Server</a> · <a href="#contributing">Contributing</a>
</p>

---

Pituitary keeps your specifications, code, and documentation from drifting out of sync. Point it at a folder of spec files, and it builds a semantic index that can detect overlapping specs, compare competing designs, trace impact when a spec changes, and catch documentation that has gone stale.

It ships as a single Go binary — no Docker, no external services. Just `pituitary` and one SQLite file.

Prebuilt release archives are published on [GitHub Releases](https://github.com/dusk-network/pituitary/releases) for `linux/amd64`, `darwin/arm64`, and `windows/amd64` if you want to evaluate the CLI without building from source.

## Why

Specs are easy to write and hard to maintain. As a corpus grows, common problems emerge:

- A new spec gets written that unknowingly duplicates an existing one.
- Two specs propose conflicting approaches and nobody notices until implementation.
- A spec is updated, but the three docs that reference it aren't.
- Code implements behavior that no spec covers — or contradicts one that does.

Pituitary catches these problems automatically, either from the CLI, via an MCP server plugged into your editor, or as a check in CI.

## Quickstart

**Prerequisites:** Go 1.25+, a C toolchain (required for the sqlite-vec extension). For platform-specific setup, see [docs/development/prerequisites.md](docs/development/prerequisites.md).

```sh
# Clone and build
git clone https://github.com/dusk-network/pituitary.git
cd pituitary
go build -o pituitary .

# Build the index from the included example specs
./pituitary index --rebuild

# Try some queries
./pituitary search-specs --query "rate limiting"
./pituitary check-overlap --path specs/rate-limit-v2
./pituitary review-spec --path specs/rate-limit-v2
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

[[sources]]
name = "contracts"
adapter = "filesystem"
kind = "markdown_contract"
path = "rfcs"
include = ["**/*.md"]
```

Selectors are always evaluated relative to the configured source `path`:

- `files` is an exact allowlist of source-relative files.
- `include` and `exclude` are glob filters over those same source-relative paths.
- If `files` is present, a path must be listed there before `include` / `exclude` are applied.
- For `spec_bundle`, `files` entries must point to `spec.toml`.
- For `markdown_docs` and `markdown_contract`, `files` entries must point to `.md` files.

`markdown_contract` treats Markdown files as inferred specs. Pituitary reads the first H1 as the title, picks up common metadata lines such as `Ref:`, `Status:`, `Domain:`, `Depends On:`, `Supersedes:`, and `Applies To:` when present, and otherwise falls back to a stable workspace-derived ref like `contract://rfcs/auth/session-policy` with status `draft`.

Inferred contracts carry confidence metadata in results. Search surfaces that confidence inline, and higher-stakes outputs like impact analysis, doc drift, and review reports emit warnings when key inferred fields are weak.

Example for a mixed-layout repo without changing source roots:

```toml
[[sources]]
name = "contracts"
adapter = "filesystem"
kind = "markdown_docs"
path = "."
files = [
  "docs/guides/api-rate-limits.md",
  "docs/runbooks/rate-limit-rollout.md",
]

[[sources]]
name = "accepted-specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
files = ["rate-limit-v2/spec.toml", "burst-handling/spec.toml"]
```

Selectors narrow what gets indexed; they do not rewrite refs. For example, a docs source rooted at `.` still produces refs like `doc://docs/guides/api-rate-limits` even when `files` narrows the selection to one file.

For an existing repo without a hand-written config yet, the default onboarding flow is:

```sh
./pituitary discover --path .
./pituitary discover --path . --write
./pituitary preview-sources
./pituitary index --rebuild
```

## Commands

Every command supports `--format json` for machine-readable output. `search-specs` also supports `--format table` for compact terminal summaries, and `review-spec` also supports `--format markdown` for shareable review reports.

| Command | What it does |
|---|---|
| `discover --path .` | Scan a repo, propose conservative sources, and show the generated local config |
| `preview-sources` | Show which files each configured source will index |
| `explain-file docs/guides/api-rate-limits.md` | Explain how one file is classified by configured sources |
| `canonicalize --path rfcs/service-sla.md` | Generate a suggested `spec.toml` + `body.md` bundle from one inferred contract |
| `index --rebuild` | Rebuild the SQLite index from all configured sources |
| `index --dry-run` | Validate config, sources, and rebuild prerequisites without writing the SQLite index |
| `status` | Report whether the configured index exists and basic spec/doc/chunk counts |
| `version` | Print Pituitary and Go runtime version information |
| `search-specs --query "..."` | Semantic search across indexed spec sections |
| `check-overlap --path specs/rate-limit-v2` | Detect specs that cover overlapping ground without looking up refs first |
| `compare-specs --path specs/rate-limit-legacy/spec.toml --path specs/rate-limit-v2/spec.toml` | Side-by-side tradeoff analysis of two specs |
| `analyze-impact --path specs/rate-limit-v2/body.md` | Trace which specs, code refs, and docs are affected by a change |
| `check-compliance --path PATH` | Check one or more code paths against accepted specs |
| `check-compliance --diff-file PATH|-` | Check a unified diff against accepted specs |
| `check-doc-drift --scope all` | Find docs that have gone stale relative to accepted specs |
| `review-spec --path specs/rate-limit-v2` | Full review: overlap + comparison + impact + drift + remediation in one report |

`canonicalize` is optional high-rigor mode. It does not replace inferred-contract indexing; it helps you promote one Markdown contract into an explicit bundle when you want stronger structure without converting the whole repo at once.

### Example: full spec review

Path-first commands accept workspace-relative paths, absolute paths, bundle directories, `spec.toml` files, `body.md` files, and inferred `markdown_contract` files. Internally they still normalize to canonical indexed refs.

```sh
$ ./pituitary review-spec --path specs/rate-limit-v2

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

Pass `--format json` to any command to get this format, suitable for piping into `jq`, CI scripts, or other tools.

## MCP Server

Pituitary ships an optional MCP server over stdio, exposing the same analysis tools to any MCP-compatible client (Claude Code, Cursor, Cowork, etc.):

```sh
./pituitary serve --config pituitary.toml
```

The MCP server exposes: `search_specs`, `check_overlap`, `compare_specs`, `analyze_impact`, `check_doc_drift`, and `review_spec`. It reuses the same analysis packages as the CLI — the MCP layer is intentionally thin.

`index --rebuild` remains CLI-only by design: indexing is an explicit, infrequent operation that shouldn't be triggered implicitly by an MCP client.

### Editor integration

Add to your MCP client config (e.g., Claude Code `settings.json`):

```json
{
  "mcpServers": {
    "pituitary": {
      "command": "/path/to/pituitary",
      "args": ["serve", "--config", "/path/to/pituitary.toml"]
    }
  }
}
```

## AI Runtime Configuration

Pituitary's current bootstrap runtime is intentionally narrow and deterministic:

- **Embedder** — `fixture` by default, with optional `openai_compatible` support for real embeddings.
- **Analysis** — `disabled` only. The shipped analysis commands are deterministic and do not call an external qualitative-analysis provider yet.

The runtime blocks are optional. If omitted, Pituitary defaults to:

```toml
[runtime.embedder]
provider = "fixture"
model = "fixture-8d"

[runtime.analysis]
provider = "disabled"
```

For `fixture`, `endpoint`, `api_key_env`, `timeout_ms`, and `max_retries` remain inert bootstrap fields. For `openai_compatible`, Pituitary uses them for the embeddings API call path.

This means the repo still works out of the box with no model credentials. Today:

- `runtime.embedder.provider` supports `fixture` and `openai_compatible`
- `runtime.analysis.provider` supports only `disabled`

For `openai_compatible` embeddings, `model` and `endpoint` are required. `api_key_env` is optional so local servers such as LM Studio can work without a token. Pituitary stores an embedder fingerprint in the index and requires `pituitary index --rebuild` when the configured embedder changes.

Example local embedding setup against LM Studio:

```toml
[runtime.embedder]
provider = "openai_compatible"
model = "pituitary-embed"
endpoint = "http://100.92.91.40:1234/v1"
timeout_ms = 30000
max_retries = 1
```

For Nomic-compatible models such as `nomic-embed-text-v1.5`, Pituitary automatically applies the required `search_document:` and `search_query:` prefixes when calling the embeddings endpoint.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full system design, including storage schema, tool design, data flow diagrams, and the implementation roadmap.

Key design decisions:

- **Deterministic bootstrap today.** Retrieval and the shipped analysis commands run without an external LLM. Richer provider-backed runtime integration is deferred until the runtime contract is implemented end to end.
- **Tools-only, no embedded agent.** Pituitary exposes discrete tools, not an autonomous agent. Orchestration is the caller's responsibility (your editor, CI, or a wrapper script).
- **Single file storage.** All state lives in one SQLite database (`pituitary.db`). Atomic rebuild via staging DB + swap ensures consistency.

## Project Status

Pituitary is in active development. The v1 shipping slice is functional: all core analysis commands work end-to-end. See the [GitHub issue queue](https://github.com/dusk-network/pituitary/issues) for active priorities and planned follow-up work.

**What works today:** indexing, semantic search, overlap detection, spec comparison, impact analysis, code compliance, doc drift detection, composite review, JSON output, table output for `search-specs`, markdown output for `review-spec`, MCP server.

**Coming next:** incremental indexing, non-filesystem source adapters, CI vendor integrations.

## Development

```sh
make fmt              # Format code
make smoke-sqlite-vec # Verify sqlite-vec is working
make test             # Run all tests
make vet              # Static analysis
make bench            # Run benchmarks
make ci               # Full CI pipeline (fmt + smoke + test + vet)
```

Requires `CGO_ENABLED=1` and a C toolchain — the sqlite-vec extension is linked via CGo. Run `make smoke-sqlite-vec` as a quick readiness check before the full suite.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and guidelines.

In short: the project is early and welcomes contributors. The best way to get started is to pick an open issue, comment to claim it, and submit a PR. The codebase is structured with clear package boundaries (`internal/analysis`, `internal/index`, `internal/mcp`, etc.) so you can contribute to one area without needing to understand the whole system.

## License

Pituitary is released under the [MIT License](LICENSE).
