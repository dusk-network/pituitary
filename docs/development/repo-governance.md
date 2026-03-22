# Repository Governance

This repo benefits from lightweight governance, not heavyweight process enforcement.

Pituitary's core risk is not continuity state across multiple agents or long-lived control loops. Its risk is narrower: user trust in a few write paths and in the correctness of the derived index that all analysis reads from.

## What Is Authoritative

Authoritative state in Pituitary is:

- repo files that define product behavior and contracts:
  - Go source
  - `README.md`
  - `ARCHITECTURE.md`
  - accepted specs and docs in a user's workspace
- explicit config files when the user chooses to keep them:
  - repo-local `pituitary.toml`
  - local `.pituitary/pituitary.toml` written by `discover --write`
- explicit spec bundles written by the user or by `canonicalize --write`

Derived, non-authoritative state is:

- the SQLite index at `.pituitary/pituitary.db`
- MCP transport responses
- CLI JSON/text output
- discovery previews and canonicalization previews before `--write`

The index is important, but it is rebuildable cache state. It must be correct and atomic, but it is not the source of truth.

## Core Trust Surfaces

These are the surfaces where regressions materially break user trust or product correctness:

1. `pituitary index --rebuild`
   - writes the derived SQLite index used by every analysis command
   - must remain atomic and must not leave a broken DB behind on failure

2. `pituitary index --dry-run`
   - promises rebuild-equivalent validation without persistent side effects
   - must not create or mutate the configured index

3. `pituitary discover --write`
   - writes a local config that should work immediately with `preview-sources` and `index`
   - must stay conservative and inspectable

4. `pituitary canonicalize --write`
   - writes explicit `spec.toml` + `body.md` bundles from inferred contracts
   - must preserve stable refs and source provenance

5. Path-first CLI resolution
   - commands that accept `--path` must honor workspace-relative semantics consistently
   - running from a subdirectory must not silently change the targeted spec

6. MCP startup and query parity
   - MCP is not a core write surface
   - it must remain a thin, read-only wrapper over the same indexed analysis surface

## Failure Modes That Matter

The failures worth blocking in CI are:

- generated config writes a file that the current parser or source loader cannot use
- canonicalization rewrites refs or strips provenance in a way that changes analysis identity
- dry-run leaves files or directories behind, or validates a different filesystem contract than rebuild
- rebuild stops being atomic and can leave a corrupt or partially swapped index
- workspace-relative `--path` resolution changes meaning based on the current shell directory
- product docs drift away from the shipped first-slice boundary badly enough that users would misunderstand the tool

By contrast, this repo does not need heavyweight ownership or state-kernel controls around:

- MCP output formatting
- planner/backlog documents
- generated local `.pituitary/` contents
- review metadata or vendor-specific GitHub integrations

## Blocking Invariants

These invariants deserve blocking coverage in `go test ./...`:

1. Discovery write contract
   - `discover --write` must produce a config that `preview-sources` and `index --rebuild` accept
   - coverage: [cmd/discover_test.go](../../cmd/discover_test.go)

2. Canonicalization write contract
   - `canonicalize --write` must produce a loadable spec bundle and preserve the stable inferred ref
   - coverage: [internal/source/canonicalize_test.go](../../internal/source/canonicalize_test.go), [cmd/canonicalize_test.go](../../cmd/canonicalize_test.go)

3. Rebuild/dry-run contract
   - dry-run must be side-effect free while validating the same relevant filesystem preconditions as rebuild
   - rebuild must remain staged and atomic
   - coverage: [cmd/index_test.go](../../cmd/index_test.go), [internal/index/rebuild_test.go](../../internal/index/rebuild_test.go)

4. Workspace-relative path contract
   - path-first commands must resolve relative paths against the workspace root, not the caller's current directory
   - coverage: [cmd/analyze_impact_test.go](../../cmd/analyze_impact_test.go) and other `cmd/*_test.go` path-first cases

5. Dogfood sanity contract
   - the internal dogfood workspace must preview, load, and dry-run successfully against the curated doc set
   - coverage: [dogfood_test.go](../../dogfood_test.go)

The current `make ci` path is sufficient for blocking enforcement because it already runs:

- format check
- SQLite readiness smoke test
- full Go test suite
- `go vet`

This repo does not currently need a second special governance workflow on top of `make ci`.

## GitHub-Side Protection Recommendation

After merge, the recommended GitHub settings for `main` are:

- require a pull request before merging
- require the `go` CI check from `.github/workflows/ci.yml`
- block force pushes
- block branch deletion

Not recommended right now:

- CODEOWNERS-based required review
- multiple mandatory approvals
- path-based ownership rules
- separate rulesets for docs vs code

Those would add process overhead without matching the current single-maintainer or small-maintainer operating model.
