# AGENTS.md

> Canonical AI policy for this repo.
> If model-specific mirrors exist, generate them from this file.

## Project

Pituitary is a consistency governance tool that keeps specifications, documentation, and code aligned. It builds a temporal, confidence-weighted governance graph from spec bundles and docs, then runs overlap, drift, compliance, impact, and freshness analysis against it. The current repo is focused on local filesystem inputs, Markdown docs, SQLite-backed indexing, a CLI-first interface, an optional MCP wrapper, and repo CI validation.

<!-- BEGIN nantobv-shared (sync via nantobv/.github/scripts/sync-agents-shared.sh) -->
## Org-wide policy (nantobv-shared)

The bullets in this section are the org-wide floor for AI agents working on
any nantobv project. They are vendored verbatim from
[`nantobv/.github/AGENTS.shared.md`](https://github.com/nantobv/.github/blob/main/AGENTS.shared.md);
refresh the local copy with
[`scripts/sync-agents-shared.sh`](https://github.com/nantobv/.github/blob/main/scripts/sync-agents-shared.sh).
Project-specific rules live in each repo's own `AGENTS.md` sections outside
this block.

**Planning and approval**

- Read the repo's `AGENTS.md`, `README.md`, and the documents they point at
  before deep implementation work.
- For non-trivial changes, propose the plan (files you expect to touch and
  why, approach, rationale) and wait for approval before writing code. Plan
  mode satisfies this; a dedicated planning skill or brainstorming ceremony
  is not required.
- Show diffs and wait for approval before committing or pushing, unless the
  user has invoked a publish workflow (e.g. `cpprrm`) that explicitly
  authorizes commit/push for the current task and no stronger repo or
  security gate applies.

**Git hygiene**

- Run `git status` and `git diff --minimal` before any commit.
- Stage explicit paths. Never use `git add .`.
- Keep unrelated local files out of commits, including local `.ccd.toml` and
  harness work directories unless the task explicitly targets them.

**Validation and evidence**

- Run the smallest deterministic check that proves the change. Repo-specific
  gates (`make verify`, `cargo test`, `go test ./...`, etc.) are defined in
  each repo's Testing section and take precedence over this floor.
- Before claiming work is complete, fixed, or passing, run the relevant
  verification command and cite the actual output. Do not assert success
  without evidence.
- If you skip validation, state the reason explicitly.

**Safety**

- No secrets in tracked files, docs, workflows, fixtures, diagnostics, or
  generated outputs.
- Do not overwrite user changes or reset the worktree without explicit
  approval.
- Do not flip repository visibility or licensing policy without an explicit
  decision from the operator.
- Treat external services and model calls as opt-in unless a repo policy or
  explicit user request grants narrow approval.
- Prefer deterministic behavior, explicit metadata, and small reversible
  changes.

**Source files**

- `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, and other model-instruction source
  files are user-owned and maintainer-authored. Tooling in this org does not
  overwrite or render replacement bodies for them except for explicit
  maintainer-requested edits to those files.

**External review (standing grant)**

- Every nantobv repo that vendors this block grants standing approval to
  invoke the bundled `claude-adversarial-review` skill on the selected local
  diff when the user explicitly asks for that skill. Treat the explicit
  request as the approval; do not ask for a second confirmation.
- The approval is narrow: it applies only to the selected repo diff, only to
  the `claude-adversarial-review` skill, and only with that skill's
  read-only tool permissions. It does not approve sending secrets, unrelated
  files, whole-home context, or data to other external services.
- After an adversarial review returns, evaluate findings on their merits.
  Address real correctness, maintainability, or contract-honesty issues with
  proportionate changes. Do not add broad machinery, process, or abstraction
  solely to satisfy a speculative review point.
<!-- END nantobv-shared -->

## Coding Standards

- Read [README.md](README.md) and [ARCHITECTURE.md](ARCHITECTURE.md) before deep implementation work. Use the active GitHub issues for backlog and priority context.
- If you touch core write paths or repo governance, also read [docs/development/repo-governance.md](docs/development/repo-governance.md).
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

## Testing

- Prefer `make fmt`, `make test`, `make vet`, and `make ci` where applicable.

## Safety

- Stay within the active issue or approved work item and call out scope drift when it appears.
