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
	Runtime    string
	Model      string
	Endpoint   string
	Token      string
	MaxRetries int
	HTTPClient *http.Client
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
	token := ""
	if envVar := strings.TrimSpace(provider.APIKeyEnv); envVar != "" {
		token = strings.TrimSpace(os.Getenv(envVar))
		if token == "" {
			return nil, NewDependencyUnavailable(runtime, "missing API key for %s", runtime)
		}
	}

	httpClient := &http.Client{}
	if provider.TimeoutMS > 0 {
		httpClient.Timeout = time.Duration(provider.TimeoutMS) * time.Millisecond
	}

	return &Client{
		Runtime:    strings.TrimSpace(runtime),
		Model:      strings.TrimSpace(provider.Model),
		Endpoint:   strings.TrimRight(strings.TrimSpace(provider.Endpoint), "/"),
		Token:      token,
		MaxRetries: provider.MaxRetries,
		HTTPClient: httpClient,
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

	return doWithRetries(ctx, c, "/embeddings", body, func(resp *http.Response, responseBody []byte) (*EmbeddingsResponse, error) {
		var payload EmbeddingsResponse
		if err := json.Unmarshal(responseBody, &payload); err != nil {
			return nil, NewDependencyUnavailable(c.Runtime, "decode %s response: %v", c.Runtime, err)
		}
		if message := ExtractErrorValue(payload.Err); message != "" {
			return nil, NewDependencyUnavailable(c.Runtime, "%s endpoint %s returned an error: %s", c.Runtime, resp.Request.URL, message)
		}
		return &payload, nil
	})
}

func (c *Client) ChatCompletionText(ctx context.Context, messages []ChatMessage, temperature float64) (string, error) {
	body, err := json.Marshal(ChatRequest{
		Model:       c.Model,
		Messages:    messages,
		Temperature: temperature,
	})
	if err != nil {
		return "", fmt.Errorf("encode %s request: %w", c.Runtime, err)
	}

	return doWithRetries(ctx, c, "/chat/completions", body, func(resp *http.Response, responseBody []byte) (string, error) {
		var payload chatResponse
		if err := json.Unmarshal(responseBody, &payload); err != nil {
			return "", NewDependencyUnavailable(c.Runtime, "decode %s response: %v", c.Runtime, err)
		}
		if message := ExtractErrorValue(payload.Err); message != "" {
			return "", NewDependencyUnavailable(c.Runtime, "%s endpoint %s returned an error: %s", c.Runtime, resp.Request.URL, message)
		}
		if len(payload.Choices) == 0 {
			return "", NewDependencyUnavailable(c.Runtime, "%s returned no choices", c.Runtime)
		}

		text := ExtractMessageText(payload.Choices[0].Message.Content)
		if text == "" {
			return "", NewDependencyUnavailable(c.Runtime, "%s returned an empty message", c.Runtime)
		}
		return text, nil
	})
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

func doWithRetries[T any](ctx context.Context, client *Client, path string, body []byte, decode func(*http.Response, []byte) (T, error)) (T, error) {
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
			lastErr = NewDependencyUnavailable(client.Runtime, "call %s endpoint %s: %v", client.Runtime, client.Endpoint, err)
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
		resp.Body.Close()
		if readErr != nil {
			err = NewDependencyUnavailable(client.Runtime, "read %s response: %v", client.Runtime, readErr)
		} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			message := ExtractErrorMessage(responseBody)
			if message == "" {
				message = strings.TrimSpace(string(responseBody))
			}
			if message == "" {
				message = http.StatusText(resp.StatusCode)
			}
			err = NewDependencyUnavailable(client.Runtime, "%s endpoint %s returned %s: %s", client.Runtime, resp.Request.URL, resp.Status, message)
		} else {
			value, decodeErr := decode(resp, responseBody)
			if decodeErr == nil {
				return value, nil
			}
			err = decodeErr
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
		return netErr.Timeout() || netErr.Temporary()
	}
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "broken pipe")
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
