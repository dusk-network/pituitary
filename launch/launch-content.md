# Pituitary Launch Content — All Channels

Prepared 2026-03-26. Ready-to-post content for each channel, plus sequencing plan.

---

## 1. Show HN (Hacker News)

**Title:** Show HN: Pituitary – Catch spec drift before it catches you (Go CLI + MCP)

**URL:** https://github.com/dusk-network/pituitary

**Text:**

I kept writing specs and decision docs with AI, then discovering they contradicted each other three sessions later. The architecture doc said one thing, CLAUDE.md said another, and the code did a third.

Grep doesn't catch semantic drift. Nothing I found watched the whole corpus. So I built Pituitary.

It's a single Go binary that indexes your local markdown specs, docs, and decision records into SQLite, then catches what you can't track by hand:

- Overlapping decisions between specs
- Docs that contradict accepted specs (with deterministic auto-fix)
- Code diffs that conflict with spec requirements
- Terminology that drifted after a conceptual migration

```
pituitary init --path .
pituitary check-doc-drift --scope all
git diff origin/main...HEAD | pituitary check-compliance --diff-file -
```

No Docker, no API keys, one SQLite file. Deterministic by default — optionally plug in any OpenAI-compatible embedding API (cloud or local) for deeper semantic retrieval.

It also ships an MCP server with 6 tools so Claude Code, Cursor, and Windsurf can query the spec index mid-session. And it runs in CI.

The problem gets worse the more AI you use. Every session produces more docs that nobody cross-checks. Pituitary is the feedback loop.

GitHub: https://github.com/dusk-network/pituitary
MCP registry: published on modelcontextprotocol.io and mcp.so

Would love feedback, especially from anyone managing 20+ decision records across a codebase.

---

### What changed from the original draft

- Added "Go CLI + MCP" to the title — HN readers scan titles and these signal the tech
- Strengthened the opening: more specific pain ("architecture doc said one thing, CLAUDE.md said another")
- Added the "problem gets worse with AI" framing — this is the hook that makes it timely
- Added the MCP registry links — signals maturity
- Kept the code block but trimmed it to the essentials
- Removed "Watch on asciinema" — the GitHub README already has the demo
- Closing ask is specific: "20+ decision records" qualifies the audience

---

## 2. Reddit — r/programming

**Title:** I built a CLI tool that catches when your specs, docs, and code silently contradict each other

**Body:**

Every team I've worked with has the same problem: you write specs, architecture docs, decision records, and CLAUDE.md files. They accumulate. Then three weeks later someone discovers the rate limiting doc says "fixed window" but the spec that was accepted two months ago says "sliding window." Nobody noticed because nobody reads everything.

AI makes it worse. Each session starts fresh. The agent writes more docs, more specs, more decisions — and nobody cross-checks them against each other.

I built **Pituitary** to fix this. It's a Go CLI that:

1. Indexes your markdown specs, docs, and decision records into SQLite
2. Detects overlapping decisions between specs
3. Finds docs that contradict accepted specs (and can auto-fix them)
4. Checks PR diffs against spec requirements before merge
5. Audits terminology drift after conceptual migrations

Single binary. No Docker. No API keys required. Deterministic by default.

```
pituitary init --path .
pituitary check-doc-drift --scope all
git diff origin/main...HEAD | pituitary check-compliance --diff-file -
```

It also ships an MCP server so Claude Code, Cursor, and Windsurf get spec awareness mid-session.

Open source, MIT licensed: https://github.com/dusk-network/pituitary

Would love to hear how others handle this problem. Do you just accept that docs go stale? Use a wiki? Something else?

---

## 3. Reddit — r/golang

**Title:** Pituitary: a Go CLI for catching spec drift — SQLite indexing, deterministic analysis, optional MCP server

**Body:**

Sharing a Go project I've been working on. **Pituitary** indexes your markdown specs, docs, and decision records into a SQLite database and detects when they contradict each other.

The core analysis surface:

- **Overlap detection** — catch decisions that cover the same ground
- **Doc drift** — find docs that contradict accepted specs, with deterministic auto-fix
- **Code compliance** — pipe your PR diff in and check it against spec requirements
- **Impact analysis** — trace what's affected when a spec changes
- **Terminology audit** — find displaced terms after conceptual migrations

Design decisions that might be interesting to Go devs:

- Single binary, no Docker — CGo for `sqlite-vec` is the only non-pure-Go dependency
- SQLite for the index with atomic rebuilds
- Deterministic retrieval by default (fixture embedder, no API keys needed) — optional OpenAI-compatible embeddings for deeper semantic search
- CLI-first architecture: the optional MCP server wraps the same CLI commands
- Heading toward a kernel/extension adapter pattern (RFC in the repo) to keep the core pure while adding external source adapters

The `spec.toml` + `body.md` bundle format and the indexing pipeline might be worth looking at if you're interested in document analysis in Go.

MIT licensed: https://github.com/dusk-network/pituitary

Contributions welcome — there are `good first issue` labels if you want to pick something up. The codebase has clear package boundaries.

---

## 4. X/Twitter — Long Post

**Scheduling:** Use X's native scheduling. Post ~1 hour after Show HN goes up (Day 0, ~10am ET). Attach the "Peet with claws" image as the headline visual.

**Image:** `assets/peet-claws.png` — Peet mascot variant with claws, spec-catching pose.

**Post:**

Your AI writes more specs and docs than you can cross-check.

Three sessions later you discover the architecture doc contradicts the spec that was accepted last month. The rate limiting doc says "fixed window" — the spec says "sliding window." Nobody noticed because nobody reads everything.

I kept hitting this so I built Pituitary. It's a Go CLI that indexes your markdown specs, docs, and decision records into SQLite, then catches what you can't track by hand:

→ Overlapping decisions between specs
→ Docs that silently contradict accepted specs (with deterministic auto-fix)
→ Code diffs that conflict with spec requirements before merge
→ Terminology that drifted after a conceptual migration

Single binary. No Docker. No API keys. One SQLite file. 30 seconds to first finding on any repo with markdown specs.

pituitary init --path .
pituitary check-doc-drift --scope all
git diff origin/main...HEAD | pituitary check-compliance --diff-file -

It also ships an MCP server with 6 tools — add it to Claude Code, Cursor, or Windsurf and your agent gets spec awareness mid-session. It can check overlap before writing a new spec, or verify a PR doesn't contradict accepted decisions.

The problem gets worse the more AI you use. Every session produces more docs. Nobody cross-checks them. Pituitary is the feedback loop between "what you decided" and "what's actually true."

Open source, MIT licensed, written in Go.
github.com/dusk-network/pituitary

Try it. Break it. Tell me what's wrong.

---

### What changed from the thread format

- Consolidated 6 tweets into one long post (~2200 chars) — threads fell out of fashion on X
- Kept the same narrative arc: hook → problem → solution → demo → MCP → why now → CTA
- Added the concrete "fixed window vs sliding window" example in the hook for specificity
- Removed the 🧵 emoji and thread numbering
- Scheduling note: use X's native scheduler, no external coordination needed

---

## 5. Launch Sequencing Plan

### Timing Strategy

Post on a **Tuesday or Wednesday morning** (US time, ~9-10am ET). This is when HN and Reddit traffic peaks for developer content. Avoid Mondays (backlog clearing) and Fridays (checked out).

### Sequence

**Day 0 (launch day):**

1. **Morning (9am ET):** Post Show HN. This is the anchor — everything else drives traffic back to it.
2. **+2 hours:** Post to r/programming. Different title and framing (problem-first, not tool-first).
3. **+3 hours:** Post to r/golang. Technical framing, Go-specific details.
4. **+1 hour (pre-scheduled on X):** Long post goes live via X's native scheduler. Attach "Peet with claws" image. Tag relevant accounts if you have relationships (MCP community, Go community figures).

**Day 0 evening:**
- Monitor HN comments and respond to every substantive one within 1 hour. HN rewards engaged authors.
- Monitor Reddit and reply conversationally.

**Day 1-3 (follow-up):**
- Submit Pituitary to awesome-mcp lists (wong2/awesome-mcp-servers, aimcp, TrueHaiq, gauravfs-14). Each is a separate PR.
- Submit to PulseMCP directory (pulsemcp.com/servers).
- If HN got traction, write a follow-up "lessons learned" post or respond to the most interesting comments with deeper technical context.

**Day 4-7:**
- Publish the GitHub Action (from launch/github-action/) as a separate repo `dusk-network/pituitary-action`. This gives you a second launch moment.
- Post a focused "Pituitary now has a GitHub Action" update to the same channels.

**Week 2:**
- Write a longer blog post: "Why your ARCHITECTURE.md is lying to you" — the problem-framing piece. Post to dev.to and cross-post to HN as a regular (non-Show) submission.
- File 5-8 well-scoped `good first issue` GitHub issues to attract contributors.

### Community Lists to Submit To

| List | URL | Action |
|------|-----|--------|
| awesome-mcp-servers (wong2) | github.com/wong2/awesome-mcp-servers | PR to add Pituitary |
| awesome-mcp (aimcp) | github.com/aimcp | PR |
| awesome-mcp (TrueHaiq) | github.com/TrueHaiq | PR |
| awesome-mcp (gauravfs-14) | github.com/gauravfs-14 | PR |
| PulseMCP | pulsemcp.com/servers | Submit |
| ADR tooling list | adr.github.io/adr-tooling | PR to add Pituitary |

### Key Talking Points for Comments/Replies

When people ask "how is this different from X?":

- **vs. ADR tools (log4brains, adr-tools):** Those organize ADRs. Pituitary detects when ADRs contradict each other, when docs go stale relative to ADRs, and when code drifts from them. It's a quality gate, not an authoring tool.
- **vs. linters/static analysis:** Linters check code. Pituitary checks the space between specs, docs, and code — the semantic layer that linters can't reach.
- **vs. "just use grep":** Grep finds text. Pituitary finds when "sliding window" in your spec contradicts "fixed window" in your doc — same concept, different surface text.
- **vs. Notion/Confluence search:** Those search within one system. Pituitary indexes across all your local markdown and cross-checks them against each other.

---

## 6. Competitive Positioning Summary

**No direct competitors** exist for Pituitary's specific combination:
- Spec drift detection (not just ADR management)
- Cross-document consistency checking (not single-system search)
- Deterministic CLI-first analysis (not SaaS)
- MCP integration (optional, not required)

**Adjacent tools** (position as complementary, not competitive):
- Log4brains, adr-tools → ADR management (Pituitary adds drift detection on top)
- GitHub Spec Kit → spec-driven development (Pituitary validates after specs are written)
- Acrolinx → content governance (commercial SaaS; Pituitary is open source, local-first)
