<p align="center">
  <strong>Pituitary</strong><br>
  <em>Catch spec drift before it catches you.</em>
</p>

<p align="center">
  <a href="#install">Install</a> · <a href="#what-it-catches">What It Catches</a> · <a href="#use-it-from-your-editor">Editor</a> · <a href="#use-it-in-ci">CI</a> · <a href="docs/cheatsheet.md">Cheatsheet</a> · <a href="docs/reference.md">Reference</a>
</p>

---

Point Pituitary at your repo. It indexes your specs, docs, and decision records — CLAUDE.md, AGENTS.md, architecture docs, RFCs, runbooks — then catches what you can't track by hand. What you wrote down should still be true. Pituitary makes sure it is.

[![demo](https://asciinema.org/a/4NBiD3tUyuwMWooT.png)](https://asciinema.org/a/4NBiD3tUyuwMWooT)

Single binary. No Docker. No API keys required. One SQLite file.

## Install

**macOS** (Homebrew):

```sh
brew install dusk-network/tap/pituitary
```

**Linux / macOS** (binary):

```sh
curl -fsSL https://github.com/dusk-network/pituitary/releases/latest/download/pituitary_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/aarch64/arm64/;s/x86_64/amd64/').tar.gz | tar xz
sudo install pituitary /usr/local/bin/
```

**Windows**: download from [GitHub Releases](https://github.com/dusk-network/pituitary/releases) and add to your PATH.

**Build from source** (contributors): see [docs/development/prerequisites.md](docs/development/prerequisites.md).

## What It Catches

**Overlapping decisions** — a new spec covers ground an existing one already handles. Nobody noticed until both were accepted.

**Stale docs** — a spec changed, but the CLAUDE.md, AGENTS.md, runbooks, and guides that reference it weren't updated.

**Code that contradicts specs** — pipe your diff in before you merge:

```sh
git diff origin/main...HEAD | pituitary check-compliance --diff-file -
```

**Terminology drift** — the team adopted new language but old terms persist across your docs and specs.

## Quick Start

```sh
pituitary init --path .              # discover, index, report
pituitary check-doc-drift --scope all  # find stale docs
pituitary review-spec --path specs/X   # full review of one spec
pituitary status                       # index health at a glance
```

All commands output JSON with `--format json`. `review-spec` also supports `--format markdown` and `--format html` for shareable reports with full evidence chains.

See the [cheatsheet](docs/cheatsheet.md) for every command, or the [full reference](docs/reference.md) for configuration, runtime setup, and spec format details.

## Semantic Retrieval

Pituitary works out of the box with no API keys and no external dependencies. If you work in a professional context with higher standards for accuracy, you can enable deeper semantic support. This improves how precisely overlap detection, drift checks, and search match related content across your specs and docs.

**Cloud: OpenAI** (if you already have a key)

```toml
[runtime.embedder]
provider = "openai_compatible"
model = "text-embedding-3-small"
endpoint = "https://api.openai.com/v1"
api_key_env = "OPENAI_API_KEY"
```

**Local: LM Studio or Ollama** (no data leaves your machine)

```toml
[runtime.embedder]
provider = "openai_compatible"
model = "nomic-embed-text-v1.5"
endpoint = "http://127.0.0.1:1234/v1"
```

Then rebuild: `pituitary index --rebuild`

Any OpenAI-compatible embedding API works. See [runtime docs](docs/runtime.md) for full setup including analysis providers.

## Use It From Your Editor

Pituitary ships an MCP server so your agent gets spec awareness mid-session. Add it to Claude Code, Cursor, Windsurf, or any MCP-compatible client:

```json
{
  "mcpServers": {
    "pituitary": {
      "command": "pituitary",
      "args": ["serve", "--config", ".pituitary/pituitary.toml"]
    }
  }
}
```

Your agent gets 6 tools: `search_specs`, `check_overlap`, `compare_specs`, `analyze_impact`, `check_doc_drift`, `review_spec`. It uses them when reviewing PRs, checking whether a change contradicts an accepted decision, or planning changes that touch governed code.

## Use It in CI

Add spec hygiene checks alongside your linter:

```yaml
- run: pituitary index --rebuild
- run: git diff origin/main...HEAD | pituitary check-compliance --diff-file -
- run: pituitary check-doc-drift --scope all
```

See [docs/development/ci-recipes.md](docs/development/ci-recipes.md) for a complete GitHub Actions recipe.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full system design. Key decisions: deterministic retrieval first, tools-only (no embedded agent), single SQLite file with atomic rebuilds.

## Project Status

Active development. Core analysis is functional end-to-end: overlap, drift, impact, compliance, terminology, and review workflows all ship today. Pituitary watches your specs, docs, and decision records — code compliance is a supporting bridge, not the product center. See [docs/rfcs/0001-spec-centric-compliance-direction.md](docs/rfcs/0001-spec-centric-compliance-direction.md).

See [ROADMAP.md](ROADMAP.md) for what's shipped, what's next, and where Pituitary is headed.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). The codebase has clear package boundaries so you can contribute to one area without understanding the whole system.

## License

[MIT](LICENSE)
