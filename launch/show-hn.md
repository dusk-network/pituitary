# Show HN submission

**Title:** Show HN: Pituitary -- catch spec drift before it catches you

**URL:** https://github.com/dusk-network/pituitary

**Text:**

I kept writing specs and decision docs with AI, then discovering they contradicted each other three sessions later. CLAUDE.md said one thing, the architecture doc said another, and the code did a third. Grep doesn't catch semantic drift. Nothing I found watched the whole corpus.

So I built Pituitary. It's a Go CLI that builds a SQLite index over your local markdown files and catches what you miss:

    pituitary init --path .
    pituitary check-doc-drift --scope all
    git diff origin/main...HEAD | pituitary check-compliance --diff-file -

It finds overlapping decisions, stale docs, code that contradicts specs, and terminology that drifted. One binary, one SQLite file, no API keys needed. Deterministic by default.

It also ships an MCP server (6 tools) so Claude Code, Cursor, and Windsurf can query the spec index mid-session. We use it in CI too.

Would love feedback, especially from anyone managing 20+ specs and docs across repos.
