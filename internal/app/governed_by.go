package app

import (
	"context"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/resultmeta"
)

// GovernedByRequest captures the input for a governed-by query.
type GovernedByRequest struct {
	Path   string `json:"path" jsonschema_description:"Workspace-relative file path to look up governing specs for"`
	AtDate string `json:"at_date,omitempty" jsonschema_description:"ISO date for point-in-time governance query (e.g. 2025-03-15)"`
}

// GovernedByResult captures the governing specs for a given path.
type GovernedByResult struct {
	Path         string                   `json:"path"`
	Refs         []string                 `json:"refs"`
	Specs        []index.GoverningSpec    `json:"specs"`
	ContentTrust *resultmeta.ContentTrust `json:"content_trust,omitempty"`
}

// GovernedBy loads config, queries the index for specs governing the given path, and classifies failures.
func GovernedBy(ctx context.Context, configPath string, request GovernedByRequest) Response[GovernedByRequest, GovernedByResult] {
	return executeWithFreshConfig(ctx, configPath, request, operationExecutionPolicy{}, func(cfg *config.Config) (*GovernedByResult, error) {
		result, err := index.GovernedByContext(ctx, cfg.Workspace.ResolvedIndexPath, request.Path, request.AtDate)
		if err != nil {
			return nil, err
		}
		return &GovernedByResult{
			Path:         result.Path,
			Refs:         result.Refs,
			Specs:        result.Specs,
			ContentTrust: resultmeta.UntrustedWorkspaceText(),
		}, nil
	})
}
