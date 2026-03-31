package openaicompat

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DependencyUnavailableError indicates that an OpenAI-compatible runtime could
// not satisfy the current request.
type DependencyUnavailableError struct {
	Runtime    string
	Message    string
	HTTPStatus int
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
