# RFC 0003: Migrate Vector/Index Layer from Direct Stroma to Hippocampus

## Status

Accepted

## Date

2026-05-19

## Related

- Issue: [#412](https://github.com/nantobv/pituitary/issues/412)
- RFC 0001: Spec-Centric Compliance Direction
- RFC 0002: Kernel/Extension Adapter Architecture

## Summary

Pituitary currently imports `github.com/dusk-network/stroma/v3` directly across 39 files spanning 7 stroma subpackages. Much of that surface — embedder wiring, chat-completion plumbing, chunking policy, RRF fusion, snapshot lifecycle, search execution, outline retrieval, quantization configuration — duplicates capabilities that [Hippocampus](https://github.com/nantobv/hippocampus) already owns as the "neutral corpus catalog above Stroma." Hippocampus exists explicitly so consumers like Pituitary do not have to know that vectors, embeddings, or quantization exist.

Replace Pituitary's direct Stroma dependency with a Hippocampus dependency after the dependency, generation, and API-gate prerequisites below are satisfied. Delete duplicated plumbing without weakening Pituitary's public CLI/MCP contracts. Restore Pituitary to its governance-only mission: consistency over specs, docs, and code.

## Context

### What Pituitary is supposed to be

Per `AGENTS.md`:

> Pituitary is a consistency governance tool that keeps specifications, documentation, and code aligned. It builds a temporal, confidence-weighted governance graph from spec bundles and docs, then runs overlap, drift, compliance, impact, and freshness analysis against it.

Nothing in that mission requires Pituitary to know about vectors, embeddings, chunking, quantization, or hybrid retrieval. Those are *implementation strategies* for the question "given this corpus, surface the chunks relevant to this query." That question is downstream infrastructure, not governance.

### What Pituitary actually does today

Direct Stroma imports by subpackage:

| Stroma subpackage | Files | Pituitary responsibility |
|---|---|---|
| `stroma/v3/index` | 27 | rebuild, update, reuse, snapshot lifecycle, search, fusion, analysis result types, retrieval benches |
| `stroma/v3/chunk` | 8 | chunking policy, contextualizer, parent-chain, outline context |
| `stroma/v3/corpus` | 6 | record-stream production, intent context |
| `stroma/v3/store` | 4 | quantization constants, low-level store ops |
| `stroma/v3/embed` | 3 | embedder wiring, OpenAI-compatible client |
| `stroma/v3/provider` | 1 | error mapping |
| `stroma/v3/chat` | 1 | chat completion for semantic analysis |

39 files; ~3,000+ lines of plumbing. Every Stroma version bump forces Pituitary to chase API changes that have nothing to do with consistency governance.

### What Hippocampus is

From its `AGENTS.md`:

> Hippocampus is a neutral corpus catalog layered on top of Stroma. It owns stable identity and provenance for corpus source items, the catalog Stroma indexes against, and the product-facing search API that maps Stroma hits back to catalog identities. It also owns configured OpenAI-compatible primitive wrappers when they are generic configuration doors over Stroma clients, such as embeddings and chat completion. It does not own governance, wiki compilation, research run state, source discovery, prompt design, chat-output semantic validation, or Stroma internals.

The boundary is precisely the one Pituitary has been crossing: registration, indexing, search, embedder/chat config are Hippocampus's job; governance is not.

### The canonical example: PR #402

PR #402 (merged 2026-05-09) added `runtime.quantization` to Pituitary's config — exposing a Stroma-internal vector storage choice through Pituitary's public TOML surface. It threaded the field through:

- `internal/config/config.go` (constants, validation, render round-trip, accept-list)
- `internal/index/rebuild.go` (`BuildOptions.Quantization`)
- `internal/index/update.go` (`UpdateOptions.Quantization` + a new `validateStoredQuantizationContext` eligibility gate to close a no-diff fast-path hole)
- `internal/index/reuse.go` (cross-quantization reuse rejection so two layers agree)
- `internal/index/corpus_snapshot.go` (shared helpers)

The PR itself is well-engineered. The shape of the work — a config surface, a no-diff gate, a reuse-state correction, a shared helper, sibling-gap notes across four adapters — is exactly what happens when an architectural boundary is in the wrong place. Every consumer of `runtime.quantization` is a Stroma internal. Hippocampus already exposes `HIPPOCAMPUS_QUANTIZATION` via `LoadIndexConfigFromEnv` and accepts the same value set. Pituitary reimplemented the surface one layer too high.

PR #402 closeout followups for #340 were going to add a benchmark and operator-facing documentation recommending int8 — both of which are also Stroma-internal questions Pituitary has no basis to answer. They were the trigger for this RFC.

### Why now

- Stroma version drift: Pituitary is on stroma v3; Hippocampus is on stroma v4. Continuing to track Stroma directly means double-tracking releases.
- Surface entrenchment: every PR that adds a `runtime.*` knob hardens the leaky abstraction. The longer this waits, the more deprecation cycles a migration costs.
- Hippocampus is ready: issues #22-#25 (source-item outlines, source-span provenance, hierarchical chunk policy, item-level search aggregation) — all driven by Pituitary's needs — have landed or are well-defined upstream. The catalog has the shape Pituitary requires.
- Pituitary's public build and release path is not yet ready for a private Hippocampus dependency. That is a blocker, not a footnote; see [Blocking Prerequisites](#blocking-prerequisites).

### Strategic significance

This migration is substrate extraction, not product abdication. Hippocampus should remember the corpus: artifact identity, provenance, spans, snapshots, chunking, retrieval, and neutral temporal memory. Pituitary should judge consistency within and across the corpus: claims, obligations, requirements, evidence, contradiction, freshness, impact, and materiality.

That boundary preserves the current developer wedge while keeping the architecture open to broader governed corpora later. Software specs, docs, and code are the first domain. Tender packs, compliance evidence, legal document sets, policies, and other high-stakes corpora have the same consistency-governance shape, but different domain semantics. Hippocampus must stay neutral by default; Pituitary must keep the domain-specific consistency graph and findings.

The safe rule for this RFC:

> Move neutral corpus memory into Hippocampus. Keep consistency judgment in Pituitary.

## Audit Findings

Every Stroma-importing file in Pituitary, grouped by responsibility cluster.

### Cluster 1: Embedder configuration & wiring → **clean swap, delete duplication**

| Pituitary | Stroma uses | Hippocampus equivalent |
|---|---|---|
| `internal/index/embedder.go` | `embed` | `Bootstrap(WithEmbedder(...))`, `LoadOpenAIEmbedderFromEnv` |
| `internal/index/openai_embedder.go` | `embed` | `NewOpenAIEmbedder`, `NewContextualOpenAIEmbedder`, batch-fallback already implemented upstream |
| `internal/runtimeerr/errors.go` | `provider` | Hippocampus exposes typed errors (`ErrIndexerUnavailable`, `ErrEmbedderUnavailable`, `ErrSnapshotQuarantined`) |
| `internal/config/config.go` (`runtime.embedder` block) | (config only) | `LoadOpenAIEmbedderConfigFromEnv` + env-var conventions |

Hippocampus already wraps the batch-fallback heuristic Pituitary partially reimplemented, plus the contextualizer (`ContextualEmbedderConfig`). Pituitary keeps a thin config-to-env-or-Option translation; everything else deletes.

### Cluster 2: Chat completion → **clean swap**

| Pituitary | Stroma uses | Hippocampus equivalent |
|---|---|---|
| `internal/analysis/openai_provider.go` | `chat` | `LoadOpenAIChatFromEnv` (generic OpenAI-compatible chat wrapper) |

Hippocampus's docstring is explicit: it owns "configuration and availability errors only; prompts and semantic output validation stay with callers." That's exactly the right split. Pituitary keeps its analysis prompts; the transport layer migrates.

### Cluster 3: Source loading → **clean swap with shape change**

| Pituitary | Stroma uses | Hippocampus equivalent |
|---|---|---|
| `internal/source/stream.go` | `corpus` | `Register(RegisterRequest{...})` per source item |

Pituitary currently produces `corpus.Record` directly and hands them to Stroma's `RebuildFromSource`. Hippocampus's model is different: callers `Register` source items into the catalog, then `Index` synchronizes Stroma from the catalog. The translation is straightforward: each `SpecRecord` becomes one `RegisterRequest` with a `SourceKind = "spec_bundle"` and one or more artifacts; each `DocRecord` becomes one with `SourceKind = "markdown_docs"`. Pituitary-specific metadata (relations, kind, owner) travels via `RegisterRequest.Metadata`.

This composes well with the kernel/extension architecture (RFC 0002): source adapters keep producing `sdk.SpecRecord` / `sdk.DocRecord`, and a single translation layer (`internal/catalog` or similar) maps to `hippocampus.Register`. Adapters do not learn about Hippocampus.

### Cluster 4: Chunking policy → **DELETE**

| Pituitary | Stroma uses | Hippocampus equivalent |
|---|---|---|
| `internal/chunk/markdown.go` | `chunk` | Subsumed by `IndexConfig.ChunkPolicy = "flat"|"late"` |
| `internal/chunk/policy.go` | `chunk`, `index` | Subsumed by `IndexConfig.ChunkPoliciesBySourceKind` and `ChunkPoliciesByArtifactRole` |
| `internal/chunk/contextualizer.go` | `chunk`, `corpus`, `index` | Subsumed by `NewContextualOpenAIEmbedder` + `ContextualEmbedderConfig` |

The entire `internal/chunk/` package deletes. Hippocampus's `IndexConfig` already supports flat vs late chunking, per-source-kind overrides, per-artifact-role overrides, parent/leaf token caps, and overlap configuration. Pituitary's `runtime.chunking.spec` / `runtime.chunking.doc` config block becomes a thin pass-through to `IndexConfig`; any move from Pituitary TOML to `HIPPOCAMPUS_*` env vars is a separate config RFC, not part of this migration.

### Cluster 5: Index rebuild, update, reuse, snapshot → **clean swap, massive deletion**

| Pituitary | Stroma uses | Hippocampus equivalent |
|---|---|---|
| `internal/index/rebuild.go` | `index` | `hippocampus.Index(IndexOptions{IndexKind: IndexKindCorpus})` |
| `internal/index/update.go` | `index`, `store` | Same — Hippocampus chooses incremental vs full-rebuild internally |
| `internal/index/reuse.go` | `index` | Deleted — Hippocampus owns reuse-eligibility decisions |
| `internal/index/corpus_snapshot.go` | `index` | Deleted — Hippocampus owns snapshot lifecycle |
| `internal/index/store.go` | `index`, `store` | Deleted |
| `internal/index/quantization_test.go` | (rebuild + update tests) | Deleted — these tests verify Hippocampus's responsibility |

Thousands of lines disappear. Hippocampus's `Index` method already handles the same decision tree (config-compat → incremental, otherwise full rebuild), wraps it in proper locking (`acquireIndexLock`), records an `index_runs` ledger row, and quarantines damaged snapshots. None of that needs to live in Pituitary.

### Cluster 6: Hybrid & lexical search → **clean swap**

| Pituitary | Stroma uses | Hippocampus equivalent |
|---|---|---|
| `internal/index/search.go` | `index` | `hippocampus.Search(SearchQuery{...})` |
| `internal/index/search_lexical.go` | `index` | `hippocampus.SearchLexical(SearchQuery{...})` |
| `internal/index/intent_context.go` | `index`, `corpus` | Partial — outline retrieval moves to Hippocampus, intent reranking stays |

Hippocampus's `Search` already does RRF fusion of FTS + vector arms, supports per-kind and per-collection filtering, and surfaces a richer `CorpusHit` than Pituitary's current `index.SearchResult` (snapshot fingerprint, per-arm provenance, source spans, canonical URI, artifact role). Pituitary-specific status/domain/repo/source-adapter filtering still needs either Hippocampus support or Pituitary-side post-filtering before each reader cutover.

### Cluster 7: RRF fusion → **DELETE**

| Pituitary | Stroma uses | Hippocampus equivalent |
|---|---|---|
| `internal/fusion/fusion.go` (69 lines) | `index` | Hippocampus fuses internally inside `Search` |

Pituitary reimplemented RRF fusion of FTS and vector arms. Hippocampus already fuses internally. Delete the whole package.

### Cluster 8: Outline, parent-chain, intent context → **partial swap**

| Pituitary | Stroma uses | Hippocampus equivalent | Verdict |
|---|---|---|---|
| `internal/index/outline_context.go` | `chunk`, `index` | `GetItemOutline`, `GetArtifactOutline`, `OutlineNode` | clean swap |
| `internal/index/parent_chain.go` | `chunk` | Late-chunking parent chain emerges from Hippocampus outline output | clean swap |
| `internal/index/intent_context.go` | `corpus`, `index` | Governance-specific reranking — **pituitary keeps** | type substitution only |

The outline retrieval primitives are exactly what hippocampus issues #22 and #23 added upstream. Pituitary's governance-side reranking (intent-aware boosting, kind-priority weighting) stays as a consumer of `CorpusHit` and `OutlineNode`.

### Cluster 9: Quantization config → **DELETE**

| Pituitary | Stroma uses | Hippocampus equivalent |
|---|---|---|
| `internal/config/config.go` (`runtime.quantization` field, validation, render, accept-list) | `store` (for `QuantizationFloat32/Int8/Binary` constants) | `IndexConfig.Quantization`, `HIPPOCAMPUS_QUANTIZATION` env var |
| `internal/index/quantization_test.go` (gate + reuse-state tests) | (rebuild + update tests) | Subsumed by Hippocampus's own tests |
| `internal/index/{corpus_snapshot,rebuild,update,reuse}.go` (quantization plumbing added by #402) | `store` | Disappears with the cluster-5 deletion |

Delete `runtime.quantization` outright in Phase 1. There is no Pituitary compatibility surface, deprecation cycle, or forward shim for this key. Quantization is a Hippocampus/Stroma concern and must not remain a Pituitary TOML contract. Older non-default Stroma snapshots are not kept alive as a compatibility mode: Pituitary forces a full rebuild before incremental update or reuse.

### Cluster 10: Analysis modules consuming search hits → **clean swap, type substitution**

| Pituitary | Stroma uses | Hippocampus equivalent |
|---|---|---|
| `internal/analysis/overlap.go` | `index.SearchResult` | `hippocampus.CorpusHit` |
| `internal/analysis/doc_drift.go` | `index.SearchResult` | `hippocampus.CorpusHit` |
| `internal/analysis/repository.go`, `repository_similarity.go` | `index.SearchResult` | `hippocampus.CorpusHit` |
| `internal/analysis/semantic_terminology.go` | `index.SearchResult` | `hippocampus.CorpusHit` |

These modules are governance logic that happens to consume search hits. The logic stays in Pituitary; the type they consume substitutes only behind Pituitary-owned adapters. `CorpusHit` appears richer than `index.SearchResult`, but each command must prove that every currently used field and ranking behavior has an equivalent before its public DTO changes internally.

### Cluster 11: Benchmarks → **rewrite as hippocampus-consumer benches**

| Pituitary | Stroma uses | Hippocampus equivalent |
|---|---|---|
| `internal/index/retrieval_precision_bench_test.go` | `index` | Same metrics, hippocampus.Search driver |
| `internal/index/retrieval_chunk_precision_bench_test.go` | `index` | Same |
| `internal/index/retrieval_armb_bench_test.go` | `index` | Same |

The benchmarks measure governance-relevant retrieval quality (precision@k on labeled cases, chunk-level recall) — that question is legitimately Pituitary's. The driver substitutes from `RebuildContext` / `OpenStromaSnapshotContext` to `hippocampus.Index` / `hippocampus.Search`.

## Decision

Adopt Hippocampus as Pituitary's sole vector/index/embedder/chat dependency, conditional on the prerequisites below. Remove all direct Stroma imports from Pituitary. Delete duplicated plumbing. Push remaining gaps upstream as Hippocampus issues before cutover.

The architectural rule going forward:

> Pituitary does not import `github.com/dusk-network/stroma/*`. Vector storage, embedding, chunking, quantization, snapshot lifecycle, and search execution are Hippocampus concerns. Pituitary consumes Hippocampus's public API and never reasons about its internals.

A lint or CI guard enforces this: `rg 'github.com/dusk-network/stroma' cmd internal --glob '*.go'` must return empty after the deletion phase. The module graph may still contain Stroma transitively through Hippocampus.

This is not permission to break the public checkout. No Phase 2+ PR may land on the default branch if `go test ./...` in a normal Pituitary checkout requires private credentials unavailable to the intended contributor and release audience.

## Blocking Prerequisites

These gates close at different points in the migration. The dependency gate blocks the first PR that imports Hippocampus. The generation gate blocks publisher cutover. The API and DTO gates block reader cutover.

1. **Buildable dependency path.** Resolve Hippocampus availability. Acceptable answers are:
   - make `github.com/nantobv/hippocampus` public with an explicit compatible license and tagged release;
   - vendor or mirror a reviewed Hippocampus release in a way that keeps `go test ./...` working from a fresh public Pituitary checkout; or
   - explicitly decide that Pituitary is private/internal for this release train.

   Merely documenting `GOPRIVATE=github.com/nantobv/*` is insufficient for a public Pituitary release because fork PRs, source installs, and unauthenticated contributors would fail before tests run.

2. **Generation boundary.** Define and test a single active generation contract across:
   - Pituitary's governance DB (`pituitary.db`);
   - the Hippocampus catalog (`hippocampus.db`);
   - the active Stroma snapshot owned by Hippocampus.

   `pituitary index --rebuild` remains the only publisher of a new Pituitary generation. It stages both the Pituitary DB and the Hippocampus root under the same `.pituitary/` staging directory, validates both, then publishes them together. Pituitary stores the active Hippocampus root path, Hippocampus content fingerprint, Stroma snapshot fingerprint, Pituitary content fingerprint, and generation ID in `pituitary.db` metadata. Query commands load Hippocampus through that generation pointer, not through an independently discovered "latest" catalog. A failed rebuild leaves the previous generation readable.

3. **Search/API parity gates.** Before each command cutover, verify that the Hippocampus surface preserves the command's current behavior or file an upstream blocker. Required checks include:
   - hybrid search with lexical fallback and provenance;
   - lexical-only search;
   - vector-only semantic-similarity paths, or an explicit decision to remove/replace them;
   - Pituitary's historical/terminology-aware reranking;
   - filters for kind, status, domain, repo, source adapter, inference confidence, and collections;
   - stable hit expansion handles carrying snapshot and chunk-content identity;
   - benchmark access to precision/recall/MRR inputs and snapshot-size metadata.

4. **Transport DTO mapping.** Hippocampus types are internal implementation inputs. Pituitary's CLI/MCP JSON schemas remain owned by Pituitary. No command exposes raw `hippocampus.CorpusHit` directly unless a separate wire-contract change is accepted.

## Migration Phases

Phases are sequenced so each one ships independently and leaves the codebase in a working state. Phase 1 is uncontroversial regardless of whether later phases proceed.

### Phase 1: Pre-migration cleanups (independent PRs)

1. **Remove PR #402** (`runtime.quantization`). Delete the config surface, rendering, validation, rebuild/update/reuse plumbing, and tests. Close #340 with the architectural rationale: quantization is a Hippocampus/Stroma concern, not a Pituitary contract.
2. **File upstream issues for verified gaps** (see [Open Questions](#open-questions)). These do not block Phase 2 start, but each must close before its dependent migration step.

### Phase 2: Insert the Hippocampus dependency

1. Add `github.com/nantobv/hippocampus` to `go.mod` only after the buildable-dependency prerequisite is resolved. If the accepted path remains private, document `GOPRIVATE=github.com/nantobv/*` in `README.md` and `docs/development/prerequisites.md`; if the path is public, document the normal source-install flow instead.
2. Introduce `internal/catalog/` (new package) with two responsibilities:
   - **Translation:** `sdk.SpecRecord` / `sdk.DocRecord` → `hippocampus.RegisterRequest`. Handles the `SourceKind` mapping, `Metadata` envelope for governance-specific fields, slug allocation via `MakeUniqueSlug`, and `FindBySourceKey` / `RegisterSourceRevision` for changed content at the same canonical URI.
   - **Artifact mirror:** mirrors indexed source artifacts into the staged Hippocampus `raw/` tree because Hippocampus requires source-owned artifact `rel_path` values under `raw/`. The mirror is derived cache state, not authority. Original `file://` provenance remains in `CanonicalURI` and metadata. For spec bundles, register `body.md` as the indexed `primary-source` artifact and record the `spec.toml` SHA in metadata; if two-file provenance is needed, register `spec.toml` as a non-indexed attachment.
   - **Bootstrap:** opens or creates a Hippocampus root directory next to the Pituitary DB, for example `.pituitary/hippocampus/`, never the `workspace.index_path` file itself. During rebuild it uses a staged root such as `.pituitary/staging/<generation>/hippocampus/` and returns a `*hippocampus.Hippocampus` bound to that generation.
   - **Config translation:** maps Pituitary's checked-in runtime config (`runtime.embedder`, `runtime.analysis`, `runtime.search`, and `runtime.chunking`) into Hippocampus options and wrapper configs. Hippocampus env vars are supported for direct Hippocampus use, but Pituitary's TOML remains the Pituitary operator contract.
3. Add an integration test that walks a fixture corpus through `internal/catalog.Register` and asserts the resulting Hippocampus state matches expectations.

Phase 2 does not touch any command. Phase 1 resolves the immediate config leak and Phase 2 introduces parallel infrastructure; nothing in `cmd/` changes yet.

### Phase 3: Publisher cutover

Migrate the generation publisher first. Search and analysis commands must not point at Hippocampus until `pituitary index` creates the catalog they read.

1. `index --rebuild`, `index --update`, and `index --dry-run`
   - load the existing Pituitary source adapters;
   - mirror indexed artifacts into the staged Hippocampus root;
   - register source items and revisions in the staged Hippocampus catalog;
   - run `hippocampus.Index`;
   - build the staged Pituitary governance DB from the same normalized records;
   - publish the generation only after both stores validate.
2. `status` and freshness checks
   - read generation metadata from `pituitary.db`;
   - report Hippocampus catalog and snapshot fingerprints;
   - preserve current runtime-config and runtime-probe output.
3. Add regression tests for failed rebuild rollback, stale generation rejection, and mixed-generation prevention.

### Phase 4: Reader cutover (command-by-command)

Migrate commands in order of blast radius. Each command is one PR.

1. `search-specs` (smallest — pure search consumer)
2. `check-overlap` (search + simple analysis)
3. `check-doc-drift` (search + analysis + chat)
4. `check-terminology` (search + chat)
5. `analyze-impact` (search + relation traversal)
6. `check-compliance` (largest — search + chat + AST + relation traversal)
7. `compare-specs`, `explain-file`, `discover`, remaining commands

Each PR:
- Routes the command through `internal/catalog` and the relevant Hippocampus read surface (`Search`, `SearchLexical`, `SearchItems`, or outline APIs).
- Deletes the now-unreachable `internal/index/` and `internal/fusion/` code for that command's path.
- Updates internal tests to consume `hippocampus.CorpusHit` through Pituitary-owned adapters, while command-level JSON tests continue to assert Pituitary DTOs.
- Adds a CI guard that the migrated command's source file contains no `stroma` imports.
- Adds a parity test covering the command-specific search behavior listed in [Blocking Prerequisites](#blocking-prerequisites).

### Phase 5: Deletion

After every command has migrated:

1. Delete `internal/fusion/` (whole package).
2. Delete `internal/chunk/` (whole package — Hippocampus handles all chunking).
3. Shrink `internal/index/` to a thin Hippocampus adapter, or delete it entirely if `internal/catalog` absorbed it.
4. Keep Pituitary's `runtime.embedder`, `runtime.analysis`, `runtime.search`, and `runtime.chunking` config blocks as Pituitary operator contracts. They now render into Hippocampus options instead of Stroma options. `runtime.quantization` is already removed by Phase 1 and does not return as a shim.
5. Pituitary now transitively depends on stroma v4 via Hippocampus. Remove the direct `stroma/v3` line from `go.mod`.
6. Add CI guard: `go list -m -json all | jq` confirms no non-indirect `github.com/dusk-network/stroma/*` module dependency remains.

### Phase 6: Purification (deferred to a separate RFC)

Out of scope for this RFC. Triggered after Phase 5 completes. Audits remaining Pituitary scope for non-governance concerns: code-compliance via AST inspection (`internal/codeinfer/`, `internal/ast/`, `extensions/astinfer/`), CLI commands that don't map to consistency questions, etc.

## Guardrails

- **Substrate extraction, not semantic abdication.** Pituitary must not hand off claims, obligations, requirements, evidence sufficiency, contradiction semantics, materiality, freshness interpretation, or impact analysis merely because those features need corpus access.
- **No data-shape breaking changes for end users in Phases 1-4.** Pituitary's `[[sources]]` config stays. Pituitary runtime config continues to load through a translation shim. Existing snapshots are not portable (rebuild required on upgrade); this is acceptable only if the generation contract keeps Pituitary governance metadata and the Hippocampus snapshot aligned.
- **CLI/MCP tool surface is preserved.** Internal handlers re-route to Hippocampus; Pituitary-owned request/response DTOs stay stable. Raw Hippocampus structs do not become the transport contract.
- **`pituitary status --check-runtime` keeps working.** It calls Hippocampus's typed availability errors instead of probing Stroma directly.
- **Governance reproducibility is upgraded, not regressed.** `hippocampus.CorpusHit.SnapshotFingerprint` becomes the canonical citation for "this finding was generated against that snapshot." Pituitary findings get a stable cite they don't reliably have today.
- **CI guard enforces the boundary.** After Phase 5, a CI check fails if any file in `cmd/` or `internal/` imports a `stroma` package.

## Risks

- **Hippocampus is in a private repo (nantobv).** This blocks Phase 2 unless resolved by the buildable-dependency prerequisite. Contributors need `GOPRIVATE=github.com/nantobv/*` configured only if the project explicitly accepts a private dependency path.
- **Upstream gap discovery during cutover.** A command-by-command migration may surface a need Hippocampus doesn't yet expose. Mitigation: Phase 2 audit identifies the largest expected gaps; Phase 4 reader PRs unblock by either (a) filing upstream and waiting, or (b) carrying a temporary `internal/catalog` shim that gets deleted when upstream lands.
- **Runtime config ownership.** This RFC keeps the main Pituitary runtime blocks (`embedder`, `analysis`, `search`, `chunking`) as Pituitary operator contracts. `runtime.quantization` is intentionally removed with no compatibility shim because it exposes a storage-layer choice above the Hippocampus boundary.
- **Stroma v3 → v4 implicit upgrade.** Hippocampus is on v4. Pituitary inherits the upgrade through Hippocampus. Confirm none of the changes in v4 affect retrieval semantics in ways that invalidate existing labeled-case benchmarks; rerun #344/#358 benches on the post-migration code as a regression check.
- **Stroma still appears in indirect deps.** Pituitary transitively depends on Stroma through Hippocampus. The CI guard checks for *direct* imports, not transitive deps. This is intentional — the goal is dependency direction, not zero coupling.
- **Two-store publish complexity.** The migration replaces one Pituitary-owned index artifact plus one Stroma snapshot pointer with a Pituitary DB, a Hippocampus catalog, and a Hippocampus-owned Stroma snapshot. Without the generation boundary above, read commands could observe mixed state. This is the highest-risk implementation area.
- **Artifact mirroring can become accidental authority.** Hippocampus requires files under `raw/`; Pituitary must treat that mirror as rebuildable cache, not as the source of truth. `file://` provenance and source/content hashes must continue to point back to the operator's workspace files.

## Alternatives Considered

### Alternative A: Keep direct Stroma coupling; just revert PR #402

Address the immediate over-scoping in #340 without touching the broader architecture. Future PRs continue to add `runtime.*` knobs to Pituitary's config as Stroma exposes new features.

**Rejected because:** the underlying problem is not only PR #402; it is that Pituitary owns vector-storage decisions at the Stroma layer. Reverting one symptom does not fix the systemic drift, and the next Stroma feature will reintroduce the same shape of work. PR #402 is the canonical case study, not the disease. The Phase 1 removal is still required because `runtime.quantization` should not remain a Pituitary contract.

### Alternative B: Fork Hippocampus into Pituitary

Copy Hippocampus's source into Pituitary's tree and maintain a hard fork.

**Rejected because:** loses upstream improvements (issues #22-#25 and any future Hippocampus capability), duplicates maintenance burden, contradicts the explicit separation-of-concerns design that Hippocampus's `AGENTS.md` codifies.

### Alternative C: Wait until Hippocampus "stabilizes"

Continue direct Stroma coupling until Hippocampus reaches some maturity threshold.

**Rejected because:** "stable" is never a defined threshold; the leakage compounds while waiting; Hippocampus is *already* stable enough for production indexing in its own consumers; and Pituitary's continued direct Stroma use blocks Hippocampus from making breaking API changes that would otherwise be cheap.

### Alternative D: Hybrid — migrate embedder/chat only, keep direct Stroma for index/search

Move just the easy clusters (embedder configuration, chat completion) and leave the index/search/snapshot plumbing on direct Stroma.

**Rejected because:** the largest deletions (and the largest architectural wins) are in clusters 5-7 (index lifecycle, search, fusion). A partial migration leaves the leaky abstraction at exactly the layer where it matters most, while still paying the cost of two dependency tracks.

## What This Does Not Change

- **Pituitary's CLI surface.** The commands stay. Their JSON output shapes stay. Operators do not see the migration except for an explicit rebuild requirement and the intentional removal of the unsupported `runtime.quantization` key.
- **Pituitary's MCP surface.** The MCP tools stay. Their request/response wire shapes stay.
- **Pituitary's governance analyses.** Overlap, drift, impact, compliance, terminology, freshness — every analysis stays in Pituitary because every analysis is governance, not retrieval.
- **Pituitary's relation-graph model.** Spec ↔ doc ↔ code relations are governance domain; they are not vector-database concerns.
- **Pituitary's source adapter contract.** The `sdk.Adapter` interface (RFC 0002) stays. Adapters produce `sdk.SpecRecord` / `sdk.DocRecord`. Translation to Hippocampus is internal to Pituitary.
- **Pituitary's CCD integration.** Lifecycle integration with CCD is independent of the retrieval layer.

## Open Questions

Resolved during Phase 1 investigation. "Confirmed" means no Hippocampus issue is required before Phase 2 for that point; "Upstream issue" means the dependent migration step remains blocked until the linked issue closes or Pituitary makes an explicit contract-change decision.

1. **Spec-bundle SourceKind — Confirmed.** Hippocampus treats `SourceKind` as operator-defined and routes chunk policy by `source_kind`. Pituitary can register `"spec_bundle"`, `"markdown_docs"`, and `"markdown_contract"` without upstream changes.

2. **`spec.toml` + `body.md` artifact shape — Confirmed.** Use one `SourceItem` per spec bundle, with `body.md` as the `primary-source` artifact. Store `spec.toml` fields and the `spec.toml` SHA in `SourceItem.Metadata` for change detection. Do not create a second metadata artifact unless a later provenance requirement proves it necessary.

3. **Compile flow placement — Confirmed.** `compile` stays in Pituitary as governance preprocessing. The compiled spec can be registered as a Hippocampus generated artifact through `RegisterGeneratedArtifact`, with Pituitary retaining the semantics of what "compiled" means.

4. **Pituitary-specific search filters — Confirmed.** Hippocampus `Kinds` and `Collections` cover coarse corpus narrowing. Pituitary keeps status, domain, source-adapter, repo, and inference-confidence interpretation as product-side metadata filters or collection mappings. This matches today's pattern where several filters are applied after retrieval.

5. **Snapshot fingerprint as governance citation — Confirmed.** Hippocampus `CorpusHit.SnapshotFingerprint` is the active snapshot content fingerprint, and `ChunkContentHash` catches same-corpus rebuilds that alter chunk layout. Pituitary can cite both in governance output and fail closed through Hippocampus hit expansion when a handle is stale.

6. **`pituitary status --check-runtime` migration — Confirmed.** Hippocampus exposes `OpenAIEmbedder.Unconfigured()`, `OpenAIChat.Unconfigured()`, and typed `ErrEmbedderUnavailable` / `ErrChatUnavailable` errors. Pituitary can keep a thin `internal/catalog` probe for the existing status command while classifying Hippocampus runtime failures through those sentinels.

7. **Bench infrastructure portability — Confirmed.** Precision@k, recall@10, and MRR can run through Hippocampus `Search` / `SearchLexical` for hybrid and lexical paths. `Index` returns an `IndexRun` whose metadata includes `snapshot_path`, counts, embedder details, content fingerprint, and quantization; benchmark code can stat `snapshot_path` for snapshot bytes.

8. **GOPRIVATE in CI and public releases — Upstream issue.** Tracked by Hippocampus [#15](https://github.com/nantobv/hippocampus/issues/15). No Pituitary PR may add `github.com/nantobv/hippocampus` to `go.mod` until there is a buildable dependency path for the intended release audience.

9. **Generation metadata shape — Confirmed.** Phase 2 must write these exact `pituitary.db.metadata` keys together during atomic generation publish: `active_generation_id`, `hippocampus_root`, `hippocampus_content_fingerprint`, `stroma_snapshot_fingerprint`, and `pituitary_content_fingerprint`. Reader cutover requires a mixed-generation test proving those keys all point at the same published generation.

10. **Hippocampus artifact mirror layout — Confirmed.** Stage raw artifacts under `.pituitary/generations/<active_generation_id>/hippocampus/raw/<source_kind>/<source_key_hash>/<artifact_role>/<artifact_rel_path>`. `source_key_hash` is a deterministic SHA-256 prefix over source adapter, repo ID, source ref, and canonical URI. Staging cleanup removes failed generation directories; successful publish preserves the active generation and leaves older-generation garbage collection as a later maintenance step.

11. **Search parity matrix — Partly confirmed, partly upstream-blocked.**

| Pituitary path | Retrieval class | Phase 4 migration answer |
| --- | --- | --- |
| `search-specs` default / MCP `search_specs` | Hybrid + historical rerank | Hippocampus `Search` plus Pituitary post-processing; blocked on Hippocampus [#17](https://github.com/nantobv/hippocampus/issues/17) for arm provenance / rerank parity. |
| `search-specs --lexical` | Lexical | Hippocampus `SearchLexical`; confirmed. |
| `search-specs --semantic-similarity` | Vector-only | Blocked on Hippocampus [#16](https://github.com/nantobv/hippocampus/issues/16) unless Pituitary explicitly removes or replaces the semantic-similarity contract before cutover. |
| `get_intent_outline` / `expand_intent_context` | Outline + hit expansion | Hippocampus `GetItemOutline`, `GetArtifactOutline`, and `ExpandHitContext`; confirmed, with Pituitary status/domain validation as post-processing. |
| repository-similarity analysis shortlist | Vector-only artifact shortlist | Blocked on Hippocampus [#16](https://github.com/nantobv/hippocampus/issues/16) unless the analysis is redesigned to avoid cosine-threshold semantics. |
| semantic terminology near-miss scan | Vector-only threshold scan | Blocked on Hippocampus [#16](https://github.com/nantobv/hippocampus/issues/16) unless Pituitary replaces the threshold contract. |
| retrieval benchmarks #344/#358 | Hybrid / lexical metrics | Hippocampus `Search`, `SearchLexical`, and `IndexRun.Metadata`; confirmed for current benchmark metrics. |
| item/document narrowing flows | Item-level search | Hippocampus `SearchItems`; confirmed. |

## Appendix: Phase 0 audit raw data

39 Pituitary files import `stroma/v3/*` directly, broken down as follows. The numbers in parens are import counts (a file using two subpackages is counted in both).

```
stroma/v3/index    (27): rebuild, update, reuse, snapshot, search, fusion,
                         analysis (overlap, drift, repository, terminology),
                         contextualizer test, chunk policy test, retrieval benches
stroma/v3/chunk    ( 8): chunk policy, contextualizer, markdown, parent_chain,
                         outline_context
stroma/v3/corpus   ( 6): source/stream, intent_context, contextualizer
stroma/v3/store    ( 4): config (quantization constants), index/store, index/update
stroma/v3/embed    ( 3): embedder, openai_embedder
stroma/v3/provider ( 1): runtimeerr
stroma/v3/chat     ( 1): analysis/openai_provider
```

Hippocampus public API surface that absorbs the above:

```
Lifecycle:     Bootstrap, Open, OpenOrBootstrap, Close
Options:       WithEmbedder, WithIndexConfig
Catalog write: Register, MakeUniqueSlug, FindBySourceKey,
               RegisterSourceRevision, RegisterGeneratedArtifact, AddRelation
Catalog read:  GetItem, ListItems, ListRelations, ListSourceRevisions
Index:         Index (incremental or full-rebuild internally)
Search:        Search (hybrid RRF), SearchLexical, SearchItems
Outline:       GetItemOutline, GetArtifactOutline
Wrappers:      LoadOpenAIEmbedderFromEnv, NewOpenAIEmbedder,
               NewContextualOpenAIEmbedder, LoadOpenAIChatFromEnv
Config:        LoadIndexConfigFromEnv, IndexConfig (chunk policy, quantization)
Errors:        ErrIndexerUnavailable, ErrEmbedderUnavailable,
               ErrSnapshotUnavailable, ErrSnapshotQuarantined,
               ErrSlugCollision, ErrContentHashMismatch
```

Every cluster in the audit maps to one or more entries in that surface, with no cluster left unmappable. The migration is shape-feasible.
