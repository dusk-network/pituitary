package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/resultmeta"
)

const (
	CodeConfigError           = "config_error"
	CodeValidationError       = "validation_error"
	CodeNotFound              = "not_found"
	CodeDependencyUnavailable = "dependency_unavailable"
	CodeInternalError         = "internal_error"

	IssueDetailPhase     = "phase"
	IssuePhaseConfigLoad = "config_load"
)

// Issue is a transport-agnostic operation failure.
type Issue struct {
	Code     string
	Message  string
	Details  map[string]any
	ExitCode int
}

// Response captures a normalized request plus either a result or an issue.
type Response[Req any, Res any] struct {
	Request Req
	Result  *Res
	Issue   *Issue
}

type operationExecutionPolicy struct {
	NotFound    func(error) bool
	DefaultCode string
}

type issueError struct {
	issue *Issue
}

func (e *issueError) Error() string {
	if e == nil || e.issue == nil {
		return ""
	}
	return e.issue.Message
}

func executeWithConfig[Req any, Res any](ctx context.Context, configPath string, request Req, run func(*config.Config) (*Res, error), classify func(*config.Config, error) *Issue) Response[Req, Res] {
	cfg, issue := loadConfig(configPath)
	if issue != nil {
		return Response[Req, Res]{
			Request: request,
			Issue:   issue,
		}
	}

	result, err := run(cfg)
	if err != nil {
		issue := classify(cfg, err)
		return Response[Req, Res]{
			Request: request,
			Issue:   issue,
		}
	}

	return success(request, result)
}

func executeWithFreshConfig[Req any, Res any](ctx context.Context, configPath string, request Req, policy operationExecutionPolicy, run func(*config.Config) (*Res, error)) Response[Req, Res] {
	return executeWithConfig(ctx, configPath, request, func(cfg *config.Config) (*Res, error) {
		if issue := ensureFreshIndex(ctx, cfg); issue != nil {
			return nil, &issueError{issue: issue}
		}
		return run(cfg)
	}, func(cfg *config.Config, err error) *Issue {
		var wrapped *issueError
		if errors.As(err, &wrapped) && wrapped != nil && wrapped.issue != nil {
			return wrapped.issue
		}
		return classifyExecutionError(cfg, err, policy)
	})
}

func classifyExecutionError(cfg *config.Config, err error, policy operationExecutionPolicy) *Issue {
	switch {
	case index.IsMissingIndex(err):
		return &Issue{
			Code:     CodeConfigError,
			Message:  missingIndexMessage(err),
			ExitCode: 2,
		}
	case policy.NotFound != nil && policy.NotFound(err):
		return &Issue{
			Code:     CodeNotFound,
			Message:  err.Error(),
			ExitCode: 2,
		}
	case index.IsDependencyUnavailable(err):
		return dependencyUnavailableIssue(cfg, err)
	default:
		code := strings.TrimSpace(policy.DefaultCode)
		if code == "" {
			code = CodeValidationError
		}
		return &Issue{
			Code:     code,
			Message:  err.Error(),
			ExitCode: 2,
		}
	}
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

	return executeWithFreshConfig(ctx, configPath, request, operationExecutionPolicy{}, func(cfg *config.Config) (*index.SearchSpecResult, error) {
		return index.SearchSpecsContext(ctx, cfg, query)
	})
}

// CheckOverlap loads config, executes overlap analysis, and classifies failures.
func CheckOverlap(ctx context.Context, configPath string, request analysis.OverlapRequest) Response[analysis.OverlapRequest, analysis.OverlapResult] {
	return executeWithFreshConfig(ctx, configPath, request, operationExecutionPolicy{
		NotFound: analysis.IsNotFound,
	}, func(cfg *config.Config) (*analysis.OverlapResult, error) {
		return analysis.CheckOverlapContext(ctx, cfg, request)
	})
}

// CompareSpecs loads config, executes comparison, and classifies failures.
func CompareSpecs(ctx context.Context, configPath string, request analysis.CompareRequest) Response[analysis.CompareRequest, analysis.CompareResult] {
	return executeWithFreshConfig(ctx, configPath, request, operationExecutionPolicy{
		NotFound: analysis.IsNotFound,
	}, func(cfg *config.Config) (*analysis.CompareResult, error) {
		return analysis.CompareSpecsContext(ctx, cfg, request)
	})
}

// AnalyzeImpact loads config, executes impact analysis, and classifies failures.
func AnalyzeImpact(ctx context.Context, configPath string, request analysis.AnalyzeImpactRequest) Response[analysis.AnalyzeImpactRequest, analysis.AnalyzeImpactResult] {
	return executeWithFreshConfig(ctx, configPath, request, operationExecutionPolicy{
		NotFound: analysis.IsNotFound,
	}, func(cfg *config.Config) (*analysis.AnalyzeImpactResult, error) {
		return analysis.AnalyzeImpactContext(ctx, cfg, request)
	})
}

// CheckTerminology loads config, executes terminology analysis, and classifies failures.
func CheckTerminology(ctx context.Context, configPath string, request analysis.TerminologyAuditRequest) Response[analysis.TerminologyAuditRequest, analysis.TerminologyAuditResult] {
	return executeWithFreshConfig(ctx, configPath, request, operationExecutionPolicy{
		NotFound: analysis.IsNotFound,
	}, func(cfg *config.Config) (*analysis.TerminologyAuditResult, error) {
		return analysis.CheckTerminologyContext(ctx, cfg, request)
	})
}

// CompileTerminology loads config, runs terminology analysis, plans deterministic edits, and optionally applies them.
func CompileTerminology(ctx context.Context, configPath string, request CompileRequest) Response[CompileRequest, CompileResult] {
	return executeWithFreshConfig(ctx, configPath, request, operationExecutionPolicy{
		NotFound: analysis.IsNotFound,
	}, func(cfg *config.Config) (*CompileResult, error) {
		return runCompileTerminology(ctx, cfg, request)
	})
}

// CheckDocDrift loads config, executes doc drift analysis, and classifies failures.
func CheckDocDrift(ctx context.Context, configPath string, request analysis.DocDriftRequest) Response[analysis.DocDriftRequest, analysis.DocDriftResult] {
	return executeWithFreshConfig(ctx, configPath, request, operationExecutionPolicy{
		NotFound: analysis.IsNotFound,
	}, func(cfg *config.Config) (*analysis.DocDriftResult, error) {
		return analysis.CheckDocDriftContext(ctx, cfg, request)
	})
}

// CheckCompliance loads config, executes compliance analysis, and classifies failures.
func CheckCompliance(ctx context.Context, configPath string, request analysis.ComplianceRequest) Response[analysis.ComplianceRequest, analysis.ComplianceResult] {
	return executeWithFreshConfig(ctx, configPath, request, operationExecutionPolicy{}, func(cfg *config.Config) (*analysis.ComplianceResult, error) {
		return analysis.CheckComplianceContext(ctx, cfg, request)
	})
}

// ReviewSpec loads config, executes the review workflow, and classifies failures.
func ReviewSpec(ctx context.Context, configPath string, request analysis.ReviewRequest) Response[analysis.ReviewRequest, analysis.ReviewResult] {
	return executeWithFreshConfig(ctx, configPath, request, operationExecutionPolicy{
		NotFound: analysis.IsNotFound,
	}, func(cfg *config.Config) (*analysis.ReviewResult, error) {
		return analysis.ReviewSpecContext(ctx, cfg, request)
	})
}

// CheckSpecFreshness loads config, executes spec-freshness analysis, and classifies failures.
func CheckSpecFreshness(ctx context.Context, configPath string, request analysis.FreshnessRequest) Response[analysis.FreshnessRequest, analysis.FreshnessResult] {
	return executeWithFreshConfig(ctx, configPath, request, operationExecutionPolicy{
		NotFound: analysis.IsNotFound,
	}, func(cfg *config.Config) (*analysis.FreshnessResult, error) {
		return analysis.CheckSpecFreshnessContext(ctx, cfg, request)
	})
}

func loadConfig(configPath string) (*config.Config, *Issue) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, &Issue{
			Code:     CodeConfigError,
			Message:  err.Error(),
			Details:  map[string]any{IssueDetailPhase: IssuePhaseConfigLoad},
			ExitCode: 2,
		}
	}
	return cfg, nil
}

func ensureFreshIndex(ctx context.Context, cfg *config.Config) *Issue {
	started := time.Now()
	err := index.ValidateFreshnessContext(ctx, cfg)
	if tracker := resultmeta.TimingTrackerFromContext(ctx); tracker != nil {
		tracker.AddIndexing(time.Since(started))
	}
	switch {
	case err == nil:
		return nil
	case index.IsDependencyUnavailable(err):
		return dependencyUnavailableIssue(cfg, err)
	case index.IsStaleIndex(err):
		return &Issue{
			Code:     CodeConfigError,
			Message:  err.Error(),
			ExitCode: 2,
		}
	default:
		return &Issue{
			Code:     CodeInternalError,
			Message:  err.Error(),
			ExitCode: 2,
		}
	}
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

func dependencyUnavailableIssue(cfg *config.Config, err error) *Issue {
	return &Issue{
		Code:     CodeDependencyUnavailable,
		Message:  FormatDependencyUnavailableMessage(cfg, err),
		Details:  index.DependencyUnavailableDetails(err),
		ExitCode: 3,
	}
}

func missingIndexMessage(err error) string {
	path := index.MissingIndexPath(err)
	if path == "" {
		return "index does not exist; run `pituitary index --rebuild`"
	}
	return fmt.Sprintf("index %s does not exist; run `pituitary index --rebuild`", path)
}

func FormatDependencyUnavailableMessage(cfg *config.Config, err error) string {
	message := strings.TrimSpace(err.Error())
	runtimeName, provider, ok := dependencyUnavailableRuntimeContext(cfg, err, message)
	if !ok {
		return message
	}

	if improved := improveOpenAICompatibleDependencyMessage(runtimeName, provider, message); improved != "" {
		return improved
	}
	return message
}

func dependencyUnavailableRuntimeContext(cfg *config.Config, err error, message string) (string, config.RuntimeProvider, bool) {
	if cfg == nil {
		return "", config.RuntimeProvider{}, false
	}

	switch index.DependencyUnavailableRuntime(err) {
	case "runtime.analysis":
		return "runtime.analysis", cfg.Runtime.Analysis, true
	case "runtime.embedder":
		return "runtime.embedder", cfg.Runtime.Embedder, true
	}

	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "runtime.analysis"):
		return "runtime.analysis", cfg.Runtime.Analysis, true
	case strings.Contains(lower, "runtime.embedder"):
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
