## Overview

This spec replaces the legacy public API rate-limiting model with a per-tenant design.

## Requirements

- Apply limits per tenant rather than per API key.
- Enforce a default limit of 200 requests per minute.
- Allow tenant-specific overrides through configuration.

## Design Decisions

- Use a sliding-window limiter rather than a fixed-window counter.
- Keep the shared middleware path but load tenant-specific limits.
- Preserve compatibility with existing rate-limit configuration keys where possible.
