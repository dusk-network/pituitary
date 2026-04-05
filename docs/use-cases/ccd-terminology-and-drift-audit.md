# Use Case: Catching Silent Drift in a Fast-Moving AI Tooling Repo

**Repo:** [dusk-network/ccd](https://github.com/dusk-network/ccd) (Continuous Context Development)
**Corpus:** 11 accepted specs, 29 docs, 5 canonical skills
**Runtime:** Local LLMs on Apple Silicon (M2 Ultra) -- no cloud API keys

---

## The Problem

CCD is an actively developed framework for AI coding agent session management. Its spec base formalizes contracts for session workflow, runtime state, memory promotion, backlog extensions, and more. The documentation spans a quickstart guide, architecture deep-dives, a cheatsheet, and skill definitions shipped to multiple AI agent platforms.

Over several months of development, two things drifted silently:

1. **Terminology evolved but docs didn't follow.** Internal concepts were renamed (`repo_id` became `locality`, `focus` became `dispatch state`, `focus.md` became the dispatch state module) as the domain model matured. The code and specs partially adopted the new terms, but 16 docs and 6 specs still used the old vocabulary -- inconsistently.

2. **Specs superseded artifacts that docs still reference.** The runtime-state-contract spec explicitly scoped out certain file-based artifacts (`handoff.md`, `work_queue.json`, `work_queue.md`) from the active contract. But the README, memory architecture guide, radar skill, and changelog still presented them as part of the runtime state model.

Neither category of drift caused build failures or test regressions. Both would mislead a human or AI agent reading the docs to understand the system.

## Setup

Pituitary was already initialized on the repo with a fixture embedder (8-dimensional placeholder vectors for CI). Switching to real local models took three config changes:

```toml
# .pituitary/pituitary.toml

[runtime.profiles.m2-router]
provider = "openai_compatible"
endpoint = "http://100.92.91.40:1234/v1"
timeout_ms = 30000
max_retries = 1

[runtime.embedder]
profile = "m2-router"
model = "nomic-embed-text-v1.5"    # Mami -- 768-dim embeddings

[runtime.analysis]
profile = "m2-router"
model = "lin"                       # Qwen3.5-9B via SwiftLM
timeout_ms = 300000
```

Terminology policies were declared inline:

```toml
[[terminology.policies]]
preferred = "locality"
historical_aliases = ["repo identity"]
deprecated_terms = ["repo_id"]
docs_severity = "warning"
specs_severity = "error"

[[terminology.policies]]
preferred = "dispatch state"
historical_aliases = ["focus state"]
deprecated_terms = ["focus", "focus.md"]
docs_severity = "warning"
specs_severity = "error"

[[terminology.policies]]
preferred = "next step"
historical_aliases = ["focus selection"]
deprecated_terms = ["focus surface"]
docs_severity = "warning"
specs_severity = "error"
```

Index rebuild with real embeddings:

```sh
pituitary index --rebuild --full
# 408 chunks, 768-dim vectors, 40 artifacts indexed
```

## What Pituitary Found

### Terminology Audit

```sh
pituitary check-terminology --scope all
```

**22 artifacts** flagged across 5 terminology policies. The three deprecated terms accounted for 90 violations:

| Deprecated Term | Preferred | Spec Hits | Doc Hits | Severity |
|----------------|-----------|-----------|----------|----------|
| `focus`        | dispatch state | 5 | 36 | error (specs) / warning (docs) |
| `repo_id`      | locality | 4 | 29 | error (specs) / warning (docs) |
| `focus.md`     | dispatch state | 2 | 14 | error (specs) / warning (docs) |

Historical aliases (tolerated but flagged for cleanup): `session handoff` (8 hits), `repo identity` (3), `backlog dispatch` (2), `focus selection` (1).

The audit distinguished between deprecated terms that actively contradict the current model and historical aliases that are merely outdated but not wrong -- giving the team a clear priority order for cleanup.

### Doc-Drift Scan

```sh
pituitary check-doc-drift --scope all
```

**7 semantic contradictions** across 4 documents, all in the same category: docs presenting file-based artifacts as active runtime state when the accepted spec explicitly excludes them.

| Document | Stale Artifact | Contradicts | Confidence |
|----------|---------------|-------------|------------|
| README.md | `work_queue.json` | runtime-state-contract | 0.786 |
| README.md | `work_queue.md` | runtime-state-contract | 0.786 |
| README.md | `work_queue.json` | backlog-extension | 0.845 |
| README.md | `work_queue.md` | backlog-extension | 0.845 |
| Memory Architecture | `handoff.md` | runtime-state-contract | 0.873 |
| CCD Radar Gate | `handoff.md` | runtime-state-contract | 0.805 |
| CHANGELOG.md | `handoff.md` | runtime-state-contract | 0.834 |

Each finding links to the specific spec section and doc section that contradict each other, with a confidence score based on semantic alignment of the evidence pair.

## What Made This Hard to Catch Manually

- **Scale.** 29 docs and 11 specs produce hundreds of potential contradiction pairs. No reviewer checks them all on every PR.
- **Gradual drift.** The terminology evolved over dozens of commits. No single PR introduced the inconsistency -- it accumulated.
- **Specs as source of truth.** The specs were updated correctly. The drift was in downstream docs that reference the specs indirectly. Traditional linters and tests don't cross that boundary.
- **Agent-facing surface area.** CCD skills are consumed by AI coding agents that treat doc text as literal instructions. A skill that says "write to `focus.md`" when the system actually uses a SQLite-backed dispatch state module will produce wrong agent behavior -- silently.

## Runtime Profile

| Step | Model | Time | Notes |
|------|-------|------|-------|
| Index rebuild | nomic-embed-text-v1.5 (Mami) | ~10s | 408 chunks, 768-dim |
| Terminology audit | Qwen3.5-35B (Bart) | ~60s | 22 findings across full corpus |
| Doc-drift scan | Qwen3.5-9B (Lin) | ~4min | 7 findings, 4 docs checked in detail |

All models ran locally on an M2 Ultra via an MLX router. No data left the local network.

## Outcome

The audit produced a concrete, prioritized remediation list:

1. **6 specs with error-severity deprecated terms** -- highest priority, these are the source of truth
2. **16 docs with stale terminology** -- systematic find-and-replace guided by the policy definitions
3. **4 docs with runtime-contract contradictions** -- targeted edits to remove references to superseded artifacts
4. **14 historical alias occurrences** -- lower priority, can be cleaned up incrementally

Total estimated remediation: a single focused session, with pituitary's JSON output structured enough to drive automated or agent-assisted fixes.
