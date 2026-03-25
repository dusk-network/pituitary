# Show HN submission

**Title:** Show HN: Pituitary -- catch spec drift before it catches you

**URL:** https://github.com/dusk-network/pituitary

**Text:**

Every AI session produces more text -- specs, architecture docs, CLAUDE.md, AGENTS.md, decision records. Each session starts fresh. The records pile up, contradict each other, and drift from the code. Nobody catches it until something breaks.

Pituitary is a CLI that indexes your specs and docs, then catches what you can't track by hand: decisions that overlap, docs that contradict accepted intent, code diffs that conflict with specs, and terminology that went stale.

Single Go binary, one SQLite file, no API keys needed. Ships an MCP server so Claude Code, Cursor, and Windsurf can query the spec index mid-session.

We built it because we hit this problem ourselves -- tens of markdown files drifting from each other, GitHub issues replacing specs as source of truth. Nothing we found watched the whole corpus.
