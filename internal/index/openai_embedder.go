package index

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
	"strings"
	"sync"
	"time"

	"github.com/dusk-network/pituitary/internal/config"
)

const (
	embeddingStrategyPlain             = "plain_v1"
	embeddingStrategyNomicSearchPrefix = "nomic_search_prefix_v1"
)

type openAICompatibleEmbedder struct {
	model      string
	endpoint   string
	token      string
	maxRetries int
	strategy   string
	client     *http.Client

	mu        sync.Mutex
	dimension int
}

type openAICompatibleEmbeddingsRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAICompatibleEmbeddingsResponse struct {
	Data []openAICompatibleEmbedding `json:"data"`
	Err  json.RawMessage             `json:"error,omitempty"`
}

type openAICompatibleEmbedding struct {
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

func newOpenAICompatibleEmbedder(provider config.RuntimeProvider) (Embedder, error) {
	token := ""
	if envVar := strings.TrimSpace(provider.APIKeyEnv); envVar != "" {
		token = strings.TrimSpace(os.Getenv(envVar))
		if token == "" {
			return nil, &DependencyUnavailableError{Message: "missing API key for runtime.embedder"}
		}
	}

	client := &http.Client{}
	if provider.TimeoutMS > 0 {
		client.Timeout = time.Duration(provider.TimeoutMS) * time.Millisecond
	}

	return &openAICompatibleEmbedder{
		model:      strings.TrimSpace(provider.Model),
		endpoint:   strings.TrimRight(strings.TrimSpace(provider.Endpoint), "/"),
		token:      token,
		maxRetries: provider.MaxRetries,
		strategy:   embeddingStrategyForModel(provider.Model),
		client:     client,
	}, nil
}

func (e *openAICompatibleEmbedder) Fingerprint() string {
	return embedderFingerprint(config.RuntimeProviderOpenAI, e.model, e.strategy)
}

func (e *openAICompatibleEmbedder) Dimension(ctx context.Context) (int, error) {
	if dimension := e.cachedDimension(); dimension > 0 {
		return dimension, nil
	}

	vectors, err := e.EmbedQueries(ctx, []string{"dimension probe"})
	if err != nil {
		return 0, err
	}
	if len(vectors) != 1 || len(vectors[0]) == 0 {
		return 0, &DependencyUnavailableError{Message: "runtime.embedder returned no embedding dimensions"}
	}
	return len(vectors[0]), nil
}

func (e *openAICompatibleEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float64, error) {
	return e.embedTexts(ctx, "document", texts)
}

func (e *openAICompatibleEmbedder) EmbedQueries(ctx context.Context, texts []string) ([][]float64, error) {
	return e.embedTexts(ctx, "query", texts)
}

func (e *openAICompatibleEmbedder) embedTexts(ctx context.Context, purpose string, texts []string) ([][]float64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	input := make([]string, 0, len(texts))
	for _, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		input = append(input, prepareEmbeddingInput(e.strategy, purpose, text))
	}

	body, err := json.Marshal(openAICompatibleEmbeddingsRequest{
		Model: e.model,
		Input: input,
	})
	if err != nil {
		return nil, fmt.Errorf("encode runtime.embedder request: %w", err)
	}

	payload, err := e.requestEmbeddings(ctx, body)
	if err != nil {
		return nil, err
	}
	if len(payload.Data) != len(input) {
		return nil, &DependencyUnavailableError{
			Message: fmt.Sprintf("runtime.embedder returned %d embedding(s) for %d input(s)", len(payload.Data), len(input)),
		}
	}

	vectors := make([][]float64, len(input))
	for i, item := range payload.Data {
		index := item.Index
		if index < 0 || index >= len(input) {
			index = i
		}
		if len(item.Embedding) == 0 {
			return nil, &DependencyUnavailableError{
				Message: fmt.Sprintf("runtime.embedder returned an empty embedding for input %d", index),
			}
		}
		if err := e.cacheDimension(len(item.Embedding)); err != nil {
			return nil, err
		}
		vectors[index] = item.Embedding
	}
	for i, vector := range vectors {
		if len(vector) == 0 {
			return nil, &DependencyUnavailableError{
				Message: fmt.Sprintf("runtime.embedder omitted embedding for input %d", i),
			}
		}
	}
	return vectors, nil
}

func (e *openAICompatibleEmbedder) requestEmbeddings(ctx context.Context, body []byte) (*openAICompatibleEmbeddingsResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint+"/embeddings", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("build runtime.embedder request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if e.token != "" {
			req.Header.Set("Authorization", "Bearer "+e.token)
		}

		resp, err := e.client.Do(req)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil {
				return nil, err
			}
			lastErr = &DependencyUnavailableError{
				Message: fmt.Sprintf("call runtime.embedder endpoint %s: %v", e.endpoint, err),
			}
			if shouldRetryOpenAICompatibleRequest(err, 0) && attempt < e.maxRetries {
				continue
			}
			return nil, lastErr
		}

		payload, err := readOpenAICompatibleEmbeddingsResponse(resp)
		resp.Body.Close()
		if err == nil {
			return payload, nil
		}
		lastErr = err
		if shouldRetryOpenAICompatibleRequest(err, resp.StatusCode) && attempt < e.maxRetries {
			continue
		}
		return nil, err
	}

	if lastErr == nil {
		lastErr = &DependencyUnavailableError{Message: "runtime.embedder request failed"}
	}
	return nil, lastErr
}

func readOpenAICompatibleEmbeddingsResponse(resp *http.Response) (*openAICompatibleEmbeddingsResponse, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, &DependencyUnavailableError{Message: fmt.Sprintf("read runtime.embedder response: %v", err)}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := extractOpenAICompatibleError(body)
		if message == "" {
			message = strings.TrimSpace(string(body))
		}
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, &DependencyUnavailableError{
			Message: fmt.Sprintf("runtime.embedder endpoint %s returned %s: %s", resp.Request.URL, resp.Status, message),
		}
	}

	var payload openAICompatibleEmbeddingsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, &DependencyUnavailableError{
			Message: fmt.Sprintf("decode runtime.embedder response: %v", err),
		}
	}
	if message := extractOpenAICompatibleErrorValue(payload.Err); message != "" {
		return nil, &DependencyUnavailableError{
			Message: fmt.Sprintf("runtime.embedder endpoint %s returned an error: %s", resp.Request.URL, message),
		}
	}
	return &payload, nil
}

func extractOpenAICompatibleError(body []byte) string {
	var payload openAICompatibleEmbeddingsResponse
	if err := json.Unmarshal(body, &payload); err == nil {
		return extractOpenAICompatibleErrorValue(payload.Err)
	}
	return ""
}

func extractOpenAICompatibleErrorValue(raw json.RawMessage) string {
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

func shouldRetryOpenAICompatibleRequest(err error, statusCode int) bool {
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
	return strings.Contains(strings.ToLower(err.Error()), "connection refused") ||
		strings.Contains(strings.ToLower(err.Error()), "connection reset") ||
		strings.Contains(strings.ToLower(err.Error()), "broken pipe")
}

func embeddingStrategyForModel(model string) string {
	if strings.Contains(strings.ToLower(model), "nomic-embed-text") {
		return embeddingStrategyNomicSearchPrefix
	}
	return embeddingStrategyPlain
}

func prepareEmbeddingInput(strategy, purpose, text string) string {
	switch strategy {
	case embeddingStrategyNomicSearchPrefix:
		switch purpose {
		case "query":
			return "search_query: " + text
		default:
			return "search_document: " + text
		}
	default:
		return text
	}
}

func (e *openAICompatibleEmbedder) cachedDimension() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.dimension
}

func (e *openAICompatibleEmbedder) cacheDimension(dimension int) error {
	if dimension <= 0 {
		return &DependencyUnavailableError{Message: "runtime.embedder returned a non-positive embedding dimension"}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dimension == 0 {
		e.dimension = dimension
		return nil
	}
	if e.dimension != dimension {
		return &DependencyUnavailableError{
			Message: fmt.Sprintf("runtime.embedder changed embedding dimension from %d to %d", e.dimension, dimension),
		}
	}
	return nil
}
