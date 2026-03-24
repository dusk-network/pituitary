package cmd

import (
	"context"
	"io"
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

func writeSpecPathResolutionError(stdout, stderr io.Writer, format, command string, request any, err error) int {
	code := "validation_error"
	switch {
	case index.IsMissingIndex(err):
		code = "config_error"
	case index.IsSpecPathNotFound(err):
		code = "not_found"
	}
	return writeCLIError(stdout, stderr, format, command, request, cliIssue{
		Code:    code,
		Message: err.Error(),
	}, 2)
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
