# Retrieval precision bench — #344

Ground-truth labels and runner scaffolding for the precision@k benchmark that
gates #344 (default docs to `LateChunkPolicy`).

## Files

- `ccd-guide-cases.json` — hand-curated query → relevant doc refs.
- `chunking-overlays/*.toml` — three chunking config overlays appended to a
  base pituitary config by `scripts/bench-precision-344.sh`:
  - `pre338.toml` — empty; pre-#338 baseline (no router, stroma `MarkdownPolicy` default).
  - `p338.toml` — router active, both kinds on `MarkdownPolicy`.
  - `p344.toml` — spec `MarkdownPolicy` + doc `LateChunkPolicy` with tuned P/C tokens.

## Running

Requires a reachable embedder (default config assumes the m2-router nomic-embed
at `http://100.92.91.40:1234/v1`).

```
BENCH_BASE_CONFIG=/path/to/pituitary.toml \
  scripts/bench-precision-344.sh
```

Outputs three JSON reports under `/tmp/pituitary-bench-344/` and a consolidated
markdown summary at `docs/development/retrieval-precision-344.md`.
