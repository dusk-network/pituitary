# Pituitary: intent governance for the AI-natives

Karpathy recently described the need for a "Linting" layer in LLM Knowledge Bases — something that catches staleness, contradictions, and inconsistency before they compound. Pituitary is that layer. It already exists and ships today.

## The treadmill that never converges

Every AI-heavy team hits the same pattern. Specs, architecture docs, CLAUDE.md files, AGENTS.md, API contracts, runbooks, and roadmaps accumulate across sessions. Drift creeps in. Someone runs an LLM cleanup pass. The LLM says "all clean." But it only covered what fit in the context window. The rest keeps rotting — and the next PR introduces fresh contradictions on top of the ones that were never actually fixed.

You fight drift with LLMs. But the LLMs created the drift. Their audits have the same partial-view problem as the sessions that wrote the inconsistencies. It's a treadmill that feels productive but never converges. The token costs add up — false starts, misdirections, and wasted context directly caused by drifting issues, conflicting specs, unaligned design, stale terminology, and obsolete documentation.

## Proof: the CCD rescue

We ran Pituitary on the [CCD framework repo](https://github.com/dusk-network/pituitary/blob/main/docs/use-cases/ccd-terminology-and-drift-audit.md) — 11 accepted specs, 29 docs, 5 canonical skills. Pituitary found:

- **90 deprecated-term violations** across 22 artifacts
- **7 semantic contradictions** across 4 documents
- Runtime contract contradictions, deprecated terminology surfacing everywhere, doc drifts that had gone undetected through multiple cleanup cycles

The project direction was derailing. Pituitary rescued it — with local LLMs on an M2 Ultra. No data left the machine.

## What it catches

Point Pituitary at your repo. It indexes the entire corpus into SQLite and checks all of it, every time:

- **Overlapping decisions.** A new spec covers ground an existing one already handles.
- **Stale docs.** A spec changed, but the docs that reference it weren't updated.
- **Code that contradicts specs.** Flag diffs that conflict with accepted decisions before merge.
- **Terminology drift.** The team adopted new language but old terms persist.
- **Stale specs.** A spec hasn't been reviewed since the code it governs changed.
- **Impact chains.** Trace which specs, docs, and code paths are affected when a decision changes, with severity classification (breaking/behavioral/cosmetic).

## What makes it structural

Pituitary doesn't sample — it indexes the full corpus. That's the difference between it and an ad-hoc LLM audit.

**Terminology policies.** Declare canonical terms in config. Pituitary separates actionable violations from tolerated historical uses and suggests replacements.

**Compile.** Turn terminology findings into patches. `pituitary compile --dry-run` generates context-aware edits that distinguish prose from identifiers from historical entries — then apply them in one pass.

**Multi-repo governance.** Bind sources to named repo roots. Search, drift, impact, and status output carries repo identity so cross-repo results stay unambiguous. Across multiple repositories, Pituitary becomes the single point of truth where governance converges.

**Evidence chains.** Section-level source refs, classification, link reasons, and suggested edits — in JSON for agents or shareable HTML reports for humans.

## Your agent creates drift. Now make it fix drift.

Every AI session starts blind. Your agent uses deprecated terminology, proposes changes against specs it hasn't read, generates docs that contradict accepted decisions. You clean up after it with another LLM session that has the same partial view.

Add the MCP server and your agent gets 13 governance tools backed by the full corpus index — not a context-window sample. Works across 7 editors: Claude Code, Cursor, Windsurf, Cline, Codex CLI, Gemini CLI, and Cowork.

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

## How to try it

Single binary (pure Go, no CGO). No Docker. No API keys required. One SQLite file.

```sh
pituitary init --path .
pituitary check-doc-drift --scope all
pituitary check-terminology --scope all
pituitary compile --dry-run
git diff origin/main...HEAD | pituitary check-compliance --diff-file -
```

Deterministic by default. Evaluate it on your repo in 30 seconds.

## What's next

Pituitary is open source under MIT. v1.0.0-beta.6 ships today with the full analysis surface: overlap, drift, impact, compliance, terminology, compile, spec-freshness, and review workflows. We're working on broader source coverage (PDFs, JSON configs, GitHub issues) and tighter CI integration patterns.

If your AI is writing specs and your cleanups never converge, try it. You'll be surprised what your audits missed.

GitHub: https://github.com/dusk-network/pituitary
