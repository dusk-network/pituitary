Ref: DOG-002
Status: accepted
Domain: developer-experience

# Pituitary Contributor Workflow Contract

## Architecture And Commands

New transport-exposed commands should follow the shared flow from `cmd/` to `internal/app/` and then into `internal/analysis` or `internal/index`.

Index rebuild remains a CLI-first operational command rather than an MCP feature.

## Testing And Tooling

Contributor docs should describe deterministic local testing with no live model credentials and no network dependency.

Contributor docs should keep `make fmt`, `make test`, `make vet`, and `make ci` as the normal validation path.

Contributor docs should describe the prerequisites as Go 1.25+, `make`, and a CGO-capable toolchain for the sqlite-vec path.
