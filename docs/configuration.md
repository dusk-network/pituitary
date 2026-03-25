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

## Source Kinds

**`spec_bundle`**: `spec.toml` + `body.md` pairs. The structured, high-rigor format.

**`markdown_docs`**: Free-form Markdown files such as guides and runbooks. Indexed for drift detection and search.

**`markdown_contract`**: Markdown files treated as inferred specs. Pituitary extracts metadata from `Ref:`, `Status:`, `Domain:`, `Depends On:`, `Supersedes:`, and `Applies To:` lines when present, or falls back to stable workspace-derived refs like `contract://rfcs/auth/session-policy` with status `draft`.

## Selectors

Selectors are evaluated relative to the configured source `path`:

- `files`: exact allowlist of source-relative paths
- `include` / `exclude`: glob filters applied after `files`
- For `spec_bundle`, `files` entries must point to `spec.toml`
- For `markdown_docs` and `markdown_contract`, `files` entries must point to `.md` files

Selectors narrow what gets indexed but do not rewrite refs.

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
