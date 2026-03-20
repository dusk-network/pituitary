## Overview

This spec defines the original rate-limiting behavior for public API endpoints.

## Requirements

- Apply limits per API key.
- Enforce a default limit of 100 requests per minute.
- Use one global configuration for all tenants.

## Design Decisions

- Use a fixed-window counter because it is simple to reason about.
- Keep configuration static and shared across tenants.
- Do not introduce burst-specific handling in this version.
