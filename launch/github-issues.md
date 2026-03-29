# GitHub Issues to Create

---

## Issue 1: Colored terminal output with semantic color palette

**Title:** Colored terminal output with semantic color palette

**Labels:** `enhancement`, `cli-ux`

**Body:**

### Problem

All CLI output is currently plain text. On a dark terminal, findings, statuses, and evidence blend together — it's hard to scan and doesn't make a strong first impression.

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

Run `bash launch/preview-v2.sh` in a terminal to see the colored preview. The asciicast `demo.cast` shows the same output as a recording.

---

## Issue 2: `pituitary fix` — apply drift remediations to source files

**Title:** `pituitary fix` — apply drift remediations to source files

**Labels:** `enhancement`, `feature`

**Body:**

### Problem

`check-doc-drift` detects drift and suggests edits ("replace X with Y"), but the user has to open each file and make the changes manually. This makes the remediation feel like a report rather than a tool. The gap between "I found the problem" and "the problem is fixed" is where adoption falls off.

### Proposal

Add a `pituitary fix` command that reads drift findings and applies the suggested edits to source files, with confirmation.

#### Scoped modes

```sh
# Fix one doc
pituitary fix --path docs/guides/api-rate-limits.md

# Fix all stale docs for one spec
pituitary fix --scope SPEC-042

# Fix everything
pituitary fix --scope all
```

#### Exact output specification

```
━━◈ fix --path docs/guides/api-rate-limits.md                [bold white · dim]

  docs/guides/api-rate-limits.md                              [dim path]

    - The public API uses a fixed-window rate limiter.        [red]
    + The public API uses a sliding-window rate limiter.      [green]

    - The default limit is 100 requests per minute for each API key.  [red]
    + The default limit is 200 requests per minute per tenant.        [green]

    - tenant-specific overrides are not supported.            [red]
    + tenant-specific overrides are supported through configuration.  [green]

  apply these edits? [y/n/diff]                               [yellow prompt, bold choices]
```

- `y` — apply and write file
- `n` — skip this file
- `diff` — show full unified diff before deciding

#### Non-interactive mode for CI

```sh
pituitary fix --scope all --dry-run    # show what would change, exit 0/1
pituitary fix --scope all --yes        # apply all without confirmation
```

### Implementation notes

- The edit evidence already exists in `check-doc-drift` results (suggested_edit field with old/new text)
- Core work: locate exact text spans in source files, produce minimal edits, write with atomic backup
- Edge cases: text that appears multiple times in a file, edits that overlap, files that changed since last index
- Should require a fresh index — fail fast if index is stale
- `--format json` should output the planned edits as structured data (for MCP consumers and scripting)

### Relationship to MCP

The MCP server should expose `fix` as a tool so agents can apply remediations directly during code review workflows. The agent calls `check_doc_drift`, reviews the findings, and then calls `fix` to apply the edits — all within the editor session.

### Out of scope for first version

- Fixing code compliance issues (only doc drift remediations)
- AI-generated rewrites (only deterministic text replacement from existing suggested_edit fields)
- Multi-file atomic transactions (each file is independent)
