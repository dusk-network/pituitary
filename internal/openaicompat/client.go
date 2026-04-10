package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dusk-network/pituitary/internal/config"
)

const responseSizeLimit = 4 << 20

type Client struct {
	Runtime           string
	Provider          string
	Model             string
	Endpoint          string
	Token             string
	TimeoutMS         int
	MaxRetries        int
	MaxResponseTokens int
	HTTPClient        *http.Client
}

type EmbeddingsRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type EmbeddingsResponse struct {
	Data []Embedding     `json:"data"`
	Err  json.RawMessage `json:"error,omitempty"`
}

type Embedding struct {
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []chatChoice    `json:"choices"`
	Err     json.RawMessage `json:"error,omitempty"`
}

type chatChoice struct {
	Message chatChoiceMessage `json:"message"`
}

type chatChoiceMessage struct {
	Content json.RawMessage `json:"content"`
}

func NewClient(provider config.RuntimeProvider, runtime string) (*Client, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(provider.Endpoint), "/")
	token := ""
	if envVar := strings.TrimSpace(provider.APIKeyEnv); envVar != "" {
		token = strings.TrimSpace(os.Getenv(envVar))
		if token == "" {
			return nil, NewDependencyUnavailableWithDetails(FailureDetails{
				Runtime:      strings.TrimSpace(runtime),
				Provider:     config.RuntimeProviderOpenAI,
				Model:        strings.TrimSpace(provider.Model),
				Endpoint:     endpoint,
				FailureClass: FailureClassAuth,
				TimeoutMS:    provider.TimeoutMS,
				MaxRetries:   provider.MaxRetries,
			}, "missing API key for %s", runtime)
		}
	}

	httpClient := &http.Client{}
	if provider.TimeoutMS > 0 {
		httpClient.Timeout = time.Duration(provider.TimeoutMS) * time.Millisecond
	}

	return &Client{
		Runtime:           strings.TrimSpace(runtime),
		Provider:          config.RuntimeProviderOpenAI,
		Model:             strings.TrimSpace(provider.Model),
		Endpoint:          endpoint,
		Token:             token,
		TimeoutMS:         provider.TimeoutMS,
		MaxRetries:        provider.MaxRetries,
		MaxResponseTokens: provider.MaxResponseTokens,
		HTTPClient:        httpClient,
	}, nil
}

func (c *Client) Embeddings(ctx context.Context, input []string) (*EmbeddingsResponse, error) {
	body, err := json.Marshal(EmbeddingsRequest{
		Model: c.Model,
		Input: input,
	})
	if err != nil {
		return nil, fmt.Errorf("encode %s request: %w", c.Runtime, err)
	}

	details := c.RequestFailureDetails("embeddings")
	details.BatchSize = len(input)
	details.InputCount = len(input)

	return doWithRetries(ctx, c, "/embeddings", body, details, func(resp *http.Response, responseBody []byte) (*EmbeddingsResponse, error) {
		var payload EmbeddingsResponse
		if err := json.Unmarshal(responseBody, &payload); err != nil {
			failure := details
			failure.FailureClass = FailureClassSchemaMismatch
			return nil, NewDependencyUnavailableWithDetails(failure, "decode %s response: %v", c.Runtime, err)
		}
		if message := ExtractErrorValue(payload.Err); message != "" {
			failure := classifiedFailureDetails(details, 0, nil, message)
			return nil, NewDependencyUnavailableWithDetails(failure, "%s endpoint %s returned an error: %s", c.Runtime, resp.Request.URL, message)
		}
		return &payload, nil
	})
}

func (c *Client) ChatCompletionText(ctx context.Context, messages []ChatMessage, temperature float64, maxTokens int) (string, error) {
	body, err := json.Marshal(ChatRequest{
		Model:       c.Model,
		Messages:    messages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("encode %s request: %w", c.Runtime, err)
	}

	details := c.RequestFailureDetails("analysis")
	details.InputCount = len(messages)

	return doWithRetries(ctx, c, "/chat/completions", body, details, func(resp *http.Response, responseBody []byte) (string, error) {
		var payload chatResponse
		if err := json.Unmarshal(responseBody, &payload); err != nil {
			failure := details
			failure.FailureClass = FailureClassSchemaMismatch
			return "", NewDependencyUnavailableWithDetails(failure, "decode %s response: %v", c.Runtime, err)
		}
		if message := ExtractErrorValue(payload.Err); message != "" {
			failure := classifiedFailureDetails(details, 0, nil, message)
			return "", NewDependencyUnavailableWithDetails(failure, "%s endpoint %s returned an error: %s", c.Runtime, resp.Request.URL, message)
		}
		if len(payload.Choices) == 0 {
			failure := details
			failure.FailureClass = FailureClassSchemaMismatch
			return "", NewDependencyUnavailableWithDetails(failure, "%s returned no choices", c.Runtime)
		}

		text := ExtractMessageText(payload.Choices[0].Message.Content)
		if text == "" {
			failure := details
			failure.FailureClass = FailureClassSchemaMismatch
			return "", NewDependencyUnavailableWithDetails(failure, "%s returned an empty message", c.Runtime)
		}
		return text, nil
	})
}

func (c *Client) RequestFailureDetails(requestType string) FailureDetails {
	return FailureDetails{
		Runtime:     strings.TrimSpace(c.Runtime),
		Provider:    strings.TrimSpace(c.Provider),
		Model:       strings.TrimSpace(c.Model),
		Endpoint:    strings.TrimSpace(c.Endpoint),
		RequestType: strings.TrimSpace(requestType),
		TimeoutMS:   c.TimeoutMS,
		MaxRetries:  c.MaxRetries,
	}
}

func ExtractMessageText(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var builder strings.Builder
		for _, part := range parts {
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(strings.TrimSpace(part.Text))
		}
		return strings.TrimSpace(builder.String())
	}

	return strings.TrimSpace(string(raw))
}

func doWithRetries[T any](ctx context.Context, client *Client, path string, body []byte, details FailureDetails, decode func(*http.Response, []byte) (T, error)) (T, error) {
	var zero T
	var lastErr error

	for attempt := 0; attempt <= client.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.Endpoint+path, bytes.NewReader(body))
		if err != nil {
			return zero, fmt.Errorf("build %s request: %w", client.Runtime, err)
		}
		req.Header.Set("Content-Type", "application/json")
		if client.Token != "" {
			req.Header.Set("Authorization", "Bearer "+client.Token)
		}

		resp, err := client.HTTPClient.Do(req)
		if err != nil {
			if errors.Is(err, context.Canceled) || (errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil) {
				return zero, err
			}
			failure := classifiedFailureDetails(details, 0, err, err.Error())
			lastErr = NewDependencyUnavailableWithDetails(failure, "call %s endpoint %s: %v", client.Runtime, client.Endpoint, err)
			if shouldRetry(err, 0) && attempt < client.MaxRetries {
				if waitErr := waitBeforeRetry(ctx, attempt, 0); waitErr != nil {
					return zero, waitErr
				}
				continue
			}
			return zero, lastErr
		}

		retryAfter := retryAfterDuration(resp.Header.Get("Retry-After"))
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, responseSizeLimit))
		closeErr := resp.Body.Close()
		if readErr != nil {
			failure := classifiedFailureDetails(details, 0, readErr, readErr.Error())
			err = NewDependencyUnavailableWithDetails(failure, "read %s response: %v", client.Runtime, readErr)
		} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			message := ExtractErrorMessage(responseBody)
			if message == "" {
				message = strings.TrimSpace(string(responseBody))
			}
			if message == "" {
				message = http.StatusText(resp.StatusCode)
			}
			failure := classifiedFailureDetails(details, resp.StatusCode, nil, message)
			err = NewDependencyUnavailableStatusWithDetails(failure, resp.StatusCode, "%s endpoint %s returned %s: %s", client.Runtime, resp.Request.URL, resp.Status, message)
		} else {
			value, decodeErr := decode(resp, responseBody)
			if decodeErr == nil {
				if closeErr != nil {
					failure := classifiedFailureDetails(details, 0, closeErr, closeErr.Error())
					return zero, NewDependencyUnavailableWithDetails(failure, "close %s response: %v", client.Runtime, closeErr)
				}
				return value, nil
			}
			err = decodeErr
		}
		if closeErr != nil && err == nil {
			failure := classifiedFailureDetails(details, 0, closeErr, closeErr.Error())
			err = NewDependencyUnavailableWithDetails(failure, "close %s response: %v", client.Runtime, closeErr)
		}

		lastErr = err
		if shouldRetry(err, resp.StatusCode) && attempt < client.MaxRetries {
			if waitErr := waitBeforeRetry(ctx, attempt, retryAfter); waitErr != nil {
				return zero, waitErr
			}
			continue
		}
		return zero, err
	}

	if lastErr == nil {
		lastErr = NewDependencyUnavailable(client.Runtime, "%s request failed", client.Runtime)
	}
	return zero, lastErr
}

func shouldRetry(err error, statusCode int) bool {
	if statusCode == http.StatusTooManyRequests || statusCode >= 500 {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "broken pipe")
}

func classifiedFailureDetails(details FailureDetails, statusCode int, err error, message string) FailureDetails {
	failure := details
	failure.HTTPStatus = statusCode
	failure.FailureClass = normalizeFailureClass(statusCode, err, message)
	return failure
}

func normalizeFailureClass(statusCode int, err error, message string) string {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return FailureClassAuth
	case http.StatusTooManyRequests:
		return FailureClassRateLimit
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return FailureClassTimeout
	}
	if statusCode >= http.StatusInternalServerError {
		return FailureClassServer
	}

	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case mentionsAuthFailure(lower):
		return FailureClassAuth
	case mentionsRateLimit(lower):
		return FailureClassRateLimit
	case isTimeoutFailure(err, lower):
		return FailureClassTimeout
	case isTransportFailure(err, lower):
		return FailureClassTransport
	default:
		return FailureClassDependency
	}
}

func mentionsAuthFailure(message string) bool {
	return strings.Contains(message, "api key") ||
		strings.Contains(message, "apikey") ||
		strings.Contains(message, "auth") ||
		strings.Contains(message, "unauthorized") ||
		strings.Contains(message, "forbidden")
}

func mentionsRateLimit(message string) bool {
	return strings.Contains(message, "rate limit") ||
		strings.Contains(message, "too many requests") ||
		strings.Contains(message, "quota exceeded")
}

func isTimeoutFailure(err error, message string) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return strings.Contains(message, "context deadline exceeded") ||
		strings.Contains(message, "client.timeout exceeded") ||
		strings.Contains(message, "timeout awaiting response headers") ||
		strings.Contains(message, "i/o timeout") ||
		strings.Contains(message, "timed out")
}

func isTransportFailure(err error, message string) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return strings.Contains(message, "connection refused") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "broken pipe") ||
		strings.Contains(message, "couldn't connect to server") ||
		strings.Contains(message, "failed to connect") ||
		strings.Contains(message, "no such host") ||
		strings.Contains(message, "network is unreachable") ||
		strings.Contains(message, "unexpected eof") ||
		message == "eof"
}

func retryAfterDuration(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
		return 0
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return 0
}

func waitBeforeRetry(ctx context.Context, attempt int, retryAfter time.Duration) error {
	delay := retryAfter
	if delay <= 0 {
		delay = 200 * time.Millisecond
		for i := 0; i < attempt; i++ {
			delay *= 2
			if delay >= 2*time.Second {
				delay = 2 * time.Second
				break
			}
		}
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
