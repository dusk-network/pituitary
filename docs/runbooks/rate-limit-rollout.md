# Rate Limit Rollout Runbook

This runbook assumes the per-tenant rate-limiting model defined in `SPEC-042` and the burst behavior defined in `SPEC-055`.

## Preparation

- Confirm that tenant-scoped limit configuration is present.
- Verify the default rate limit is 200 requests per minute unless a tenant override is configured.
- Confirm burst settings are defined in the same tenant configuration tree.

## Rollout

- Enable the new sliding-window limiter in the shared middleware path.
- Roll out tenant overrides gradually.
- Watch rejection rates for tenants that receive custom limits or burst budgets.

## Rollback

- Remove tenant-specific overrides.
- Disable burst handling first if overload behavior becomes difficult to interpret.
