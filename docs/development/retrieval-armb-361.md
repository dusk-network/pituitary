# Retrieval Arm B benchmark - #361

Generated: 2026-05-07T15:44:58.842238Z

Cases file: `testdata/retrieval-bench/armb-rag-cases.json`

Case count: 21 (mid-body: 21)

Corpus doc count: 216

Analysis model: `gpt-4o-mini`

Actionability ceiling: review-spec --outline-context: production-positive only when quality improves without default credentials and p95 retrieval latency stays within an interactive budget

Corpus: detached pinned worktrees for `ccd-guide` at `bac0cd6` and Pituitary docs at `b946548`. The run indexed each repo's `docs/` tree plus supplemental Markdown from specs, skills, repo policy, dogfood, and testdata so the retrieval corpus exceeded the 192-doc floor while preserving the original case doc refs.

## Headline

| variant | leaf mean | parent mean | delta | leaf median | parent median | parent p95 retrieval ms | snapshot bytes |
|---------|-----------|-------------|-------|-------------|---------------|-------------------------|----------------|
| pre338 | 2.67 | 2.71 | +0.05 | 3.0 | 3.0 | 1491.89 | 36405248 |
| p338 | 2.67 | 2.71 | +0.05 | 3.0 | 3.0 | 1483.66 | 35610624 |
| p344 | 2.57 | 2.71 | +0.14 | 3.0 | 3.0 | 1466.12 | 36405248 |

## Quality Distribution

| variant | arm | distribution (score:count) | errored cases |
|---------|-----|----------------------------|---------------|
| pre338 | leaf_only | 0:0 1:5 2:5 3:5 4:4 5:2 | 0 |
| pre338 | leaf_plus_parent | 0:0 1:5 2:4 3:6 4:4 5:2 | 0 |
| p338 | leaf_only | 0:0 1:5 2:5 3:5 4:4 5:2 | 0 |
| p338 | leaf_plus_parent | 0:0 1:5 2:5 3:4 4:5 5:2 | 0 |
| p344 | leaf_only | 0:0 1:5 2:5 3:7 4:2 5:2 | 0 |
| p344 | leaf_plus_parent | 0:0 1:5 2:5 3:4 4:5 5:2 | 0 |

## Cost And Latency Envelope

Model pricing is not configured in this harness; prompt/completion token counts are the neutral cost envelope.

| variant | arm | retrieval searches | expansion calls | generator calls | grader calls | prompt tokens | completion tokens | context bytes | context token estimate |
|---------|-----|--------------------|-----------------|-----------------|--------------|---------------|-------------------|---------------|------------------------|
| pre338 | leaf_only | 21 | 105 | 21 | 21 | 21208 | 2627 | 69862 | 17463 |
| pre338 | leaf_plus_parent | 21 | 105 | 21 | 21 | 22692 | 2567 | 75776 | 18944 |
| p338 | leaf_only | 21 | 105 | 21 | 21 | 21048 | 2620 | 69117 | 17278 |
| p338 | leaf_plus_parent | 21 | 105 | 21 | 21 | 21051 | 2617 | 69117 | 17278 |
| p344 | leaf_only | 21 | 105 | 21 | 21 | 21184 | 2516 | 69862 | 17463 |
| p344 | leaf_plus_parent | 21 | 105 | 21 | 21 | 22691 | 2565 | 75776 | 18944 |

## Interpretation

`p344` parent inclusion improved mean LLM-graded score by 0.14, with unchanged median score. This is research-positive for the parent-lineage story, but too small to justify a broad default-quality claim by itself; keep parent inclusion as the explicit outline-context path and use the token envelope above when judging opt-in workflows.
