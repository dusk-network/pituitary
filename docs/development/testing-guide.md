# Testing Guide

This guide explains how testing works in Pituitary today and how to add new coverage without requiring live model credentials or network access.

## Testing Principles

Pituitary tests are built around a deterministic local fixture setup.

The goals are:

- no API keys required
- no network access required
- stable semantic behavior across runs
- clear fixture expectations for overlap, impact, and drift

The default contributor workflow should stay fast and predictable.

## Main Commands

Use the checked-in `Makefile` targets:

```sh
make fmt
make smoke-sqlite-vec
make test
make vet
make bench
make ci
```

What they do:

- `make smoke-sqlite-vec`: verifies the SQLite and `sqlite-vec` runtime path
- `make test`: runs the full Go test suite
- `make vet`: runs static analysis
- `make bench`: runs index and analysis benchmarks
- `make ci`: runs the checked-in validation pipeline

## Fixture Workspace

The repo root contains the bootstrap fixture workspace:

- `pituitary.toml`
- `specs/`
- `docs/`

Supporting expectations live in:

- `testdata/bootstrap_expectations.json`
- `testdata/README.md`
- malformed fixture cases under `testdata/invalid-spec-bundle/`

The repo-level fixture checks live in [fixtures_test.go](/Users/emanuele/devel/pituitary/fixtures_test.go).

If you change expected fixture behavior, update both the fixtures and the expectation files together.

## Deterministic Provider Model

Tests and CI use deterministic provider behavior rather than live APIs.

In practice that means:

- no real model credentials should be required
- retrieval and analysis tests should stay stable
- missing provider behavior should be tested as explicit `dependency_unavailable` failures when relevant

If you are writing a test for behavior that depends on provider failure, follow the existing index and analysis tests that stub or force that path explicitly.

## Where Tests Live

### CLI tests

Location:

- `cmd/*_test.go`
- `cmd/root_test.go`

These verify:

- flag parsing
- validation behavior
- output envelopes
- command reachability from the root dispatcher

### App-layer tests

Location:

- `internal/app/operations_test.go`

These verify:

- shared request normalization
- transport-agnostic error classification

### Analysis tests

Location:

- `internal/analysis/*_test.go`

These verify:

- overlap logic
- comparison behavior
- impact traversal
- doc drift behavior
- review composition

### Index tests

Location:

- `internal/index/*_test.go`

These verify:

- rebuild behavior
- SQLite readiness
- vector search behavior
- benchmark coverage

### MCP tests

Location:

- `internal/mcp/*_test.go`

These verify:

- tool registration and startup validation
- transport-level cancellation behavior
- MCP to shared app wiring

## Adding New Tests

Prefer the smallest test that proves the behavior.

Typical choices:

- unit-style test in one package for isolated logic
- command-level test when flags or rendering changed
- MCP test only if the MCP surface actually changed

When possible, reuse the existing fixture workspace and helpers rather than building a new synthetic environment.

## Adding Or Changing Fixtures

When fixture changes are necessary:

1. update the relevant files under `specs/`, `docs/`, or `testdata/`
2. update `testdata/bootstrap_expectations.json` if expected behavior changed
3. adjust the package-level tests that assert those expectations

Keep malformed cases isolated under `testdata/` instead of polluting the main fixture workspace.

## Benchmarks

Benchmarks live in:

- `internal/index/benchmark_test.go`
- `internal/analysis/benchmark_test.go`

Run them with:

```sh
make bench
```

Use these when a change affects:

- rebuild performance
- retrieval behavior
- overlap, impact, drift, or review hot paths

If you optimize a hot path, benchmark it before and after so the change is measurable.

## Common Test Patterns

- use temporary workspaces when a test needs isolated files or config
- use the existing fixture workspace when behavior is meant to reflect the shipped example corpus
- keep failure assertions specific and actionable
- avoid tests that depend on external services

If a test needs CGO-backed SQLite behavior, `make smoke-sqlite-vec` and the normal test targets already cover that environment assumption.
