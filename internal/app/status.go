package app

import (
	"context"
	"fmt"

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
	WorkspaceRoot    string
	ConfigPath       string
	EmbedderProvider string
	AnalysisProvider string
	RuntimeConfig    *RuntimeConfigStatus
	Index            *index.Status
	Freshness        *index.FreshnessStatus
	RelationGraph    *index.RelationGraphStatus
	Runtime          *runtimeprobe.Result
	Guidance         []string
}

type RuntimeConfigStatus struct {
	Embedder       RuntimeProviderStatus
	Analysis       RuntimeProviderStatus
	Contextualizer string // empty means disabled; otherwise the configured format name.
}

type RuntimeProviderStatus struct {
	Profile    string
	Provider   string
	Model      string
	Endpoint   string
	TimeoutMS  int
	MaxRetries int
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
			WorkspaceRoot:    cfg.Workspace.RootPath,
			ConfigPath:       cfg.ConfigPath,
			EmbedderProvider: cfg.Runtime.Embedder.Provider,
			AnalysisProvider: cfg.Runtime.Analysis.Provider,
			RuntimeConfig: &RuntimeConfigStatus{
				Embedder:       runtimeProviderStatus(cfg.Runtime.Embedder),
				Analysis:       runtimeProviderStatus(cfg.Runtime.Analysis),
				Contextualizer: cfg.Runtime.Chunking.Contextualizer.Format,
			},
			Index:         status,
			Freshness:     freshness,
			RelationGraph: index.InspectRelationGraph(records.Specs),
			Runtime:       runtimeResult,
			Guidance:      fixtureEmbedderGuidance(cfg, status),
		}, nil
	}, classifyStatusError)
}

func fixtureEmbedderGuidance(cfg *config.Config, status *index.Status) []string {
	if cfg == nil || status == nil {
		return nil
	}
	if cfg.Runtime.Embedder.Provider != config.RuntimeProviderFixture {
		return nil
	}
	totalArtifacts := status.SpecCount + status.DocCount
	if totalArtifacts < 5 {
		return nil
	}
	return []string{
		fmt.Sprintf(
			"runtime.embedder is still %q on %d indexed artifact(s); for better retrieval quality on a real corpus, switch to %q, rebuild the index, then run `pituitary status --check-runtime embedder`",
			config.RuntimeProviderFixture,
			totalArtifacts,
			config.RuntimeProviderOpenAI,
		),
	}
}

func runtimeProviderStatus(provider config.RuntimeProvider) RuntimeProviderStatus {
	return RuntimeProviderStatus{
		Profile:    provider.Profile,
		Provider:   provider.Provider,
		Model:      provider.Model,
		Endpoint:   provider.Endpoint,
		TimeoutMS:  provider.TimeoutMS,
		MaxRetries: provider.MaxRetries,
	}
}

func classifyStatusError(cfg *config.Config, err error) *Issue {
	switch {
	case index.IsDependencyUnavailable(err):
		return dependencyUnavailableIssue(cfg, err)
	default:
		return &Issue{
			Code:     CodeInternalError,
			Message:  err.Error(),
			ExitCode: 2,
		}
	}
}
