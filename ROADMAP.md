# Roadmap

What's shipped, what's next, and where Pituitary is headed.

## Shipped

The core analysis surface is functional end-to-end:

- **Spec indexing** — `spec.toml` + `body.md` bundles, markdown docs, inferred contracts
- **Overlap detection** — catch decisions that cover the same ground
- **Doc drift** — find docs that contradict accepted specs, with deterministic auto-fix
- **Code compliance** — check diffs against accepted spec requirements before merge
- **Impact analysis** — trace which specs, docs, and code paths are affected by a change
- **Terminology audit** — find displaced terms after conceptual migrations
- **Composite review** — `review-spec` runs overlap + comparison + impact + drift in one pass
- **MCP server** — 6 spec-awareness tools for Claude Code, Cursor, Windsurf
- **Multi-format output** — JSON, Markdown, HTML reports with evidence chains
- **One-shot onboarding** — `pituitary init` discovers sources, writes config, builds index
- **Deterministic mode** — no API keys required; fixture embedder for reproducible CI

## Next

Near-term work driven by the positioning direction (spec-centric, intent-focused):

- **Broader discovery defaults** — auto-detect CLAUDE.md, AGENTS.md, ARCHITECTURE.md as indexable sources
- **Stronger spec review workflows** — richer comparison output, better evidence presentation
- **Clearer onboarding** — better error messages, progress indicators during index rebuilds
- **CI recipe polish** — tested GitHub Actions workflow, pre-commit hook example

## Later

Extending coverage to match the full problem space:

- **PDF ingestion** — index PDF decision records alongside markdown
- **JSON config indexing** — treat structured config files as intent artifacts
- **GitHub issues as sources** — index issue bodies when they carry decision context
- **Non-filesystem adapters** — Notion, Confluence, GitHub repo sources
- **`pituitary new`** — scaffold a spec bundle from a template
- **GitHub Action** — run `review-spec` on PRs that touch specs, post results as a comment
- **Shell completion** — `pituitary completion bash/zsh/fish`

## Not on the roadmap

Pituitary is spec-centric. These are out of scope:

- General-purpose static analysis or code linting
- Broad code authority outside explicit spec governance
- Autonomous agent behavior (Pituitary is a tool agents use, not an agent itself)
- Spec generation or authoring

See [RFC 0001](docs/rfcs/0001-spec-centric-compliance-direction.md) for the full rationale.
