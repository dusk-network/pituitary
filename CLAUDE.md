<!-- GENERATED: source=AGENTS.md format-version=1 sha256=a57e5972882e4d713f35cce0665c22e1cb39f6517382fb4ddfac5e6fc1ebf819 -->

> Generated from `AGENTS.md`. Edit `AGENTS.md`, then rerun `scripts/sync-ai-docs.sh --write`.
> If this file conflicts with `AGENTS.md`, `AGENTS.md` wins.

# AGENTS.md

> Canonical AI policy for this repo.
> If model-specific mirrors exist, generate them from this file.

## Project

Pituitary is a spec-management tool for keeping specifications and documentation aligned during the first shipping slice. The current repo is an early bootstrap focused on local filesystem inputs, Markdown docs, SQLite-backed indexing, and a CLI-first interface.

## Coding Standards

- Read [README.md](README.md), [ARCHITECTURE.md](ARCHITECTURE.md), and [IMPLEMENTATION_BACKLOG.md](IMPLEMENTATION_BACKLOG.md) before deep implementation work.
- Treat repository docs as the primary source of truth; keep GitHub issues aligned with them rather than the reverse.
- Prefer small, reversible changes and deterministic tooling over hand-maintained generated state.
- Keep outputs machine-readable where the architecture expects JSON-first behavior.

## Architecture Rules

- First ship is CLI-only. Do not add or depend on MCP in the v1 path.
- Scope is local filesystem only: `spec.toml` + `body.md` bundles, Markdown docs, SQLite, and `sqlite-vec`.
- Spec and doc analysis come before code-compliance features.
- Canonical contract decisions live in `ARCHITECTURE.md`; update docs and backlog together when those contracts change.

## Workflow

- Keep GitHub issues aligned with the repo docs and in priority order.
- When planning or implementing, name the files you expect to touch and why.
- Run `git status` and `git diff --minimal` before any commit.
- Stage explicit paths. Never use `git add .`.
- Show diffs and wait for approval before committing or pushing.

## Testing

- Run the smallest check that proves the change.
- Prefer `make fmt`, `make test`, `make vet`, and `make ci` where applicable.
- If you skip validation, state the reason explicitly.

## Safety

- No secrets in tracked files, docs, or fixtures.
- Stay within the active backlog item and call out scope drift when it appears.
- Do not overwrite user changes or reset the worktree without explicit approval.
