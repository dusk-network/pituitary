# Pituitary: Architecture Design

> *The master gland doesn't do the work — it regulates the whole system.*
> Pituitary keeps specifications, code, and docs from drifting out of sync.

## Problem Statement

Pituitary is for managing 20-100+ specifications and the artifacts around them, with automated support for:

1. **Overlap detection** — catching when a new spec covers ground already addressed
2. **Tradeoff analysis** — comparing competing or overlapping specs
3. **Impact analysis** — understanding what changes when a spec is accepted, modified, or deprecated
4. **Code compliance** — validating that code adheres to accepted specs, or flagging gaps
5. **Documentation sync** — keeping non-spec docs aligned with accepted specs

The key constraint is that Pituitary should not be defined by one storage or workflow choice. Specifications may originate in local files, git repositories, PDFs, databases, or other systems. Pituitary's job is to normalize those inputs into a common analysis model, not to own source control, CI, or authoring.

---

## Product Boundary

Pituitary core is responsible for:

- Normalizing source material into canonical spec and document records
- Building a searchable index plus an explicit dependency graph
- Running overlap, comparison, impact, compliance, and doc-drift analysis
- Exposing those capabilities through a stable CLI and thin programmatic transports such as MCP

Pituitary core is **not** responsible for:

- Being tied to GitHub, pull requests, or any specific CI vendor
- Requiring git as the source of truth
- Requiring Markdown frontmatter as the source format
- Owning review workflows, comment posting, or issue tracking

Those concerns belong in adapters and integrations layered around the core.

---

## First Shipping Slice

The first shipping slice should be intentionally narrow. It exists to prove that Pituitary can ingest specs, build a consistent index, and answer the core spec-management questions without being entangled with CI vendors, source-control providers, or document-extraction complexity.

### Required in the first ship

- Local filesystem only
- One metadata format for specs: `spec.toml`
- One body format for specs and docs: Markdown
- One index backend: local SQLite + `sqlite-vec`
- One required transport: CLI
- Five required analysis capabilities:
  - `search_specs`
  - `check_overlap`
  - `compare_specs`
  - `analyze_impact`
  - `check_doc_drift`
- One required compound workflow: `review_spec`

### Explicitly out of scope for the first ship

- GitHub PR comments and vendor-specific CI or reporting flows
- PDF ingestion
- Database-backed source adapters
- Incremental index updates
- Stored code-summary embeddings
- Provider-backed code-compliance adjudication beyond the shipped deterministic CLI slice

### Also shipped in this repo during v1

- An optional MCP server transport that wraps the same analysis packages as the CLI
- A repository CI workflow that runs fmt, readiness, test, and vet validation

These are shipped alongside the first slice, but only the CLI is required for first-ship completeness. The CI job is delivery plumbing, not a GitHub-specific product integration surface.

### Workspace configuration

The first ship should use one repo-local config file:

`pituitary.toml`

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

This keeps the first ship explicit and easy to reason about. The indexed config remains explicit even as the repo grows into inferred-contract sources: `pituitary discover` may propose a local `.pituitary/pituitary.toml`, but it must stay conservative, inspectable before write, and never introduce hidden indexing behavior behind the user's back.

Inferred `markdown_contract` records must preserve confidence metadata alongside the normalized artifact so result surfaces can distinguish strong explicit extraction from weaker path/default fallbacks. Search should expose those confidence signals inline, while higher-stakes outputs such as impact and doc-drift should elevate weak inference as warnings instead of silently treating it as equally strong.

When teams want more rigor, Pituitary may optionally generate an explicit spec bundle from one inferred contract. That canonicalization flow must preserve the stable inferred ref, preserve source provenance, preview the generated `spec.toml` and `body.md` before write, and remain incremental rather than forcing whole-repo migration.

---

## System Overview

```text
┌──────────────────────────────────────────────────────────────┐
│                     Source Systems / Files                    │
│                                                              │
│  V1: local spec bundles + docs                               │
│  Later: git repos, PDFs, databases, remote APIs              │
└──────────────────────────────┬───────────────────────────────┘
                               │
                               ▼
┌──────────────────────────────────────────────────────────────┐
│                        Source Adapters                        │
│                                                              │
│  • filesystem adapter (V1)                                   │
│  • pdf adapter (later)                                       │
│  • database adapter (later)                                  │
│  • git / GitHub adapter (later)                              │
└──────────────────────────────┬───────────────────────────────┘
                               │ normalized records
                               ▼
┌──────────────────────────────────────────────────────────────┐
│                         Core Pipeline                         │
│                                                              │
│  1. Normalize records                                        │
│  2. Chunk text                                               │
│  3. Generate embeddings                                      │
│  4. Build relations graph                                    │
│  5. Atomically rebuild pituitary.db                          │
└──────────────────────────────┬───────────────────────────────┘
                               │
                               ▼
┌──────────────────────────────────────────────────────────────┐
│                     Unified Analysis Index                    │
│                                                              │
│  • artifacts   (canonical records)                           │
│  • chunks      (text sections)                               │
│  • chunks_vec  (embeddings)                                  │
│  • edges       (depends_on / supersedes / applies_to)        │
└──────────────────────────────┬───────────────────────────────┘
                               │ queries
                               ▼
┌──────────────────────────────────────────────────────────────┐
│                        Analysis Engine                        │
│                                                              │
│  • check_overlap                                             │
│  • compare_specs                                             │
│  • analyze_impact                                            │
│  • check_compliance                                          │
│  • check_doc_drift                                           │
│  • search_specs                                              │
│  • review_spec                                               │
└──────────────────────────────┬───────────────────────────────┘
                               │
                               ▼
┌──────────────────────────────────────────────────────────────┐
│                     Transport / Extensions                    │
│                                                              │
│  V1 required transport: CLI                                   │
│  V1 optional wrapper: MCP                                     │
│  Shipped repo validation: CI                                  │
│  Later integrations: git hooks, PR comments, editors         │
└──────────────────────────────────────────────────────────────┘
```

---

## Key Design Decision: Tools-Only, No Embedded Agent

Pituitary is a **tools-only system**, not an autonomous agent. Each tool does one job: retrieve context from the index and graph, apply deterministic analysis in the current bootstrap, and return structured output. Orchestration lives outside Pituitary, in the calling client or automation layer.

This keeps the core simple, testable, and composable:

- MCP clients can call individual tools in any order
- CLI automation can invoke the same logic without a separate orchestration runtime
- Deterministic retrieval remains testable without involving an LLM

**When to revisit this decision:** if the same multi-step workflow keeps reappearing, add a thin compound tool such as `review_spec` on top of the primitives. Do not move orchestration policy into the storage or analysis layers.

---

## Component Details

### 1. Canonical Model and V1 Authoring Format

Pituitary should reason over a **canonical internal model**, not over source-specific files.

At ingestion time, every adapter normalizes inputs into the same conceptual shape:

```text
SpecRecord
  ref            stable Pituitary reference (for example "SPEC-042")
  kind           "spec"
  title
  status         draft | review | accepted | deprecated | superseded
  domain
  authors[]
  tags[]
  relations[]    depends_on / supersedes
  applies_to[]   code or config refs governed by the spec
  source_ref     where the record came from
  body_format    markdown | plaintext | extracted_pdf_text | ...
  body_text
  metadata       adapter-specific extras

DocRecord
  ref
  kind           "doc"
  title
  source_ref
  body_format
  body_text
  metadata
```

#### V1 reference rules

Pituitary should distinguish between three different identifier classes:

- `ref`: the canonical identifier used by the index and tool inputs.
  - Specs use their declared spec IDs such as `SPEC-042`.
  - Docs use a canonical doc ref derived from the workspace-relative Markdown path, for example `docs/guides/api-rate-limits.md` -> `doc://guides/api-rate-limits`.
- `source_ref`: provenance for where a record came from.
  - For the filesystem adapter, this should be a `file://` URI rooted at the workspace, for example `file://specs/rate-limit-v2/spec.toml` or `file://docs/guides/api-rate-limits.md`.
- `applies_to`: logical references governed by a spec.
  - V1 uses canonical scheme-specific refs such as `code://...` and `config://...`.

Tool inputs for indexed artifacts should use canonical `ref` values, not `source_ref` values. Provenance should remain available in outputs and stored metadata, but it is not the primary query surface.

#### V1 status and supersession rules

- Valid persisted spec statuses are `draft`, `review`, `accepted`, `superseded`, and `deprecated`.
- If spec `A` declares `supersedes = ["B"]`, then persisted fixture data for `B` should normally use `status = "superseded"` once `A` is accepted.
- Default semantic search should include `draft`, `review`, and `accepted` specs, and should exclude `superseded` and `deprecated` specs unless the caller explicitly asks for them.
- Overlap analysis should include `superseded` specs as historical comparison candidates, but it should exclude `deprecated` specs by default.
- Impact analysis should traverse explicit graph relations regardless of status, and should label superseded artifacts as historical findings in the response when they appear.

Pituitary v1 should ship exactly **one first-party source format** for specs:

```text
specs/
  rate-limit/
    spec.toml
    body.md
```

`spec.toml`

```toml
id = "SPEC-042"
title = "Rate Limiting for Public API Endpoints"
status = "accepted"
domain = "api"
authors = ["emanuele"]
tags = ["rate-limiting", "api", "security"]
body = "body.md"

depends_on = ["SPEC-012", "SPEC-031"]
supersedes = ["SPEC-008"]
applies_to = [
  "code://src/api/middleware/ratelimiter.go",
  "config://src/api/config/limits.yaml",
]
```

`body.md`

```md
## Overview
...

## Requirements
...

## Design Decisions
...
```

**Why this is a better v1 choice than YAML frontmatter:**

- TOML is much simpler to parse and validate than YAML
- Metadata is not coupled to Markdown as a container format
- The split between `spec.toml` and `body.md` maps cleanly to the internal model
- Future adapters can emit the same model without pretending they have frontmatter

This does **not** mean TOML is the product's identity. It is only the first adapter format.

---

### 2. Source Adapters

Source adapters are the boundary between external systems and the Pituitary core.

Each adapter has four jobs:

1. Enumerate source items
2. Load raw content
3. Normalize into canonical records
4. Report stable `source_ref` and content hashes for change detection

The core should not care whether a record came from:

- a local `spec.toml` + `body.md` bundle
- a Markdown doc directory
- a PDF that has been text-extracted
- a database row
- a git revision or pull request diff

The adapter contract keeps that variability out of the analysis engine.

**Current scope:**

- `filesystem` adapter for spec bundles
- `filesystem` adapter for docs directories
- `filesystem` adapter for inferred Markdown contracts

**V1 filesystem enumeration rules:**

- For `kind = "spec_bundle"`, recursively walk the configured source root and treat each directory containing a `spec.toml` file as one bundle.
- Selector matching for spec bundles is done against the source-relative `spec.toml` path.
- A valid bundle must contain exactly one `spec.toml`; its `body` field must resolve to exactly one file relative to the bundle directory.
- Nested bundles inside another bundle directory are invalid and should fail with a clear path-specific error.
- For `kind = "markdown_docs"`, recursively index `*.md` files under the configured source root, then apply selectors against source-relative paths.
- For `kind = "markdown_contract"`, recursively index `*.md` files under the configured source root, infer spec metadata from common Markdown fields, and normalize the file into a `SpecRecord`.
- `files` is an optional exact allowlist of source-relative files.
- `include` and `exclude` are optional glob filters over those same source-relative paths.
- If `files` is present, it narrows the candidate set before `include` / `exclude` are applied.
- For `kind = "spec_bundle"`, `files` entries must point to `spec.toml`.
- For `kind = "markdown_docs"` and `kind = "markdown_contract"`, `files` entries must point to `.md` files.
- A doc title should come from the first H1 heading when present; otherwise it should fall back to the filename stem.
- A doc `ref` should be derived from the Markdown path relative to the configured doc source root, without the `.md` suffix.
- An inferred contract title should come from the first H1 heading when present; otherwise it should fall back to the filename stem.
- An inferred contract should use explicit `Ref:` / `ID:` metadata when present; otherwise it should fall back to a stable workspace-relative `contract://...` ref and default `status = "draft"` when no valid status is declared.

**Later, as extensions:**

- `pdf` adapter
- `database` adapter
- `git` adapter
- `github` adapter

Git and GitHub are therefore integration surfaces, not architectural assumptions.

---

### 3. Ingestion and Indexing Pipeline

The indexing pipeline should operate on normalized records, not on source-specific files.

```text
Step 1: Load
  ├── Ask one or more adapters for canonical records
  └── Validate required fields for specs and docs

Step 2: Normalize
  ├── Persist canonical metadata for each record
  ├── Extract explicit relations (depends_on, supersedes, applies_to)
  └── Attach provenance (adapter, source_ref, content hash)

Step 3: Chunk
  ├── For markdown, split by heading-aware sections
  ├── For plaintext or extracted PDF text, split by paragraphs / headings
  └── Store chunks keyed to the parent record

Step 4: Embed
  ├── Generate embeddings for spec and doc chunks
  └── Store vectors in chunks_vec keyed by chunk_id

Step 5: Graph Build
  ├── Add explicit spec-to-spec relations
  ├── Add spec-to-code refs from applies_to
  └── Keep all refs in canonical string form

Step 6: Atomic Swap
  └── Replace the active pituitary.db with the rebuilt staging DB
```

**Embedding model recommendation:** At 20-100 specs, a local model such as `nomic-embed-text` is sufficient and keeps the system offline-friendly. Cloud embeddings remain easy to swap in behind an `Embedder` interface.

**Bootstrap runtime contract (current implementation):**

- The current runtime supports two embedder providers: `fixture` and `openai_compatible`.
- The current runtime supports two analysis providers: `disabled` and `openai_compatible`.
- Retrieval, indexing, and candidate shortlisting remain deterministic even when provider-backed analysis is enabled.
- Provider-backed analysis currently applies only to bounded adjudication steps in `compare-specs` and `check-doc-drift`; `review-spec` inherits those refined results.
- Tests and CI should use the deterministic fixture embedder and require no live model credentials.
- Unsupported runtime providers should fail during config validation with clear, intentional errors.
- Provider-backed embeddings and bounded provider-backed analysis are both now part of the runtime contract.

V1 runtime configuration should be explicit in `pituitary.toml` under `[runtime.embedder]` and `[runtime.analysis]`:

| Field | Embedder | Analysis | Notes |
|---|---|---|---|
| `provider` | optional, defaults to `fixture` | optional, defaults to `disabled` | Embedder and analysis currently support `openai_compatible`; analysis also supports `disabled` |
| `model` | defaults to `fixture-8d` for `fixture`; required for `openai_compatible` | required for `openai_compatible`, ignored when disabled | Part of the embedder fingerprint stored in index metadata |
| `endpoint` | required for `openai_compatible`, ignored for `fixture` | required for `openai_compatible`, ignored when disabled | Expected to point at an OpenAI-compatible API root such as `http://host:1234/v1` |
| `api_key_env` | optional | optional | Optional so local servers such as LM Studio can run without credentials |
| `timeout_ms` | optional, defaults to `1000` | optional, defaults to `1000` | Active for `openai_compatible` embedding requests |
| `max_retries` | optional, defaults to `0` | optional, defaults to `0` | Active for retryable `openai_compatible` runtime failures |

Degraded behavior rules:

- The `fixture` embedder must be deterministic, require no network access, and be the default mode for CI and local tests.
- Unsupported embedder or analysis providers must fail during config validation rather than degrading silently.
- Indexed metadata must store both embedder dimension and embedder fingerprint so provider/model changes fail clearly and require a rebuild.
- Provider-backed analysis must preserve the current storage and transport contracts rather than widening them implicitly.

Retry and timeout rules:

- `timeout_ms` and `max_retries` remain parsed for both runtime blocks so the config shape does not need a second contract change later.
- For `openai_compatible` embeddings, those fields control the HTTP client timeout and retry behavior.
- For `openai_compatible` analysis, those fields control the HTTP client timeout and retry behavior.
- For `fixture` embeddings and `disabled` analysis, those fields remain inert.

**Chunking strategy:** The current implementation uses a lightweight internal Markdown scanner that splits on ATX headings, preserves the nested heading path in each section title, and falls back to one title-scoped chunk when a document has no headings. For non-Markdown inputs, adapters should either provide text with lightweight structural markers or let the chunker fall back to paragraph-based splitting.

**Filtered vector queries:** `chunks_vec` should store only vectors. Metadata filters stay in canonical tables: vector search returns candidate `chunk_id`s, then the query joins back through `chunks` and `artifacts` to filter by `kind`, `status`, `domain`, or other metadata before ranking the final candidate set.

---

### 4. Storage Layer — Unified SQLite Index

All indexed state lives in a **single SQLite database** (`pituitary.db`) using `sqlite-vec` for vector operations. At this scale, SQLite is enough, keeps deployment simple, and makes full atomic rebuilds straightforward. In the current Go implementation, `vec0` is wired through `github.com/mattn/go-sqlite3` plus `github.com/asg017/sqlite-vec-go-bindings/cgo`, so local and CI builds need a CGO-capable C toolchain.

#### Schema

```sql
-- Canonical records from any adapter
CREATE TABLE artifacts (
  ref           TEXT PRIMARY KEY,   -- "SPEC-042", "doc://guides/api-rate-limits"
  kind          TEXT NOT NULL,      -- "spec" | "doc"
  title         TEXT,
  status        TEXT,               -- NULL for docs
  domain        TEXT,
  source_ref    TEXT NOT NULL,      -- provenance such as "file://docs/guides/api-rate-limits.md"
  adapter       TEXT NOT NULL,      -- "filesystem", "pdf", "database", ...
  body_format   TEXT NOT NULL,      -- "markdown", "plaintext", ...
  content_hash  TEXT NOT NULL,
  metadata_json TEXT NOT NULL       -- adapter-specific metadata
);

-- Chunked body text
CREATE TABLE chunks (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  artifact_ref  TEXT NOT NULL,
  section       TEXT,
  content       TEXT NOT NULL,
  FOREIGN KEY (artifact_ref) REFERENCES artifacts(ref)
);

-- sqlite-vec virtual table for similarity search
CREATE VIRTUAL TABLE chunks_vec USING vec0(
  chunk_id INTEGER PRIMARY KEY,
  embedding float[EMBEDDING_DIM]
);

-- Canonical graph edges
CREATE TABLE edges (
  from_ref      TEXT NOT NULL,
  to_ref        TEXT NOT NULL,
  edge_type     TEXT NOT NULL,      -- "depends_on" | "supersedes" | "applies_to"
  PRIMARY KEY (from_ref, to_ref, edge_type)
);

CREATE INDEX idx_artifacts_kind_status_domain
  ON artifacts(kind, status, domain);

CREATE INDEX idx_chunks_artifact_ref
  ON chunks(artifact_ref);

CREATE INDEX idx_edges_from_ref_type
  ON edges(from_ref, edge_type);

CREATE INDEX idx_edges_to_ref_type
  ON edges(to_ref, edge_type);
```

#### Two indexed collections

| Collection | Source | Used For |
|---|---|---|
| `spec` artifacts | Canonical spec records | Overlap detection, tradeoff analysis, impact analysis, compliance |
| `doc` artifacts | Canonical non-spec docs | Documentation drift detection |

Code is intentionally **not** indexed as a third stored semantic corpus in v1. For compliance checks, Pituitary embeds the current file or diff at request time and searches against spec chunks directly. That preserves the retrieval fallback without adding a second invalidation problem for stored code summaries.

#### Atomic Rebuild

The indexer always writes to a **staging database** (`pituitary.db.new`) and then swaps it in. That guarantees each rebuilt index is internally consistent: metadata, chunks, vectors, and edges all come from the same snapshot.

```text
pituitary index --rebuild

  1. Create pituitary.db.new
  2. Load all records from configured adapters
  3. Populate artifacts + edges
  4. Chunk text and populate chunks + chunks_vec
  5. Run integrity checks
  6. Rename pituitary.db.new -> pituitary.db
  7. On failure: delete pituitary.db.new, keep existing index untouched
```

To make the swap visible to a running process, tool handlers should open a fresh read-only SQLite connection per request, or explicitly reload when the active index generation changes.

**V1 simplification:** full rebuilds should be the default and the only required mode. Incremental updates are optional later if rebuild time becomes a measured bottleneck.

#### Scaling Path

If SQLite stops being sufficient, the vector and storage layers can be abstracted behind interfaces. That is a later optimization, not a v1 requirement.

---

### 5. Analysis Tools

#### Design Principle: Deterministic First, Provider-Backed Adjudication Later

The current bootstrap implementation uses deterministic analysis end to end. References to LLM adjudication below describe the intended extension point for later runtime work, not a shipped requirement today.

All tools follow the same pattern:

1. **Deterministic retrieval** narrows the candidate set using SQL, graph traversal, and vector search
2. **Deterministic analysis today** produces the shipped result; richer provider-backed adjudication may be added later on the narrowed set

This keeps retrieval reproducible, testable, and cheap.

All shipped commands should also share one JSON envelope and one issue-item shape:

```json
{
  "request": { "...": "normalized tool input" },
  "result": { "...": "tool-specific payload" },
  "warnings": [
    {
      "code": "string",
      "message": "human-readable warning",
      "path": "optional/workspace-relative/path"
    }
  ],
  "errors": [
    {
      "code": "string",
      "message": "human-readable error",
      "path": "optional/workspace-relative/path"
    }
  ]
}
```

Contract rules:

- `request` echoes the normalized input after CLI parsing, using canonical `ref` values rather than `source_ref`.
- `result` is command-specific and should be `null` when a command exits with errors before producing a domain result.
- `warnings` and `errors` use the same object shape. `path` is optional and should stay workspace-relative when present.
- Common `errors[].code` values in v1 are `validation_error`, `config_error`, `not_found`, `dependency_unavailable`, and `internal_error`.

CLI exit codes should stay simple:

- `0` success, including success-with-warnings
- `2` validation or configuration error
- `3` dependency unavailable, reserved for runtime dependencies such as future provider-backed analysis

#### V1 JSON command contracts

- `index` (`pituitary index --rebuild`)
  - Request: `{ "rebuild": true }`
  - Result: `{ "workspace_root": ".", "index_path": ".pituitary/pituitary.db", "artifact_counts": { "spec": N, "doc": N }, "chunk_count": N, "edge_count": N }`
- `search_specs` (`pituitary search-specs`)
  - Request: `{ "query": "...", "filters": { "domain": "...", "statuses": ["accepted"] }, "limit": 10 }`
  - Result: `{ "matches": [{ "ref": "SPEC-042", "title": "...", "section_heading": "...", "score": 0.0, "excerpt": "...", "source_ref": "file://..." }] }`
- `check_overlap` (`pituitary check-overlap`)
  - Request: `{ "spec_ref": "SPEC-042" }` or `{ "spec_record": { ... canonical spec record ... } }`
  - Result: `{ "candidate": { "ref": "SPEC-042", "title": "..." }, "overlaps": [{ "ref": "SPEC-008", "score": 0.0, "overlap_degree": "high", "relationship": "extends" }], "recommendation": "proceed_with_supersedes" }`
- `compare_specs` (`pituitary compare-specs`)
  - Request: `{ "spec_refs": ["SPEC-008", "SPEC-042"] }`
  - Result: `{ "spec_refs": ["SPEC-008", "SPEC-042"], "comparison": { "shared_scope": [...], "differences": [...], "tradeoffs": [...], "recommendation": "..." } }`
- `analyze_impact` (`pituitary analyze-impact`)
  - Request: `{ "spec_ref": "SPEC-042", "change_type": "accepted" | "modified" | "deprecated" }`
  - Result: `{ "spec_ref": "SPEC-042", "change_type": "accepted", "affected_specs": [...], "affected_refs": [...], "affected_docs": [...] }`
- `check_terminology` (`pituitary check-terminology`)
  - Request: `{ "terms": ["repo", "workflow"], "canonical_terms": ["locality", "continuity"], "spec_ref": "SPEC-LOCALITY", "scope": "all" | "docs" | "specs" }`
  - Result: `{ "scope": { "mode": "workspace" | "spec_ref", "artifact_kinds": ["doc", "spec"], "spec_ref": "SPEC-LOCALITY" }, "terms": [...], "canonical_terms": [...], "anchor_specs": [...], "findings": [{ "ref": "...", "kind": "doc" | "spec", "terms": [...], "sections": [{ "section": "...", "terms": [...], "excerpt": "...", "evidence": { "spec_ref": "SPEC-LOCALITY", "section": "...", "score": 0.0 } | null }] }] }`
- `check_doc_drift` (`pituitary check-doc-drift`)
  - Request: exactly one of `{ "doc_ref": "doc://guides/api-rate-limits" }`, `{ "doc_refs": ["doc://guides/api-rate-limits"] }`, or `{ "scope": "all" }`
  - Result: `{ "scope": { "mode": "doc_ref" | "doc_refs" | "all", "doc_refs": [...] }, "drift_items": [{ "doc_ref": "...", "findings": [{ "spec_ref": "SPEC-042", "code": "...", "message": "...", "rationale": "...", "evidence": { "spec_ref": "SPEC-042", "spec_section": "...", "doc_section": "..." }, "confidence": { "level": "high" | "medium" | "low", "score": 0.0 } }] }], "assessments": [{ "doc_ref": "...", "status": "drift" | "possible_drift", "rationale": "...", "evidence": { ... }, "confidence": { ... } }], "remediation": { ... } }`
- `review_spec` (`pituitary review-spec`)
  - Request: `{ "spec_ref": "SPEC-042" }` or `{ "spec_record": { ... canonical spec record ... } }`
  - Result: `{ "spec_ref": "SPEC-042", "overlap": { ... }, "comparison": { ... } | null, "impact": { ... }, "doc_drift": { ... } }`

The shared `errors[]` shape above applies to every shipped command. Path-specific validation errors should use `code = "validation_error"` or `code = "config_error"` with the offending workspace-relative path.

`search_specs.limit` defaults to `10` when omitted and must stay within `1..50` in v1 so retrieval work stays bounded.

#### V1 tool matrix

| Tool | First shipping slice | Notes |
|---|---|---|
| `search_specs` | required | First proof that indexing and retrieval work |
| `check_overlap` | required | Primary product value |
| `compare_specs` | required | Used only on overlapping or user-selected specs |
| `analyze_impact` | required | Depends on explicit graph plus doc retrieval |
| `check_terminology` | shipped after first ship | Hybrid lexical-plus-semantic audit for conceptual migrations |
| `check_doc_drift` | required | Markdown docs only in first ship |
| `review_spec` | required | Compound wrapper over the required tools |
| `check_compliance` | shipped after first ship | CLI-first deterministic slice; MCP exposure can follow once the request shape settles |

The first shipping slice was intentionally **spec-and-doc centric**. Code remained in the model through `applies_to` references until the core spec workflows were shipped and validated. The current repo now includes a CLI-first deterministic `check_compliance` slice on top of that same index.

#### Tool: `check_overlap`

**Purpose:** detect when a new or changed spec overlaps existing specs.

```text
Input:
  { spec_ref: "SPEC-042" }
  OR { spec_record: { ... canonical record ... } }   // draft not yet indexed

Process:
  Phase 1 — retrieval
  1. Parse or load the candidate spec body
  2. Chunk and embed it
  3. Query chunks_vec for candidate chunk_ids
  4. Join through chunks + artifacts to keep kind = "spec"
     and status != "deprecated"
     while still allowing `superseded` specs as historical candidates
  5. Group by artifact_ref and rank by similarity

  Phase 2 — adjudication
  6. Ask the LLM for overlap degree, affected sections, and
     whether the new spec extends, contradicts, or duplicates

Output:
  overlaps[]
  recommendation = proceed_with_supersedes | merge_into_existing | no_overlap
```

#### Tool: `compare_specs`

**Purpose:** compare two or more overlapping specs.

```text
Input:
  { spec_refs: ["SPEC-008", "SPEC-042"] }

Output:
  structured comparison of design decisions, tradeoffs,
  compatibility, and recommendation
```

#### Tool: `analyze_impact`

**Purpose:** determine what changes when a spec changes state or content.

```text
Input:
  { spec_ref: "SPEC-042", change_type: "accepted" | "modified" | "deprecated" }

Process:
  1. Traverse edges for dependent specs
  2. Collect applies_to refs for code/config impact
  3. Search docs semantically for related concepts
  4. Use the LLM only to assess severity and explain why

Output:
  affected_specs[]
  affected_code[]
  affected_docs[]
```

#### Tool: `check_compliance`

**Purpose:** determine whether code matches accepted specs.

**Status:** shipped in the CLI as a deterministic first follow-on after the first shipping slice.

```text
Input:
  { paths: ["src/api/middleware/ratelimiter.go"] }
  OR { diff_text: "..." }

Process:
  1. Identify relevant specs:
     a. via applies_to reverse lookups in the graph
     b. via semantic search from the current file or diff into spec chunks
  2. Read actual source or use the supplied diff as primary evidence
  3. Deterministically classify findings into:
     - compliant
     - conflicting
     - unspecified / no-governing-spec
  4. Cite spec refs, section headings, and changed paths

Output:
  compliant[]
  conflicts[]
  unspecified[]
```

#### Tool: `check_doc_drift`

**Purpose:** detect when non-spec docs contradict or lag behind specs.

```text
Input:
  { doc_ref: "doc://guides/api-rate-limits" }
  OR { doc_refs: ["doc://guides/api-rate-limits", "doc://runbooks/rate-limit-rollout"] }
  OR { scope: "all" }

Output:
  drift_items[]
```

Exactly one selector must be present in v1: `doc_ref`, `doc_refs`, or `scope`. The only valid `scope` value is `"all"`.

#### Tool: `search_specs`

**Purpose:** general semantic search across active specs by default.

```text
Input:
  { query: "how do we handle websocket authentication?",
    filters: { domain: "api", status: "accepted" } }

Output:
  ranked spec sections with excerpts
```

Unless the caller explicitly asks otherwise, `search_specs` should search `draft`, `review`, and `accepted` specs and exclude `superseded` and `deprecated` specs.

#### Tool: `review_spec`

**Purpose:** convenience compound tool for the common spec-review workflow.

```text
Input:
  { spec_ref: "SPEC-042" }
  OR { spec_record: { ... canonical record ... } }

Process:
  1. Run check_overlap
  2. If overlaps exist, run compare_specs
  3. Run analyze_impact
  4. Run check_doc_drift with `doc_refs` from `analyze_impact.affected_docs`
  5. Return one combined report
```

This tool adds convenience, not a new architectural layer.
It should not silently widen doc drift to `{ scope: "all" }` in v1.

---

### 6. Transport Surfaces

Pituitary core ships two first-party surfaces in this repo:

- **CLI** for local automation and scripts, and the required transport for v1 completeness
- **MCP server** as an optional thin wrapper over the same analysis packages

MCP must not introduce separate logic, state, or workflows. `index` remains CLI-only.

CLI examples:

```text
pituitary index --rebuild
pituitary search-specs --query "rate limiting" --format json
pituitary check-overlap --path specs/rate-limit-v2
pituitary check-doc-drift --scope all --format json
pituitary review-spec --path specs/rate-limit-v2 --format json
```

When MCP is present, its tool names should mirror the shipped analysis tools:

- `check_overlap`
- `compare_specs`
- `analyze_impact`
- `check_doc_drift`
- `search_specs`
- `review_spec`

`index` remains a CLI-only operation in this architecture. MCP is a query-and-analysis wrapper over an already-built local workspace index, not a second orchestration surface for rebuilds.

---

### 7. Integrations and Extensions

Integrations should live **outside** the core and consume the CLI or MCP surface.

Examples:

- CI runner that calls `pituitary review-spec`
- git hook that rebuilds the index after local changes
- GitHub adapter that turns PR diffs into `diff_text` and posts results back as comments
- editor plugin that opens findings inline
- PDF ingestion adapter that emits canonical records into the indexer

This is the intended layering:

```text
integration -> CLI/MCP -> core analysis engine -> SQLite index
```

Not:

```text
GitHub workflow logic -> buried inside storage or analysis code
```

The repo may ship a CI job that runs the checked-in `make ci` flow, but GitHub-specific review, commenting, and reporting behavior still lives outside the core architecture.

---

### 8. CLI Server Structure (Go) and Shipped MCP Wrapper

```text
pituitary/
├── go.mod
├── go.sum
├── main.go
├── cmd/
│   ├── index.go                 # rebuild index from configured adapters
│   ├── check.go                 # invoke core analysis from the CLI
│   ├── report.go                # render JSON / markdown / table output
│   └── serve.go                 # optional MCP server mode
├── internal/
│   ├── model/
│   │   └── types.go             # SpecRecord, DocRecord, relation types
│   ├── mcp/                     # optional MCP transport
│   │   ├── server.go            # MCP setup and tool registration
│   │   └── tools.go             # MCP handlers -> core analysis calls
│   ├── source/
│   │   ├── adapter.go           # SourceAdapter interface
│   │   ├── filesystem.go        # V1 filesystem adapter
│   │   └── specbundle.go        # spec.toml + body.md loader
│   ├── chunk/
│   │   └── markdown.go          # heading-aware chunking
│   ├── index/
│   │   ├── store.go             # SQLite metadata + vectors + graph
│   │   ├── graph.go             # relation traversal helpers
│   │   ├── rebuild.go           # full atomic rebuild
│   │   └── embedder.go          # local or API embeddings
│   ├── analysis/
│   │   ├── overlap.go
│   │   ├── compare.go
│   │   ├── impact.go
│   │   ├── drift.go
│   │   └── llm.go               # provider wrapper behind an interface
│   └── config/
│       └── config.go            # adapter and runtime config
├── examples/
│   └── rate-limit/
│       ├── spec.toml
│       └── body.md
└── pituitary.json               # optional later MCP manifest
```

#### Key Dependency and Implementation Choices

| Choice | Purpose | Why |
|---|---|---|
| `github.com/mark3labs/mcp-go` | Optional MCP server framework | Keeps the shipped MCP wrapper thin over the same analysis packages |
| `github.com/asg017/sqlite-vec-go-bindings` | Vector search | Provides the `vec0` virtual table used by the index |
| `github.com/mattn/go-sqlite3` | SQLite engine | Reliable `database/sql` driver for the cgo-backed `sqlite-vec` path |
| Go standard library `flag` package | CLI parsing | The current command surface is small enough that stdlib flags keep startup and dependencies minimal |
| Internal restricted TOML parsers in `internal/config` and `internal/source` | `pituitary.toml` and `spec.toml` parsing | The bootstrap only needs a narrow validated TOML subset, so the parser stays internal instead of adding a TOML dependency |
| Internal heading-aware Markdown chunker in `internal/chunk` | Markdown sectioning | Retrieval only needs ATX heading splits plus title-scoped fallback chunks, not a full Markdown AST |

**Why this works well in Go:**

- Single binary distribution
- Fast startup for the CLI today, with room for an on-demand MCP process later
- Easy parallel embedding calls during rebuilds
- Clean interfaces between adapters, index, and analysis

---

## Data Flow Summary by Goal

| Goal | Current status | Trigger | Key Data Path |
|---|---|---|---|
| 1. Overlap detection | yes | New or changed spec | Spec record -> embed -> candidate retrieval -> deterministic overlap analysis today |
| 2. Tradeoff analysis | yes | Overlap detected | Spec refs -> full text retrieval -> deterministic comparison today |
| 3. Impact analysis | yes | Spec accepted/modified/deprecated | Graph traversal + doc search -> deterministic impact analysis today |
| 4. Code compliance | yes (CLI) | Changed code or diff | Source/diff -> applies_to lookup + semantic fallback -> deterministic compliance findings today |
| 5. Doc sync | yes | Changed docs or changed spec | Doc chunks vs spec chunks -> deterministic drift detection today |

All tools keep the same discipline: retrieval first, then deterministic analysis today or provider-backed adjudication later.

---

## Implementation Roadmap

### Workstream 0: Contract Freeze

- Freeze canonical ref, source-ref, and applies-to schemes
- Freeze status and supersession semantics
- Freeze JSON request, response, and error envelopes
- Freeze the bootstrap runtime contract while preserving the config shape for later provider-backed runtime work
- Lock fixture expectations for overlap, impact, and doc drift

### Workstream 1: Workspace and Ingestion

- Parse `pituitary.toml`
- Implement the filesystem spec-bundle loader
- Implement the filesystem Markdown-doc loader
- Normalize both into canonical `artifacts`
- Reject invalid records with actionable errors

### Workstream 2: Index and Retrieval

- Build the SQLite schema
- Implement full atomic rebuild into `.pituitary/pituitary.db`
- Chunk Markdown by heading
- Generate embeddings for spec and doc chunks
- Implement filtered vector retrieval via `chunks_vec -> chunks -> artifacts`
- Ship `search_specs`

### Workstream 3: Core Spec Analysis

- Implement `check_overlap`
- Implement `compare_specs`
- Implement `analyze_impact`
- Implement CLI-first deterministic `check_compliance`
- Implement `check_doc_drift`
- Implement `review_spec`

### Workstream 4: Interface and Output

- Ship a JSON-first CLI for every required command
- Add Markdown rendering for human-readable reports
- Keep transport code as a thin layer over the same analysis packages
- Keep the shipped MCP wrapper thin and aligned with CLI behavior without blocking the first ship

### Deferred Until After First Ship

- Non-filesystem source adapters
- GitHub-specific flows and vendor-specific CI/reporting integrations
- Incremental indexing
- Stored code embeddings or code-summary corpora

---

## Acceptance Criteria for the First Shipping Slice

The first shipping slice is done when all of the following are true:

1. `pituitary index --rebuild` reads `pituitary.toml`, builds a fresh SQLite index, and swaps it atomically.
2. A fixture workspace with at least three specs and two docs can be indexed without manual intervention.
3. `pituitary search-specs --query "..." --format json` returns ranked spec sections with stable artifact refs.
4. `pituitary check-overlap --path specs/<bundle> --format json` detects a known overlapping fixture pair without requiring a ref lookup first.
5. `pituitary compare-specs --path path/to/spec-a --path path/to/spec-b --format json` returns a structured comparison for indexed specs.
6. `pituitary analyze-impact --path path/to/spec --format json` returns dependent specs and affected docs from the graph and retrieval layers.
7. `pituitary check-doc-drift --scope all --format json` flags at least one known contradictory fixture doc.
8. `pituitary review-spec --path path/to/spec --format json` composes overlap, comparison, impact, and doc-drift findings in one response.
9. All required commands work without GitHub, git metadata, or network-only integrations.
10. All required commands fail with clear validation errors when a spec bundle is malformed.
11. All shipped commands follow the documented JSON envelope, and unsupported runtime providers fail clearly during config validation.

---

## Cost and Performance Estimates (50-spec scale)

- **Embedding storage:** low single-digit MBs
- **Full index rebuild:** comfortably under 20s with the fixture embedder on the bootstrap corpus
- **Per-query latency:** typically subsecond to low-single-digit seconds on the bootstrap corpus, depending on command
- **Binary size:** roughly 15-20MB
- **Marginal analysis cost:** none in the current bootstrap runtime

The important v1 property is not raw speed. It is that the system stays simple, deterministic in retrieval, and decoupled from any one source or workflow stack.
