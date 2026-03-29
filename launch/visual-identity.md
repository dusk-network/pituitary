# Pituitary Visual Identity — Terminal Output

## Design Principles

- **Bold and opinionated**: Pituitary knows what's wrong and says it clearly
- **Minimal logo**: 2-3 line mark, not a big ASCII banner
- **Color means something**: every color carries semantic weight, never decorative

---

## Logo Mark Options

### Option A — The Gland (abstract)

```
 ◉ pituitary
```

Single Unicode glyph. The bullseye/target evokes "precision" and "the thing that regulates everything." Compact, works in any terminal width.

### Option B — The Signal

```
⣿ pituitary
```

Braille block character. Dense, technical, suggests "scanning everything." Unusual enough to be memorable.

### Option C — The Brackets

```
⟪pit⟫ pituitary v0.4.0
```

Angle brackets suggest "spec boundaries" — which is literally what the tool checks. The abbreviated form `⟪pit⟫` could become the recurring mark in output headers.

### Option D — The Pulse

```
━━◈ pituitary
```

A line terminating in a diamond. Suggests "tracing through a graph" — which is what impact analysis does. Clean, technical.

---

## Color Palette

### Semantic colors (every color means something)

| Color | Meaning | Used for |
|-------|---------|----------|
| **Red** (bright) | Drift / conflict / action needed | `status: drift`, `conflicts`, overlap `high` similarity |
| **Green** (bright) | Aligned / compliant / clean | `status: aligned`, `compliant`, no findings |
| **Yellow** | Warning / medium confidence / review | `medium` overlap, `unspecified` compliance, confidence warnings |
| **Cyan** | Spec references and identifiers | `SPEC-042`, `doc://guides/...`, spec titles |
| **Bold white** | Section headers and commands | Command name, finding labels |
| **Dim** | Supporting context, metadata | Confidence basis, excerpt attribution, file paths |

### What NOT to color

- Don't color the actual excerpt text — it should read naturally
- Don't use blue (poor contrast on dark terminals)
- Don't use magenta/pink (no semantic meaning to assign)

---

## Output Redesign: check-doc-drift

### Current output (plain)

```
pituitary check-doc-drift: find docs that drift from specs
1. doc://guides/api-rate-limits | Public API Rate Limits | status: drift | confidence: high (0.910) | findings: 3 | remediation: 3
   rationale: accepted spec says tenant-specific overrides are supported through configuration, but the doc still says tenant-specific overrides are not supported
   spec evidence: SPEC-042 | Requirements
     excerpt: Apply limits per tenant rather than per API key.
   doc evidence: Public API Rate Limits / Configuration
     excerpt: All tenants share the same rate-limit configuration, and tenant-specific overrides are not supported.
```

### Proposed output (colored + structured)

```
━━◈ check-doc-drift

  doc://guides/api-rate-limits                           ██ DRIFT  (0.910)
  Public API Rate Limits · 3 findings · 3 remediations

    ✗ override_support_mismatch
      spec says tenant overrides are supported; doc says they are not
      expected: supported through configuration
      observed: tenant-specific overrides are not supported
      ├─ spec  SPEC-042 §Requirements
      │        "Apply limits per tenant rather than per API key."
      └─ doc   Public API Rate Limits / Configuration
               "All tenants share the same rate-limit configuration..."

    ✗ window_mismatch
      expected: sliding-window
      observed: fixed-window
      ├─ spec  SPEC-042 §Design Decisions
      └─ doc   Public API Rate Limits

    ✗ default_limit_mismatch
      expected: 200 req/min
      observed: 100 req/min
      ├─ spec  SPEC-042 §Requirements
      └─ doc   Public API Rate Limits / Default Limit

    → replace "fixed-window rate limiter" with "sliding-window rate limiter"
    → replace "100 requests per minute" with "200 requests per minute"
    → replace "tenant-specific overrides are not supported" with
      "tenant-specific overrides are supported through configuration"

  doc://runbooks/rate-limit-rollout                      ██ ALIGNED (0.865)
  Rate Limit Rollout Runbook
```

### Color mapping for the above

- `██ DRIFT` → bright red, bold
- `██ ALIGNED` → bright green, bold
- `✗` → red
- `expected:` / `observed:` → yellow label, white value
- `SPEC-042` → cyan
- `doc://...` → cyan
- `├─ spec` / `└─ doc` → dim (tree chrome)
- Excerpts in `"..."` → default color (reads naturally)
- `→` remediation arrows → yellow
- `━━◈` header → bold white

---

## Output Redesign: check-overlap

### Proposed output

```
━━◈ check-overlap · SPEC-042

  1. SPEC-008   0.955  ██ HIGH   extends · boundary review
     Legacy Rate Limiting for Public API Endpoints

  2. SPEC-055   0.929  ██ HIGH   adjacent · boundary review
     Burst Handling for Public API Rate Limits

  3. DOG-001    0.692  ▒▒ MED    adjacent · boundary review
     Pituitary Product Scope Contract

  4. DOG-002    0.687  ▒▒ MED    adjacent · boundary review
     Pituitary Contributor Workflow Contract

  recommendation: proceed with supersedes — candidate already declares the replacement path
```

### Color mapping

- `██ HIGH` → red, bold
- `▒▒ MED` → yellow
- `░░ LOW` → dim
- Score numbers → bold white
- Spec IDs → cyan
- Recommendation → green (positive outcome) or yellow (needs review)

---

## Output Redesign: review-spec

### Proposed output

```
━━◈ review-spec · SPEC-042
    Per-Tenant Rate Limiting for Public API Endpoints

  OVERLAP   4 specs · recommendation: proceed with supersedes
  ├─ SPEC-008  0.955  extends
  ├─ SPEC-055  0.929  adjacent
  ├─ DOG-001   0.692  adjacent
  └─ DOG-002   0.687  adjacent

  IMPACT    2 specs · 2 refs · 2 docs
  ├─ SPEC-055  Burst Handling · depends_on
  ├─ SPEC-008  Legacy Rate Limiting · supersedes · historical
  ├─ doc://runbooks/rate-limit-rollout  0.956
  └─ doc://guides/api-rate-limits       0.902

  DOC DRIFT 1 item · 3 remediations
  └─ doc://guides/api-rate-limits  ██ DRIFT
     → 3 suggested edits (see check-doc-drift for detail)

  COMPARISON  prefer SPEC-042 as the primary reference
```

### Color mapping

- Section headers (`OVERLAP`, `IMPACT`, `DOC DRIFT`, `COMPARISON`) → bold white
- Tree chrome (`├─`, `└─`) → dim
- Status blocks → red/green/yellow per severity
- Spec IDs → cyan throughout

---

## Output Redesign: status

### Proposed output (compact)

```
━━◈ pituitary status

  workspace  ~/devel/pituitary
  config     .pituitary/pituitary.toml
  index      .pituitary/pituitary.db · fresh
  specs      5     docs  2     chunks  23
  graph      valid
  embedder   fixture · deterministic
```

Color: labels dim, values bold white, `fresh` green, `stale` red, `valid` green.

---

## Output Redesign: init

### Proposed output

```
━━◈ pituitary init

  discovered 3 sources
    specs         spec_bundle       specs/           5 bundles
    docs          markdown_docs     docs/            2 files
    contracts     markdown_contract rfcs/            0 files

  wrote .pituitary/pituitary.toml
  index rebuilt: 5 specs · 2 docs · 23 chunks
  graph: valid

  ██ ready — run pituitary check-doc-drift --scope all to see findings
```

---

## Implementation Notes

### Terminal color detection

- Check `NO_COLOR` env var (https://no-color.org/) — if set, emit plain text
- Check `TERM=dumb` — no colors
- Check if stdout is a TTY — no colors if piped (preserves machine-readability)
- `--format json` always emits plain JSON regardless of color settings
- `--color=always|never|auto` override flag

### Unicode considerations

- The box-drawing characters (`├─`, `└─`, `━━◈`) require UTF-8 terminal
- Fall back to ASCII (`|--`, `+--`, `---*`) if `LANG` doesn't include UTF-8
- The `██` / `▒▒` / `░░` blocks are Unicode Block Elements — widely supported
- Alternatively use plain text labels `[DRIFT]` `[ALIGNED]` `[HIGH]` `[MED]` with color only

### What `--format json` should NOT do

- No ANSI codes in JSON output, ever
- JSON format remains the stable machine interface
- Color is purely a human-facing presentation layer
