# Pituitary First-Ship Backlog

This backlog is derived from the first-shipping-slice definition in `ARCHITECTURE.md`.

## Goal

Ship a local, filesystem-only Pituitary that can index Markdown-based specs and docs, build a consistent SQLite index, and answer the core spec-management questions:

- `search_specs`
- `check_overlap`
- `compare_specs`
- `analyze_impact`
- `check_doc_drift`
- `review_spec`

`check_compliance` is explicitly deferred until after the first ship.
The minimum first-ship contract remains CLI-first, but the repo also ships an optional MCP wrapper over the same analysis packages.
The repo also ships CI for fmt, readiness, test, and vet validation, while GitHub- or vendor-specific reporting integrations remain deferred.

## Milestone 0: Contract Freeze

- [ ] Freeze canonical `ref`, `source_ref`, and `applies_to` schemes
- [ ] Freeze status and supersession semantics
- [ ] Define one shared JSON envelope plus per-command request/result shapes for shipped commands
- [ ] Define the embedding and qualitative-analysis provider contracts, including config shape, env-based credentials, timeout/retry policy, and degraded-mode behavior
- [ ] Freeze the targeted `check_doc_drift` scope contract used by `review_spec`
- [ ] Add a deterministic fixture provider mode for tests and CI
- [ ] Lock the expected findings for the bootstrap fixture workspace

Definition of done:

- the architecture, backlog, README, and fixtures agree on identifier, status, discovery, and output rules
- every shipped command has a documented JSON request/result shape and shared error object
- tests and CI can run against the deterministic fixture provider without live model credentials
- AI-backed commands have explicit dependency-unavailable behavior

## Milestone 1: Workspace and Config

- [ ] Parse `pituitary.toml`
- [ ] Resolve workspace root relative to the config file
- [ ] Validate `workspace.index_path`
- [ ] Validate configured `sources`
- [ ] Produce actionable config errors for unknown adapters or missing paths

Definition of done:

- `pituitary index --rebuild` fails fast on invalid config with human-readable errors
- relative paths in `pituitary.toml` resolve consistently

## Milestone 2: Source Adapters

- [ ] Implement `filesystem` adapter for `spec_bundle`
- [ ] Discover spec bundles recursively by directories containing `spec.toml`
- [ ] Require exactly one `spec.toml` and one referenced body file per spec bundle
- [ ] Implement `filesystem` adapter for `markdown_docs`
- [ ] Derive doc refs from workspace-relative Markdown paths
- [ ] Normalize spec bundles into canonical artifact records
- [ ] Normalize docs into canonical artifact records
- [ ] Emit stable `source_ref` values and content hashes

Definition of done:

- the current fixture workspace indexes at least 3 specs and 2 docs
- malformed spec bundles fail with a clear path-specific error

## Milestone 3: Index and Rebuild

- [ ] Create SQLite schema for `artifacts`, `chunks`, `chunks_vec`, and `edges`
- [ ] Size `chunks_vec` from the configured embedder dimension rather than a hardcoded constant
- [ ] Add secondary indexes for filtered retrieval and graph traversal
- [ ] Implement full rebuild into a staging DB
- [ ] Run integrity checks before swap
- [ ] Atomically swap staging DB into the configured index path
- [ ] Open a fresh read-only DB handle per request or implement generation-based reload

Definition of done:

- `pituitary index --rebuild` produces a complete index from the fixture workspace
- a failed rebuild never corrupts the last good index

## Milestone 4: Chunking and Retrieval

- [ ] Chunk Markdown by heading-aware sections
- [ ] Generate embeddings for every spec/doc chunk
- [ ] Insert chunk vectors into `chunks_vec`
- [ ] Implement vector retrieval as `chunks_vec -> chunks -> artifacts`
- [ ] Support filtering by `kind`, `status`, and `domain`
- [ ] Ship `search_specs` as the first end-to-end query

Definition of done:

- `pituitary search-specs --query "rate limiting" --format json` returns ranked sections with stable artifact refs
- filtered vector queries do not require denormalized metadata in `chunks_vec`

## Milestone 5: Core Spec Analysis

- [x] Implement `check_overlap`
- [x] Implement `compare_specs`
- [x] Implement `analyze_impact`
- [x] Support `doc_ref`, `doc_refs[]`, and `scope=all` in `check_doc_drift`
- [x] Implement `check_doc_drift`
- [x] Implement `review_spec` as a composition layer over the other tools

Definition of done:

- known overlap between `SPEC-008` and `SPEC-042` is detected
- `SPEC-055` is reported as impacted by changes to `SPEC-042`
- the outdated API guide is reported as drifting from `SPEC-042`

## Milestone 6: CLI and Output

- [x] Add a JSON-first CLI contract for every shipped command
- [x] Add shared human-readable rendering derived from the shipped result types
- [x] Keep CLI logic thin over shared analysis packages

Definition of done:

- every required command supports `--format json`
- `review_spec` returns one composed JSON report suitable for automation

## Shipped Alongside the Minimum First Ship

- [x] MCP transport
- [x] Repository CI workflow for fmt, readiness, test, and vet validation

## Still Deferred

- [ ] `check_compliance`
- [ ] non-filesystem adapters
- [ ] GitHub or CI vendor integrations beyond the shipped repo CI workflow
- [ ] incremental indexing
- [ ] stored code-summary embeddings

## Fixture Expectations

The scaffolded fixture workspace is intended to support early end-to-end checks:

- `SPEC-008` and `SPEC-042` intentionally overlap
- `SPEC-008` is marked `superseded`
- `SPEC-042` supersedes `SPEC-008`
- `SPEC-055` depends on `SPEC-042`
- `docs/guides/api-rate-limits.md` intentionally contains stale values
- `docs/runbooks/rate-limit-rollout.md` is aligned with the newer design

## Suggested Build Order

1. Freeze the v1 contracts, provider behavior, and fixture expectations.
2. Make `pituitary index --rebuild` work on the fixture workspace.
3. Ship `search_specs`.
4. Ship `check_overlap`.
5. Ship `compare_specs`.
6. Ship `analyze_impact`.
7. Ship `check_doc_drift`.
8. Ship `review_spec`.
