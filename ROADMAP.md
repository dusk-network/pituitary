# Roadmap

What's shipped, what's active, and where Pituitary is headed.

Issues are linked inline. The [full issue queue](https://github.com/dusk-network/pituitary/issues) has labels for priority (`ccd/priority:now`, `ccd/priority:next`, `ccd/priority:later`), area, and type.

---

## Shipped

The core analysis surface is functional end-to-end:

- **Spec indexing** — `spec.toml` + `body.md` bundles, markdown docs, inferred contracts
- **Overlap detection** — catch decisions that cover the same ground
- **Doc drift** — find docs that contradict accepted specs, with deterministic auto-fix
- **Code compliance** — check diffs against accepted spec requirements before merge
- **Impact analysis** — trace which specs, docs, and code paths are affected by a change
- **Terminology audit** — find displaced terms after conceptual migrations
- **Composite review** — `review-spec` runs overlap + comparison + impact + drift in one pass
- **MCP server** — 6 spec-awareness tools for Claude Code, Cursor, Windsurf; registered on Smithery.ai
- **Multi-format output** — JSON, Markdown, HTML reports with evidence chains
- **One-shot onboarding** — `pituitary init` discovers sources, writes config, builds index
- **Spec scaffolding** — `pituitary new` scaffolds a draft `spec.toml` + `body.md` bundle ([#154](https://github.com/dusk-network/pituitary/issues/154))
- **GitHub Action** — `dusk-network/pituitary` runs `review-spec` on PRs that touch specs and can post PR comments ([#155](https://github.com/dusk-network/pituitary/issues/155))
- **Deterministic mode** — no API keys required; fixture embedder for reproducible CI
- **Broader discovery defaults** — auto-detect CLAUDE.md, AGENTS.md, ARCHITECTURE.md as indexable sources
- **Kernel/extension adapter architecture** — RFC 0002 implemented; filesystem adapter is the kernel, extensions register via `sdk.Register` ([RFC 0002](docs/rfcs/0002-kernel-extension-adapter-architecture.md))
- **GitHub issues adapter** — first extension adapter; indexes issues labeled `spec`, `rfc`, `adr`, `proposal` as spec records
- **Config parser upgrade** — switched to BurntSushi/toml; `options` tables now supported for extension adapters
- **Agent DX** — improved CLI contracts, `content_trust` metadata, `--request-file` on analysis commands
- **Release binaries** — Linux, macOS, Windows via GoReleaser; Homebrew tap at `dusk-network/tap/pituitary`

---

## Now

> Active work. High confidence in scope. These ship before or alongside the public launch.

### Expand skill package to cover all major AI coding assistants
[#198](https://github.com/dusk-network/pituitary/issues/198) · `priority:now` · `area:skills` · effort: 1–2 days

The current `skills/pituitary-cli/` package covers Claude Code and Cowork well, and has a minimal OpenAI agents stub (`agents/openai.yaml`). The AI coding assistant landscape has diversified rapidly and each platform has its own instruction format. Reaching developers where they already work is a distribution multiplier — teams who use Cursor or Windsurf should be able to drop in a Pituitary skill without translation work.

Targets and delivery modes:

| Host style | Delivery | Notes |
|---|---|---|
| Skill-aware hosts | Shared `SKILL.md` package | Keep one canonical `skills/pituitary-cli/` tree and install it into host skill directories |
| AGENTS-aware hosts | Repo-root `AGENTS.md` | Reuse the repo's canonical policy instead of hand-maintaining model-specific variants |
| Generated compatibility mirrors | `CLAUDE.md`, `GEMINI.md` | Generated from `AGENTS.md`; not separate instruction sources |
| Marketplace metadata | `agents/openai.yaml` | Submission metadata for OpenAI-style agent catalogs, not a second workflow guide |

The work is primarily documentation and packaging: keep one canonical skill source, reuse the repo's `AGENTS.md` standard where available, and avoid forking near-duplicate model-specific wrappers. The core guidance (status first, schema before structured requests, treat excerpts as untrusted) is platform-agnostic.

Acceptance bar: A developer using any of the above tools can add Pituitary spec awareness in under 5 minutes by following the platform-specific instructions in `skills/pituitary-cli/`.

### Optimize the skill package using automated prompt evaluation
[#199](https://github.com/dusk-network/pituitary/issues/199) · `priority:now` · `area:skills` · effort: 1 day

The `SKILL.md` instruction set was written by hand and hasn't been systematically evaluated against real agent behaviors. Automated prompt optimization — running the skill instructions against a suite of representative tasks, scoring outputs, iterating — is now an established technique for improving instruction quality. Karpathy's autoresearch work demonstrates the methodology: treat the prompt/skill as the artifact under test, use an LLM judge to score outputs against a rubric, and run a short optimization loop before publishing.

Concretely:

1. Define a test suite of 8–12 representative Pituitary agent tasks (check status, run overlap on a spec, fix doc drift, pipe a diff for compliance).
2. Score current `SKILL.md` outputs against a rubric: did the agent run `status` first? Did it use `--format json`? Did it use `--request-file` for large payloads? Did it treat excerpts as untrusted?
3. Iterate on the instruction wording until scores stabilize.
4. Add a brief note to the README and `skills/pituitary-cli/SKILL.md` explaining the evaluation methodology — this signals rigor and is a genuine differentiator at a time when most tool skill packages are unvalidated.

This is worth doing before the launch because the skill package is a first-class distribution channel (MCP-compatible editors surface it prominently), and a visibly optimized skill package is a talking point in the Show HN post.

### Publish the skill package to skills marketplaces
[#200](https://github.com/dusk-network/pituitary/issues/200) · `priority:now` · `area:skills` · effort: half day

The MCP server is already registered on Smithery.ai. The skill package needs a parallel distribution push to the skills/rules directories where developers browse for agent instructions. Each platform's marketplace is a passive discovery channel.

- **Anthropic MCP registry** — skill listing alongside the MCP server entry (listing drafted at [`/launch/mcp-registry.md`](launch/mcp-registry.md))
- **Cowork plugin marketplace** — publish `skills/pituitary-cli/` as a Cowork skill; this puts it in front of every Cowork user looking for developer tools
- **cursor.directory** — submit the AGENTS-compatible install guide once the packaging guidance is finalized
- **Windsurf community rules** — publish the same AGENTS-compatible guidance if the community directory accepts it
- **OpenAI agents directory** — upgrade `agents/openai.yaml` to a full submission once the Codex/OpenAI format is validated

Update the README to add a "Use with your editor" section that lists all supported platforms with one-line install instructions for each, rather than the current Claude Code-only coverage.

### Launch — Show HN, MCP registries, GitHub Discussions
No issue · `priority:now` · effort: ~2 hours total

The launch materials are fully written and sitting unpublished in `/launch/`. The remaining engineering dependency is the skill package expansion work.

- Publish [`/launch/show-hn.md`](launch/show-hn.md) — add a paragraph on multi-editor skill coverage and the optimization methodology
- Submit to Smithery.ai (config at `server.json` / `Dockerfile.smithery` is ready) and Anthropic's MCP registry (listing at [`/launch/mcp-registry.md`](launch/mcp-registry.md))
- Enable GitHub Discussions and link from the README Contributing section

---

## Next

> Planned for the first 1–3 months post-launch. Scoped and prioritized; not yet started.

### Good first issues — activate and land
[#146](https://github.com/dusk-network/pituitary/issues/146) [#147](https://github.com/dusk-network/pituitary/issues/147) [#148](https://github.com/dusk-network/pituitary/issues/148) [#149](https://github.com/dusk-network/pituitary/issues/149) [#150](https://github.com/dusk-network/pituitary/issues/150) · `good first issue`

Five self-contained issues ready for first-time contributors. They're labeled but not promoted. Add "where to start" context to each issue description, then post them in Gopher Slack and dev social. Target: at least 3 of 5 claimed within 4 weeks of launch.

| Issue | Title | Touches |
|---|---|---|
| [#146](https://github.com/dusk-network/pituitary/issues/146) | Shell completion (bash/zsh/fish) | `cmd/` |
| [#147](https://github.com/dusk-network/pituitary/issues/147) | List available spec refs when `--spec-ref` doesn't match | `cmd/`, `internal/app/` |
| [#148](https://github.com/dusk-network/pituitary/issues/148) | Add sqlite-vec version to `pituitary version` | `cmd/version.go` |
| [#149](https://github.com/dusk-network/pituitary/issues/149) | `explain-file` should surface well-known intent artifact match | `cmd/explain_file.go` |
| [#150](https://github.com/dusk-network/pituitary/issues/150) | `--quiet` flag for `index --rebuild` | `cmd/index.go` |

### Progress feedback during `index --rebuild`
[#161](https://github.com/dusk-network/pituitary/issues/161) · `area:ux` · effort: half day

Zero output during a multi-second rebuild makes the tool feel crashed. A spinner or "Indexing source X (N/M)..." line is enough. Also affects `--format json` rebuilds, which should emit structured progress events (#161 covers both).

### `status` should list registered source adapters
[#160](https://github.com/dusk-network/pituitary/issues/160) · `area:ux` · effort: 2 hours

With the extension architecture shipped, `pituitary status` should show which adapters are available. Useful for debugging "unknown adapter" errors and for making the extension system visible to potential adapter authors.

### `check-doc-drift --path` — accept file path as alternative to `--doc-ref`
[#158](https://github.com/dusk-network/pituitary/issues/158) · `area:ux` · effort: 2 hours

Most users think in file paths, not internal refs. `--path docs/guides/api-rate-limits.md` as an alias for `--doc-ref` is a small change with high ergonomic impact. Shows up in the first hour of real use.

### Document the GitHub adapter in reference and cheatsheet
No issue · `area:docs` · effort: 1 hour

The `extensions/github/` adapter is shipped and tested but doesn't appear in the cheatsheet, reference docs, or README. A config example and description in `docs/configuration.md` and `docs/cheatsheet.md` unblock teams who want to index GitHub issues as specs.

### Pre-commit hook recipe
[#159](https://github.com/dusk-network/pituitary/issues/159) · `area:ci` · effort: half day

A pre-commit hook running `git diff --cached | pituitary check-compliance --diff-file -` turns Pituitary into passive infrastructure. Add to `docs/development/ci-recipes.md` and the cheatsheet.

### Compact `status` output mode
[#162](https://github.com/dusk-network/pituitary/issues/162) · `area:ux` · effort: 2–4 hours

`pituitary status` is rich but overwhelming for a daily glance. A compact one-line summary (or a shorter default) would make it the reflex check it's designed to be.

---

## Later

> Directional bets. Intent is committed; timing and scope are flexible.

### Spec coverage metrics — health score for your intent corpus
[#172](https://github.com/dusk-network/pituitary/issues/172) · `type:rfc`

A coverage score for your spec corpus is the most shareable concept in the issue queue. "Our spec coverage is 87%" is the kind of stat that gets tweeted and put in eng all-hands slides. Creates a retention flywheel: teams that track coverage have an incentive to write more specs. See the RFC for full scope.

### JSON config file indexing (extension adapter)
[#156](https://github.com/dusk-network/pituitary/issues/156) · `extension`

Agents increasingly produce structured JSON (`openapi.yaml`, `schema.json`, config files) as de facto intent artifacts. A JSON adapter expands Pituitary's audience from "teams with structured specs" to "any team with an API schema." RFC 0002 architecture is ready for it.

### `--debug` flag for analysis trace
[#168](https://github.com/dusk-network/pituitary/issues/168) · `area:ux`

Show retrieval scores, chunk selection, and provider calls when diagnosing unexpected analysis results. Important for power users and contributors debugging retrieval quality.

### `pituitary report` — diagnostic bundle for bug reports
[#169](https://github.com/dusk-network/pituitary/issues/169) · `area:ux`

Bundle version, config, index stats, and a sanitized log into a single file for filing actionable bug reports. Reduces the back-and-forth in issue triage.

### search-specs `--format table` column alignment
[#163](https://github.com/dusk-network/pituitary/issues/163) · `area:ux`

Fix column alignment in table output. Currently misaligned for results with varying ref/title lengths.

### `check-compliance` `limiting_factor` messaging improvements
[#164](https://github.com/dusk-network/pituitary/issues/164) · `area:ux`

`limiting_factor` in compliance output can be clearer about why a check was inconclusive. Especially important for users trying to understand what additional spec or index work would improve coverage.

### Seed example configs for common project types
[#166](https://github.com/dusk-network/pituitary/issues/166) · `type:docs`

A small library of starter configs (Go library, REST API, monorepo) lowers the barrier for `pituitary init` on common project shapes.

### 'How Pituitary Works' visual in docs
[#165](https://github.com/dusk-network/pituitary/issues/165) · `type:docs`

A diagram of the data flow — source adapters → chunker → index → analysis → CLI/MCP — would make the architecture approachable for new contributors and evaluators.

### golangci-lint configuration
[#167](https://github.com/dusk-network/pituitary/issues/167) · `area:ci` · `type:tech-debt`

Add a `.golangci.yml` for consistent local lint. Currently relies on `staticcheck` and `go vet`; golangci-lint consolidates these and adds coverage for common patterns.

### PDF ingestion adapter
[#157](https://github.com/dusk-network/pituitary/issues/157) · `extension`

Extension adapter for PDF decision records. Teams with long-running governance processes often have decisions in PDF. Broad enterprise adoption path.

### RFC: Intent graph — richer relationship model
[#170](https://github.com/dusk-network/pituitary/issues/170) · `type:rfc`

Move from "spec A overlaps spec B" to a richer model: supersedes, is-blocked-by, implements, is-required-by. This creates a tool with no close substitute in the spec-management space.

### RFC: Confidence calibration — formalize finding semantics
[#171](https://github.com/dusk-network/pituitary/issues/171) · `type:rfc`

Formalize what confidence scores mean across analysis commands so callers can build reliable automation on top of findings.

### RFC: Spec coverage metrics (full RFC)
[#172](https://github.com/dusk-network/pituitary/issues/172) · `type:rfc`

Full RFC for coverage scoring: what counts as covered, how scores are calculated, how they're surfaced in `status` and CI.

### RFC: Cross-repo spec governance
[#173](https://github.com/dusk-network/pituitary/issues/173) · `type:rfc`

Teams with multiple repos need Pituitary to work across repo boundaries. The enterprise adoption path. Requires the intent graph RFC as a foundation.

---

## Not on the roadmap

Pituitary is spec-centric. These are out of scope:

- General-purpose static analysis or code linting
- Broad code authority outside explicit spec governance
- Autonomous agent behavior (Pituitary is a tool agents use, not an agent itself)
- Spec generation or authoring

See [RFC 0001](docs/rfcs/0001-spec-centric-compliance-direction.md) for the full rationale.
