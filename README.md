# Pituitary

Pituitary is a spec-management tool for keeping specifications, docs, and eventually code behavior aligned.

The current repository is the bootstrap for the first shipping slice defined in `ARCHITECTURE.md`. The initial scope is intentionally narrow:

- local filesystem only
- `spec.toml` + `body.md` spec bundles
- Markdown docs
- SQLite + `sqlite-vec`
- CLI-first workflow
- spec/doc analysis before code compliance

## Repository Layout

- `ARCHITECTURE.md`: architecture and first-ship scope
- `IMPLEMENTATION_BACKLOG.md`: milestone backlog derived from the architecture
- `pituitary.toml`: repo-local workspace config
- `specs/`: fixture spec bundles for bootstrap and testing
- `docs/`: fixture docs, including one intentional drift case
- `main.go`: minimal CLI bootstrap

## Bootstrap CLI

The current CLI is only a placeholder for the planned command surface:

- `index`
- `search-specs`
- `check-overlap`
- `compare-specs`
- `analyze-impact`
- `check-doc-drift`
- `review-spec`

Example:

```sh
go run . help
go run . index
go run . search-specs
```

At this stage the commands only confirm the intended interface and return a not-yet-implemented message.
