# Complexity Baseline - 2026-05-07

Baseline captured with the `code-complexity-review` skill and the Nanto skills CLI after moving the working branch to Stroma v3.0.0.

Command:

```sh
skills complexity-scan --path . --top 20
```

## Scanner Snapshot

- Packages: 25
- Go files: 257
- Functions: 2,390
- Largest production files:
  - `internal/analysis/doc_drift.go` - 1,896 lines
  - `internal/analysis/compliance.go` - 1,872 lines
  - `internal/config/config.go` - 1,636 lines
  - `internal/index/rebuild.go` - 1,211 lines
  - `internal/source/filesystem.go` - 993 lines
  - `cmd/render_review.go` - 969 lines
  - `internal/source/discover.go` - 968 lines
  - `extensions/json/adapter.go` - 861 lines
  - `internal/analysis/openai_provider.go` - 796 lines
- Highest production branch scores:
  - `cmd/run_command.go:161` `runCommand` - score 47
  - `internal/config/config.go:1177` `validateRuntime` - score 41
  - `internal/config/parse_toml.go:56` `undecodedKeyMessage` - score 40
  - `cmd/render_status.go:11` `renderStatusResult` - score 34
  - `cmd/render_docdrift.go:11` `renderDocDriftResult` - score 32
  - `internal/index/rebuild.go:419` `finalizeBusinessIndexContext` - score 31
  - `internal/index/update.go:64` `updateContext` - score 30
  - `cmd/index.go:40` `runIndexContext` - score 29
- Highest churn production files:
  - `cmd/render.go` - 5,801 changed lines
  - `internal/config/config.go` - 3,431
  - `internal/index/rebuild.go` - 3,157
  - `internal/analysis/doc_drift.go` - 2,824
  - `internal/analysis/compliance.go` - 2,292
  - `internal/index/update.go` - 1,986
  - `internal/source/filesystem.go` - 1,985
  - `internal/analysis/openai_provider.go` - 1,536
  - `internal/analysis/terminology.go` - 1,224
  - `internal/index/search.go` - 1,111

## Findings

1. High - Analysis workflows co-locate selection, deterministic scoring, optional model refinement, warning assembly, and remediation output.

   Locations: `internal/analysis/doc_drift.go:214`, `internal/analysis/doc_drift.go:288`, `internal/analysis/doc_drift.go:412`, `internal/analysis/compliance.go:258`, `internal/analysis/compliance.go:325`, `internal/analysis/compliance.go:409`.

   Reader cost: understanding one user-facing result requires keeping source scope, repository loading, relevance selection, deterministic findings, model adjudication, warning propagation, remediation, and final sorting in working memory at once. This is partly domain complexity, but the current locality makes behavior changes expensive.

   Simplification path: keep public result types stable, but split each workflow into explicit phases: input/scope resolution, candidate selection, deterministic evaluation, optional semantic refinement, and result assembly. The existing helper seams are present; the next step is file/module separation around those phases.

2. High - Command lifecycle is centralized behind a large option matrix.

   Location: `cmd/run_command.go:161`.

   Reader cost: `runCommand` owns flag registration, positional validation, config resolution, request-file handling, request building, normalization, timing, execution, post-processing, multirepo warnings, and rendering. Adding one command option requires auditing unrelated branches.

   Simplification path: preserve the generic command contract, but extract `validateCommandPlan`, `parseCommandRequest`, `loadCommandConfig`, and `executeCommandPlan` helpers with small return types. This should reduce branch pressure without duplicating command behavior.

3. Medium - Runtime config validation duplicates provider rules across embedder and analysis providers.

   Location: `internal/config/config.go:1177`.

   Reader cost: profile resolution, provider-specific required fields, endpoint URL validation, and retry/token checks are interleaved twice. The duplicated shape makes future provider changes easy to apply to one runtime surface and miss on the other.

   Simplification path: introduce a small `validateRuntimeProvider(label, provider, supported, requirements)` helper, then keep `validateRuntime` as orchestration over profile resolution plus provider validation.

4. Medium - Index rebuild/update crosses Stroma snapshot state and Pituitary business-index state in several places.

   Locations: `internal/index/rebuild.go:419`, `internal/index/update.go:64`, `internal/index/reuse.go:30`.

   Reader cost: the Stroma v3 upgrade exposed a hidden coupling: Stroma now owns normalized record content hashes, while Pituitary still has its own artifact content hashes. Reuse accounting had to be realigned with Stroma-normalized record identity. Future snapshot API changes can create similar mistakes if the boundary remains implicit.

   Simplification path: make the boundary explicit with a small internal type such as `stromaRecordIdentity` or `snapshotReuseIdentity`, and route rebuild, update, and dry-run reuse accounting through it.

5. Medium - Text renderers are branch-heavy and mix section selection with string formatting.

   Locations: `cmd/render_status.go:11`, `cmd/render_docdrift.go:11`, `cmd/render_review.go`.

   Reader cost: presentation rules, filtering rules, fallback rules, and exact CLI strings are interleaved. These files are user-visible and high-churn, so small output changes carry high regression risk.

   Simplification path: keep output text stable, but extract section-level renderers that accept already-selected view models. Tests can then assert view-model selection separately from exact formatting.

## Domain Complexity To Preserve

- Split-store model: Pituitary business DB plus immutable Stroma snapshot.
- CLI-first JSON contract and optional MCP wrapper over the same core.
- Deterministic retrieval before optional provider-backed adjudication.
- Temporal and confidence-aware governance semantics.
- Local filesystem scope and explicit source configuration.

## Suggested Next Steps

- Use this file as the baseline for future complexity deltas.
- Prioritize simplification where high churn overlaps high branch pressure: `internal/analysis/doc_drift.go`, `internal/analysis/compliance.go`, `internal/index/rebuild.go`, and `cmd/run_command.go`.
- Treat scanner output as leads, not pass/fail gates. A future CI gate should compare trends only after at least one follow-up baseline confirms stability.
