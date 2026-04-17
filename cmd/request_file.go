package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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
