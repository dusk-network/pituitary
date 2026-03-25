# Pituitary: catch spec drift before it catches you

Human+AI continuously produce records of intent, decisions, and documentation — specs, architecture docs, CLAUDE.md files, AGENTS.md, API contracts, runbooks, roadmaps. These accumulate across sessions. They overlap, contradict each other, and drift from the code. Nobody's watching the whole corpus.

Every AI session makes it worse. Each session starts fresh. The records pile up but nobody catches the contradictions. What you wrote down and should be the source of truth becomes unreliable — silently.

You find out in a PR review that the doc you're referencing was outdated two sprints ago. Or you spend an hour writing a decision only to discover someone already wrote one covering the same ground. Or worse — you don't find out, and it ships.

## What it does

Point Pituitary at your repo. It indexes your specs, docs, and decision records, then catches what you can't track by hand:

- **Overlapping decisions** — a new spec covers ground an existing one already handles
- **Stale docs** — a spec changed, but the docs that reference it weren't updated
- **Code that contradicts specs** — flag diffs that conflict with accepted decisions before merge
- **Terminology drift** — the team adopted new language but old terms persist
- **Impact chains** — trace which specs, docs, and code paths are affected when a decision changes

What you wrote down should still be true. Pituitary makes sure it is.

## How to try it

Single binary. No Docker. No API keys required. One SQLite file.

```sh
# Download from GitHub Releases (Linux, macOS, Windows)
# https://github.com/dusk-network/pituitary/releases

pituitary init --path .
pituitary check-doc-drift --scope all
pituitary review-spec --path specs/your-spec
git diff origin/main...HEAD | pituitary check-compliance --diff-file -
```

Deterministic by default. Evaluate it on your repo in 30 seconds.

## For AI-native workflows

In MCP-compatible editors, your agent gets 6 spec-awareness tools mid-session. Add Pituitary to Claude Code, Cursor, or Windsurf:

```json
{
  "mcpServers": {
    "pituitary": {
      "command": "pituitary",
      "args": ["serve", "--config", "pituitary.toml"]
    }
  }
}
```

## Why we built it

We hit this problem ourselves. Tens of markdown files, specs drifting from each other, GitHub issues replacing specs as source of truth. Nothing we found watched the whole corpus, so we built it.

Code review tools watch code. Documentation tools watch docs. Pituitary catches the drift across both.

## What's next

Pituitary is open source under MIT. The core analysis surface is functional end-to-end. We're working on broader source coverage (PDFs, JSON configs, GitHub issues) and tighter CI integration patterns.

If your repo has more than a handful of specs and docs, give it a try.

GitHub: https://github.com/dusk-network/pituitary
