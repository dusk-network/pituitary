# Adding A Command

This guide walks through the current Pituitary command stack:

```text
cmd/ -> internal/app/ -> internal/analysis or internal/index
                \
                 -> internal/mcp/ (optional wrapper)
```

Use this when adding a new CLI command or exposing an existing capability through MCP.

## Decide What Kind Of Command You Are Adding

There are two common cases.

### Analysis or query command

Examples:

- `search-specs`
- `check-overlap`
- `compare-specs`
- `analyze-impact`
- `check-doc-drift`
- `review-spec`

These should usually go through:

- `internal/analysis` or `internal/index` for core logic
- `internal/app` for config loading and error classification
- `cmd/` for CLI flag parsing and rendering
- `internal/mcp/` if the feature should also be available over MCP

### CLI-only operational command

Example:

- `index --rebuild`

This kind of command can stay CLI-only if the architecture says it should not be transport-shared. In the current codebase, rebuild is intentionally not exposed through MCP.

## Step 1: Add Or Extend Core Logic

If the capability is a new analysis feature, start in `internal/analysis`.

Typical responsibilities there:

- load data from the repository or index
- perform the core comparison, ranking, or traversal
- return a structured result type
- stay independent from CLI and MCP details

If the capability is retrieval-focused, it may belong in `internal/index` instead.

## Step 2: Add A Shared App Operation

If the command is transport-exposed, add an operation in `internal/app/operations.go`.

That layer should:

- load config
- call the core package
- normalize or classify failures into stable issue codes
- return a typed `Response`

Keep this layer free of flag parsing and output formatting.

## Step 3: Add The CLI Command

Add a file in `cmd/` or extend an existing one.

Most command files follow this pattern:

1. build a `flag.FlagSet`
2. parse flags into a request struct
3. validate CLI-specific requirements
4. call the shared app operation
5. render success or error through shared helpers

The command should also be registered in [cmd/root.go](/Users/emanuele/devel/pituitary/cmd/root.go) so it appears in `help` and dispatch works.

## Step 4: Add MCP Exposure If Appropriate

If the command belongs on the MCP surface:

1. add a typed request shape in `internal/mcp/tools.go` if the CLI and MCP input shapes differ
2. register the tool with description and input/output schema
3. route the handler through `internal/app`

Do not duplicate analysis logic inside the MCP layer.

Do not expose commands through MCP if the architecture intentionally keeps them CLI-only.

## Step 5: Add Tests At The Right Layers

Typical coverage looks like this:

- core behavior in `internal/analysis/*_test.go` or `internal/index/*_test.go`
- transport classification in `internal/app/operations_test.go` if needed
- CLI behavior in `cmd/*_test.go`
- MCP behavior in `internal/mcp/*_test.go` if the tool is exposed there
- command reachability in `cmd/root_test.go` if the command surface changes

The existing tests are good templates. Reuse helper patterns instead of building fresh scaffolding for each command.

## Step 6: Update Documentation

If the command is user-facing, update:

- `README.md` for the command surface
- `ARCHITECTURE.md` if contracts or architecture boundaries changed
- `IMPLEMENTATION_BACKLOG.md` if backlog state changed

If you changed contributor workflow, update the development docs too.

## A Minimal Checklist

- [ ] core logic added in the right internal package
- [ ] app operation added or updated if transport-shared
- [ ] CLI command added in `cmd/`
- [ ] root dispatch updated
- [ ] MCP tool added only if appropriate
- [ ] tests added where behavior changed
- [ ] docs updated

## Concrete Example

For a new analysis command, the usual sequence is:

1. add request and result types plus logic in `internal/analysis`
2. add `app.<Operation>()`
3. add `cmd/<command>.go`
4. add `cmd/<command>_test.go`
5. add MCP registration in `internal/mcp/tools.go` if the command belongs there
6. update `cmd/root.go`, `cmd/root_test.go`, and docs

If you find yourself pushing parsing, rendering, or transport-specific behavior into `internal/analysis`, stop and move that concern outward.
