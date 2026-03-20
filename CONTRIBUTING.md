# Contributing

This repository is still in the bootstrap phase. Keep changes aligned with the first-shipping slice in `ARCHITECTURE.md` and avoid pulling deferred work into the codebase early.

## Working Rules

- Keep the first ship local-first and filesystem-only.
- Prefer deterministic foundations before LLM-backed analysis.
- Keep CI and local tests on the deterministic fixture AI provider; do not require live model credentials or network access.
- Do not start GitHub-, CI-, or MCP-specific implementation work unless the current issue explicitly calls for it.
- Keep transport code thin and push future business logic into `internal/`.

## Reserved Layout

The repository is scaffolded for the package boundaries already described in the architecture:

- `cmd/`: CLI transport and future command wiring
- `internal/config`: workspace config parsing and validation
- `internal/model`: canonical records and shared types
- `internal/source`: source adapters and normalization
- `internal/chunk`: Markdown-aware chunking
- `internal/index`: SQLite storage, embeddings, and rebuild flow
- `internal/analysis`: overlap, comparison, impact, and drift analysis
- `testdata/`: test-only fixtures, including malformed cases

These directories are intentionally present before feature code lands so implementation can grow into stable boundaries instead of accreting around `main.go`.

## Local Commands

Use the checked-in task targets:

```sh
make fmt
make smoke-sqlite-vec
make test
make vet
make bench
make ci
```

The `Makefile` writes `GOCACHE` into `.cache/` inside the repository so local runs stay sandbox-friendly and reproducible.

The current index implementation also requires a CGO-capable C toolchain because `sqlite-vec` is linked through `github.com/mattn/go-sqlite3` and `github.com/asg017/sqlite-vec-go-bindings/cgo`. Use `make smoke-sqlite-vec` when you want a fast, explicit readiness check for that path before running the full suite.
