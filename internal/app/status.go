package app

import (
	"context"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/runtimeprobe"
	"github.com/dusk-network/pituitary/internal/source"
)

// StatusRequest captures normalized status input.
type StatusRequest struct {
	CheckRuntime runtimeprobe.Scope
}

// StatusResult captures transport-agnostic status details for one workspace.
type StatusResult struct {
	WorkspaceRoot string
	ConfigPath    string
	Index         *index.Status
	Freshness     *index.FreshnessStatus
	RelationGraph *index.RelationGraphStatus
	Runtime       *runtimeprobe.Result
}

// Status loads config, inspects the current index, and optionally probes runtime dependencies.
func Status(ctx context.Context, configPath string, request StatusRequest) Response[StatusRequest, StatusResult] {
	return executeWithConfig(ctx, configPath, request, func(cfg *config.Config) (*StatusResult, error) {
		records, err := source.LoadFromConfig(cfg)
		if err != nil {
			return nil, err
		}

		status, err := index.ReadStatusContext(ctx, cfg.Workspace.ResolvedIndexPath)
		if err != nil {
			return nil, err
		}
		freshness, err := index.InspectFreshnessContext(ctx, cfg)
		if err != nil {
			return nil, err
		}

		var runtimeResult *runtimeprobe.Result
		if request.CheckRuntime != runtimeprobe.ScopeNone {
			runtimeResult, err = runtimeprobe.Run(ctx, cfg, request.CheckRuntime)
			if err != nil {
				return nil, err
			}
		}

		return &StatusResult{
			WorkspaceRoot: cfg.Workspace.RootPath,
			ConfigPath:    cfg.ConfigPath,
			Index:         status,
			Freshness:     freshness,
			RelationGraph: index.InspectRelationGraph(records.Specs),
			Runtime:       runtimeResult,
		}, nil
	}, classifyStatusError)
}

func classifyStatusError(cfg *config.Config, err error) *Issue {
	switch {
	case index.IsDependencyUnavailable(err):
		return &Issue{
			Code:     CodeDependencyUnavailable,
			Message:  FormatDependencyUnavailableMessage(cfg, err),
			ExitCode: 3,
		}
	default:
		return &Issue{
			Code:     CodeInternalError,
			Message:  err.Error(),
			ExitCode: 2,
		}
	}
}
