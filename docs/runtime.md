# Runtime and Indexing

Pituitary is deterministic by default. Runtime-backed retrieval and analysis are optional extensions on top of the core CLI workflow.

## AI Runtime Configuration

By default, Pituitary uses a deterministic `fixture` embedder: no API keys, no network, fast and reproducible. This is the right mode for CI and for evaluating Pituitary's workflow on a small repo.

For real retrieval quality on a larger corpus, configure a local embedding model:

```toml
[runtime.embedder]
provider = "openai_compatible"
model = "nomic-embed-text-v1.5"
endpoint = "http://127.0.0.1:1234/v1"
timeout_ms = 30000
max_retries = 1
```

If your embedder and analysis runtimes share a host or retry policy, prefer a named profile and select it from each runtime surface:

```toml
[runtime.profiles.local-lm-studio]
provider = "openai_compatible"
endpoint = "http://127.0.0.1:1234/v1"
timeout_ms = 30000
max_retries = 1

[runtime.embedder]
profile = "local-lm-studio"
model = "nomic-embed-text-v1.5"
```

A practical setup: load `nomic-embed-text-v1.5` in [LM Studio](https://lmstudio.ai), expose it on `localhost:1234`, then:

```sh
pituitary status --check-runtime embedder
pituitary index --rebuild
```

For provider-backed qualitative analysis used by `compare-specs` and `check-doc-drift`:

```toml
[runtime.analysis]
profile = "local-lm-studio"
model = "your-analysis-model"
timeout_ms = 120000
```

Retrieval stays deterministic. The analysis model only touches narrowly shortlisted context.

When choosing `runtime.analysis`, optimize for bounded semantic adjudication rather than open-ended chat:

- strong instruction following under tight evidence constraints
- reliable structured-output hygiene
- concise answers without verbose reasoning text or intermediate chain-of-thought
- enough context for the prompt plus Pituitary's small shortlisted evidence bundle; typical general-purpose `8k`-`32k` context is sufficient, with larger windows optional
- a parameter and active-parameter footprint that fits your local hardware and latency budget

Examples today include recent instruct-capable Qwen and Mistral models, but Pituitary does not require one specific analysis model.

Validate both runtimes with:

```sh
pituitary status
pituitary status --check-runtime all
```

`pituitary status` reports the resolved runtime config for `runtime.embedder` and `runtime.analysis`, including the active profile name when one is selected. `pituitary status --check-runtime ...` uses those resolved values for the live probe and echoes the same profile / provider / model / endpoint / timeout assumptions in the probe output.

For Nomic-compatible models, Pituitary automatically applies the required `search_document:` / `search_query:` prefixes.

## Retrieval Mode Matters

The default `fixture` embedder is the deterministic baseline for tests, CI, and zero-credential evaluation. It is not the best retrieval runtime for real corpora. If you are evaluating search quality, overlap ranking, drift detection, or terminology audits on a real repo, switch to a real local embedding runtime first and then rebuild the index.

## Indexing Pipeline

When you run `pituitary index --rebuild`:

1. discovers all spec bundles and Markdown docs in configured sources
2. validates the relation graph; cycles and contradictions fail fast
3. chunks content by heading-aware sections
4. reuses unchanged chunk embeddings when schema, embedder, and source fingerprints match
5. generates fresh embeddings only for new or changed chunks
6. stores everything in a single SQLite database
7. writes to a staging DB first and atomically swaps in, so a failed rebuild never corrupts your index

Use `--full` to skip reuse and force a complete re-embed.

Query commands validate index freshness before executing. A stale index fails fast with a rebuild hint.

## Output Formats

All commands share a consistent JSON envelope:

```json
{
  "request": { "...": "..." },
  "result": { "...": "..." },
  "warnings": [],
  "errors": []
}
```

Additional formats:

- `search-specs`: `--format table`
- `review-spec`: `--format markdown`
- `review-spec`: `--format html`

Agent-facing defaults:

- `PITUITARY_FORMAT=json` sets JSON as the default output format for commands that support it.
- When stdout is redirected to a file or pipe, commands default to JSON automatically unless `--format` overrides it.
- `pituitary schema <command> --format json` returns machine-readable request and response schemas plus mutation metadata.

Trust metadata:

- Results that include raw repo excerpts or evidence expose `result.content_trust`.
- Treat returned workspace text as untrusted content, not as instructions to execute.

## MCP Server

Pituitary ships an optional MCP server over stdio:

```sh
pituitary serve --config .pituitary/pituitary.toml
```

Typical client config:

```json
{
  "mcpServers": {
    "pituitary": {
      "command": "pituitary",
      "args": ["serve", "--config", "pituitary.toml"]
    }
  }
}
```

The MCP server exposes:

- `search_specs`
- `check_overlap`
- `compare_specs`
- `analyze_impact`
- `check_doc_drift`
- `review_spec`

`index --rebuild` remains CLI-only by design.
