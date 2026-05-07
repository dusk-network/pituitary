package cmd

import (
	"context"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
)

func resolveIndexedSpecRefWithConfigContext(ctx context.Context, cfg *config.Config, rawPath string) (string, error) {
	return index.ResolveIndexedSpecRefWithConfigContext(ctx, cfg, rawPath)
}

func resolveIndexedSpecRefsWithConfigContext(ctx context.Context, cfg *config.Config, rawPaths []string) ([]string, error) {
	return index.ResolveIndexedSpecRefsWithConfigContext(ctx, cfg, rawPaths)
}

// specPathResolutionIssue classifies a spec-path resolution error into the
// cliIssue code surfaced via specPathResolutionError.
func specPathResolutionIssue(err error) cliIssue {
	code := "validation_error"
	switch {
	case index.IsMissingIndex(err):
		code = "config_error"
	case index.IsSpecPathNotFound(err):
		code = "not_found"
	}
	return cliIssue{Code: code, Message: err.Error()}
}

// specPathResolutionError wraps a spec-path resolution error in a
// cliIssueError so runCommand's BuildRequest callback can surface the
// classified code through the standard error channel.
func specPathResolutionError(err error) error {
	return &cliIssueError{issue: specPathResolutionIssue(err), exitCode: 2}
}

func nonEmptyCount(values ...string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}
