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
- **Broader discovery defaults** — auto-detect CLAUDE.md, AGENTS.md, ARCHITECTURE.md as indexable sources

## Next

Enabling architecture and the highest-impact source expansion:

- **Kernel/extension adapter architecture** — separate Pituitary's core from external source adapters using Go's registry pattern. The kernel stays pure (local filesystem, SQLite, deterministic, no vendor dependencies). Extensions are separate Go packages that compile into the same single binary. See [RFC 0002](docs/rfcs/0002-kernel-extension-adapter-architecture.md).
- **GitHub issues as sources** — first extension adapter. Teams use issues to propose and modify specs (BIP/EIP pattern). This shifts Pituitary from end-of-workflow (CI catches drift at merge) to beginning-of-workflow (catch contradictions when an issue is created).
- **`pituitary new`** — scaffold a spec bundle from a template. Bridges the gap from loose markdown to structured specs.
- **GitHub Action** — run `review-spec` on PRs that touch specs, post results as a comment. Makes Pituitary invisible infrastructure.

## Soon After

Broadening source coverage and deepening governance workflows:

- **JSON config indexing** — extension adapter for structured JSON files as intent artifacts. Agents increasingly persist structured output that becomes de facto source of truth.
- **Stronger spec review workflows** — richer comparison output, better evidence presentation
- **CI recipe polish** — tested GitHub Actions workflow, pre-commit hook example
- **Clearer onboarding** — better error messages, progress indicators during index rebuilds

## Later

Extending coverage to the full problem space:

- **PDF ingestion** — extension adapter for PDF decision records
- **Non-filesystem adapters** — Notion, Confluence, GitLab, Jira (each as an extension adapter)
- **Shell completion** — `pituitary completion bash/zsh/fish`

## Not on the roadmap

Pituitary is spec-centric. These are out of scope:

- General-purpose static analysis or code linting
- Broad code authority outside explicit spec governance
- Autonomous agent behavior (Pituitary is a tool agents use, not an agent itself)
- Spec generation or authoring

See [RFC 0001](docs/rfcs/0001-spec-centric-compliance-direction.md) for the full rationale.
