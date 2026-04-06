# Positioning Reassessment & Revised Launch Strategy

Prepared 2026-03-31. Responds to the product shift identified in the post-#215 analysis.

---

## The core diagnosis

The report is right. The product has outgrown its positioning through real features. But the timing creates a specific constraint: launch is tomorrow. The question isn't "should the positioning change" — it's "what changes now, what changes in Week 1, and what changes in Week 2+."

The answer is a **three-beat launch** instead of a single announcement, where each beat tells a progressively deeper story.

---

## What the current materials get right

The Show HN text and announcement are calibrated well for first contact. They lead with a specific pain ("architecture doc said one thing, CLAUDE.md said another"), show a 30-second evaluation path, and close with a qualified ask. The r/golang post is technically grounded. The X post has good narrative arc.

None of these should be rewritten from scratch. The "catch spec drift" entry point is a good acquisition message. People install tools for one reason. You can't lead with "specification governance platform" — that's a retention message, not an install message.

## What the current materials miss

Three things:

**1. The governance story has no surface area.** The README says "Catch spec drift before it catches you" and describes five capabilities. But the product now has terminology governance policies, role-aware drift, diff-aware pre-merge checks, multi-repo workspaces, and evidence chains. These are governance features, not drift-detection features. The README treats them as technical details in a command reference rather than value propositions.

**2. The agent story is buried.** The MCP server gets a config block. The skill package gets a sentence. But the entire evidence-chain infrastructure (#222), content_trust metadata, --request-file transport, schema introspection, and multi-editor skill packaging are a first-class distribution strategy. "Your AI coding assistant gets spec awareness" is a headline, not a footnote.

**3. There's no second-beat content.** The launch plan has Day 0 (announcements), Days 1-3 (awesome-list PRs), Day 4-7 (GitHub Action), and Week 2 ("Why your ARCHITECTURE.md is lying"). But the most interesting story — how the product became a governance platform — has no scheduled vehicle. The Week 2 blog post is problem-framing, not product-deepening.

---

## Revised positioning: three layers

The anchoring line:

> **Intent governance for the AI-natives.**

"Intent" is already the internal vocabulary — the codebase says "intent artifacts," the ROADMAP says "intent corpus" and "intent graph." It's more accurate than "specification" because the product governs specs, docs, decisions, terminology, and role-specific artifacts. These are all expressions of intent. "The AI-natives" names the audience without defining them — people who start with AI in the loop as a default, not people who bolted it on. "The" does quiet work: it makes it sound like a group that already exists and already knows they have this problem. A nod, not a pitch.

### Layer 1 — Acquisition (Day 0)
**"Intent governance for the AI-natives"** (title) + **"Catch drifts before they catch you"** (subline)

Lead with the category and audience — that's the distinctive claim. Ground it immediately with the pain point. The title is what makes someone stop scrolling; the subline tells them what the tool does. "Drifts" (plural, without "spec") is broader and more accurate — the product catches doc drift, terminology drift, compliance drift, not just spec drift.

### Layer 2 — Retention (Day 3-5)
**"From drift detection to intent governance"**

This is a new content piece. It tells the story of what happens after install: terminology policies replace ad-hoc audits, diff-aware checks catch drift at PR time instead of audit time, multi-repo workspaces expand scope from one repo to the org, role-aware analysis understands that CLAUDE.md and ARCHITECTURE.md have different governance roles.

The message: "You installed it to catch stale docs. You keep it because it governs your intent corpus."

### Layer 3 — Distribution (Week 2)
**"Spec awareness for your AI coding assistant"**

This is the agent story. Evidence chains give agents structured provenance. content_trust metadata lets agents treat workspace excerpts as untrusted input. The skill package works across Claude Code, Cursor, Windsurf, Codex, and Gemini. The MCP server exposes 6 tools. Schema introspection lets agents discover contracts at runtime.

The message: "Every AI session produces more docs that nobody cross-checks. Give the AI the feedback loop."

---

## Specific material revisions

### Show HN text (Day 0)

The current text is strong. Two additions:

**Add after the MCP paragraph:**

> It also ships terminology governance policies (declare them in config, enforce in CI) and diff-aware drift checks — pipe your PR diff in and catch stale docs before merge, not after. Multi-repo workspaces are supported if your specs span repositories.

**Add to the closing ask:**

> Would love feedback, especially from anyone managing 20+ specs and docs across repos — or from teams where AI agents are producing decision records faster than anyone can cross-check.

These additions seed the governance and agent stories without overloading the acquisition message.

### README (Day 0, before launch)

The README needs three changes, not a rewrite:

**1. Flip the headline hierarchy:**

Change:
```
Catch spec drift before it catches you.
Solves intent drift. When your specs, docs, and decisions
silently contradict each other across sessions.
```

To:
```
Intent governance for the AI-natives.
Catch drifts before they catch you.
```

The category claim leads. "Intent governance" names what the product is — more accurate than "specification governance" because it governs specs, docs, decisions, terminology, and role-specific artifacts. "The AI-natives" names the audience without a footnote. The subline grounds it with the concrete pain. "Drifts" (plural, no "spec" qualifier) covers all the drift types the product catches. Together they tell the full story in two lines.

**2. Add a "Beyond drift detection" section after "What It Catches":**

```markdown
## What It Becomes

Pituitary starts as a drift detector. As your intent corpus grows, it becomes
the governance layer:

**Terminology policies.** Declare canonical terms in config. Pituitary
separates actionable violations from tolerated historical uses and suggests
replacements. No more ad-hoc grep audits.

**Diff-aware pre-merge checks.** Pipe your PR diff in. Catch stale docs
and spec contradictions before merge, not during quarterly audits.

**Multi-repo workspaces.** Bind sources to named repo roots. Search, drift,
impact, and status output carries repo identity so cross-repo results stay
unambiguous.

**Role-aware drift.** CLAUDE.md, AGENTS.md, runbooks, and architecture docs
have different governance roles. Pituitary understands the difference between
"this runbook drifted from a spec" and "this meta-doc needs updating."

**Evidence chains.** Section-level source refs, classification, link reasons,
and suggested edits — in JSON. Machine-readable provenance for agent
consumption or shareable HTML reports for humans.
```

**3. Elevate the editor/agent section:**

The current "Use It From Your Editor" section leads with MCP config JSON. It should lead with the value proposition:

```markdown
## Spec Awareness for Your AI Coding Assistant

Your agent writes specs, reviews PRs, and proposes changes — but it doesn't
know what's already been decided. Pituitary gives it that context.

Add the MCP server and your agent gets 6 tools: search specs, check overlap,
compare specs, analyze impact, check doc drift, and full composite review.
It uses them when reviewing PRs, checking whether a change contradicts an
accepted decision, or planning work that touches governed areas.

Evidence chains carry `content_trust` metadata so agents can treat returned
workspace text as untrusted input. `pituitary schema <command>` lets agents
inspect request/response contracts at runtime.
```

Then follow with the MCP config block and the skill package paragraph.

### Announcement.md (Day 0)

Add a "What it becomes" section between "What it does" and "How to try it", using the same content as the README addition above but in announcement tone. Also add a paragraph before the closing:

> **For AI-native teams.** Every AI coding session produces more docs and decisions that nobody cross-checks. Pituitary gives your agent spec awareness mid-session — and gives you the feedback loop that catches what accumulates between sessions. Evidence chains, content trust metadata, and structured JSON output mean agents can consume findings directly, not just humans reading terminal output.

### MCP registry listing

The current listing leads with drift detection. Add to the short description:

> Intent governance for the AI-natives — detect overlap, stale docs, code contradictions, terminology drift, and impact across your specs, docs, and decision records. Diff-aware pre-merge checks, multi-repo workspaces, and evidence chains for agent consumption.

### r/programming post

Add a paragraph after the feature list:

> The interesting part is what happens after you install it. Once you declare terminology policies in config, Pituitary enforces them in CI. Once you pipe your PR diff in, stale docs get caught before merge. Once you bind multiple repo roots, you get cross-repo intent governance. It goes from "run this occasionally" to "part of the workflow."

### X post

Add before the CTA:

> It also ships terminology governance policies (declare them, enforce in CI), diff-aware pre-merge checks, and multi-repo workspaces. The intent governance story is what keeps it installed after the first week.

---

## New content pieces

### Beat 2: Blog post — "From drift detection to intent governance" (Day 3-5)

This replaces the generic "hardening pass" follow-up window. Publish on dev.to, cross-post to the repo as `docs/guides/from-drift-to-governance.md`.

Structure:
1. **Hook:** "You installed Pituitary to catch stale docs. Here's what happens next."
2. **The drift detection story** — what pituitary init finds on day 1.
3. **The governance inflection** — when ad-hoc check-doc-drift runs become terminology policies, diff-piped pre-merge checks, and multi-repo workspaces.
4. **The three shifts** — reactive→proactive, single-repo→multi-repo, human-first→human+agent. Each with a concrete before/after command example.
5. **What Pituitary is NOT becoming** — not a linter, not an autonomous agent, not a spec generator. The RFC 0001 discipline.
6. **CTA:** "If you tried it for drift detection and stopped there, try `pituitary check-terminology` with a declared policy. Then pipe your next PR diff in."

This is the piece that turns one-time evaluators into retained users.

### Beat 3: Blog post — "Spec awareness for your AI coding assistant" (Week 2)

This replaces "Why your ARCHITECTURE.md is lying to you" — or rather, subsumes it. The ARCHITECTURE.md-is-lying framing is a good hook but it's a problem post. The agent story is a solution post that includes the problem.

Structure:
1. **Hook:** "Your AI writes more decision records than your team can review. Here's how to give it the feedback loop."
2. **The accumulation problem** — every AI session produces specs, docs, decisions. Nobody cross-checks them. (This is the ARCHITECTURE.md-is-lying content, reframed.)
3. **What spec awareness means** — the 6 MCP tools, what each one does in a real PR review workflow.
4. **Evidence chains for agents** — #222's section-level source refs, classification, link_reason, suggested edits. What an agent can do with structured provenance vs. scraping prose.
5. **content_trust and untrusted input** — why returning workspace text needs metadata, and how agents should consume it.
6. **Multi-editor distribution** — the skill package story. Claude Code, Cursor, Windsurf, Codex, Gemini. One canonical skill source, platform-specific install.
7. **CTA:** MCP config block + skill package install path.

Submit to HN as a regular (non-Show) post. This gives you a second HN moment.

### Beat 2.5: "Pituitary now has a GitHub Action" (Day 4-7)

This already exists in the launch plan. Keep it. But frame it as governance infrastructure, not just CI:

> Add intent governance to your PR workflow. Pituitary's GitHub Action runs review-spec on PRs that touch specs and posts the report as a PR comment. Combined with diff-piped compliance and drift checks, your intent corpus gets the same CI treatment as your code.

---

## Revised launch sequencing

> Launch moved to **Monday 2026-04-06** to ride the Karpathy "LLM Knowledge Bases" wave. The `compile` command ships this weekend; launch Monday with the full governance story fresh. Monday isn't the optimal HN day, but timing the Karpathy reply while the thread is hot is worth more than the Tuesday traffic bump.

### Pre-launch (2026-04-04 → 2026-04-05) — Weekend sprint

- Ship `pituitary compile` (issue #232)
- Finalize and test all README, ROADMAP, and launch material changes
- Harden the product: run the full CI surface, test new features end-to-end
- Record new asciinema if governance features warrant it
- Final review of all launch content against the "intent governance" framing

### Day 0 (2026-04-06, Monday) — Acquisition beat

- Morning: README updates land (title, "What It Becomes" section, elevated agent section)
- Morning: Show HN goes up (with governance and agent additions)
- +2h: r/programming (with governance paragraph)
- +3h: r/golang (unchanged — technical audience, keep it technical)
- +4h: X long post (with governance line)
- Evening: Monitor and respond to comments. Governance and agent angles are ready talking points.

### Days 1-3 (Apr 7-9) — Awesome-list and registry push

- Submit to awesome-mcp lists, PulseMCP, ADR tooling list (unchanged)
- Submit to Anthropic MCP registry with updated description

### Days 3-5 (Apr 9-11) — Governance beat

- Publish "From drift detection to intent governance" on dev.to
- Cross-post to repo as a guide
- Share on X with a shorter hook version
- Share in relevant communities (Go Slack, spec management discussions)
- If HN had traction, submit to HN as a regular post

### Days 4-7 (Apr 10-13) — GitHub Action beat

- Publish the Action (unchanged from current plan)
- Frame as governance infrastructure in the announcement

### Week 2 (Apr 13-17) — Agent beat

- Publish "Spec awareness for your AI coding assistant"
- Submit to HN as a regular post
- This is the piece aimed at the AI-native team audience
- Share in MCP community channels, AI coding assistant communities

### Week 3+ (Apr 20+) — Platform beat

- If traction warrants it, write the extensibility story: "Building a Pituitary adapter"
- This targets the RFC 0002 audience: teams who want to ingest specs from Jira, GitLab, Notion, JSON schemas
- Opens the contributor funnel for extension authors

---

## Positioning questions resolved

**1. Is the tagline still right?**
It was right for the original product. The product has outgrown it. Flip the hierarchy: "Intent governance for the AI-natives" as the title, "Catch drifts before they catch you" as the subline. The category claim leads; the pain point grounds it.

**2. Is the ROADMAP telling a story the README isn't?**
Yes, and the "What It Becomes" README section closes the gap. The ROADMAP's Later section (intent graph, coverage metrics, cross-repo governance) now has a narrative bridge from the README.

**3. Should the "Not on the roadmap" section evolve?**
Add one line: "Pituitary is becoming a specification governance platform. See the roadmap for where that's headed." This connects the negative space to the positive direction.

**4. Does the kernel/extension architecture need a public narrative?**
Not yet. Week 3+ is the right time. The Day 0 audience wants to solve drift. The Week 2 audience wants agent integration. The extension audience is smaller and more technical — they'll find the RFC. A dedicated post when the second adapter ships (JSON, Jira, or GitLab) is the right moment.

**5. Is "spec-centric" the right framing?**
Yes, but expand to "intent governance." The word "governance" captures the proactive, policy-driven, multi-repo nature of what the product has become. "Intent" is more accurate than "specification" — the product governs specs, docs, decisions, terminology policies, and role-specific artifacts. "Spec-centric" is an architecture principle (RFC 0001). "Intent governance" is a product category.

---

## What NOT to change

- Don't rename the project or change the mascot. Peet is good. The name is memorable.
- Don't over-explain "intent governance" — the subline and the feature list do the grounding. Let the term earn its meaning through the product.
- Don't add the extensibility story to Day 0. It's a platform play that matters to a smaller audience.
- Don't rewrite the r/golang post. That audience wants technical substance, not positioning.
- Don't change the ARCHITECTURE.md. It's an internal document. The public narrative is README + launch content.
- Don't lose the "catch drifts" line. It works as a grounding subline even though it's no longer the headline.
