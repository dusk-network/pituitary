# GitHub Issues to Create

---

## Issue 1: Colored terminal output with semantic color palette

**Title:** Colored terminal output with semantic color palette

**Labels:** `enhancement`, `cli-ux`

**Body:**

### Problem

The CLI already has basic ANSI/UTF-8 styling support, but many commands still render largely as plain text and the coloring is not applied consistently or with a clear semantic palette. On a dark terminal, findings, statuses, and evidence still blend together — it's hard to scan and doesn't make a strong first impression.

### Proposal

Add ANSI color to human-facing terminal output. Every color carries semantic weight:

| Color | Meaning | Examples |
|-------|---------|---------|
| Red (bold) | Drift / conflict / action needed | `██ DRIFT`, `✗` finding markers, `██ HIGH` overlap |
| Green (bold) | Aligned / clean / valid | `██ OK`, `✓`, `fresh`, `valid` |
| Yellow | Warning / review needed | `expected` / `got` labels, `▒▒ MED` overlap, `→` remediation arrows |
| Cyan | Identifiers | `SPEC-042`, `doc://guides/...`, doc paths |
| Bold white | Section headers and labels | Command header, finding names, section names like `OVERLAP`, `IMPACT` |
| Dim | Supporting context | Subtitles, tree chrome (`├─`, `└─`), metadata, file paths in evidence |

### Constraints

- Respect `NO_COLOR` env var (https://no-color.org/)
- No color when `TERM=dumb` or stdout is not a TTY
- `--format json` never emits ANSI codes
- `--color=always|never|auto` override flag
- Fall back to ASCII (`[DRIFT]`, `[OK]`, `|--`) when UTF-8 is not available
- Logo mark `━━◈` as command output header (or ASCII fallback `---*`)

### Exact output specifications by command

#### `check-doc-drift`

Compact. One line per finding with expected/got. Remediation hint. HTML hint at end.

```
━━◈ check-doc-drift                                          [bold white header]

  docs/guides/api-rate-limits.md                        ██ DRIFT
                                                        [cyan path]  [red bold badge]

    ✗ wrong window model           expected sliding  got fixed
    ✗ wrong default limit           expected 200/min  got 100/min
    ✗ tenant overrides unsupported  expected yes      got no
    [red ✗] [bold white name]      [yellow labels]   [white values]

    fix: pituitary fix --path docs/guides/api-rate-limits.md (3 edits)
    [green fix:]                                      [dim count]
    ℹ  run review-spec --format html for the full evidence report
    [dim hint]

  docs/runbooks/rate-limit-rollout.md                   ██ OK
  [cyan path]                                           [green bold badge]
```

#### `check-overlap`

Compact. Color-coded severity blocks. One-line recommendation at end.

```
━━◈ check-overlap · SPEC-042                                 [bold white · cyan]

  ██ SPEC-008  .955  Legacy Rate Limiting          extends this spec
  ██ SPEC-055  .929  Burst Handling                adjacent scope
  [red ██]     [bold score]  [dim title]           [white relationship]

  ▒▒ DOG-001   .692  Product Scope Contract        adjacent scope
  ▒▒ DOG-002   .687  Contributor Workflow           adjacent scope
  [yellow ▒▒]  [bold score]  [dim title]           [white relationship]

  ✓ SPEC-042 already supersedes SPEC-008 — no action needed
  [green ✓ and text]
```

#### `review-spec`

Full tree structure. This is the most detailed command — do NOT flatten it. The tree is the value.

```
━━◈ review-spec · SPEC-042                                   [bold white · cyan]
    Per-Tenant Rate Limiting for Public API Endpoints         [dim subtitle]

  OVERLAP   4 specs · recommendation: proceed with supersedes
  [bold white section header]   [white text]
  ├─ SPEC-008  0.955  extends
  ├─ SPEC-055  0.929  adjacent
  ├─ DOG-001   0.692  adjacent
  └─ DOG-002   0.687  adjacent
  [dim tree chrome] [cyan spec IDs] [white scores and relationships]

  IMPACT    2 specs · 2 refs · 2 docs
  [bold white section header]
  ├─ SPEC-055  Burst Handling · depends_on
  ├─ SPEC-008  Legacy Rate Limiting · supersedes · historical
  [dim tree chrome] [cyan IDs] [white titles] [dim "historical"]
  ├─ doc://runbooks/rate-limit-rollout  0.956
  └─ doc://guides/api-rate-limits       0.902
  [dim tree chrome] [cyan doc refs] [white scores]

  DOC DRIFT 1 item · 3 remediations
  [bold white section header]
  └─ doc://guides/api-rate-limits  ██ DRIFT
  [dim tree chrome] [cyan ref]     [red bold badge]
     → 3 suggested edits (see check-doc-drift for detail)
     [yellow →] [white text] [dim parenthetical]

  COMPARISON  prefer SPEC-042 as the primary reference
  [bold white label] [white text with cyan spec ID]

  ℹ  run review-spec --format html for the full evidence report
  [dim hint]
```

#### `status`

Single-line compact summary.

```
━━◈ status                                                    [bold white]

  5 specs  2 docs  23 chunks  fresh  fixture embedder
  [bold white counts] [dim labels] [green "fresh"] [dim embedder info]
```

#### `init`

Compact final summary with finding count.

```
━━◈ init                                                      [bold white]

  discovered 3 sources · wrote .pituitary/pituitary.toml
  index rebuilt: 5 specs · 2 docs · 23 chunks
  graph: valid                                                [green "valid"]

  ██ ready — run pituitary check-doc-drift --scope all
  [green ██ ready] [white text]
```

### Commands to color (priority order)

1. `check-doc-drift` — the "aha" command for new users
2. `check-overlap`
3. `review-spec` (text format, full tree)
4. `check-compliance`
5. `status`
6. `init` (final summary)

### Runnable mockup

Run `bash launch/preview-v2.sh` in a terminal to see the colored preview.

---

## Issue 2: `pituitary fix` should support `--request-file` for structured workflows

**Title:** `pituitary fix` should support `--request-file` for structured workflows

**Labels:** `enhancement`, `agent-dx`

**Body:**

### Problem

`pituitary fix` already supports `--path`, `--scope`, `--dry-run`, and `--yes`, but it is the odd command out in the CLI contract surface: other analysis commands accept `--request-file` for structured automation while `fix` still requires flags and an interactive TTY flow. That makes agent/editor integrations clumsier than they need to be.

### Proposal

Add `--request-file PATH|-` support to `pituitary fix` so structured callers can plan or apply deterministic remediations without shell-escaping brittle flag combinations.

#### Example request payloads

```json
{ "path": "docs/guides/api-rate-limits.md", "dry_run": true }
```

```json
{ "scope": "all", "dry_run": true }
```

```json
{ "doc_refs": ["doc://guides/api-rate-limits"], "apply": true }
```

#### Expected CLI forms

```sh
# Dry-run plan via structured request
pituitary fix --request-file request.json --format json

# Apply a pre-selected set of doc refs non-interactively
cat request.json | pituitary fix --request-file - --format json
```

#### Request/response contract

- Match the architecture contract for `fix`: selector plus apply/dry-run semantics.
- Support the same structured workflow style as `review-spec`, `compare-specs`, `check-doc-drift`, and `check-compliance`.
- Keep deterministic behavior exactly as-is; this is a transport and ergonomics improvement, not a semantic rewrite.

### Implementation notes

- Extend the existing fix request parser rather than adding a second execution path.
- Preserve the current TTY confirmation flow for plain text interactive use.
- `--request-file` should work with both `--dry-run` planning and non-interactive apply modes.
- Validate selector exclusivity the same way the flag-based path does now.

### Relationship to MCP

This is the missing piece for exposing `fix` cleanly through agent workflows. Once `fix` accepts structured input, MCP wrappers and editor integrations can pass exact doc refs and apply intent without shell-specific quoting rules.

### Out of scope for first version

- Changing the deterministic remediation model
- Introducing AI-generated rewrites
- Expanding `fix` beyond doc-drift remediations
