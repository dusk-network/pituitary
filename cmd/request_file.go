package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// maxCLIStdinBytes caps how much a CLI command will buffer from stdin. Without
// this cap, piping a multi-gigabyte file to `pituitary check-overlap -` would
// consume all available memory before any parsing happened.
const maxCLIStdinBytes = 16 * 1024 * 1024

// readBoundedStdin reads at most maxCLIStdinBytes+1 bytes from cliStdin and
// returns a clear error if the input is larger than the cap.
func readBoundedStdin(label string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(cliStdin, maxCLIStdinBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s from stdin: %w", label, err)
	}
	if int64(len(data)) > maxCLIStdinBytes {
		return nil, fmt.Errorf("%s from stdin exceeds %d-byte size limit", label, maxCLIStdinBytes)
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
		// #nosec G304 -- absPath is validated to remain under the configured workspace root.
		data, err = os.ReadFile(absPath)
		if err != nil {
			return zero, fmt.Errorf("read %s %q: %w", label, rawPath, err)
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
