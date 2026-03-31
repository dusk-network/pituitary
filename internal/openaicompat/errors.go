package openaicompat

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	FailureClassAuth           = "auth"
	FailureClassDependency     = "dependency_unavailable"
	FailureClassRateLimit      = "rate_limit"
	FailureClassSchemaMismatch = "schema_mismatch"
	FailureClassServer         = "server"
	FailureClassTimeout        = "timeout"
	FailureClassTransport      = "transport"
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

// DependencyUnavailableError indicates that an OpenAI-compatible runtime could
// not satisfy the current request.
type DependencyUnavailableError struct {
	Runtime    string
	Message    string
	HTTPStatus int
	Details    *FailureDetails
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

// NewDependencyUnavailableStatusWithDetails records structured failure metadata plus an HTTP status.
func NewDependencyUnavailableStatusWithDetails(details FailureDetails, status int, format string, args ...any) *DependencyUnavailableError {
	details.HTTPStatus = status
	return &DependencyUnavailableError{
		Runtime:    details.Runtime,
		Message:    fmt.Sprintf(format, args...),
		HTTPStatus: status,
		Details:    &details,
	}
}

// ExtractErrorMessage returns a human-readable error message from a full
// OpenAI-compatible response body.
func ExtractErrorMessage(body []byte) string {
	var payload struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	if strings.TrimSpace(payload.Message) != "" {
		return strings.TrimSpace(payload.Message)
	}
	return ExtractErrorValue(payload.Error)
}

// ExtractErrorValue returns a human-readable error message from an
// OpenAI-compatible `error` field that may be either a string or an object.
func ExtractErrorValue(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}

	var payload struct {
		Message string `json:"message"`
		Error   string `json:"error"`
		Detail  string `json:"detail"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil {
		switch {
		case strings.TrimSpace(payload.Message) != "":
			return strings.TrimSpace(payload.Message)
		case strings.TrimSpace(payload.Error) != "":
			return strings.TrimSpace(payload.Error)
		case strings.TrimSpace(payload.Detail) != "":
			return strings.TrimSpace(payload.Detail)
		}
	}

	return ""
}
