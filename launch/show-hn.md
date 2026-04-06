# Show HN submission

**Title:** Show HN: Pituitary – Intent governance for the AI-natives (Go CLI + MCP)

**URL:** https://github.com/dusk-network/pituitary

**Text:**

Karpathy's recent thread on LLM Knowledge Bases describes a "Linting" layer — something that checks your knowledge base for staleness, contradictions, and inconsistency. That's exactly what Pituitary is, except it already exists and ships today.

I ran it on a real repo last week — 11 specs, 29 docs. It found 90 deprecated-term violations across 22 artifacts and 7 semantic contradictions. The project direction was plagued by doc drifts, runtime contract contradictions, and deprecated terminology surfacing everywhere. The team had been doing routine LLM cleanups — the classic treadmill. The LLM says "all clean," but it only covers what fits in the context window. The rest keeps rotting. Next PR introduces fresh contradictions on top of the ones that were never actually fixed. It never converges.

Pituitary breaks this cycle. It's a Go CLI that indexes your entire corpus — specs, docs, decision records — into SQLite and checks all of it structurally, every time:

    pituitary init --path .
    pituitary check-doc-drift --scope all
    pituitary check-terminology --scope all
    pituitary compile --dry-run
    git diff origin/main...HEAD | pituitary check-compliance --diff-file -

What it catches: overlapping decisions, stale docs, code that contradicts specs, terminology that drifted, specs that went stale since the code they govern changed. When it finds terminology drift, `pituitary compile` generates context-aware patches that distinguish prose from identifiers from historical entries — then apply them in one pass.

What makes it structural: it indexes the full corpus, not a context-window sample. Declared terminology policies, deterministic drift detection, compile-to-patch. This matters even more across multiple repos — Pituitary becomes the single point of truth where cross-repo governance converges.

One binary (pure Go, no CGO), one SQLite file, no API keys. The CCD audit ran entirely on local LLMs (M2 Ultra). It ships an MCP server with 13 tools across 7 editors (Claude Code, Cursor, Windsurf, Cline, Codex, Gemini, Cowork), and runs in CI.

v1.0.0-beta.6: https://github.com/dusk-network/pituitary

Would love feedback — especially from anyone fighting the cleanup treadmill or managing specs across multiple repos.
