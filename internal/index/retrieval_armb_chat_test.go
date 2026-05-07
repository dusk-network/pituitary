//go:build precision_bench

package index

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dusk-network/pituitary/internal/config"
)

type armBChatClient struct {
	endpoint   string
	model      string
	apiToken   string
	maxRetries int
	client     *http.Client
}

type armBChatUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type armBChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type armBChatRequest struct {
	Model       string            `json:"model"`
	Messages    []armBChatMessage `json:"messages"`
	Temperature float64           `json:"temperature"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
}

type armBChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage armBChatUsage `json:"usage,omitempty"`
}

func newArmBChatClient(provider config.RuntimeProvider) (*armBChatClient, error) {
	if strings.TrimSpace(provider.Provider) != config.RuntimeProviderOpenAI {
		return nil, fmt.Errorf("runtime.analysis.provider must be %q for Arm B, got %q", config.RuntimeProviderOpenAI, provider.Provider)
	}
	endpoint := strings.TrimRight(strings.TrimSpace(provider.Endpoint), "/")
	model := strings.TrimSpace(provider.Model)
	if endpoint == "" || model == "" {
		return nil, fmt.Errorf("runtime.analysis endpoint and model are required for Arm B")
	}
	token := ""
	if envVar := strings.TrimSpace(provider.APIKeyEnv); envVar != "" {
		token = strings.TrimSpace(os.Getenv(envVar))
		if token == "" {
			return nil, fmt.Errorf("runtime.analysis api key env %s is empty", envVar)
		}
	}
	timeout := 60 * time.Second
	if provider.TimeoutMS > 0 {
		timeout = time.Duration(provider.TimeoutMS) * time.Millisecond
	}
	return &armBChatClient{
		endpoint:   endpoint,
		model:      model,
		apiToken:   token,
		maxRetries: provider.MaxRetries,
		client:     &http.Client{Timeout: timeout},
	}, nil
}

func (c *armBChatClient) complete(ctx context.Context, messages []armBChatMessage, maxTokens int) (string, armBChatUsage, error) {
	request := armBChatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: 0,
		MaxTokens:   maxTokens,
	}
	body, err := json.Marshal(request)
	if err != nil {
		return "", armBChatUsage{}, err
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		text, usage, err := c.completeOnce(ctx, body)
		if err == nil {
			return text, usage, nil
		}
		lastErr = err
		if attempt < c.maxRetries && isArmBRetryable(err) {
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			continue
		}
		break
	}
	return "", armBChatUsage{}, lastErr
}

func (c *armBChatClient) completeOnce(ctx context.Context, body []byte) (string, armBChatUsage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", armBChatUsage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", armBChatUsage{}, err
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", armBChatUsage{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", armBChatUsage{}, armBHTTPError{StatusCode: resp.StatusCode, Body: string(responseBody)}
	}

	var decoded armBChatResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return "", armBChatUsage{}, fmt.Errorf("decode chat completion: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", armBChatUsage{}, fmt.Errorf("chat completion returned no choices")
	}
	text := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if text == "" {
		return "", armBChatUsage{}, fmt.Errorf("chat completion returned empty content")
	}
	return text, decoded.Usage, nil
}

type armBHTTPError struct {
	StatusCode int
	Body       string
}

func (e armBHTTPError) Error() string {
	body := strings.TrimSpace(e.Body)
	if len(body) > 300 {
		body = body[:300] + "..."
	}
	return fmt.Sprintf("chat completion HTTP %d: %s", e.StatusCode, body)
}

func isArmBRetryable(err error) bool {
	httpErr, ok := err.(armBHTTPError)
	return ok && (httpErr.StatusCode == http.StatusTooManyRequests || httpErr.StatusCode >= 500)
}
