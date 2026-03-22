# Self-Dogfood Phase 1

This directory is the first internal dogfood workspace for Pituitary.

It is intentionally small:

- a tiny accepted contract set under `dogfood/contracts/`
- the product docs in `README.md` and `ARCHITECTURE.md`
- a narrow contributor-doc set under `docs/development/`

It intentionally excludes planning and backlog material such as `IMPLEMENTATION_BACKLOG.md` and `tasklist.md`.

This is a bridge to true repo-direct dogfooding later. For now, the goal is to keep the contract set explicit and the target docs easy to reason about.

Typical local flow:

```sh
go run . preview-sources --config dogfood/pituitary.toml
go run . index --rebuild --config dogfood/pituitary.toml
go run . check-doc-drift --config dogfood/pituitary.toml --scope all
```

If you want to inspect the inferred contracts first:

```sh
go run . search-specs --config dogfood/pituitary.toml --query "CLI-first local filesystem"
```
