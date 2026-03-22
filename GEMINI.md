<!-- GENERATED: source=AGENTS.md format-version=1 sha256=43cb2256fd95fcc94773e58c4068857453e6c352edba3a431ac1348b413dc8da -->

> Generated from `AGENTS.md`. Edit `AGENTS.md`, then rerun `scripts/sync-ai-docs.sh --write`.
> If this file conflicts with `AGENTS.md`, `AGENTS.md` wins.
> Gemini-specific refresh behavior belongs in tooling, not in a second policy file.

# AGENTS.md

> Canonical AI policy for this repo.
> If model-specific mirrors exist, generate them from this file.

## Project

Pituitary is a spec-management tool for keeping specifications and documentation aligned during the first shipping slice. The current repo is an early bootstrap focused on local filesystem inputs, Markdown docs, SQLite-backed indexing, a CLI-first interface, an optional MCP wrapper, and repo CI validation.

## Coding Standards

- Read [README.md](README.md) and [ARCHITECTURE.md](ARCHITECTURE.md) before deep implementation work. Use the active GitHub issues for backlog and priority context.
- Treat repository docs as the primary source of truth; keep GitHub issues aligned with them rather than the reverse.
- Prefer small, reversible changes and deterministic tooling over hand-maintained generated state.
- Keep outputs machine-readable where the architecture expects JSON-first behavior.

## Architecture Rules

- First ship is CLI-first. Keep the CLI as the required transport and do not make core behavior depend on MCP, even though the repo also ships an optional MCP wrapper.
- Scope is local filesystem only: `spec.toml` + `body.md` bundles, Markdown docs, SQLite, and `sqlite-vec`.
- The repo ships CI for fmt, readiness, test, and vet validation, but GitHub- or vendor-specific reporting integrations remain out of scope.
- Spec and doc analysis come before code-compliance features.
- Canonical contract decisions live in `ARCHITECTURE.md`; update repo docs and GitHub issues together when those contracts change.

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
- Stay within the active issue or approved work item and call out scope drift when it appears.
- Do not overwrite user changes or reset the worktree without explicit approval.
