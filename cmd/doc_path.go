package cmd

import (
	"context"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
)

func resolveIndexedDocRefsWithConfigContext(ctx context.Context, cfg *config.Config, rawPaths []string) ([]string, error) {
	return index.ResolveIndexedDocRefsWithConfigContext(ctx, cfg, rawPaths)
}

// docPathResolutionIssue classifies a doc-path resolution error into the
// cliIssue code surfaced via docPathResolutionError.
func docPathResolutionIssue(err error) cliIssue {
	code := "validation_error"
	switch {
	case index.IsMissingIndex(err):
		code = "config_error"
	case index.IsDocPathNotFound(err):
		code = "not_found"
	}
	return cliIssue{Code: code, Message: err.Error()}
}

// docPathResolutionError wraps a doc-path resolution error in a cliIssueError
// so runCommand's BuildRequest callback can surface the classified code.
func docPathResolutionError(err error) error {
	return &cliIssueError{issue: docPathResolutionIssue(err), exitCode: 2}
}
