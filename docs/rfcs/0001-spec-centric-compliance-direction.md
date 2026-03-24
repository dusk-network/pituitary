# RFC 0001: Pituitary Compliance Direction

## Status

Accepted

## Date

2026-03-24

## Related

- Issue: [#78](https://github.com/dusk-network/pituitary/issues/78)

## Summary

Pituitary should remain specification-first. Code compliance stays in scope only as a narrow bridge from accepted specifications to explicit, bounded enforcement points. Pituitary is not adopting a roadmap to become a general-purpose static-analysis platform or a broad code-authority product.

## Context

Pituitary's strongest current value is in specification and documentation workflows:

- discovery and indexing of canonical spec corpora
- overlap and comparison analysis
- impact analysis
- terminology migration support
- documentation drift detection
- review workflows for spec-led repo changes

The repo also ships a deterministic `check-compliance` slice, but it is intentionally limited. It helps trace code or diffs back to accepted specs, yet it does not provide deep semantic certainty or broad language-aware authority.

The product risk is drift: if Pituitary expands too far into code analysis, it starts competing in a crowded tooling category and dilutes the clearer specification-management story that is already working.

## Options Considered

### Option A: Stay Spec-Centric

Keep Pituitary focused on specifications and documentation. Treat code compliance as a secondary helper and avoid deeper enforcement ambitions.

### Option B: Spec-Centric Core with Narrow Compliance Bridge

Keep specifications as the product center, but allow compliance work when it directly reinforces specification workflows and remains explicitly bounded.

### Option C: Expand Toward Full Code Authority

Reposition Pituitary toward being a primary code/spec authority layer with deeper analyzer integrations, broader semantic coverage, and CI-grade enforcement semantics.

## Decision

Adopt Option B with strict guardrails.

That means:

- Pituitary remains specification-first.
- Specification and documentation workflows stay ahead of compliance work in priority.
- Compliance is allowed only when it is directly traceable to accepted spec requirements.
- Pituitary does not adopt a general-purpose static-analysis mission.
- Pituitary does not claim broad code authority outside explicit covered requirements.

This keeps the existing `check-compliance` surface valid, but constrains future expansion so it supports the core product instead of redefining it.

## Guardrails

If compliance evolves further, all of the following remain true:

- Specs remain the primary source of truth.
- Every stronger compliance check must trace back to an explicit accepted spec requirement.
- The product must abstain when coverage is insufficient.
- Passing compliance checks must not imply full semantic correctness.
- Messaging must not position Pituitary as a general code-analysis tool.
- Implementation choices must work for the languages that matter to active users, not just the language Pituitary is written in.

## Implications

Near-term product work should continue to prioritize:

- stronger spec review workflows
- better drift and terminology auditing
- higher-confidence retrieval and explanation
- clearer onboarding and adoption paths

Compliance work remains valid only when it reinforces those workflows. Examples:

- better traceability from accepted specs to code or config surfaces
- clearer diff-based compliance outputs for pre-merge checks
- explicit machine-checkable coverage markers in spec metadata

Out of scope for this decision:

- committing to a language-specific analyzer stack
- turning Pituitary into a general CI policy engine
- marketing Pituitary as an authoritative code verifier

## Open Questions

This RFC resolves direction, not every implementation detail. Open questions still include:

- how to represent machine-checkable requirement coverage in accepted specs
- how far diff-based compliance should go before it becomes product drift
- what abstention and confidence semantics should be exposed in future compliance output

## Near-Term Conclusion

Pituitary should continue investing primarily in specification management. Compliance remains a supporting feature, useful when tightly bounded, but not the product center of gravity.
