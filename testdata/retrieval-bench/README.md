# Retrieval precision bench — #344

Ground-truth labels and runner scaffolding for the precision@k benchmark that
gates #344 (default docs to `LateChunkPolicy`).

## Files

- `ccd-guide-cases.json` — hand-curated query → relevant doc refs.
- `base-config.example.toml` — portable template for the base pituitary config
  the runner consumes. Copy, edit `[workspace].root` and `[runtime.embedder].endpoint`
  for your environment.
- `chunking-overlays/*.toml` — three chunking config overlays appended to the
  base config by `scripts/bench-precision-344.sh`:
  - `pre338.toml` — empty; pre-#338 baseline (no router, stroma `MarkdownPolicy` default).
  - `p338.toml` — router active, both kinds on `MarkdownPolicy`.
  - `p344.toml` — spec `MarkdownPolicy` + doc `LateChunkPolicy` with tuned P/C tokens.

## Running

Requires a reachable OpenAI-compatible embedder.

```
cp testdata/retrieval-bench/base-config.example.toml /tmp/pituitary-bench-base.toml
# edit [workspace].root, [workspace].index_path, [runtime.embedder].endpoint
BENCH_BASE_CONFIG=/tmp/pituitary-bench-base.toml \
  scripts/bench-precision-344.sh
```

Outputs three JSON reports under `/tmp/pituitary-bench-344/` (override with
`BENCH_OUT_DIR`) and a consolidated markdown summary at
`docs/development/retrieval-precision-344.md` (override with `BENCH_REPORT_MD`).
