# Pituitary Cheatsheet

Quick command map for the common workflows. For the full install, config, runtime, and workflow details, start with the [reference index](reference.md).

## Onboarding

```sh
pituitary init --path .                         # discover + config + index + status
pituitary init --path . --dry-run               # preview without writing
pituitary discover --path .                     # propose sources (lower-level)
pituitary preview-sources                       # show what will be indexed
pituitary explain-file README.md                # why is this file in/out of scope?
```

## Indexing

```sh
pituitary index --rebuild                       # build/rebuild, reuse unchanged embeddings
pituitary index --rebuild --full                # force complete re-embed
pituitary index --dry-run                       # validate without writing
pituitary status                                # index health, config, freshness
pituitary status --check-runtime embedder       # probe embedder readiness
pituitary status --check-runtime all            # probe embedder + analysis readiness
```

## Analysis

```sh
pituitary search-specs --query "rate limiting"  # semantic search
pituitary check-overlap --path specs/X          # find overlapping specs
pituitary compare-specs --path specs/A --path specs/B  # side-by-side tradeoff
pituitary analyze-impact --path specs/X         # what's affected if this changes?
pituitary check-doc-drift --scope all           # find stale docs
pituitary check-doc-drift --scope SPEC-042      # drift for one spec only
pituitary check-terminology --term repo \
  --canonical-term locality --spec-ref SPEC-042 # terminology migration audit
pituitary fix --path docs/guides/api-rate-limits.md --dry-run  # preview deterministic drift fixes
pituitary fix --scope all --yes                 # apply deterministic fixes without prompting
```

## Compliance

```sh
pituitary check-compliance --path src/api/      # check code paths against specs
git diff --cached | pituitary check-compliance --diff-file -    # pre-commit
git diff origin/main...HEAD | pituitary check-compliance --diff-file -  # CI
```

## Review

```sh
pituitary review-spec --path specs/X            # full composite review
pituitary review-spec --path specs/X --format markdown  # shareable report
pituitary review-spec --path specs/X --format html      # rich HTML report
pituitary review-spec --path specs/X --format json      # machine-readable
```

## Runtime Setup

```sh
pituitary status --check-runtime embedder       # verify local embeddings runtime
pituitary status --check-runtime analysis       # verify optional analysis runtime
pituitary index --rebuild                       # rebuild after runtime changes
```

## Spec Management

```sh
pituitary canonicalize --path rfcs/sla.md       # promote inferred contract to bundle
pituitary migrate-config --path pituitary.toml --write  # upgrade legacy config
```

## MCP Server

```sh
pituitary serve --config pituitary.toml         # start MCP server over stdio
```

Tools exposed: `search_specs`, `check_overlap`, `compare_specs`, `analyze_impact`, `check_doc_drift`, `review_spec`.

## Output Formats

- All commands: `--format json`
- `search-specs`: `--format table`
- `review-spec`: `--format markdown`, `--format html`

## Useful Flags

```sh
--path          # workspace-relative path (accepts dirs, spec.toml, body.md)
--config        # explicit config file path
--format json   # machine-readable output
--check-runtime # probe runtime dependencies (embedder, analysis, all)
--dry-run       # validate without side effects (index, init)
--full          # force complete re-embed (index --rebuild)
--yes           # apply planned fixes without prompting (fix)
```

## Typical Flows

```sh
# First run on an existing repo
pituitary init --path .
pituitary check-doc-drift --scope all
git diff --cached | pituitary check-compliance --diff-file -

# Full review before a spec change lands
pituitary review-spec --path specs/X --format markdown
pituitary analyze-impact --path specs/X

# Drift remediation loop
pituitary check-doc-drift --scope all
pituitary fix --scope all --dry-run
pituitary fix --scope all --yes
pituitary index --rebuild
```
