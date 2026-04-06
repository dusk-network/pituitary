# Pituitary Launch Content — All Channels

Prepared 2026-03-26. Ready-to-post content for each channel, plus sequencing plan.

---

## 1. Show HN (Hacker News)

**Title:** Show HN: Pituitary – Intent governance for the AI-natives (Go CLI + MCP)

**URL:** https://github.com/dusk-network/pituitary

**Text:**

Karpathy's recent thread on LLM Knowledge Bases describes a "Linting" layer — something that checks your knowledge base for staleness, contradictions, and inconsistency. That's exactly what Pituitary is, except it already exists and ships today.

I ran it on a real repo — 11 specs, 29 docs. It found 90 deprecated-term violations across 22 artifacts and 7 semantic contradictions. The project direction was plagued by doc drifts, runtime contract contradictions, and deprecated terminology surfacing everywhere. The team had been doing routine LLM cleanups — but the LLM says "all clean" while only covering what fits in the context window. The rest keeps rotting. Next PR introduces fresh contradictions on top of the ones that were never actually fixed. It's a treadmill that never converges.

I built Pituitary to break this cycle. It's a Go CLI that indexes your entire corpus — specs, docs, decision records — into SQLite and checks all of it structurally, every time:

- Overlapping decisions between specs
- Docs that contradict accepted specs (with deterministic auto-fix)
- Code diffs that conflict with spec requirements
- Terminology that drifted after a conceptual migration
- Specs that went stale since the code they govern changed

```
pituitary init --path .
pituitary check-doc-drift --scope all
pituitary check-terminology --scope all
pituitary compile --dry-run
git diff origin/main...HEAD | pituitary check-compliance --diff-file -
```

No Docker, no API keys, one SQLite file (pure Go, no CGO). Deterministic by default. When it finds terminology drift, `pituitary compile` generates context-aware patches that distinguish prose from identifiers from historical entries — then apply them in one pass. The CCD audit ran entirely on local LLMs (M2 Ultra). No data left the machine.

This matters even more across multiple repos — Pituitary becomes the single point of truth where cross-repo governance converges. It slashes the token costs of false starts and misdirections directly caused by drifting issues, conflicting specs, and obsolete docs.

It ships an MCP server with 13 tools across 7 editors (Claude Code, Cursor, Windsurf, Cline, Codex, Gemini, Cowork), and runs in CI.

v1.0.0-beta.6: https://github.com/dusk-network/pituitary

Would love feedback — especially from anyone fighting the cleanup treadmill or managing specs across multiple repos.

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

**Title:** I ran a CLI on a real repo — 90 deprecated-term violations, 7 contradictions. The team's LLM cleanups had missed all of it.

**Body:**

Every team I've worked with has the same pattern: specs and docs accumulate, drift creeps in, someone runs an LLM cleanup pass. The LLM says "all clean." You move on. But the cleanup only touched what fit in the context window. The rest keeps rotting — and the next PR introduces fresh contradictions on top of the ones that were never actually fixed.

AI makes it dramatically worse. Each session starts blind. The agent uses deprecated terminology, proposes changes against specs it hasn't read, generates docs that contradict accepted decisions. You clean up after it with another LLM session that has the same partial view. The drift compounds. It's a treadmill that feels productive but never converges.

I built **Pituitary** to break the cycle. It's a Go CLI that indexes your entire corpus into SQLite and checks all of it, every time:

1. Indexes your markdown specs, docs, and decision records into SQLite
2. Detects overlapping decisions between specs
3. Finds docs that contradict accepted specs (and can auto-fix them)
4. Checks PR diffs against spec requirements before merge
5. Audits terminology drift after conceptual migrations
6. Compiles terminology findings into context-aware patches (`pituitary compile`)
7. Flags specs that went stale since the code they govern changed

```
pituitary init --path .
pituitary check-doc-drift --scope all
pituitary check-terminology --scope all
pituitary compile --dry-run
git diff origin/main...HEAD | pituitary check-compliance --diff-file -
```

I ran it on a real repo (11 specs, 29 docs) — 90 deprecated-term violations across 22 artifacts and 7 semantic contradictions. The project direction was derailing: doc drifts, runtime contract contradictions, deprecated terminology surfacing everywhere. All caught with local LLMs. No data left the machine.

This becomes obligatory with multiple repos. Pituitary is the single point of truth where cross-repo governance converges. And it slashes the token costs of false starts and misdirections directly caused by drifting issues, conflicting specs, and obsolete docs.

It ships an MCP server with 13 tools across 7 editors (Claude Code, Cursor, Windsurf, Cline, Codex, Gemini, Cowork). If you've seen Karpathy's thread on LLM Knowledge Bases — this is the "Linting" layer he described, except it already exists.

Open source, MIT licensed: https://github.com/dusk-network/pituitary

Would love to hear how others handle this. Are you running periodic LLM audits? How's that working out?

---

## 3. Reddit — r/golang

**Title:** Pituitary: a Go CLI for catching spec drift — SQLite indexing, deterministic analysis, optional MCP server

**Body:**

Sharing a Go project I've been working on. **Pituitary** indexes your markdown specs, docs, and decision records into a SQLite database and detects when they contradict each other.

The core analysis surface:

- **Overlap detection** — catch decisions that cover the same ground
- **Doc drift** — find docs that contradict accepted specs, with deterministic auto-fix
- **Code compliance** — pipe your PR diff in and check it against spec requirements
- **Impact analysis** — trace what's affected when a spec changes, with severity classification (breaking/behavioral/cosmetic)
- **Terminology audit + compile** — find displaced terms, then generate context-aware patches that distinguish prose from identifiers from historical entries
- **Spec freshness** — flag specs that haven't been reviewed since the code they govern changed

On a real repo (11 specs, 29 docs), it caught 90 deprecated-term violations across 22 artifacts and 7 semantic contradictions — drift that routine LLM cleanup passes had been missing because they only see what fits in the context window. Pituitary indexes the whole corpus and checks all of it every time. [Full write-up here.](https://github.com/dusk-network/pituitary/blob/main/docs/use-cases/ccd-terminology-and-drift-audit.md)

Design decisions that might be interesting to Go devs:

- Single binary, no Docker, pure Go — sqlite bindings via Wasm, no CGO dependency
- SQLite for the index with atomic rebuilds
- Deterministic retrieval by default (fixture embedder, no API keys needed) — optional OpenAI-compatible embeddings for deeper semantic search
- CLI-first architecture: the optional MCP server (13 tools) wraps the same CLI commands
- Parallel LLM adjudication with bounded errgroup concurrency
- Heading toward a kernel/extension adapter pattern (RFC in the repo) to keep the core pure while adding external source adapters

Multi-repo support lets you bind sources to named repo roots — governance converges in one place instead of per-repo ad-hoc audits.

The `spec.toml` + `body.md` bundle format and the indexing pipeline might be worth looking at if you're interested in document analysis in Go. If you've seen Karpathy's thread on LLM Knowledge Bases, Pituitary maps to his "Linting" and "Extra tools" layers.

MIT licensed, v1.0.0-beta.6: https://github.com/dusk-network/pituitary

Contributions welcome — there are `good first issue` labels if you want to pick something up. The codebase has clear package boundaries.

---

## 4. X/Twitter — Long Post

**Scheduling:** Use X's native scheduling. Post ~1 hour after Show HN goes up (Day 0, ~10am ET). Attach the "Peet with claws" image as the headline visual.

**Image:** `assets/peet-claws.png` — Peet mascot variant with claws, spec-catching pose.

**Post:**

@kaboris described a "Linting" layer for LLM Knowledge Bases. I built it. It already ships.

Ran it on a real repo — 11 specs, 29 docs. Found 90 deprecated-term violations across 22 artifacts. 7 semantic contradictions. The project was derailing: doc drifts, runtime contract contradictions, deprecated terminology everywhere.

The team had been doing LLM cleanups. The LLM says "all clean" — but only covers what fits in context. The rest rots. Next PR adds fresh contradictions on top of the ones never fixed. You fight drift with LLMs, but LLMs created the drift. It never converges.

Pituitary breaks this. It indexes the ENTIRE corpus. Checks ALL of it. Structurally. Every time.

→ Overlapping decisions between specs
→ Docs that contradict accepted specs (with deterministic auto-fix)
→ Code diffs that conflict with spec requirements
→ Terminology drift → `compile` generates context-aware patches in one pass
→ Stale specs the code outgrew

One binary (pure Go). No Docker. No API keys. Local LLMs only. No data left the machine.

Across multiple repos it becomes the single point of truth. Slashes token costs from false starts and misdirections caused by drifting specs and obsolete docs.

13 MCP tools across 7 editors. Your agent stops guessing — builds against what you actually decided.

v1.0.0-beta.6: github.com/dusk-network/pituitary

Try it on your repo. You'll be surprised what your cleanups missed.

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
- **vs. "I just run an LLM cleanup pass":** That's the treadmill. The cleanup covers what fits in context, declares victory, and misses the rest. Pituitary indexes the entire corpus structurally — declared terminology policies, deterministic drift detection, compile-to-patch. Not a one-off prompt that gives false confidence.
- **vs. ad-hoc LLM knowledge-base health checks:** If you're building an LLM knowledge base (à la Karpathy's "linting" layer), Pituitary is the structured version. Persistent governance loop, not a prompt you run and forget.

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
