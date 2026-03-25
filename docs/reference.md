# Pituitary Reference

Detailed configuration, spec format, runtime setup, and output conventions. For quick usage, see the [cheatsheet](cheatsheet.md).

## Install

### Homebrew

```sh
export HOMEBREW_GITHUB_API_TOKEN="$(gh auth token)"
brew install dusk-network/tap/pituitary
```

### One-line installer

```sh
gh api repos/dusk-network/pituitary/contents/scripts/install.sh?ref=main \
  -H 'Accept: application/vnd.github.raw' \
  | sh
```

The installer downloads the latest released archive for the current platform through `gh release download`, verifies it against the published checksum manifest, and installs `pituitary` to `/usr/local/bin` when that path is writable or `~/.local/bin` otherwise.

You can also pin the release or install directory:

```sh
gh api repos/dusk-network/pituitary/contents/scripts/install.sh?ref=main \
  -H 'Accept: application/vnd.github.raw' \
  | PITUITARY_VERSION=v1.0.0-alpha PITUITARY_INSTALL_DIR="$HOME/.local/bin" sh
```

### Manual releases

Prebuilt archives are published on [GitHub Releases](https://github.com/dusk-network/pituitary/releases) for:

- `linux/amd64`
- `darwin/arm64`
- `windows/amd64`

If you need a different platform or want full manual control, download and extract the matching archive from Releases directly.

## Quickstart Modes

### Evaluate on an existing repo

```sh
pituitary init --path .
pituitary status
pituitary check-doc-drift --scope all

# Optional pre-merge guardrail
git diff --cached | pituitary check-compliance --diff-file -
```

If your repo already has a config, skip `init` and go straight to `status`, `index --rebuild`, or the analysis commands.

### Build from source

If you are contributing to Pituitary itself or want to try the bundled example workspace in this repo:

```sh
git clone https://github.com/dusk-network/pituitary.git
cd pituitary
go build -o pituitary .

./pituitary index --rebuild
./pituitary review-spec --path specs/rate-limit-v2
./pituitary analyze-impact --path specs/rate-limit-v2/body.md
./pituitary check-doc-drift --scope all
```

The repo ships with a small example workspace under `specs/` and curated fixture docs under `docs/guides/` and `docs/runbooks/`.

## Spec Bundle Format

Pituitary manages specs written as **spec bundles**: a `spec.toml` metadata file paired with a `body.md` Markdown file.

```
specs/
├── rate-limit-v2/
│   ├── spec.toml
│   └── body.md
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
status = "accepted"               # draft | review | accepted | superseded | deprecated
domain = "api"
authors = ["emanuele"]
tags = ["rate-limiting", "api", "multi-tenant", "security"]
body = "body.md"

depends_on = ["SPEC-012"]         # optional
supersedes = ["SPEC-008"]         # optional
applies_to = [                    # optional: governed code/config paths
  "code://src/api/middleware/ratelimiter.go",
  "config://src/api/config/limits.yaml",
]
```

## Workspace Configuration

`pituitary init` generates a `pituitary.toml` at your project root. You can also hand-write one:

```toml
schema_version = 2

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

### Source Kinds

**`spec_bundle`** — `spec.toml` + `body.md` pairs. The structured, high-rigor format.

**`markdown_docs`** — Free-form Markdown files (guides, runbooks). Indexed for drift detection and search.

**`markdown_contract`** — Markdown files treated as inferred specs. Pituitary extracts metadata from `Ref:`, `Status:`, `Domain:`, `Depends On:`, `Supersedes:`, and `Applies To:` lines when present, or falls back to stable workspace-derived refs like `contract://rfcs/auth/session-policy` with status `draft`.

### Selectors

Selectors are evaluated relative to the configured source `path`:

- `files` — exact allowlist of source-relative paths
- `include` / `exclude` — glob filters (applied after `files`)
- For `spec_bundle`, `files` entries must point to `spec.toml`
- For `markdown_docs` and `markdown_contract`, `files` entries must point to `.md` files

Selectors narrow what gets indexed but do not rewrite refs.

### Config Resolution

Precedence: command-local `--config` > global `--config` > `PITUITARY_CONFIG` env var > discovered `pituitary.toml` in working directory or parent.

`pituitary status` shows which config won and where artifacts are written. If you keep defaults, add `.pituitary/` to `.gitignore`.

### Artifact Hygiene

`pituitary status` also reports where generated state lives:

- active index path
- index directory
- `discover --write` default config path
- default `canonicalize` bundle root

Relocation knobs:

- set `[workspace].index_path` to move the SQLite index
- use `pituitary discover --config-path PATH --write` to place generated config elsewhere
- use `pituitary canonicalize --bundle-dir PATH` to place generated bundles elsewhere

## Inferred Contracts

`markdown_contract` lets you index existing Markdown documents as specs without restructuring them. Pituitary reads the first H1 as the title, picks up common metadata lines, and attaches inference confidence to results. Higher-stakes outputs like impact analysis and doc drift emit warnings when key inferred fields are weak.

Use `pituitary canonicalize --path rfcs/service-sla.md` to promote one inferred contract into a full `spec.toml` + `body.md` bundle when you want stronger structure.

## AI Runtime Configuration

By default, Pituitary uses a deterministic `fixture` embedder — no API keys, no network, fast and reproducible. This is the right mode for CI and for evaluating Pituitary's workflow on a small repo.

For real retrieval quality on a larger corpus, configure a local embedding model:

```toml
[runtime.embedder]
provider = "openai_compatible"
model = "nomic-embed-text-v1.5"
endpoint = "http://127.0.0.1:1234/v1"
timeout_ms = 30000
max_retries = 1
```

A practical setup: load `nomic-embed-text-v1.5` in [LM Studio](https://lmstudio.ai), expose it on `localhost:1234`, then:

```sh
pituitary status --check-runtime embedder
pituitary index --rebuild
```

For provider-backed qualitative analysis (used by `compare-specs` and `check-doc-drift`):

```toml
[runtime.analysis]
provider = "openai_compatible"
model = "qwen3-coder-next"
endpoint = "http://127.0.0.1:1234/v1"
timeout_ms = 30000
max_retries = 1
```

Retrieval stays deterministic. The analysis model only touches narrowly shortlisted context.

Validate both runtimes with `pituitary status --check-runtime all`.

For Nomic-compatible models, Pituitary automatically applies the required `search_document:` / `search_query:` prefixes.

### Retrieval mode matters

The default `fixture` embedder is the deterministic baseline for tests, CI, and zero-credential evaluation. It is not the best retrieval runtime for real corpora. If you are evaluating search quality, overlap ranking, drift detection, or terminology audits on a real repo, switch to a real local embedding runtime first and then rebuild the index.

## Indexing Pipeline

When you run `pituitary index --rebuild`:

1. Discovers all spec bundles and Markdown docs in configured sources.
2. Validates the relation graph (cycles and contradictions fail fast).
3. Chunks content by heading-aware sections.
4. Reuses unchanged chunk embeddings when schema, embedder, and source fingerprints match.
5. Generates fresh embeddings only for new or changed chunks.
6. Stores everything in a single SQLite database.
7. Writes to a staging DB first and atomically swaps in — a failed rebuild never corrupts your index.

Use `--full` to skip reuse and force a complete re-embed.

Query commands validate index freshness before executing. A stale index fails fast with a rebuild hint.

## Output Formats

All commands share a consistent JSON envelope:

```json
{
  "request": { "...": "..." },
  "result": { "...": "..." },
  "warnings": [],
  "errors": []
}
```

Additional formats:

- `search-specs`: `--format table`
- `review-spec`: `--format markdown`
- `review-spec`: `--format html`

## Compliance Traceability

`check-compliance` is strongest when specs declare governed surfaces through `applies_to`:

```toml
applies_to = [
  "code://src/api/middleware/ratelimiter.go",
  "config://src/api/config/limits.yaml",
]
```

When a changed path has no explicit governance, findings include a `limiting_factor`:

- `spec_metadata_gap` — missing `applies_to`; tighten governance in the spec
- `code_evidence_gap` — governance is explicit, but the code doesn't expose enough literal evidence

## Commands

| Command | What it does |
|---|---|
| `init --path .` | One-shot onboarding: discover, write config, rebuild index, report status |
| `discover --path .` | Scan a repo and propose conservative sources |
| `migrate-config --path FILE --write` | Upgrade a legacy config to the current schema |
| `preview-sources` | Show which files each configured source will index |
| `explain-file PATH` | Explain how one file is classified by configured sources |
| `canonicalize --path PATH` | Promote one inferred contract into a spec bundle |
| `index --rebuild [--full]` | Rebuild the SQLite index |
| `index --dry-run` | Validate config and sources without writing |
| `status [--check-runtime all]` | Report index state, config, freshness, runtime readiness |
| `version` | Print version info |
| `search-specs --query "..."` | Semantic search across indexed spec sections |
| `check-overlap --path SPEC` | Detect specs that cover overlapping ground |
| `compare-specs --path A --path B` | Side-by-side tradeoff analysis |
| `analyze-impact --path SPEC` | Trace what's affected by a change |
| `check-terminology --term X --canonical-term Y --spec-ref Z` | Terminology migration audit |
| `check-compliance --path PATH` | Check code paths against accepted specs |
| `check-compliance --diff-file PATH\|-` | Check a unified diff against accepted specs |
| `check-doc-drift --scope all\|SPEC_REF` | Find docs that have gone stale |
| `fix --path PATH --dry-run` | Preview deterministic doc-drift remediations before writing |
| `fix --scope all --yes` | Apply deterministic doc-drift remediations without prompting |
| `review-spec --path SPEC` | Full review: overlap + comparison + impact + drift + remediation |
| `serve --config FILE` | Start MCP server over stdio |

`fix` is intentionally narrow: it only applies deterministic `replace_claim` remediations that `check-doc-drift` can justify from accepted specs and exact document evidence. Use `--dry-run` first, then rerun with `--yes` when the replacements look correct. After any successful apply, run `pituitary index --rebuild`.

## Review Reports

`review-spec` is the compound workflow. It composes:

- overlap detection
- comparison
- impact analysis
- doc drift
- remediation suggestions

Use `--format markdown` for PR-friendly reports and `--format html` for a richer shareable report with expandable evidence.

## MCP Server

Pituitary ships an optional MCP server over stdio:

```sh
pituitary serve --config pituitary.toml
```

Typical client config:

```json
{
  "mcpServers": {
    "pituitary": {
      "command": "pituitary",
      "args": ["serve", "--config", "pituitary.toml"]
    }
  }
}
```

The MCP server exposes:

- `search_specs`
- `check_overlap`
- `compare_specs`
- `analyze_impact`
- `check_doc_drift`
- `review_spec`

`index --rebuild` remains CLI-only by design.

## CI

`check-compliance --diff-file` is the easiest pre-merge guardrail:

```sh
git diff --cached | pituitary check-compliance --diff-file -
git diff origin/main...HEAD | pituitary check-compliance --diff-file -
```

For copy-paste workflow examples that install the released binary in CI and run both compliance and spec-hygiene checks, see [docs/development/ci-recipes.md](development/ci-recipes.md).
