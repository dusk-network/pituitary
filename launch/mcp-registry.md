# MCP Registry Listing

**Name:** Pituitary

**Short description:** Intent governance for the AI-natives — detect overlap, stale docs, code contradictions, terminology drift, and impact across your specs, docs, and decision records. Indexes the full corpus so your agent stops guessing and builds against what you actually decided.

**Category:** Developer Tools / Documentation

**Transport:** stdio

**Command:** `pituitary serve --config .pituitary/pituitary.toml`

## Tools

| Tool | Description |
|------|-------------|
| `search_specs` | Semantic search across indexed spec sections. Filter by domain and status. |
| `check_overlap` | Detect specs that cover overlapping ground. Returns similarity scores and relationship classification. |
| `compare_specs` | Side-by-side tradeoff analysis of two specs. |
| `analyze_impact` | Trace which specs, code refs, and docs are affected when a spec changes. |
| `check_doc_drift` | Find docs that have gone stale relative to accepted specs, with cited evidence. |
| `review_spec` | Full composite review: overlap + comparison + impact + drift + remediation. |
| `check_compliance` | Check a PR diff against accepted specs for contradictions. |
| `check_terminology` | Audit terminology against declared policies. Separates actionable violations from tolerated historical uses. |
| `governed_by` | Look up which specs govern a given file or path. |
| `compile_preview` | Preview context-aware terminology patches before applying. |
| `fix_preview` | Preview deterministic auto-fix edits before applying. |
| `status` | Index health at a glance: artifact counts, runtime profile, staleness. |
| `explain_file` | Explain a file's role in the spec/doc corpus and its governance relationships. |

## Requirements

- Pituitary binary on PATH ([releases](https://github.com/dusk-network/pituitary/releases))
- A config file (generate one with `pituitary init --path .`, writes `.pituitary/pituitary.toml`)
- A built index (`pituitary index --rebuild`)

No API keys required in deterministic mode. Optional local embedding server (e.g., LM Studio + nomic-embed-text) for improved retrieval quality on larger corpora.

## Example config

```json
{
  "mcpServers": {
    "pituitary": {
      "command": "pituitary",
      "args": ["serve", "--config", ".pituitary/pituitary.toml"]
    }
  }
}
```

## Use cases

- **PR review:** Agent calls `check_overlap` and `analyze_impact` when a PR touches specs or decision records
- **Pre-merge drift check:** Agent calls `check_doc_drift` to surface stale docs before merging
- **Decision authoring:** Agent calls `review_spec` to get a full assessment of a new or changed spec
- **Impact questions:** "What specs and docs would be affected if I change the auth middleware?" triggers `analyze_impact`

## Links

- GitHub: https://github.com/dusk-network/pituitary
- Releases: https://github.com/dusk-network/pituitary/releases
