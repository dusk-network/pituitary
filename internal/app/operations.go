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
		case index.IsDependencyUnavailable(err):
			return failure[analysis.CompareRequest, analysis.CompareResult](request, CodeDependencyUnavailable, improveDependencyUnavailableMessage(cfg, err), 3)
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
		case index.IsDependencyUnavailable(err):
			return failure[analysis.DocDriftRequest, analysis.DocDriftResult](request, CodeDependencyUnavailable, improveDependencyUnavailableMessage(cfg, err), 3)
		default:
			return failure[analysis.DocDriftRequest, analysis.DocDriftResult](request, CodeValidationError, err.Error(), 2)
		}
	}

	return success(request, result)
}

// CheckCompliance loads config, executes compliance analysis, and classifies failures.
func CheckCompliance(ctx context.Context, configPath string, request analysis.ComplianceRequest) Response[analysis.ComplianceRequest, analysis.ComplianceResult] {
	cfg, issue := loadConfig(configPath)
	if issue != nil {
		return failure[analysis.ComplianceRequest, analysis.ComplianceResult](request, issue.Code, issue.Message, issue.ExitCode)
	}

	result, err := analysis.CheckComplianceContext(ctx, cfg, request)
	if err != nil {
		switch {
		case index.IsMissingIndex(err):
			return failure[analysis.ComplianceRequest, analysis.ComplianceResult](request, CodeConfigError, missingIndexMessage(err), 2)
		case index.IsDependencyUnavailable(err):
			return failure[analysis.ComplianceRequest, analysis.ComplianceResult](request, CodeDependencyUnavailable, improveDependencyUnavailableMessage(cfg, err), 3)
		default:
			return failure[analysis.ComplianceRequest, analysis.ComplianceResult](request, CodeValidationError, err.Error(), 2)
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
	message := strings.TrimSpace(err.Error())
	runtimeName, provider, ok := dependencyUnavailableRuntimeContext(cfg, message)
	if !ok {
		return message
	}

	if improved := improveOpenAICompatibleDependencyMessage(runtimeName, provider, message); improved != "" {
		return improved
	}
	return message
}

func dependencyUnavailableRuntimeContext(cfg *config.Config, message string) (string, config.RuntimeProvider, bool) {
	if cfg == nil {
		return "", config.RuntimeProvider{}, false
	}

	switch {
	case strings.Contains(message, "runtime.analysis"):
		return "runtime.analysis", cfg.Runtime.Analysis, true
	case strings.Contains(message, "runtime.embedder"):
		return "runtime.embedder", cfg.Runtime.Embedder, true
	default:
		return "", config.RuntimeProvider{}, false
	}
}

func improveOpenAICompatibleDependencyMessage(runtimeName string, provider config.RuntimeProvider, rawMessage string) string {
	if strings.TrimSpace(provider.Provider) != config.RuntimeProviderOpenAI {
		return maybeAddAPIKeyEnv(rawMessage, strings.TrimSpace(provider.APIKeyEnv))
	}

	details := runtimeDescriptor(runtimeName, provider)
	lower := strings.ToLower(rawMessage)

	switch {
	case mentionsMissingAPIKey(lower):
		message := fmt.Sprintf("%s is missing credentials", details)
		if envVar := strings.TrimSpace(provider.APIKeyEnv); envVar != "" && !strings.Contains(rawMessage, envVar) {
			message = fmt.Sprintf("%s; set %s in the environment", message, envVar)
		}
		return fmt.Sprintf("%s. Raw provider error: %s", message, rawMessage)
	case mentionsModelUnloaded(lower):
		return fmt.Sprintf(
			"%s is unavailable because the configured model appears to be unloaded. If you are using LM Studio, load or pin model %q and retry. Raw provider error: %s",
			details,
			strings.TrimSpace(provider.Model),
			rawMessage,
		)
	case mentionsEndpointTimeout(lower):
		return fmt.Sprintf(
			"%s timed out while waiting for the provider. Check that the endpoint is reachable, that the provider is responsive, and that model %q is loaded. Raw provider error: %s",
			details,
			strings.TrimSpace(provider.Model),
			rawMessage,
		)
	case mentionsEndpointReachabilityFailure(lower):
		return fmt.Sprintf(
			"%s is unreachable. Check that the endpoint is correct, that the provider is running, and that it is bound to an address reachable from this machine. If you are using LM Studio, start the local server or fix the server binding. Raw provider error: %s",
			details,
			rawMessage,
		)
	default:
		return maybeAddAPIKeyEnv(rawMessage, strings.TrimSpace(provider.APIKeyEnv))
	}
}

func runtimeDescriptor(runtimeName string, provider config.RuntimeProvider) string {
	return fmt.Sprintf(
		"%s (provider %q, model %q, endpoint %q)",
		runtimeName,
		strings.TrimSpace(provider.Provider),
		strings.TrimSpace(provider.Model),
		strings.TrimSpace(provider.Endpoint),
	)
}

func maybeAddAPIKeyEnv(message, envVar string) string {
	if envVar == "" {
		return message
	}
	lower := strings.ToLower(message)
	if !mentionsMissingAPIKey(lower) || strings.Contains(message, envVar) {
		return message
	}
	return fmt.Sprintf("%s; set %s in the environment", message, envVar)
}

func mentionsMissingAPIKey(message string) bool {
	return strings.Contains(message, "api key") || strings.Contains(message, "apikey")
}

func mentionsModelUnloaded(message string) bool {
	return strings.Contains(message, "model unloaded")
}

func mentionsEndpointTimeout(message string) bool {
	return strings.Contains(message, "context deadline exceeded") ||
		strings.Contains(message, "client.timeout exceeded") ||
		strings.Contains(message, "timeout awaiting response headers") ||
		strings.Contains(message, "i/o timeout") ||
		strings.Contains(message, "timed out")
}

func mentionsEndpointReachabilityFailure(message string) bool {
	return strings.Contains(message, "connection refused") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "couldn't connect to server") ||
		strings.Contains(message, "failed to connect") ||
		strings.Contains(message, "no such host") ||
		strings.Contains(message, "network is unreachable")
}
