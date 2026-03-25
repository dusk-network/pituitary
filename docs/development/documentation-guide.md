# Documentation Guide

This repo keeps product documentation split by reader intent so the landing page stays short without losing durable detail.

## Documentation Layout

- [README.md](../../README.md): landing page for new users; install, quick start, value proposition, editor and CI entry points
- [docs/cheatsheet.md](../cheatsheet.md): task-oriented command map for day-to-day use
- [docs/reference.md](../reference.md): entrypoint into the user manual
- [docs/install.md](../install.md): installation and quickstart variants
- [docs/configuration.md](../configuration.md): spec format, workspace config, source kinds, selectors, artifact locations
- [docs/runtime.md](../runtime.md): runtime setup, indexing behavior, output formats, MCP server
- [docs/workflows.md](../workflows.md): compliance, reports, CI, and command reference
- [ARCHITECTURE.md](../../ARCHITECTURE.md): product contracts and system-level decisions
- `docs/development/`: contributor-facing process and implementation guides
- `docs/rfcs/`: product-direction and contract decisions that need durable rationale

## Update Rules

When you change the product surface, update docs by intent:

- New user-facing command or flag:
  - update [docs/cheatsheet.md](../cheatsheet.md)
  - update the relevant topic doc under `docs/`
- New install path:
  - update [README.md](../../README.md)
  - update [docs/install.md](../install.md)
- New config or source behavior:
  - update [docs/configuration.md](../configuration.md)
- New runtime or output format behavior:
  - update [docs/runtime.md](../runtime.md)
- New workflow or report behavior:
  - update [docs/workflows.md](../workflows.md)
- Contract or product-boundary changes:
  - update [ARCHITECTURE.md](../../ARCHITECTURE.md)
  - update or add an RFC when rationale matters long-term

Do not push detailed procedural material back into the landing-page README unless it materially improves the first five minutes.

## Link and Structure Guardrails

- Every new local doc link should land in the same PR as its target.
- Keep stable paths where possible; prefer adding a new topic doc over moving existing entrypoints.
- Run `make docs-check` before shipping documentation changes.
- CI runs the same docs link check for `README.md` and `docs/**/*.md`.

## Documentation Review Checklist

Before merging doc-affecting work, check:

1. the README still reads like a landing page rather than a manual
2. the cheatsheet still supports task-oriented command lookup
3. the detailed material lives in topic docs, not scattered across README and development guides
4. all local links resolve
5. command examples match the current CLI surface
