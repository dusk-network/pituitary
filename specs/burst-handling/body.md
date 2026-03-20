## Overview

This spec defines short burst behavior on top of the per-tenant rate-limiting model.

## Requirements

- Allow short bursts above the steady-state tenant limit.
- Derive burst configuration from the same tenant-scoped limit settings defined by SPEC-042.
- Reject requests once both the steady-state budget and burst budget are exhausted.

## Design Decisions

- Model burst capacity as an additive tenant-level budget.
- Keep burst configuration in the same config hierarchy introduced by SPEC-042.
- Reuse the same middleware integration point as the main rate limiter.
