package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/dusk-network/pituitary/internal/config"
)

// maxCLIRequestBytes caps how much a CLI command will buffer from a request
// file or stdin. Without this cap, piping or pointing to a multi-gigabyte file
// would consume all available memory before any parsing happened. An
// io.LimitReader enforces the bound at read time so a file swapped between
// open and read cannot defeat the guard.
const maxCLIRequestBytes = 16 * 1024 * 1024

// readBoundedStdin reads at most maxCLIRequestBytes+1 bytes from cliStdin and
// returns a clear error if the input is larger than the cap.
func readBoundedStdin(label string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(cliStdin, maxCLIRequestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s from stdin: %w", label, err)
	}
	if int64(len(data)) > maxCLIRequestBytes {
		return nil, fmt.Errorf("%s from stdin exceeds %d-byte size limit", label, maxCLIRequestBytes)
	}
	return data, nil
}

// readBoundedRequestFile reads absPath through an io.LimitReader so a workspace
// file that is swapped or grows between open and read still cannot cause an
// unbounded allocation.
func readBoundedRequestFile(absPath, label string) ([]byte, error) {
	// #nosec G304 -- absPath is validated to remain under the configured workspace root; LimitReader enforces the allocation bound.
	f, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %s %q: %w", label, absPath, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxCLIRequestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s %q: %w", label, absPath, err)
	}
	if int64(len(data)) > maxCLIRequestBytes {
		return nil, fmt.Errorf("%s file %q exceeds %d-byte size limit", label, absPath, maxCLIRequestBytes)
	}
	return data, nil
}

// autoLoadWorkspaceRequest is a LoadRequestFile callback for commands whose
// --request-file contents are a bare workspace-scoped JSON document that
// unmarshals directly into Req, with no post-load enrichment. Commands that
// need additional resolution after the JSON parse (e.g. resolving a diff-file
// reference inside the parsed request) should keep their own callback rather
// than reaching for this helper.
//
// Use by taking the instantiated value: `LoadRequestFile:
// autoLoadWorkspaceRequest[analysis.OverlapRequest]`.
//
// The helper needs a non-nil *config.Config to resolve workspace-scoped paths,
// so callers must set Options.ConfigForFile = true. A nil cfg returns a clear
// error rather than nil-panicking, so a future misconfiguration surfaces as a
// classified CLI error instead of crashing.
func autoLoadWorkspaceRequest[Req any](_ context.Context, cfg *config.Config, trimmedPath string) (*Req, error) {
	if cfg == nil {
		return nil, fmt.Errorf("autoLoadWorkspaceRequest requires Options.ConfigForFile = true; cfg was nil")
	}
	req, err := loadWorkspaceScopedJSONFile[Req](cfg.Workspace.RootPath, trimmedPath, "request file")
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func loadWorkspaceScopedJSONFile[T any](workspaceRoot, rawPath, label string) (T, error) {
	var zero T
	rawPath, err := validateCLIPathValue(rawPath, label)
	if err != nil {
		return zero, err
	}

	var data []byte
	switch rawPath {
	case "-":
		data, err = readBoundedStdin(label)
		if err != nil {
			return zero, err
		}
	default:
		absPath, err := resolveWorkspaceScopedCLIPath(workspaceRoot, rawPath, label)
		if err != nil {
			return zero, err
		}
		data, err = readBoundedRequestFile(absPath, label)
		if err != nil {
			return zero, err
		}
	}

	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		if rawPath == "-" {
			return zero, fmt.Errorf("parse %s from stdin: %w", label, err)
		}
		return zero, fmt.Errorf("parse %s %q: %w", label, rawPath, err)
	}
	return value, nil
}
