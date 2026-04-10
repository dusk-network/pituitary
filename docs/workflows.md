# Workflows and Reports

This page covers higher-level workflows: compliance, review reports, CI composition, and the full command surface.

## Compliance Traceability

`check-compliance` is strongest when specs declare governed surfaces through `applies_to`:

```toml
applies_to = [
  "code://src/api/middleware/ratelimiter.go",
  "config://src/api/config/limits.yaml",
]
```

When a changed path has no explicit governance, findings include a `limiting_factor`:

- `spec_metadata_gap`: missing `applies_to`; tighten governance in the spec
- `code_evidence_gap`: governance is explicit, but the code does not expose enough literal evidence

The JSON result also carries `unspecified_summary`, which splits `unspecified` findings into `missing_governance_edge` versus `explicit_but_underexercised` so CI and operators do not treat those remediations as the same class of problem.

Text output now also promotes the strongest per-finding guidance into a short `TOP SUGGESTIONS` block, and JSON mirrors that guidance in `result.top_suggestions`, so operators do not need to dig through every individual finding to see the best next action.

## Commands

| Command | What it does |
|---|---|
| `init --path .` | One-shot onboarding: discover, write config, rebuild index, report status |
| `discover --path .` | Scan a repo and propose conservative sources |
| `migrate-config --path FILE --write` | Upgrade a legacy config to the current schema |
| `preview-sources` | Show which files each configured source will index |
| `preview-sources --verbose` | Add rejected candidates plus selector-match diagnostics |
| `explain-file PATH` | Explain how one file is classified by configured sources |
| `canonicalize --path PATH` | Promote one inferred contract into a spec bundle |
| `index --rebuild [--full]` | Rebuild the SQLite index |
| `index --update [--show-delta]` | Incremental update: diff and write only changed artifacts |
| `index --dry-run` | Validate config and sources without writing |
| `status [--check-runtime all] [--show-families]` | Report index state, config, freshness, runtime readiness, spec families |
| `version` | Print version info |
| `search-specs --query "..."` | Semantic search across indexed spec sections |
| `check-overlap --path SPEC` | Detect specs that cover overlapping ground |
| `compare-specs --path A --path B` | Side-by-side tradeoff analysis |
| `analyze-impact --path SPEC` | Trace what is affected by a change |
| `check-terminology --spec-ref Z` | Terminology governance audit from configured `[[terminology.policies]]` |
| `check-terminology --term X --canonical-term Y --spec-ref Z` | Ad hoc terminology migration audit |
| `check-compliance --path PATH` | Check code paths against accepted specs |
| `check-compliance --diff-file PATH\\|-` | Check a unified diff against accepted specs |
| `check-doc-drift --scope all\\|SPEC_REF` | Find docs that have gone stale across the workspace |
| `check-doc-drift --diff-file PATH\\|-` | Rank stale docs and specs implicated by a unified diff |
| `fix --path PATH --dry-run` | Preview deterministic doc-drift remediations before writing |
| `fix --scope all --yes` | Apply deterministic doc-drift remediations without prompting |
| `review-spec --path SPEC` | Full review: overlap + comparison + impact + drift + remediation |
| `schema COMMAND --format json` | Inspect the machine-readable request/response contract for one command |
| `serve --config FILE` | Start MCP server over stdio |

`fix` is intentionally narrow: it only applies deterministic `replace_claim` remediations that `check-doc-drift` can justify from accepted specs and exact document evidence. Use `--dry-run` first, then rerun with `--yes` when the replacements look correct. After any successful apply, run `pituitary index --rebuild`.

When file selection or classification looks wrong, run `pituitary explain-file PATH` before anything else. It is the fastest way to confirm which source matched the file, which selectors fired, and whether the path was excluded on purpose.

## Diff-Driven Doc Drift

When you already have a patch, `check-doc-drift --diff-file` narrows the stale-doc search to the changed files, the implicated specs, and the docs linked through those specs. The JSON response includes `changed_files`, `implicated_specs`, `implicated_docs`, and the usual `drift_items` / `assessments` payload so agents can explain why each doc was shortlisted.

Each drift finding and remediation suggestion now also carries a section-level evidence chain:

- source refs for the accepted spec section and the drifting doc section
- a `classification` and `link_reason`
- likely edit targets such as `target_source_ref` / `target_section`
- `suggested_bullets` for the next manual edit step when deterministic auto-fix is not appropriate

```sh
git diff --cached | pituitary check-doc-drift --diff-file -
git diff origin/main...HEAD | pituitary check-doc-drift --diff-file - --format json
```

## Terminology Governance

When you declare `[[terminology.policies]]` in `pituitary.toml`, `check-terminology` no longer needs a repeated term list for common migrations:

```sh
pituitary check-terminology --spec-ref SPEC-LOCALITY
pituitary check-terminology --scope docs --format json
```

The result now separates:

- `findings`: actionable current-state violations
- `tolerated`: historical or compatibility-only uses that are still indexed for context

Use `[terminology].exclude_paths` when specific files or folders are historically frozen and should be skipped by terminology sweeps and `compile` without being removed from the wider index.

Each matched term includes structured `classification`, `context`, `severity`, and `replacement` fields so CI or editor tooling can turn JSON output into warnings or errors without scraping prose.

`check-compliance`, `check-doc-drift`, `check-terminology`, and `compile` also accept `--timings` with JSON output. The top-level CLI envelope then includes `total_ms`, `indexing_ms`, `embedding_ms`, `analysis_ms`, and call counts so local runs and CI can spot unexpected runtime regressions.

## Temporal Governance Queries

Analysis commands support `--at DATE` (YYYY-MM-DD) for point-in-time governance. When a spec is superseded or deprecated, its governance edges get `valid_to` set automatically — the historical links are preserved but excluded from current queries.

```sh
pituitary governed-by --path src/api/ratelimiter.go --at 2026-03-15
pituitary check-compliance --diff-file - --at 2026-03-01
pituitary check-doc-drift --scope all --at 2026-03-20
pituitary analyze-impact --spec-ref SPEC-042 --at 2026-02-15
```

Temporal values are populated from the index rebuild timestamp. Edges carry `valid_from` (when created) and `valid_to` (when closed by supersession or deprecation). `valid_to IS NULL` means the edge is currently active.

## Confidence-Weighted Governance

Governance edges carry a confidence tier and numeric score:

| Tier | Source | Score |
|------|--------|-------|
| **extracted** | Declared in spec.toml (`applies_to`, `depends_on`, `supersedes`) | 1.0 |
| **inferred** | AST symbol matching during tree-sitter pass | 0.7 |
| **ambiguous** | Weak or conflicting signals | 0.1–0.3 |

Use `--min-confidence` to control the trust threshold:

```sh
pituitary check-compliance --diff-file - --min-confidence extracted   # strict: declared only
pituitary check-doc-drift --scope all --min-confidence inferred       # broader: include inferred
pituitary governed-by --path src/api/ratelimiter.go --min-confidence extracted
```

The `governed-by` JSON output includes `confidence` and `confidence_score` on each governing spec, so consumers can weight results programmatically.

## Deliberate Deviation vs Accidental Drift

When `check-compliance` finds a conflict, it checks for rationale comments in the source code near the conflicting code:

- **Recognized tags:** `// WHY:`, `// RATIONALE:`, `// NOTE:`, `// HACK:`, `// FIXME:`, `// TODO:` (with language-appropriate variants for Python, Rust, etc.)
- **Decision language:** Comments containing "because," "instead of," "chose," "trade-off," "deliberately," "intentionally"

The compliance finding is classified as:

- `deliberate_deviation` — rationale found. The remediation path is: update the spec to reflect the decision, or update the code.
- `unintentional_drift` — no documented rationale. The remediation path is: fix the code to match the spec.

The JSON output includes `classification`, `rationale_text`, `rationale_kind`, and `rationale_symbol` in the evidence chain.

## Governance Graph Delta

`index --update --show-delta` reports what changed in the governance posture since the last rebuild:

```sh
pituitary index --update --show-delta
```

```
Governance delta since last rebuild:
  + SPEC-043 added (status: draft, domain: auth)
  + SPEC-042 now governs 3 additional files (inferred)
  - SPEC-008 superseded by SPEC-042
  ~ doc://guides/api-rate-limits: was aligned, now drifting
  summary: 1 spec(s) added, 2 edge(s) added, 1 edge(s) removed
```

JSON output includes the full `delta` object with `added_specs`, `removed_specs`, `updated_specs`, `added_edges`, `removed_edges`, `updated_edges`, and a `summary` string.

## Spec Families

`status --show-families` runs community detection (Louvain algorithm) on the spec dependency graph to discover natural governance clusters:

```sh
pituitary status --show-families
```

Connections are built from `depends_on`, `supersedes`, `relates_to` edges and shared `applies_to` targets. The output includes:

- **Families** with member lists, sizes, and cohesion scores (intra-family edge density)
- **Ungoverned files** — source files in `ast_cache` not covered by any `applies_to` edge (coverage gaps between families)

Use `search-specs --family N` to restrict search results to a specific family. `analyze-impact` annotates impacted specs that cross family boundaries with `cross_family: true` in JSON output.

## Agent-Friendly Input

For shell-driven agents, prefer JSON transport instead of long flag lists:

```sh
pituitary compare-specs --request-file request.json --format json
pituitary check-doc-drift --request-file request.json --format json
pituitary check-compliance --request-file request.json --format json
```

`--request-file PATH|-` keeps requests explicit, avoids shell-escaping mistakes, and is workspace-scoped by default for local file inputs. For diff-driven drift or compliance checks, prefer embedding `diff_text` directly in the request JSON, or provide `diff_file` when you want the CLI to resolve the patch from disk or stdin first.

## Review Reports

`review-spec` is the compound workflow. It composes:

- overlap detection
- comparison
- impact analysis
- doc drift
- remediation suggestions

Use `--format markdown` for PR-friendly reports and `--format html` for a richer shareable report with expandable evidence.

`analyze-impact` uses the same pattern for docs it shortlists: each impacted doc can include a `classification`, a source-linked `evidence` object, and `suggested_targets` with likely doc sections to inspect first.

## CI

`check-compliance --diff-file` is the easiest pre-merge guardrail for spec/code alignment, and `check-doc-drift --diff-file` complements it when you want change-scoped stale-doc detection:

```sh
git diff --cached | pituitary check-compliance --diff-file -
git diff origin/main...HEAD | pituitary check-compliance --diff-file -
git diff --cached | pituitary check-doc-drift --diff-file -
git diff origin/main...HEAD | pituitary check-doc-drift --diff-file -
```

For copy-paste workflow examples that install the released binary in CI and run both compliance and spec-hygiene checks, see [docs/development/ci-recipes.md](development/ci-recipes.md).
