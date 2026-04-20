# Retrieval precision benchmark — #358 (chunk-level)

Generated: 2026-04-20T11:59:41.777513Z

Cases file: `/Users/emanuele/devel/pituitary-358-bench/testdata/retrieval-bench/next-iteration-cases.json`

Case count: 27 (chunk-eligible: 27)

## Doc-level (for continuity with #344)

| variant | p@5   | p@10  | recall@10 | MRR   | snapshot bytes |
|---------|-------|-------|-----------|-------|----------------|
| pre338 | 0.215 | 0.107 | 0.963 | 0.856 | 4018176 |
| p338 | 0.215 | 0.107 | 0.963 | 0.854 | 3997696 |
| p344 | 0.215 | 0.107 | 0.963 | 0.856 | 4018176 |

## Chunk-level (#358 Arm A)

| variant | p@5   | p@10  | recall@10 | MRR   |
|---------|-------|-------|-----------|-------|
| pre338 | 0.163 | 0.096 | 0.821 | 0.471 |
| p338 | 0.163 | 0.096 | 0.852 | 0.499 |
| p344 | 0.163 | 0.096 | 0.821 | 0.471 |

## Arm B (parent-inclusion, LLM-graded RAG)

_Reserved — closes on the upstream ExpandContext issue; not measured here._
