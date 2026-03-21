package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
)

const (
	CodeConfigError           = "config_error"
	CodeValidationError       = "validation_error"
	CodeNotFound              = "not_found"
	CodeDependencyUnavailable = "dependency_unavailable"
	CodeInternalError         = "internal_error"
)

// Issue is a transport-agnostic operation failure.
type Issue struct {
	Code     string
	Message  string
	ExitCode int
}

// Response captures a normalized request plus either a result or an issue.
type Response[Req any, Res any] struct {
	Request Req
	Result  *Res
	Issue   *Issue
}

// SearchSpecs loads config, normalizes the request, executes search, and classifies failures.
func SearchSpecs(ctx context.Context, configPath string, request index.SearchSpecRequest) Response[index.SearchSpecRequest, index.SearchSpecResult] {
	query, err := request.ToQuery()
	if err != nil {
		return failure[index.SearchSpecRequest, index.SearchSpecResult](request, CodeValidationError, err.Error(), 2)
	}

	request.Query = query.Query
	request.Filters.Domain = query.Domain
	request.Filters.Statuses = query.Statuses
	limit := query.Limit
	request.Limit = &limit

	cfg, issue := loadConfig(configPath)
	if issue != nil {
		return failure[index.SearchSpecRequest, index.SearchSpecResult](request, issue.Code, issue.Message, issue.ExitCode)
	}

	result, err := index.SearchSpecsContext(ctx, cfg, query)
	if err != nil {
		switch {
		case index.IsMissingIndex(err):
			return failure[index.SearchSpecRequest, index.SearchSpecResult](request, CodeConfigError, missingIndexMessage(err), 2)
		case index.IsDependencyUnavailable(err):
			return failure[index.SearchSpecRequest, index.SearchSpecResult](request, CodeDependencyUnavailable, improveDependencyUnavailableMessage(cfg, err), 3)
		default:
			return failure[index.SearchSpecRequest, index.SearchSpecResult](request, CodeInternalError, err.Error(), 2)
		}
	}

	return success(request, result)
}

// CheckOverlap loads config, executes overlap analysis, and classifies failures.
func CheckOverlap(ctx context.Context, configPath string, request analysis.OverlapRequest) Response[analysis.OverlapRequest, analysis.OverlapResult] {
	cfg, issue := loadConfig(configPath)
	if issue != nil {
		return failure[analysis.OverlapRequest, analysis.OverlapResult](request, issue.Code, issue.Message, issue.ExitCode)
	}

	result, err := analysis.CheckOverlapContext(ctx, cfg, request)
	if err != nil {
		switch {
		case index.IsMissingIndex(err):
			return failure[analysis.OverlapRequest, analysis.OverlapResult](request, CodeConfigError, missingIndexMessage(err), 2)
		case analysis.IsNotFound(err):
			return failure[analysis.OverlapRequest, analysis.OverlapResult](request, CodeNotFound, err.Error(), 2)
		case index.IsDependencyUnavailable(err):
			return failure[analysis.OverlapRequest, analysis.OverlapResult](request, CodeDependencyUnavailable, improveDependencyUnavailableMessage(cfg, err), 3)
		default:
			return failure[analysis.OverlapRequest, analysis.OverlapResult](request, CodeValidationError, err.Error(), 2)
		}
	}

	return success(request, result)
}

// CompareSpecs loads config, executes comparison, and classifies failures.
func CompareSpecs(ctx context.Context, configPath string, request analysis.CompareRequest) Response[analysis.CompareRequest, analysis.CompareResult] {
	cfg, issue := loadConfig(configPath)
	if issue != nil {
		return failure[analysis.CompareRequest, analysis.CompareResult](request, issue.Code, issue.Message, issue.ExitCode)
	}

	result, err := analysis.CompareSpecsContext(ctx, cfg, request)
	if err != nil {
		switch {
		case index.IsMissingIndex(err):
			return failure[analysis.CompareRequest, analysis.CompareResult](request, CodeConfigError, missingIndexMessage(err), 2)
		case analysis.IsNotFound(err):
			return failure[analysis.CompareRequest, analysis.CompareResult](request, CodeNotFound, err.Error(), 2)
		default:
			return failure[analysis.CompareRequest, analysis.CompareResult](request, CodeValidationError, err.Error(), 2)
		}
	}

	return success(request, result)
}

// AnalyzeImpact loads config, executes impact analysis, and classifies failures.
func AnalyzeImpact(ctx context.Context, configPath string, request analysis.AnalyzeImpactRequest) Response[analysis.AnalyzeImpactRequest, analysis.AnalyzeImpactResult] {
	cfg, issue := loadConfig(configPath)
	if issue != nil {
		return failure[analysis.AnalyzeImpactRequest, analysis.AnalyzeImpactResult](request, issue.Code, issue.Message, issue.ExitCode)
	}

	result, err := analysis.AnalyzeImpactContext(ctx, cfg, request)
	if err != nil {
		switch {
		case index.IsMissingIndex(err):
			return failure[analysis.AnalyzeImpactRequest, analysis.AnalyzeImpactResult](request, CodeConfigError, missingIndexMessage(err), 2)
		case analysis.IsNotFound(err):
			return failure[analysis.AnalyzeImpactRequest, analysis.AnalyzeImpactResult](request, CodeNotFound, err.Error(), 2)
		default:
			return failure[analysis.AnalyzeImpactRequest, analysis.AnalyzeImpactResult](request, CodeValidationError, err.Error(), 2)
		}
	}

	return success(request, result)
}

// CheckDocDrift loads config, executes doc drift analysis, and classifies failures.
func CheckDocDrift(ctx context.Context, configPath string, request analysis.DocDriftRequest) Response[analysis.DocDriftRequest, analysis.DocDriftResult] {
	cfg, issue := loadConfig(configPath)
	if issue != nil {
		return failure[analysis.DocDriftRequest, analysis.DocDriftResult](request, issue.Code, issue.Message, issue.ExitCode)
	}

	result, err := analysis.CheckDocDriftContext(ctx, cfg, request)
	if err != nil {
		switch {
		case index.IsMissingIndex(err):
			return failure[analysis.DocDriftRequest, analysis.DocDriftResult](request, CodeConfigError, missingIndexMessage(err), 2)
		case analysis.IsNotFound(err):
			return failure[analysis.DocDriftRequest, analysis.DocDriftResult](request, CodeNotFound, err.Error(), 2)
		default:
			return failure[analysis.DocDriftRequest, analysis.DocDriftResult](request, CodeValidationError, err.Error(), 2)
		}
	}

	return success(request, result)
}

// ReviewSpec loads config, executes the review workflow, and classifies failures.
func ReviewSpec(ctx context.Context, configPath string, request analysis.ReviewRequest) Response[analysis.ReviewRequest, analysis.ReviewResult] {
	cfg, issue := loadConfig(configPath)
	if issue != nil {
		return failure[analysis.ReviewRequest, analysis.ReviewResult](request, issue.Code, issue.Message, issue.ExitCode)
	}

	result, err := analysis.ReviewSpecContext(ctx, cfg, request)
	if err != nil {
		switch {
		case index.IsMissingIndex(err):
			return failure[analysis.ReviewRequest, analysis.ReviewResult](request, CodeConfigError, missingIndexMessage(err), 2)
		case analysis.IsNotFound(err):
			return failure[analysis.ReviewRequest, analysis.ReviewResult](request, CodeNotFound, err.Error(), 2)
		case index.IsDependencyUnavailable(err):
			return failure[analysis.ReviewRequest, analysis.ReviewResult](request, CodeDependencyUnavailable, improveDependencyUnavailableMessage(cfg, err), 3)
		default:
			return failure[analysis.ReviewRequest, analysis.ReviewResult](request, CodeValidationError, err.Error(), 2)
		}
	}

	return success(request, result)
}

func loadConfig(configPath string) (*config.Config, *Issue) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, &Issue{
			Code:     CodeConfigError,
			Message:  err.Error(),
			ExitCode: 2,
		}
	}
	return cfg, nil
}

func success[Req any, Res any](request Req, result *Res) Response[Req, Res] {
	return Response[Req, Res]{
		Request: request,
		Result:  result,
	}
}

func failure[Req any, Res any](request Req, code, message string, exitCode int) Response[Req, Res] {
	return Response[Req, Res]{
		Request: request,
		Issue: &Issue{
			Code:     code,
			Message:  message,
			ExitCode: exitCode,
		},
	}
}

func missingIndexMessage(err error) string {
	path := index.MissingIndexPath(err)
	if path == "" {
		return "index does not exist; run `pituitary index --rebuild`"
	}
	return fmt.Sprintf("index %s does not exist; run `pituitary index --rebuild`", path)
}

func improveDependencyUnavailableMessage(cfg *config.Config, err error) string {
	message := err.Error()
	envVar := configuredAPIKeyEnv(cfg)
	if envVar == "" {
		return message
	}

	lower := strings.ToLower(message)
	if !strings.Contains(lower, "api key") && !strings.Contains(lower, "apikey") {
		return message
	}
	if strings.Contains(message, envVar) {
		return message
	}
	return fmt.Sprintf("%s; set %s in the environment", message, envVar)
}

func configuredAPIKeyEnv(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if envVar := strings.TrimSpace(cfg.Runtime.Embedder.APIKeyEnv); envVar != "" {
		return envVar
	}
	return strings.TrimSpace(cfg.Runtime.Analysis.APIKeyEnv)
}
