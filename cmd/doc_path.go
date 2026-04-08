package cmd

import (
	"context"
	"io"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
)

func resolveIndexedDocRefWithConfigContext(ctx context.Context, cfg *config.Config, rawPath string) (string, error) {
	return index.ResolveIndexedDocRefWithConfigContext(ctx, cfg, rawPath)
}

func resolveIndexedDocRefsWithConfigContext(ctx context.Context, cfg *config.Config, rawPaths []string) ([]string, error) {
	return index.ResolveIndexedDocRefsWithConfigContext(ctx, cfg, rawPaths)
}

func writeDocPathResolutionError(stdout, stderr io.Writer, format, command string, request any, err error) int {
	code := "validation_error"
	switch {
	case index.IsMissingIndex(err):
		code = "config_error"
	case index.IsDocPathNotFound(err):
		code = "not_found"
	}
	return writeCLIError(stdout, stderr, format, command, request, cliIssue{
		Code:    code,
		Message: err.Error(),
	}, 2)
}
