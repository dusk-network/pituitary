# Marketplace Submission Guide

Checklist and copy for submitting the Pituitary skill package and MCP server to platform-specific marketplaces.

## Status

| Marketplace | Status | Notes |
|-------------|--------|-------|
| Smithery.ai | Ready to verify | `server.json` + `Dockerfile.smithery` in repo root |
| Anthropic MCP registry | Ready to submit | Listing copy at `launch/mcp-registry.md` |
| cursor.directory | Ready to submit | `.cursorrules` at `skills/pituitary-cli/platforms/cursor/` |
| Windsurf community rules | Ready to submit | `.windsurfrules` at `skills/pituitary-cli/platforms/windsurf/` |
| OpenAI agents directory | Ready to submit | `agents/openai.yaml` at `skills/pituitary-cli/agents/` |

## Smithery.ai

Config files are committed:
- `server.json` — MCP server manifest (schema 2025-12-11)
- `Dockerfile.smithery` — runtime container for Smithery distribution

Verify the listing is live at the Smithery dashboard. The description and version should match `server.json`.

## Anthropic MCP Registry

Full listing copy is at [`launch/mcp-registry.md`](mcp-registry.md). Key fields:

- **Name:** Pituitary
- **Description:** Intent governance for AI-native teams. Detect spec overlap, stale docs, code contradictions, terminology drift, and change impact. 13 tools. No API keys required.
- **Transport:** stdio
- **Tools:** 13 (search_specs, check_overlap, compare_specs, analyze_impact, check_doc_drift, review_spec, check_compliance, check_terminology, governed_by, compile_preview, fix_preview, status, explain_file)
- **Category:** Developer Tools / Documentation

## cursor.directory

Submit the `.cursorrules` file from `skills/pituitary-cli/platforms/cursor/.cursorrules`.

Listing metadata:
- **Name:** Pituitary — Spec-Aware Repository Analysis
- **Description:** Rules for AI coding agents using Pituitary for specification governance. Covers workspace status checks, schema verification, command selection, mutation safety, and evidence trust boundaries.
- **Tags:** specifications, documentation, governance, drift-detection, compliance
- **URL:** https://github.com/dusk-network/pituitary

## Windsurf Community Rules

Submit the `.windsurfrules` file from `skills/pituitary-cli/platforms/windsurf/.windsurfrules`.

Same listing metadata as cursor.directory above.

## OpenAI Agents Directory

Submit `skills/pituitary-cli/agents/openai.yaml` with the following metadata:

- **Display name:** Pituitary CLI
- **Description:** Spec-aware repository analysis — catch drift between specs, docs, and code
- **Categories:** developer-tools, code-quality, documentation
- **License:** MIT
- **Source:** https://github.com/dusk-network/pituitary

## Skill Package Quality Signals

Include these in marketplace descriptions where applicable:

- **Validated instructions:** SKILL.md scored 0.95 on an 8-dimension automated evaluation rubric (see `skills/pituitary-cli/eval/`)
- **Platform coverage:** Claude Code, Cowork, Codex CLI, Gemini CLI, Cursor, Windsurf, Cline
- **13 MCP tools** for full spec governance mid-session
- **No API keys required** in deterministic mode
- **Real-world validation:** Caught 90 deprecated-term violations and 7 semantic contradictions in a live AI tooling repo (see `docs/use-cases/`)
