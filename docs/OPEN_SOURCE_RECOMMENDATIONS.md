# Open Source Recommendations for Pituitary

Prioritized recommendations for making Pituitary a compelling open-source project that attracts contributors and builds a community.

---

## 1. Repository Essentials (Do First)

### Add a License

The repo has no LICENSE file. Without one, the code is technically "all rights reserved" and nobody can legally use or contribute to it. For a community-oriented project, **Apache 2.0** or **MIT** are the standard choices. Apache 2.0 is more common for tools with potential enterprise adoption (it includes a patent grant). Add a `LICENSE` file at the root.

### Add Issue Templates

Create `.github/ISSUE_TEMPLATE/` with at least two templates:

- **Bug report** — steps to reproduce, expected vs. actual behavior, environment (Go version, OS).
- **Feature request** — problem statement, proposed solution, alternatives considered.

This structures contributions and saves maintainer time triaging vague issues.

### Add a PR Template

Create `.github/PULL_REQUEST_TEMPLATE.md` with a lightweight checklist:

- What does this PR do?
- Which issue does it address?
- Did you run `make ci`?
- Any new tests?

### Add GitHub Topics

On the repo settings, add topics like: `specifications`, `documentation`, `mcp`, `ai-tools`, `golang`, `spec-management`, `developer-tools`. This is how people discover projects on GitHub.

---

## 2. README Improvements (Done — see updated README.md)

The previous README read like internal engineering notes. The new version:

- Leads with the problem (why specs drift) before the solution.
- Includes a quickstart that gets a new user from `git clone` to a working query in under a minute.
- Shows real `spec.toml` examples so readers immediately understand the data model.
- Documents the MCP server with copy-paste editor config.
- Keeps the technical contracts in ARCHITECTURE.md rather than cluttering the README.

---

## 3. Contributor Experience

### Good First Issues

Label 5-10 issues as `good first issue`. Ideal candidates from the issue queue:

- Add `--verbose` flag to `index --rebuild` showing per-source chunk counts.
- Implement `--dry-run` for `index --rebuild` (parse and validate but don't write).
- Add shell completion generation (`pituitary completion bash/zsh/fish`).
- Add a `pituitary version` command that prints the build version and Go version.
- Improve error messages for common config mistakes (missing `spec.toml`, invalid TOML syntax).

These are small, self-contained, and don't require understanding the full analysis pipeline.

### Developer Documentation

Create a `docs/development/` directory with:

- **`architecture-guide.md`** — a shorter, more approachable version of ARCHITECTURE.md aimed at contributors. Explain the package structure, how data flows from spec files through indexing to query results, and where to add new features.
- **`adding-a-command.md`** — step-by-step guide for adding a new CLI command + MCP tool. Walk through the layers: `cmd/` → `internal/app/` → `internal/analysis/`.
- **`testing-guide.md`** — explain the fixture provider model, how to write tests that don't need API keys, and how to add new fixture expectations.

### Reduce the Build Barrier

The CGo requirement for sqlite-vec is the biggest friction point for new contributors. Consider:

- Document the exact system packages needed on Ubuntu, macOS (Homebrew), and Fedora in a single "prerequisites" section.
- Add a `Dockerfile` for contributors who don't want to install a C toolchain:
  ```dockerfile
  FROM golang:1.25
  RUN apt-get update && apt-get install -y gcc
  WORKDIR /app
  COPY . .
  RUN make ci
  ```
- Investigate whether `modernc.org/sqlite` + a pure-Go sqlite-vec binding could remove the CGo requirement entirely. This would dramatically lower the contribution barrier.

---

## 4. UX Improvements

### CLI Polish

- **`pituitary init`** — interactive setup that creates a `pituitary.toml` from a few questions ("Where are your specs? Where are your docs?"). Right now a new user has to write the TOML by hand.
- **`pituitary status`** — show index health: number of indexed specs, docs, chunks, last rebuild time, configured providers. Like `git status` for Pituitary.
- **`pituitary version`** — print version, Go version, sqlite-vec version. Essential for bug reports.
- **Colored terminal output** — use color to distinguish warnings, errors, and success states in human-readable mode. Keep `--format json` clean.
- **Progress indicators** — `index --rebuild` should show progress when embedding many chunks (a spinner or progress bar). Currently there's no feedback during what could be a multi-second operation.

### Error Messages

- When a user runs `check-overlap` before running `index --rebuild`, the error should say exactly that: "No index found. Run `pituitary index --rebuild` first."
- When an API key is missing, name the specific env var: "ANTHROPIC_API_KEY not set. Configure it in your shell or set `api_key_env` in pituitary.toml."
- When a `--spec-ref` doesn't match any indexed spec, list the available spec IDs.

### Output Improvements

- **`--format table`** — add a compact table format for terminal users who don't want full prose but also don't want JSON. Useful for `search-specs` results and drift summaries.
- **Markdown output for review-spec** — `--format markdown` that produces a report suitable for pasting into a PR comment or a design review thread.

---

## 5. Community Building

### Write a Blog Post / Announcement

When you're ready to publicize, write a post explaining the problem Pituitary solves, show a real example workflow, and share it on:

- Hacker News (Show HN)
- Reddit (r/golang, r/programming, r/artificial)
- Twitter/X, Bluesky
- The MCP community Discord/forums (Pituitary's MCP integration is a differentiator)

### Create a Demo GIF / Asciicast

Record a 30-second terminal session (using [asciinema](https://asciinema.org/) or [VHS](https://github.com/charmbracelet/vhs)) showing:

1. `pituitary index --rebuild`
2. `pituitary search-specs --query "rate limiting"`
3. `pituitary review-spec --spec-ref SPEC-042`

Embed this in the README. A visual demo is worth 1000 words of documentation.

### Publish Releases

Set up GoReleaser to produce pre-built binaries for Linux, macOS, and Windows on every tag. This lets users install without Go or a C toolchain:

```sh
# Instead of cloning and building
brew install dusk-network/tap/pituitary   # macOS
# or download from GitHub Releases
```

### Register as an MCP Server

Once stable, register Pituitary on Anthropic's MCP registry / plugin marketplace. This puts it in front of every Claude Code and Cowork user looking for spec management tools.

---

## 6. Long-Term Differentiation

### Spec Templates

Ship a `pituitary new` command that scaffolds a new spec bundle from a template. This lowers the barrier to writing specs in the first place and ensures consistent metadata.

### GitHub Integration

A GitHub Action that runs `pituitary review-spec` on every PR that touches `specs/` and posts the report as a PR comment. This is the killer feature for adoption — it makes Pituitary invisible infrastructure rather than a tool you have to remember to run.

### Ecosystem Adapters

Non-filesystem source adapters (Notion, Confluence, Google Docs) would dramatically broaden the audience. Even just a "fetch specs from a GitHub repo" adapter would let teams use Pituitary across multiple repos without monorepo requirements.

---

## Priority Summary

| Priority | Action | Effort |
|---|---|---|
| P0 | Add LICENSE file | 5 min |
| P0 | Add issue/PR templates | 30 min |
| P0 | Add GitHub topics | 5 min |
| P1 | Label good-first-issues | 1 hr |
| P1 | `pituitary init` command | 1-2 days |
| P1 | `pituitary status` and `version` commands | Half day |
| P1 | Better error messages (no index, missing key, unknown ref) | Half day |
| P1 | Record a demo GIF for the README | 1 hr |
| P2 | Developer docs (architecture guide, adding a command, testing guide) | 1-2 days |
| P2 | Dockerfile for contributors | 1 hr |
| P2 | Set up GoReleaser + Homebrew tap | Half day |
| P2 | Colored output + progress indicators | 1 day |
| P3 | Blog post / announcement | Half day |
| P3 | `pituitary new` (spec scaffolding) | 1 day |
| P3 | GitHub Action for PR reviews | 1-2 days |
| P3 | Register on MCP registry | 1 hr |
