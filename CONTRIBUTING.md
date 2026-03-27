# Contributing to Pituitary

Pituitary is in active early development and welcomes contributions. Whether you're fixing a typo, improving error messages, or implementing a new analysis command, here's how to get started.

## Finding Something to Work On

GitHub issues track what's shipped, in progress, and what's next. Look for issues labeled `good first issue` — these are self-contained tasks that don't require understanding the full codebase.

If you want to work on something, open an issue (or comment on an existing one) to claim it. This avoids duplicate work.

## Setting Up Your Environment

**Prerequisites:**

- Go 1.25+
- A C compiler (gcc or clang) — required for the sqlite-vec CGo bindings
- Make

For a copyable per-platform setup reference, see [docs/development/prerequisites.md](docs/development/prerequisites.md).

**On macOS:**

```sh
# Xcode command line tools provide the C compiler
xcode-select --install
```

**On Ubuntu/Debian:**

```sh
sudo apt-get install build-essential
```

**On Fedora:**

```sh
sudo dnf install gcc
```

**Verify everything works:**

```sh
git clone https://github.com/dusk-network/pituitary.git
cd pituitary
make smoke-sqlite-vec   # Verifies sqlite-vec is linked correctly
make ci                 # Full check: fmt + smoke + test + vet
```

If `make smoke-sqlite-vec` passes, your C toolchain is working and you're ready to develop.

If you want to mirror the Linux-only analyzer lane from CI locally, install the pinned tools and run:

```sh
go install honnef.co/go/tools/cmd/staticcheck@v0.7.0
go install golang.org/x/vuln/cmd/govulncheck@v1.1.4
export PATH="$(go env GOPATH)/bin:$PATH"
make analyze
make test-race
```

If you do not want to install the local toolchain, the repo also ships a contributor `Dockerfile`:

```sh
docker build -t pituitary-dev .
docker run --rm -it pituitary-dev
```

Contributor docs:

- [docs/development/prerequisites.md](docs/development/prerequisites.md)
- [docs/development/documentation-guide.md](docs/development/documentation-guide.md)
- [docs/development/architecture-guide.md](docs/development/architecture-guide.md)
- [docs/development/ci-recipes.md](docs/development/ci-recipes.md)
- [docs/development/testing-guide.md](docs/development/testing-guide.md)
- [docs/development/adding-a-command.md](docs/development/adding-a-command.md)
- [docs/development/releasing.md](docs/development/releasing.md)

## Project Structure

```
pituitary/
├── cmd/                    # CLI commands (thin transport layer)
├── internal/
│   ├── analysis/           # Core analysis: overlap, comparison, impact, drift
│   ├── app/                # Transport-agnostic operations layer
│   ├── chunk/              # Markdown-aware text chunking
│   ├── config/             # pituitary.toml parsing and validation
│   ├── index/              # SQLite storage, embeddings, rebuild
│   ├── mcp/                # MCP server (thin wrapper over app/)
│   ├── model/              # Canonical types and data structures
│   └── source/             # Source adapters (filesystem, future: remote)
├── specs/                  # Example spec bundles (also used as test fixtures)
├── docs/                   # Example docs (also used as test fixtures)
├── testdata/               # Test-only fixtures
└── pituitary.toml          # Workspace configuration
```

The data flow is: **source adapters** discover specs/docs → **chunker** splits them into sections → **index** embeds and stores them → **analysis** queries the index and invokes the LLM → **app** wraps analysis into transport-agnostic operations → **cmd/** (CLI) and **mcp/** (MCP server) expose those operations.

If you're adding a new feature, start by identifying which layer it belongs to.

## Development Workflow

```sh
# Format your code
make fmt

# Check local documentation links
make docs-check

# Run tests (uses fixture providers — no API keys needed)
make test

# Run static analysis
make vet

# Run the Linux analyzer lane (requires staticcheck and govulncheck on PATH)
make analyze

# Run the full race-detector suite
make test-race

# Run the full CI pipeline
make ci

# Run benchmarks
make bench
```

The Makefile writes Go build cache to a local `.cache/` directory so builds are sandboxed and reproducible.

## Testing

Tests use a **deterministic fixture provider** — no live API keys or network access required. If you're adding tests:

- Write tests that use the fixture provider, not live models.
- Add new fixture expectations in `testdata/` or the existing spec/doc fixtures.
- Run the smallest check that proves your change works.

If a command needs live AI to function, ensure it fails gracefully with `dependency_unavailable` when the provider is absent.

## Release Process

Maintainers cut releases by pushing an annotated `v*` tag from `main`. The tag-driven workflow packages prebuilt archives with GoReleaser and publishes them to GitHub Releases.

See [docs/development/releasing.md](docs/development/releasing.md) for the exact steps and the currently automated platform targets.

## Submitting a PR

1. Fork the repo and create a branch from `main`.
2. Make your changes. Follow the existing code style (run `make fmt`).
3. Add or update tests as needed.
4. Run `make ci` to verify everything passes.
5. Open a PR with a clear description of what you changed and why.

**Guidelines:**

- Keep changes small and focused. One feature or fix per PR.
- Stage specific files in your commits — avoid `git add .`.
- If your change affects the public API or CLI interface, update the relevant docs.
- No secrets in tracked files, docs, or fixtures.

## Architecture Principles

These guide design decisions in Pituitary:

- **Deterministic first, LLM second.** Retrieval and ranking use vector similarity — reproducible and testable without an LLM. The LLM is only invoked for qualitative judgment.
- **CLI-first.** The CLI is the required transport. MCP is an optional wrapper. Core behavior must never depend on MCP.
- **Local filesystem only** (v1). No remote sources or cloud dependencies.
- **Tools-only, no embedded agent.** Pituitary exposes discrete tools. Orchestration is the caller's responsibility.
- **Atomic index operations.** A failed rebuild must never corrupt the existing index.

When in doubt, read [ARCHITECTURE.md](ARCHITECTURE.md) — it's the canonical source of truth for design decisions.

## Questions?

Open an issue with the `question` label. We're happy to help you find the right area to contribute.
