package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dusk-network/pituitary/internal/source"
)

func validateCLIPathValue(rawPath, label string) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return "", fmt.Errorf("%s must not be empty", label)
	}
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("%s %q contains control characters", label, rawPath)
		}
	}
	return trimmed, nil
}

func resolveWorkspaceScopedCLIPath(workspaceRoot, rawPath, label string) (string, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return "", fmt.Errorf("workspace root is required to resolve %s", label)
	}
	trimmed, err := validateCLIPathValue(rawPath, label)
	if err != nil {
		return "", err
	}

	rootPath := filepath.Clean(workspaceRoot)
	if !filepath.IsAbs(rootPath) {
		rootPath, err = filepath.Abs(rootPath)
		if err != nil {
			return "", fmt.Errorf("resolve workspace root %q: %w", workspaceRoot, err)
		}
	}

	absPath := trimmed
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(rootPath, absPath)
	}
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve %s %q: %w", label, rawPath, err)
	}
	absPath = filepath.Clean(absPath)
	if !cliPathWithinRoot(rootPath, absPath) {
		return "", fmt.Errorf("%s %q resolves outside workspace root %q", label, rawPath, filepath.ToSlash(rootPath))
	}

	info, err := os.Stat(absPath)
	switch {
	case err != nil:
		return "", fmt.Errorf("stat %s %q: %w", label, rawPath, err)
	case info.IsDir():
		return "", fmt.Errorf("%s %q is a directory", label, rawPath)
	default:
		return absPath, nil
	}
}

func cliPathWithinRoot(root, path string) bool {
	realRoot, err := source.EvalSymlinksBestEffort(root)
	if err != nil {
		return false
	}
	realPath, err := source.EvalSymlinksBestEffort(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(realRoot, realPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
