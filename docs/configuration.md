# Configuration and Spec Format

This page covers the bundle format, workspace configuration, source selection, and generated artifact locations.

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

`pituitary init` generates `.pituitary/pituitary.toml` inside your project. You can also hand-write one:

```toml
schema_version = 3

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
role = "current_state"
path = "docs"
include = ["guides/*.md", "runbooks/*.md"]

[[sources]]
name = "contracts"
adapter = "filesystem"
kind = "markdown_contract"
path = "rfcs"
include = ["**/*.md"]
```

`workspace.root` is the primary repo root for the logical workspace. For cross-repo analysis, name it with `workspace.repo_id`, declare additional roots under `[[workspace.repos]]`, and bind a source to a secondary root with `repo = "..."`:

```toml
[workspace]
root = "."
repo_id = "product-docs"
index_path = ".pituitary/pituitary.db"

[[workspace.repos]]
id = "runtime"
root = "../runtime"

[[sources]]
name = "runtime-docs"
adapter = "filesystem"
kind = "markdown_docs"
repo = "runtime"
path = "docs"
include = ["guides/*.md"]
```

Pituitary keeps source paths repo-relative, adds `repo` to search/drift/impact/status JSON, and scopes non-primary generated doc refs as `doc://<repo>/...` so duplicate paths from sibling repos stay distinct.

Important path-resolution rule: `workspace.root` and `[[workspace.repos]].root` resolve relative to the config file's base directory, not the current working directory. That matters when you keep a scratch config outside the repo, for example `/tmp/pit-fake.toml` with `root = ".."`. In that case `..` resolves relative to `/tmp`, not relative to the shell directory where you invoked `pituitary`. If the resolved root is wrong, Pituitary now reports the derived absolute path in the validation error so the cause is visible immediately.

AST-inferred `applies_to` links are enabled automatically when the loaded specs declare at least one `code://...` target. Set `workspace.infer_applies_to = false` to opt out, or `workspace.infer_applies_to = true` to force inference. When inference is enabled but the AST inferer is not registered, rebuild fails fast instead of silently producing a degraded index.

### Runtime Profiles

Runtime config also supports reusable named profiles under `[runtime.profiles.<name>]`. Select one from `[runtime.embedder]` or `[runtime.analysis]` with `profile = "..."`, then override only the fields that differ:

```toml
[runtime.profiles.local-lm-studio]
provider = "openai_compatible"
endpoint = "http://127.0.0.1:1234/v1"
timeout_ms = 30000
max_retries = 1

[runtime.embedder]
profile = "local-lm-studio"
model = "nomic-embed-text-v1.5"

[runtime.analysis]
profile = "local-lm-studio"
model = "qwen3.5-35b"
timeout_ms = 120000
```

`pituitary status` reports the resolved runtime config, including the selected profile plus the effective provider, model, endpoint, timeout, and retry settings that commands will use.

### Terminology Governance

`check-terminology` can also load reusable policy from config instead of requiring `--term` flags every time:

```toml
[terminology]
exclude_paths = ["CHANGELOG.md", "docs/archive/*.md"]

[[terminology.policies]]
preferred = "locality"
historical_aliases = ["repo"]
forbidden_current = ["repository"]
docs_severity = "error"
specs_severity = "warning"

[[terminology.policies]]
preferred = "continuity"
deprecated_terms = ["workflow"]
docs_severity = "error"
specs_severity = "warning"
```

Each policy describes one preferred current-state term plus any combination of:

- `historical_aliases`: tolerated in historical or compatibility-only context, but reported as current-state violations elsewhere
- `deprecated_terms`: still reported, even in historical context, so migrations can fully remove them over time
- `forbidden_current`: allowed historically, but never acceptable in current-state docs/specs

Severity is configured per artifact scope:

- `docs_severity`: `ignore`, `warning`, or `error`
- `specs_severity`: `ignore`, `warning`, or `error`

When `pituitary check-terminology` runs without `--term`, Pituitary audits every governed term from `[[terminology.policies]]`, infers `canonical_terms` from the configured `preferred` values, and emits structured `classification`, `context`, `severity`, `replacement`, and `tolerated` fields in JSON output.

Use `[terminology].exclude_paths` when you want terminology sweeps and `compile` to skip historically frozen containers such as `CHANGELOG.md`, release notes, or archive folders without dropping those files from indexing, drift, or compliance.

### Example: Optional GitHub issues source

Schema `3` also supports adapter-specific typed options under `[sources.options]`:

```toml
[[sources]]
name = "github-issues"
adapter = "github"
kind = "issue"

[sources.options]
repo = "dusk-network/pituitary"
labels = ["spec", "rfc"]
state = "open"
api_key_env = "GITHUB_TOKEN"  # optional for private repos or higher rate limits
per_page = 100
```

The built-in GitHub adapter indexes RFC/spec-style issues as `SpecRecord`s and other issues as `DocRecord`s. Keep GitHub-specific settings inside `sources.options`; the kernel still treats `name`, `adapter`, `kind`, `path`, `files`, `include`, and `exclude` as the explicit shared config surface.

### Source Roles

Set `role` on a source when its artifacts should be interpreted with an authority level during semantic workflows such as `check-doc-drift`.

- `canonical`: stable source of truth for accepted design intent
- `current_state`: current operational guidance or runtime-facing documentation
- `runtime_authoritative`: runtime truth that should override softer documentation surfaces
- `planning`: forward-looking design or rollout notes
- `historical`: migration notes or superseded context that should not be treated as current drift by default
- `generated`: generated output that should be checked against its stronger source
- `mirror`: mirrored compatibility surface derived from another canonical document

Roles are applied per source. If one directory mixes current-state and historical docs, split it into separate `[[sources]]` blocks with different `include` patterns.

### Example: Indexing AI agent instructions

If your repo uses CLAUDE.md, AGENTS.md, or similar AI-policy files, add them as a `markdown_docs` or `markdown_contract` source:

```toml
[[sources]]
name = "agent-instructions"
adapter = "filesystem"
kind = "markdown_docs"
path = "."
files = ["CLAUDE.md", "AGENTS.md", "ARCHITECTURE.md"]
```

This indexes them alongside your specs so drift detection and search cover them. Note: `markdown_docs` sources participate in drift checks and semantic search, not overlap or review — for that, use `markdown_contract` or promote to a spec bundle with `pituitary canonicalize`.

### Example: Indexing structured JSON intent artifacts

If your repo keeps intent in JSON files such as API schemas or machine-generated policy/config records, add a JSON adapter source:

```toml
[[sources]]
name = "api-json"
adapter = "json"
kind = "json_spec"
path = "schemas"
include = ["*.json"]

[sources.options]
ref_pointer = "/x-pituitary/ref"
title_pointer = "/info/title"
status_pointer = "/x-pituitary/status"
domain_pointer = "/x-pituitary/domain"
tags_pointer = "/x-pituitary/tags"
applies_to_pointer = "/x-pituitary/applies_to"
```

`kind = "json_spec"` normalizes each matched JSON file into a `SpecRecord`; `kind = "json_doc"` emits `DocRecord`s instead. Pointer options use JSON Pointer syntax (`/a/b/0`). When a pointer is omitted, Pituitary falls back to a stable path-based ref, the filename as title, `draft` status for specs, the source name as the default spec domain, and the whole JSON document as the body rendered into markdown. Configured pointer values must resolve to non-empty strings or arrays; omit the pointer to use the built-in fallback instead.

## Source Kinds

**`spec_bundle`**: `spec.toml` + `body.md` pairs. The structured, high-rigor format.

**`markdown_docs`**: Free-form Markdown files such as guides and runbooks. Indexed for drift detection and search.

**`markdown_contract`**: Markdown files treated as inferred specs. Pituitary extracts metadata from `Ref:`, `Status:`, `Domain:`, `Depends On:`, `Supersedes:`, and `Applies To:` lines when present, or falls back to stable workspace-derived refs like `contract://rfcs/auth/session-policy` with status `draft`.

**`issue`**: Optional adapter-defined kind used by the built-in GitHub source adapter. GitHub issues labeled like specs/RFCs are normalized as specs; other issues are normalized as docs.

**`json_spec` / `json_doc`**: Optional adapter-defined kinds used by the built-in JSON source adapter. Each matched `.json` file is normalized into a spec or doc using JSON Pointer mappings from `sources.options`.

## Selectors

Selectors are evaluated relative to the configured source `path`:

- `files`: exact allowlist of source-relative paths
- `include` / `exclude`: glob filters applied after `files`
- For `spec_bundle`, `files` entries must point to `spec.toml`
- For `markdown_docs` and `markdown_contract`, `files` entries must point to `.md` files

Selectors narrow what gets indexed but do not rewrite refs.

Two debugging commands are worth learning early:

- `pituitary preview-sources` shows what each source would index. Use `--verbose` to list rejected candidates and the include/exclude selectors that decided them.
- `pituitary explain-file PATH` explains one file end-to-end: matching source, selector results, and why it was included or excluded. When a file looks wrong, run this first.

## Config Resolution

Precedence:

1. command-local `--config`
2. global `--config`
3. `PITUITARY_CONFIG` environment variable
4. discovered `pituitary.toml` in the working directory or a parent

`pituitary status` shows which config won and where artifacts are written. If you keep defaults, add `.pituitary/` to `.gitignore`.

## Artifact Hygiene

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
