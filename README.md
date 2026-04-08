<p align="center">
  <img src="assets/peet.png" alt="Peet the Pituitary mascot" height="120">
</p>

<h1 align="center">Pituitary</h1>

<p align="center">
  <em>Consistency governance for AI-native development.</em><br><br>
  Catch drifts before they catch you.
</p>

<p align="center">
  <a href="https://github.com/dusk-network/pituitary/actions/workflows/ci.yml"><img src="https://github.com/dusk-network/pituitary/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI"></a>
  <a href="https://goreportcard.com/report/github.com/dusk-network/pituitary"><img src="https://goreportcard.com/badge/github.com/dusk-network/pituitary" alt="Go Report Card"></a>
  <a href="https://github.com/dusk-network/pituitary/releases/latest"><img src="https://img.shields.io/github/v/release/dusk-network/pituitary?include_prereleases" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue" alt="License"></a>
</p>

<p align="center">
  <a href="#what-it-catches">What It Catches</a> · <a href="#quick-start">Quick Start</a> · <a href="#stop-your-agent-from-building-on-stale-decisions">Agent</a> · <a href="#use-it-in-ci">CI</a> · <a href="docs/cheatsheet.md">Cheatsheet</a> · <a href="docs/reference.md">Reference</a>
</p>

---

*For developers and teams where human+AI continuously produce specs, docs, and decisions that should agree — and silently stop agreeing.*

![demo](demo.gif)

[Watch on asciinema](https://asciinema.org/a/4NBiD3tUyuwMWooT) for the interactive version.

Single binary. No Docker. No API keys required. One SQLite file.

## What It Catches

You already know your docs drift. You fight it with LLM cleanup passes. The LLM says "all clean" — but it only covered what fit in the context window. The rest keeps rotting. Next PR introduces fresh contradictions on top of the ones that were never actually fixed. It's a treadmill that feels productive but never converges. Meanwhile the token costs pile up: false starts, misdirections, and wasted context directly caused by drifting issues, conflicting specs, and obsolete docs.

Pituitary replaces that treadmill with a structural guarantee: it indexes the entire corpus and checks all of it, every time. On a [real repo](docs/use-cases/ccd-terminology-and-drift-audit.md) with 11 specs and 29 docs, it found 90 deprecated-term violations across 22 artifacts and 7 semantic contradictions. The project direction was plagued by doc drifts, runtime contract contradictions, and deprecated terminology surfacing everywhere — Pituitary rescued it. Across multiple repos, it becomes the single point of truth where governance converges.

**Overlapping decisions.** A new spec covers ground an existing one already handles.

**Stale docs.** A spec changed, or a code diff implies docs likely went stale, but the CLAUDE.md, AGENTS.md, runbooks, and guides that reference it weren't updated.

**Code that contradicts specs.** Pipe your diff in before you merge:

```sh
git diff origin/main...HEAD | pituitary check-compliance --diff-file -
```

**Terminology drift.** The team adopted new language but old terms persist across your docs and specs.

## What It Becomes

Pituitary starts as a drift detector. As your intent corpus grows, it becomes the consistency governance layer:

**Temporal governance.** Point-in-time queries against the governance graph. When a spec is superseded, historical governance links are preserved but excluded from current queries. Use `--at DATE` with `check-compliance`, `check-doc-drift`, or `analyze-impact`. Timestamps are derived from index build/update time.

**Confidence-weighted edges.** Governance links carry trust tiers — extracted (declared in spec), inferred (AST symbol matching), or ambiguous. Use `--min-confidence` with `check-compliance` and `check-doc-drift` to trade precision for recall.

**Deliberate deviation vs accidental drift.** When code contradicts a spec, Pituitary checks for rationale comments (`// WHY:`, `// HACK:`, decision language). Deliberate deviations get a different remediation path than unintentional drift.

**Governance changelog.** `index --update --show-delta` reports what changed: specs added/removed, edges created/severed, governance posture shifts. The feedback loop CI needs.

**Spec families.** Community detection on the dependency graph discovers natural governance clusters. Coverage gaps between families are the highest-risk ungoverned areas.

**Governance protocol for AI.** The MCP server teaches your AI assistant *when* to check governance — before modifying files, before committing, after accepting specs, when writing docs — not just how.

## Quick Start

```sh
pituitary init --path .              # discover, index, report
pituitary new --title "Rate limiting policy" --domain api  # scaffold a draft spec
pituitary check-doc-drift --scope all  # find stale docs
pituitary review-spec --path specs/X   # full review of one spec
pituitary status                       # index health at a glance
```

## Command Reference

| What you want to do | Command |
|---|---|
| First run on a repo | `pituitary init --path .` |
| Scaffold a new draft spec | `pituitary new --title "Rate limiting policy" --domain api` |
| Find stale docs | `pituitary check-doc-drift --scope all` |
| Find stale docs implicated by a diff | `git diff origin/main...HEAD \| pituitary check-doc-drift --diff-file -` |
| Check a PR diff against specs | `git diff origin/main...HEAD \| pituitary check-compliance --diff-file -` |
| Full spec review | `pituitary review-spec --path specs/X` |
| Auto-fix deterministic drift | `pituitary fix --scope all --dry-run` |
| Search specs semantically | `pituitary search-specs --query "rate limiting"` |
| Trace impact of a spec change | `pituitary analyze-impact --path specs/X` |
| Compare two specs | `pituitary compare-specs --path specs/A --path specs/B` |
| Detect stale specs | `pituitary check-spec-freshness --scope all` |
| Inspect command contracts | `pituitary schema review-spec --format json` |

All commands output JSON with `--format json`. Agents can set `PITUITARY_FORMAT=json`, and redirected stdout defaults to JSON automatically. `review-spec` also supports `--format markdown` and `--format html` for shareable reports with full evidence chains.

One `pituitary.toml` can also span multiple repository roots. Bind a source to a named repo root with `repo = "..."`, and Pituitary carries that repo identity through search, drift, impact, status, and index output so cross-repo results stay unambiguous.

For terminology migrations, you can keep running ad hoc audits with `--term` / `--canonical-term`, or declare reusable `[[terminology.policies]]` in config and run `pituitary check-terminology` directly. Results now separate actionable current-state violations from tolerated historical uses and include replacement suggestions in both text and JSON output.

`analyze-impact`, `check-doc-drift`, and `review-spec` now emit section-level evidence chains in JSON: source refs on both sides of the match, a `classification`, a `link_reason`, and likely edit targets or suggested bullets. That gives agents enough structure to explain the next manual edit without scraping prose or auto-editing speculative changes.

For agent integrations, use `pituitary schema <command> --format json` to inspect request/response contracts, and prefer `--request-file PATH|-` on analysis commands when shell escaping would be brittle. Results that include raw repo excerpts or evidence now carry `result.content_trust` metadata so callers can treat returned workspace text as untrusted input instead of executable instructions.

See the [cheatsheet](docs/cheatsheet.md) for every command, the [full reference](docs/reference.md) for configuration/runtime/spec details, the reusable multi-editor package at [skills/pituitary-cli/README.md](skills/pituitary-cli/README.md), and [AGENTS.md](AGENTS.md) for repo-native agent instructions.

## Install

**macOS** (Homebrew):

```sh
brew install dusk-network/tap/pituitary
```

**Linux / macOS** (binary): download from [GitHub Releases](https://github.com/dusk-network/pituitary/releases), then:

```sh
tar xzf pituitary_*_*.tar.gz
sudo install pituitary /usr/local/bin/
```

**Windows**: download `pituitary_<version>_windows_amd64.zip` from [GitHub Releases](https://github.com/dusk-network/pituitary/releases), extract `pituitary.exe`, and add its location to your PATH.

**Build from source** (contributors): see [docs/development/prerequisites.md](docs/development/prerequisites.md).

## Stop Your Agent From Building on Stale Decisions

Your agent writes specs, reviews PRs, and proposes changes — but it doesn't know what's already been decided. Pituitary gives it that context. The governance protocol teaches it when to check, not just how.

### MCP Server (Claude Code, Cursor, Windsurf, or any MCP client)

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

Your agent gets 13 tools: `search_specs`, `check_overlap`, `compare_specs`, `analyze_impact`, `check_doc_drift`, `review_spec`, `check_compliance`, `check_terminology`, `governed_by`, `compile_preview`, `fix_preview`, `status`, and `explain_file`.

### Shared Skills (Claude Code, Cowork)

```sh
cp -R skills/pituitary-cli ~/.claude/skills/pituitary-cli
```

Codex CLI and Gemini CLI get project policy from `AGENTS.md` / `GEMINI.md` automatically. For the full Pituitary analysis workflow, also install the skill package: `cp -R skills/pituitary-cli ~/.codex/skills/pituitary-cli` or `~/.gemini/skills/`.

### Editor Rules (Cursor, Windsurf, Cline)

```sh
cp skills/pituitary-cli/platforms/cursor/.cursorrules .cursorrules        # Cursor
cp skills/pituitary-cli/platforms/windsurf/.windsurfrules .windsurfrules  # Windsurf
cp skills/pituitary-cli/platforms/cline/.clinerules .clinerules           # Cline
```

### AGENTS-Aware Tools (Codex CLI, Gemini CLI, and others)

The repo's [AGENTS.md](AGENTS.md) is the canonical project policy. Tools that read `AGENTS.md` get Pituitary awareness automatically. Generated mirrors ([CLAUDE.md](CLAUDE.md), [GEMINI.md](GEMINI.md)) are compatibility outputs, not separate policy sources.

See [skills/pituitary-cli/README.md](skills/pituitary-cli/README.md) for the full install guide, request templates, and security notes.

## Use It in CI

For pull requests that change specs, use the shipped GitHub Action to run `review-spec` and post the report as a PR comment:

```yaml
permissions:
  contents: read
  pull-requests: read
  issues: write

steps:
  - uses: dusk-network/pituitary@v1.0.0-beta.7
    with:
      fail-on: error
      # Set this when your repo keeps config at the root instead.
      # config-path: pituitary.toml
```

Add spec hygiene checks alongside your linter:

```yaml
- run: pituitary index --rebuild
- run: git diff origin/main...HEAD | pituitary check-compliance --diff-file -
- run: git diff origin/main...HEAD | pituitary check-doc-drift --diff-file -
- run: pituitary check-doc-drift --scope all
```

See [docs/development/ci-recipes.md](docs/development/ci-recipes.md) for a complete GitHub Actions recipe.

<details>
<summary><strong>Semantic Runtime</strong> (optional retrieval + bounded analysis beyond the deterministic default)</summary>

<br>

Pituitary works out of the box with no API keys and no external dependencies. For higher-quality semantic retrieval on a real corpus, configure an embedding runtime and rebuild the index. For bounded provider-backed adjudication in `compare-specs` and `check-doc-drift`, also configure a separate analysis runtime.

> "Pituitary got me from a large architecture spec plus drifted docs to a scoped, inspectable set of affected files quickly."
>
> From a real Hermes/Raoh doc-convergence pass across `harper`, `openclaw`, `autoskiller`, and `souther`: semantic support was most useful for narrowing scope, tracing terminology drift, and reducing omission risk before manual code/runtime verification.

**Cloud: OpenAI-compatible embeddings** (if you already have a key)

```toml
[runtime.embedder]
provider = "openai_compatible"
model = "text-embedding-3-small"
endpoint = "https://api.openai.com/v1"
api_key_env = "OPENAI_API_KEY"
```

**Named runtime profiles** (recommended when embedder and analysis share the same host assumptions)

```toml
[runtime.profiles.local-lm-studio]
provider = "openai_compatible"
endpoint = "http://127.0.0.1:1234/v1"
timeout_ms = 30000
max_retries = 1

[runtime.embedder]
profile = "local-lm-studio"
model = "nomic-embed-text-v1.5"

[runtime.analysis]
profile = "local-lm-studio"
model = "your-analysis-model"
timeout_ms = 120000
```

For `runtime.analysis`, prefer a text model that is good at bounded adjudication rather than a generic embedding or agent stack:

- strong instruction following and schema adherence
- concise answers without verbose reasoning text or intermediate chain-of-thought
- enough context for Pituitary's shortlisted evidence bundle; typical general-purpose `8k`-`32k` context is sufficient, with larger windows optional
- active-parameter cost that fits your latency and hardware budget

Examples today include recent instruct models from the Qwen and Mistral families, but the important choice is the capability profile, not one fixed model name.

Then validate and rebuild:

```sh
pituitary status
pituitary status --check-runtime all
pituitary index --rebuild
```

`pituitary status` now shows the active runtime profile plus the resolved provider, model, endpoint, timeout, and retry settings for both embedder and analysis. `--check-runtime` probes those resolved settings directly.

Retrieval remains deterministic. The analysis model only sees narrowly shortlisted context for `compare-specs` and `check-doc-drift`. Any OpenAI-compatible embedding or analysis API works. See [runtime docs](docs/runtime.md) for full setup.

</details>

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for the full system design. Key decisions: deterministic retrieval first, tools-only (no embedded agent), single SQLite file with atomic rebuilds.

## Project Status

Active development. Core analysis is functional end-to-end: overlap, drift, impact, compliance, terminology, compile, spec-freshness, and review workflows all ship today. The governance graph carries temporal validity, confidence tiers, and spec family assignments. Pituitary is consistency governance, not code linting — it keeps your project building against what you actually decided, not against stale echoes of decisions that routine LLM cleanups missed. See [docs/rfcs/0001-spec-centric-compliance-direction.md](docs/rfcs/0001-spec-centric-compliance-direction.md).

See [ROADMAP.md](ROADMAP.md) for what's shipped, what's next, and where Pituitary is headed.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). The codebase has clear package boundaries so you can contribute to one area without understanding the whole system.

## License

[MIT](LICENSE)
