// Package runtimeerr describes failures that a configured runtime surface
// (embedder, analyzer, probe) could not satisfy. The substrate classification
// already shipped by stroma's provider package is preserved; this package
// overlays the Pituitary-specific product labels (runtime name, provider id,
// request type) that governance callers branch on for diagnostics and
// graceful degradation.
package runtimeerr

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dusk-network/stroma/v2/provider"
)

// Failure classes are mirrored from stroma's provider package so callers
// can classify failures without importing stroma directly.
const (
	FailureClassAuth           = provider.FailureClassAuth
	FailureClassDependency     = provider.FailureClassDependencyUnavailable
	FailureClassRateLimit      = provider.FailureClassRateLimit
	FailureClassSchemaMismatch = provider.FailureClassSchemaMismatch
	FailureClassServer         = provider.FailureClassServer
	FailureClassTimeout        = provider.FailureClassTimeout
	FailureClassTransport      = provider.FailureClassTransport
)

// FailureDetails captures structured runtime metadata for provider failures.
type FailureDetails struct {
	Runtime      string
	Provider     string
	Model        string
	Endpoint     string
	RequestType  string
	FailureClass string
	HTTPStatus   int
	TimeoutMS    int
	MaxRetries   int
	BatchSize    int
	InputCount   int
}

// Map returns the details as a JSON-friendly object with empty fields omitted.
func (d FailureDetails) Map() map[string]any {
	values := map[string]any{}
	if value := strings.TrimSpace(d.Runtime); value != "" {
		values["runtime"] = value
	}
	if value := strings.TrimSpace(d.Provider); value != "" {
		values["provider"] = value
	}
	if value := strings.TrimSpace(d.Model); value != "" {
		values["model"] = value
	}
	if value := strings.TrimSpace(d.Endpoint); value != "" {
		values["endpoint"] = value
	}
	if value := strings.TrimSpace(d.RequestType); value != "" {
		values["request_type"] = value
	}
	if value := strings.TrimSpace(d.FailureClass); value != "" {
		values["failure_class"] = value
	}
	if d.HTTPStatus > 0 {
		values["http_status"] = d.HTTPStatus
	}
	if d.TimeoutMS > 0 {
		values["timeout_ms"] = d.TimeoutMS
	}
	if d.MaxRetries > 0 {
		values["max_retries"] = d.MaxRetries
	}
	if d.BatchSize > 0 {
		values["batch_size"] = d.BatchSize
	}
	if d.InputCount > 0 {
		values["input_count"] = d.InputCount
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

// DependencyUnavailableError indicates that a runtime surface could not
// satisfy the request.
type DependencyUnavailableError struct {
	Runtime    string
	Message    string
	HTTPStatus int
	Details    *FailureDetails
	cause      error
}

func (e *DependencyUnavailableError) Error() string {
	return e.Message
}

// RuntimeName returns the associated runtime surface.
func (e *DependencyUnavailableError) RuntimeName() string {
	return strings.TrimSpace(e.Runtime)
}

// HTTPStatusCode returns the associated HTTP status when the dependency
// failure came from an HTTP response, or zero otherwise.
func (e *DependencyUnavailableError) HTTPStatusCode() int {
	return e.HTTPStatus
}

// DiagnosticFields returns normalized structured diagnostics when available.
func (e *DependencyUnavailableError) DiagnosticFields() map[string]any {
	if e == nil || e.Details == nil {
		return nil
	}
	details := *e.Details
	if strings.TrimSpace(details.Runtime) == "" {
		details.Runtime = e.Runtime
	}
	if details.HTTPStatus == 0 {
		details.HTTPStatus = e.HTTPStatus
	}
	return details.Map()
}

// Unwrap exposes the wrapped cause, enabling errors.Is/errors.As traversal
// back to the underlying stroma provider.Error when present.
func (e *DependencyUnavailableError) Unwrap() error {
	return e.cause
}

// NewDependencyUnavailable formats a dependency-unavailable runtime error.
func NewDependencyUnavailable(runtime, format string, args ...any) *DependencyUnavailableError {
	return &DependencyUnavailableError{
		Runtime: runtime,
		Message: fmt.Sprintf(format, args...),
	}
}

// NewDependencyUnavailableStatus formats a dependency-unavailable runtime
// error and records the associated HTTP status code.
func NewDependencyUnavailableStatus(runtime string, status int, format string, args ...any) *DependencyUnavailableError {
	return &DependencyUnavailableError{
		Runtime:    runtime,
		Message:    fmt.Sprintf(format, args...),
		HTTPStatus: status,
	}
}

// NewDependencyUnavailableWithDetails records structured failure metadata.
func NewDependencyUnavailableWithDetails(details FailureDetails, format string, args ...any) *DependencyUnavailableError {
	return &DependencyUnavailableError{
		Runtime: details.Runtime,
		Message: fmt.Sprintf(format, args...),
		Details: &details,
	}
}

// NewDependencyUnavailableStatusWithDetails records structured failure
// metadata plus an HTTP status.
func NewDependencyUnavailableStatusWithDetails(details FailureDetails, status int, format string, args ...any) *DependencyUnavailableError {
	details.HTTPStatus = status
	return &DependencyUnavailableError{
		Runtime:    details.Runtime,
		Message:    fmt.Sprintf(format, args...),
		HTTPStatus: status,
		Details:    &details,
	}
}

// FromProviderError wraps a stroma *provider.Error with the supplied product
// labels. Substrate classification, HTTP status, endpoint/model/batch metrics
// recorded on the stroma error survive; labels overlay the Pituitary-specific
// Runtime, Provider, RequestType fields. Non-provider errors pass through
// unwrapped so callers retain errors.Is visibility to context cancellation.
func FromProviderError(err error, labels FailureDetails) error {
	if err == nil {
		return nil
	}
	var perr *provider.Error
	if !errors.As(err, &perr) {
		return err
	}

	details := labels
	if details.FailureClass == "" {
		if class := strings.TrimSpace(perr.FailureClass()); class != "" {
			details.FailureClass = class
		} else {
			details.FailureClass = FailureClassDependency
		}
	}
	status := perr.HTTPStatusCode()
	if details.HTTPStatus == 0 {
		details.HTTPStatus = status
	}
	if substrate := perr.Details; substrate != nil {
		if details.Model == "" {
			details.Model = substrate.Model
		}
		if details.Endpoint == "" {
			details.Endpoint = substrate.Endpoint
		}
		if details.TimeoutMS == 0 {
			details.TimeoutMS = substrate.TimeoutMS
		}
		if details.MaxRetries == 0 {
			details.MaxRetries = substrate.MaxRetries
		}
		if details.BatchSize == 0 {
			details.BatchSize = substrate.BatchSize
		}
		if details.InputCount == 0 {
			details.InputCount = substrate.InputCount
		}
	}

	return &DependencyUnavailableError{
		Runtime:    details.Runtime,
		Message:    perr.Error(),
		HTTPStatus: status,
		Details:    &details,
		cause:      perr,
	}
}

// ExtractErrorMessage returns a human-readable error message from a full
// OpenAI-compatible response body. Delegates to stroma's provider helper so
// callers do not need to import stroma directly.
func ExtractErrorMessage(body []byte) string {
	return provider.ExtractErrorMessage(body)
}

// ExtractErrorValue returns a human-readable error message from an
// OpenAI-compatible `error` field that may be either a string or an object.
func ExtractErrorValue(raw json.RawMessage) string {
	return provider.ExtractErrorValue(raw)
}
