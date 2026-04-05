package app

import (
	"context"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/source"
)

// ExplainFileRequest captures the input for an explain-file operation.
type ExplainFileRequest struct {
	Path string `json:"path" jsonschema_description:"File path to classify against configured sources"`
}

// ExplainFile loads config and classifies a file path against the configured sources.
func ExplainFile(ctx context.Context, configPath string, request ExplainFileRequest) Response[ExplainFileRequest, source.ExplainFileResult] {
	return executeWithConfig(ctx, configPath, request, func(cfg *config.Config) (*source.ExplainFileResult, error) {
		return source.ExplainFile(cfg, request.Path)
	}, func(cfg *config.Config, err error) *Issue {
		return &Issue{
			Code:     CodeInternalError,
			Message:  err.Error(),
			ExitCode: 2,
		}
	})
}
