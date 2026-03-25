# MCP Registry Listing

**Name:** Pituitary

**Short description:** Catch spec drift before it catches you — detect overlap, stale docs, code contradictions, and impact across your specs, docs, and decision records.

**Category:** Developer Tools / Documentation

**Transport:** stdio

**Command:** `pituitary serve --config pituitary.toml`

## Tools

| Tool | Description |
|------|-------------|
| `search_specs` | Semantic search across indexed spec sections. Filter by domain and status. |
| `check_overlap` | Detect specs that cover overlapping ground. Returns similarity scores and relationship classification. |
| `compare_specs` | Side-by-side tradeoff analysis of two specs. |
| `analyze_impact` | Trace which specs, code refs, and docs are affected when a spec changes. |
| `check_doc_drift` | Find docs that have gone stale relative to accepted specs, with cited evidence. |
| `review_spec` | Full composite review: overlap + comparison + impact + drift + remediation. |

## Requirements

- Pituitary binary on PATH ([releases](https://github.com/dusk-network/pituitary/releases))
- A `pituitary.toml` config file (generate one with `pituitary init --path .`)
- A built index (`pituitary index --rebuild`)

No API keys required in deterministic mode. Optional local embedding server (e.g., LM Studio + nomic-embed-text) for improved retrieval quality on larger corpora.

## Example config

```json
{
  "mcpServers": {
    "pituitary": {
      "command": "pituitary",
      "args": ["serve", "--config", "/path/to/pituitary.toml"]
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
