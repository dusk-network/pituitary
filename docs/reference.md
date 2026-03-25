# Pituitary Reference

Detailed user documentation is split by topic so the landing page can stay short without losing the durable product manual.

For quick command lookup, start with the [cheatsheet](cheatsheet.md).

## Reference Map

| Topic | Document |
|---|---|
| Install paths and quickstart modes | [docs/install.md](install.md) |
| Spec format, workspace config, sources, selectors, artifacts | [docs/configuration.md](configuration.md) |
| Runtime setup, indexing, output formats, MCP server | [docs/runtime.md](runtime.md) |
| Compliance, reports, CI, command reference | [docs/workflows.md](workflows.md) |

## Fastest Routes

- New user evaluating Pituitary on an existing repo:
  - [docs/install.md](install.md)
  - [docs/cheatsheet.md](cheatsheet.md)
- Repo owner wiring Pituitary into CI:
  - [docs/workflows.md](workflows.md)
  - [docs/development/ci-recipes.md](development/ci-recipes.md)
- Contributor changing config or source behavior:
  - [docs/configuration.md](configuration.md)
  - [ARCHITECTURE.md](../ARCHITECTURE.md)
- User tuning local retrieval quality:
  - [docs/runtime.md](runtime.md)

## Product Boundaries

Pituitary is specification-first. The core workflow is local filesystem input, deterministic indexing, and CLI-first analysis. Optional runtime-backed retrieval and MCP integration sit on top of that core rather than replacing it.

See:

- [ARCHITECTURE.md](../ARCHITECTURE.md)
- [RFC 0001: Spec-Centric Compliance Direction](rfcs/0001-spec-centric-compliance-direction.md)
