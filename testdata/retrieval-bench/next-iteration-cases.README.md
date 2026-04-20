# Next-iteration public smoke case set — curation notes

This file documents the curation procedure for
`next-iteration-cases.json`. The JSON itself stays pure (no `_comment`
field) so `loadPrecisionCases` in
`internal/index/retrieval_precision_bench_test.go` accepts it without
validator changes.

## Corpus

Cases are labelled against the two pinned repos in
`next-iteration-corpus.toml`:

- `/Users/emanuele/devel/ccd-guide` at HEAD `bac0cd6`
- `/Users/emanuele/devel/pituitary-358-bench` itself (this repo's `docs/`
  tree)

## Procedure

1. Start from the 20 existing doc-level cases in
   `ccd-guide-cases.json`. For each case, open every
   `relevant_doc_ref` source file, read the section that actually
   answers the query, and pick a 5-15 word anchor phrase from that
   passage.
2. Add 5+ new cases against the Pituitary `docs/` tree (7 added in
   this pass, spanning `docs/development/`, `docs/runbooks/`, and the
   `retrieval-precision-344` report itself).
3. Verify each anchor appears exactly once across the pinned corpus
   via `rg --fixed-strings --count-matches`. When an anchor collided
   (same sentence quoted in both `05-memory-architecture.md` and
   `08-patterns.md`, or the `07-guardrails.md` line echoed in an
   archived RFC), we tightened the phrase until it was unique.
4. Record a provenance `start_line` / `end_line` pair on every span.
   These fields are informational only — scoring resolves anchors by
   substring match inside the chunk body, not by line number.

## Mid-body tag

A case carries `"tags": ["mid_body"]` when its anchor is NOT in any of
the following positions of the answering doc:

- the single top-level `# Title` heading
- any TL;DR / summary block at the top of the doc
- the first body paragraph after the top-level heading

The intent is the stress case `retrieval-precision-344.md` names
explicitly: questions whose answer lives in mid-doc body text far from
any heading, where chunk boundaries actually shift which fragment
ranks first.

Current counts (verify with `jq '[.[] | select((.tags // []) |
index("mid_body"))] | length'`): 21 mid-body cases out of 27 total.

## Private corpus

This file is the public smoke set only. A parallel private case set
lives outside the repo (see Task 12) and is referenced by configured
path, never committed.
