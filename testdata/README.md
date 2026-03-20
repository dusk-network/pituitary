# Test Data

This directory holds test-only fixtures for the bootstrap repository state.

## Bootstrap Expectations

`bootstrap_expectations.json` is the executable expectation set for the shipped fixture workspace:

- `SPEC-008` is the superseded historical rate-limit spec
- `SPEC-042` supersedes `SPEC-008`
- `SPEC-055` depends on `SPEC-042`
- `SPEC-008` and `SPEC-042` intentionally overlap on the same governed refs
- `docs/guides/api-rate-limits.md` is intentionally drifted
- `docs/runbooks/rate-limit-rollout.md` is intentionally aligned

## Malformed Bundles

Malformed spec-bundle cases live under `invalid-spec-bundle/`.

- `invalid-spec-bundle/missing-body/` reserves a bundle whose `spec.toml` references a body file that does not exist
- `invalid-spec-bundle/pituitary.toml` points the loader at those malformed bundle fixtures during tests
