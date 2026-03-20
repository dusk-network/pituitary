# Public API Rate Limits

The public API uses a fixed-window rate limiter.

## Default Limit

The default limit is 100 requests per minute for each API key.

## Configuration

All tenants share the same rate-limit configuration, and tenant-specific overrides are not supported.

## Operational Notes

If clients exceed the fixed-window threshold, they should retry after the next minute boundary.
