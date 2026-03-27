package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

func loadWorkspaceScopedJSONFile[T any](workspaceRoot, rawPath, label string) (T, error) {
	var zero T
	rawPath, err := validateCLIPathValue(rawPath, label)
	if err != nil {
		return zero, err
	}

	var data []byte
	switch rawPath {
	case "-":
		data, err = io.ReadAll(cliStdin)
		if err != nil {
			return zero, fmt.Errorf("read %s from stdin: %w", label, err)
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
