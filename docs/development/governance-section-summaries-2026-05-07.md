# Governance Section Summaries - Evaluation Contract

This note closes the #387 design question: Pituitary may evaluate generated summaries as retrieval aids for long governance records, but summaries are product-specific analysis state, not Stroma index content and not authoritative evidence.

## Decision

Do not add generated summaries to the default retrieval path yet. The accepted contract is an opt-in evaluation path whose output can be compared against plain outline-guided retrieval before any default behavior changes.

The first supported use case is governance analysis that already uses outline-guided retrieval: candidate search, bounded outline inspection, deterministic section selection, and context expansion from original source chunks. Summaries may influence which sections are shortlisted, but final evidence must remain the original section text returned from the current Stroma snapshot.

## Storage Boundary

Persist summaries in Pituitary-owned state, not in Stroma snapshots and not in chunk text. Stroma remains responsible for chunk identity, content hashes, parent lineage, and snapshot fingerprints. Pituitary may keep derived summary rows keyed by the Stroma identity they describe.

Proposed state shape:

| field | purpose |
|---|---|
| `snapshot_fingerprint` | Stroma snapshot the summary was generated against |
| `record_kind` | `spec` or `doc` |
| `record_ref` | canonical Pituitary record ref |
| `source_ref` | source document ref used in user-facing reports |
| `chunk_id` | Stroma chunk ID summarized |
| `source_content_hash` | Stroma-normalized source content hash for invalidation |
| `summary_profile_id` | named summary profile chosen by the caller |
| `summary_config_hash` | hash of prompt/schema/settings that affect output |
| `model_id` | configured analysis model identifier |
| `prompt_schema_version` | Pituitary prompt/output schema version |
| `generated_at` | generation timestamp for diagnostics |
| `summary_text` | concise retrieval aid text |
| `summary_json` | optional structured diagnostics, never authoritative evidence |
| `invalidated_at` | optional tombstone for explicit stale-state cleanup |

The active lookup key is `(snapshot_fingerprint, record_kind, record_ref, chunk_id, summary_profile_id)`. Historical rows may remain for diagnostics, but lookup for current retrieval must reject any row whose snapshot or configuration does not match the active request.

## Invalidation

A summary is usable only when all of these match the active retrieval request:

- `snapshot_fingerprint`
- `source_content_hash`
- `summary_profile_id`
- `summary_config_hash`
- `model_id`
- `prompt_schema_version`

Any mismatch makes the row stale. A stale summary can be ignored silently for selection, but JSON diagnostics should expose a neutral count such as `stale_summary_count` when summaries were requested.

`index --rebuild` and `index --update` must not generate summaries implicitly. If a future summary-generation command exists, it should be explicit and repeatable over the current snapshot. `index --update` may preserve historical rows, but active lookup must require the current snapshot fingerprint so no summary from an older snapshot can influence current section selection.

## Opt-In Generation

Summary generation requires an analysis runtime and an explicit caller choice. No default model credentials are introduced.

A future command or flag should require:

- a named summary profile;
- an active `runtime.analysis` configuration;
- a bounded source scope such as record refs, source refs, or selected chunk IDs;
- a deterministic write policy: identical input identity plus identical summary config updates the same logical row, while changed content or config produces stale historical state rather than mutating evidence semantics.

Generation failure must not make the underlying governance command fail unless the user explicitly requested a summary-only operation. Without valid summaries, commands fall back to the existing outline-guided retrieval path.

## Influence Reporting

When summaries affect section selection, governance commands must report that fact. A future JSON envelope can use a shape like:

```json
{
  "selection_aids": [
    {
      "type": "governance_section_summary",
      "profile": "default-governance-v1",
      "chunk_id": "doc:guide.md#3",
      "influence": "shortlisted",
      "snapshot_fingerprint": "..."
    }
  ]
}
```

Permitted influence values should distinguish at least `shortlisted`, `selected`, and `ignored`. Text and markdown renderers can summarize this as "summary-aided section selection" without printing model-derived text as evidence.

The evidence contract is unchanged: findings cite original source refs, chunk IDs, headings, source spans, and expanded source content. A generated summary can explain why a section was considered; it cannot establish drift, compliance, overlap, or freshness on its own.

## Ablation Gate

Summaries stay non-default until an ablation report compares:

1. outline-guided retrieval without summaries;
2. outline-guided retrieval with summaries enabled for the same summary profile;
3. the same corpus, Stroma snapshot, retrieval query set, embedder, analysis runtime, and cost ceiling.

The report must include:

- command or workflow being optimized;
- quality metric and distribution, not only the mean;
- selection changes caused by summaries;
- false-positive and false-negative examples;
- retrieval latency;
- model calls;
- prompt and completion token counts where available;
- estimated cost, or neutral token/call counts when pricing is unknown;
- context bytes or tokens passed into the downstream analysis step.

The initial actionability ceiling is `review-spec --outline-context` on local governance corpora: summary-aided selection is production-positive only if it improves downstream review quality without adding default credentials, without increasing p95 retrieval latency beyond the command's interactive budget, and without making original-source evidence harder to audit.

A null or negative result means summaries remain a research-only aid. It does not change the Stroma contract, and it does not change the default outline-guided retrieval path.

## Complexity Guardrails

Summary evaluation must reduce PageIndex-related complexity, not add a second retrieval system.

- Keep the Stroma/Pituitary boundary explicit: summaries live beside Pituitary state and refer to snapshot identities; they do not alter Stroma chunk text or generic index contracts.
- Keep the selector surface narrow: summary scores or snippets should enter the existing outline selection phase as optional side input, not as a parallel tree-search engine.
- Keep zero-credential workflows untouched: indexing, search, outline retrieval, MCP context expansion, and deterministic governance commands continue to work without analysis runtime credentials.
- Keep generated text quarantined: summary text is untrusted, model-derived diagnostic material and must not be treated as source evidence.
- Keep reporting additive: influence metadata can be omitted when summaries are disabled, so existing JSON consumers continue to parse the deterministic result.

This keeps the upcoming PageIndex work focused on bounded context selection and measurable retrieval quality rather than a broad summarization subsystem.
