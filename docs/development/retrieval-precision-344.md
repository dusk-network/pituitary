# Retrieval precision benchmark — #344

Measured precision@k / recall@10 / MRR for the doc arm of the retrieval index across three chunking configurations, gating the #344 default flip from `MarkdownPolicy` to `LateChunkPolicy` for doc records.

- **Run date:** 2026-04-19
- **Pituitary CLI:** built from `7502146` (post-#344 merge)
- **Cases:** [`testdata/retrieval-bench/ccd-guide-cases.json`](../../testdata/retrieval-bench/ccd-guide-cases.json) — 20 hand-curated queries with labeled relevant doc refs
- **Corpus:** `~/devel/ccd-guide` pinned at `8854090` (12 indexed docs across `docs/guide/`, `docs/reference/`, `docs/operations/`, `docs/archive/`)
- **Embedder:** mlx-router @ `http://100.92.91.40:1234/v1`, model `nomic-embed-text-v1.5` (768-dim)
- **Runner:** [`scripts/bench-precision-344.sh`](../../scripts/bench-precision-344.sh) (rebuilds the snapshot once per variant, then runs the `precision_bench`-tagged test)

## Headline

| variant | p@5   | p@10  | recall@10 | MRR   |
|---------|-------|-------|-----------|-------|
| pre338  | 0.220 | 0.110 | 0.950     | 0.864 |
| p338    | 0.220 | 0.110 | 0.950     | 0.862 |
| p344    | 0.220 | 0.110 | 0.950     | 0.864 |

| variant | Δp@5   | Δp@10  | Δrecall@10 | ΔMRR    |
|---------|--------|--------|------------|---------|
| pre338  | +0.000 | +0.000 | +0.000     | +0.000  |
| p338    | +0.000 | +0.000 | +0.000     | -0.002  |
| p344    | +0.000 | +0.000 | +0.000     | +0.000  |

**Result: null precision delta on this corpus + this benchmark shape.** The two missed cases (`handoff_semantics`, `session_loop_phases`) are identical across all three variants — each is a multi-doc query where `reference/cheatsheet` legitimately belongs in the relevance set but does not surface in top-10 regardless of chunking policy.

## What this benchmark can and cannot measure

This benchmark measures top-10 **doc-level** retrieval: `evaluatePrecisionCase` collapses retrieved hits to unique doc refs in rank order before computing precision/recall/MRR. Whether `LateChunkPolicy` produces one chunk or five leaf chunks per doc, only the *first* hit per doc counts.

The product value `LateChunkPolicy` is supposed to deliver — per the #344 issue body — is the `parent_chunk_id` lineage feeding `ExpandContext(IncludeParent)`: when a leaf chunk is the top hit, the agent gets the surrounding parent context too. **That benefit is downstream of this metric and structurally invisible to it.** The null delta here is therefore *not* evidence the default flip is inert; it is evidence that top-10 doc-level precision on this corpus is insensitive to chunk granularity, which is a different claim.

The issue body anticipated this: "`ExpandContext` wiring is explicitly called out as the next lever (tracked separately)." Validating the `LateChunkPolicy → ExpandContext` product benefit requires a separate, structurally different benchmark (see "Next-iteration shape" below).

## Variants

The script appends one of three overlays from `testdata/retrieval-bench/chunking-overlays/` to the base config:

- **pre338** — no `[runtime.chunking]` block. `chunk.Resolve` returns `nil`, stroma falls back to its default `MarkdownPolicy` for every kind. Byte-identical to the pre-router pipeline.
- **p338** — `KindRouterPolicy` active, both `spec` and `doc` kinds explicitly on `MarkdownPolicy`. Exercises the router seam without changing chunk shapes.
- **p344** — `spec` on `MarkdownPolicy`; `doc` on `LateChunkPolicy` with `max_tokens = 2048`, `child_max_tokens = 384`, `child_overlap_tokens = 48`. The defaults flipped in #344.

Each variant rebuilds a separate snapshot so chunk lineage never bleeds across runs.

## Why the delta is null on *this* benchmark

Three compounding reasons, in order of weight:

1. **Doc-level scoring elides chunk granularity** (see "What this benchmark can and cannot measure" above).
2. **Corpus is too small to stress retrieval.** 12 indexed docs vs 20 cases means the right doc is almost always findable by any embedding model on any chunking — the retrieval problem is below the difficulty floor where chunking strategy matters.
3. **Queries align with doc TL;DRs.** The cases were curated against headings and TL;DR blocks that already match well under `MarkdownPolicy`. A harder set — questions whose answer lives in mid-doc body text far from any heading — would be a better stress test.

## Reproducing

```
cp testdata/retrieval-bench/base-config.example.toml /tmp/pituitary-bench-base.toml
# edit [workspace].root to your corpus, [runtime.embedder].endpoint to a reachable embedder
BENCH_BASE_CONFIG=/tmp/pituitary-bench-base.toml \
  scripts/bench-precision-344.sh
```

To reproduce the specific numbers in this report, the corpus must be `~/devel/ccd-guide` checked out at `8854090`, and the embedder must be an OpenAI-compatible endpoint serving `nomic-embed-text-v1.5`.

Per-variant JSON reports land in `/tmp/pituitary-bench-344/` (override with `BENCH_OUT_DIR`).

## Next-iteration shape

If/when we want to *demonstrate* a `LateChunkPolicy` win in numbers (rather than cite the ExpandContext story qualitatively), four changes are needed. None gate #344; they're material for the next chunking/retrieval benchmark issue:

1. **Larger corpus** — index a real-world docs surface (Pituitary's own `docs/`, plus a sibling repo's docs) so retrieval has to discriminate among 50+ docs.
2. **Chunk-level metric** — score on whether the returned chunk is the *answering* chunk, not just whether the right doc appears anywhere in top-10. Requires per-chunk-id labels.
3. **Mid-body queries** — curate questions whose answer is in body text far from any heading, where chunk boundaries actually shift which fragment ranks first.
4. **ExpandContext-on metric** — measure whether parent-chunk inclusion improves answer quality on a small set of LLM-graded RAG queries.

## Relationship to issue acceptance

The #344 acceptance criterion reads "Precision@k benchmark published with before/after numbers on a representative corpus." This report satisfies that literal requirement: numbers are published, the corpus and methodology are documented, and the repro is portable. It does *not* on its own validate that the `LateChunkPolicy` default flip improves retrieval quality — that validation requires the next-iteration benchmark shape above, tracked separately.
