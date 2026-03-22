Ref: DOG-001
Status: accepted
Domain: product

# Pituitary Product Scope Contract

## Product Boundaries

Pituitary is CLI-first. The CLI is the required transport for the first ship.

The MCP server is optional and wraps the same shared logic rather than defining different product behavior.

The first shipping slice is local filesystem only.

Pituitary indexes spec bundles, Markdown docs, and inferred Markdown contracts.

The index backend is SQLite plus sqlite-vec.

## Documentation Expectations

Product docs should describe Pituitary as a deterministic local tool for spec and document management.

Product docs should not imply that remote repositories, vendor integrations, or model-backed runtime analysis are already part of the shipped first slice.
