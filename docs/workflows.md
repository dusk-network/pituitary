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

## Commands

| Command | What it does |
|---|---|
| `init --path .` | One-shot onboarding: discover, write config, rebuild index, report status |
| `discover --path .` | Scan a repo and propose conservative sources |
| `migrate-config --path FILE --write` | Upgrade a legacy config to the current schema |
| `preview-sources` | Show which files each configured source will index |
| `explain-file PATH` | Explain how one file is classified by configured sources |
| `canonicalize --path PATH` | Promote one inferred contract into a spec bundle |
| `index --rebuild [--full]` | Rebuild the SQLite index |
| `index --dry-run` | Validate config and sources without writing |
| `status [--check-runtime all]` | Report index state, config, freshness, runtime readiness |
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

## Diff-Driven Doc Drift

When you already have a patch, `check-doc-drift --diff-file` narrows the stale-doc search to the changed files, the implicated specs, and the docs linked through those specs. The JSON response includes `changed_files`, `implicated_specs`, `implicated_docs`, and the usual `drift_items` / `assessments` payload so agents can explain why each doc was shortlisted.

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

Each matched term includes structured `classification`, `context`, `severity`, and `replacement` fields so CI or editor tooling can turn JSON output into warnings or errors without scraping prose.

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

## CI

`check-compliance --diff-file` is the easiest pre-merge guardrail for spec/code alignment, and `check-doc-drift --diff-file` complements it when you want change-scoped stale-doc detection:

```sh
git diff --cached | pituitary check-compliance --diff-file -
git diff origin/main...HEAD | pituitary check-compliance --diff-file -
git diff --cached | pituitary check-doc-drift --diff-file -
git diff origin/main...HEAD | pituitary check-doc-drift --diff-file -
```

For copy-paste workflow examples that install the released binary in CI and run both compliance and spec-hygiene checks, see [docs/development/ci-recipes.md](development/ci-recipes.md).
