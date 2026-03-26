# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Pituitary, please report it privately. Do not open a public issue.

**Email:** security@dusk.network

We will acknowledge your report within 48 hours and aim to provide a fix or mitigation within 7 days for confirmed vulnerabilities.

## Scope

Pituitary is a local CLI tool. Its primary attack surface is:

- Malicious input in spec files, markdown docs, or config files that could cause unexpected behavior
- Path traversal or file-write issues in `fix`, `canonicalize --write`, or `discover --write`
- Index corruption or data integrity issues

Out of scope:

- Vulnerabilities in third-party dependencies (report those upstream)
- Issues that require local filesystem access beyond what the user already has
