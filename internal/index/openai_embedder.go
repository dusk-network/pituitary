package index

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/openaicompat"
	stembed "github.com/dusk-network/stroma/embed"
)

const (
	embeddingStrategyPlain             = "plain_v1"
	embeddingStrategyNomicSearchPrefix = "nomic_search_prefix_v1"
	openAICompatibleEmbedderRuntime    = "runtime.embedder"
	// Use a conservative default because some local OpenAI-compatible providers,
	// notably LM Studio serving nomic-embed-text, fail or destabilize on larger
	// embedding batches even when single-item requests succeed.
	openAICompatibleEmbeddingBatchSize = 8
)

type openAICompatibleEmbedder struct {
	model    string
	strategy string
	client   *openaicompat.Client

	mu        sync.Mutex
	dimension int
}

var _ Embedder = (*openAICompatibleEmbedder)(nil)
var _ stembed.ContextualEmbedder = (*openAICompatibleEmbedder)(nil)

func newOpenAICompatibleEmbedder(provider config.RuntimeProvider) (Embedder, error) {
	client, err := openaicompat.NewClient(provider, openAICompatibleEmbedderRuntime)
	if err != nil {
		return nil, err
	}

	return &openAICompatibleEmbedder{
		model:    strings.TrimSpace(provider.Model),
		strategy: embeddingStrategyForModel(provider.Model),
		client:   client,
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
		return 0, e.schemaMismatchError(1, "%s returned no embedding dimensions", openAICompatibleEmbedderRuntime)
	}
	return len(vectors[0]), nil
}

func (e *openAICompatibleEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float64, error) {
	return e.embedTexts(ctx, "document", texts)
}

func (e *openAICompatibleEmbedder) EmbedQueries(ctx context.Context, texts []string) ([][]float64, error) {
	return e.embedTexts(ctx, "query", texts)
}

func (e *openAICompatibleEmbedder) EmbedDocumentChunks(ctx context.Context, _ string, chunks []string) ([][]float64, error) {
	return e.embedTexts(ctx, "document", chunks)
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

	vectors := make([][]float64, 0, len(input))
	for start := 0; start < len(input); start += openAICompatibleEmbeddingBatchSize {
		end := start + openAICompatibleEmbeddingBatchSize
		if end > len(input) {
			end = len(input)
		}
		batchVectors, err := e.embedBatchAdaptive(ctx, input[start:end])
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, batchVectors...)
	}
	return vectors, nil
}

func (e *openAICompatibleEmbedder) embedBatchAdaptive(ctx context.Context, input []string) ([][]float64, error) {
	vectors, err := e.embedBatch(ctx, input)
	if err == nil || !shouldSplitEmbeddingBatch(err, len(input)) {
		return vectors, err
	}

	mid := len(input) / 2
	left, err := e.embedBatchAdaptive(ctx, input[:mid])
	if err != nil {
		return nil, err
	}
	right, err := e.embedBatchAdaptive(ctx, input[mid:])
	if err != nil {
		return nil, err
	}
	return append(left, right...), nil
}

func shouldSplitEmbeddingBatch(err error, batchSize int) bool {
	if batchSize <= 1 || !IsDependencyUnavailable(err) {
		return false
	}

	var target interface {
		HTTPStatusCode() int
	}
	if !errors.As(err, &target) {
		return false
	}

	status := target.HTTPStatusCode()
	return status == http.StatusRequestEntityTooLarge || status >= http.StatusInternalServerError
}

func (e *openAICompatibleEmbedder) embedBatch(ctx context.Context, input []string) ([][]float64, error) {
	payload, err := e.client.Embeddings(ctx, input)
	if err != nil {
		return nil, err
	}
	if len(payload.Data) != len(input) {
		return nil, e.schemaMismatchError(len(input), "%s returned %d embedding(s) for %d input(s)", openAICompatibleEmbedderRuntime, len(payload.Data), len(input))
	}

	vectors := make([][]float64, len(input))
	for i, item := range payload.Data {
		index := item.Index
		if index < 0 || index >= len(input) {
			index = i
		}
		if len(item.Embedding) == 0 {
			return nil, e.schemaMismatchError(len(input), "%s returned an empty embedding for input %d", openAICompatibleEmbedderRuntime, index)
		}
		if err := e.cacheDimension(len(item.Embedding)); err != nil {
			return nil, err
		}
		vectors[index] = item.Embedding
	}
	for i, vector := range vectors {
		if len(vector) == 0 {
			return nil, e.schemaMismatchError(len(input), "%s omitted embedding for input %d", openAICompatibleEmbedderRuntime, i)
		}
	}
	return vectors, nil
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
		return e.schemaMismatchError(1, "%s returned a non-positive embedding dimension", openAICompatibleEmbedderRuntime)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dimension == 0 {
		e.dimension = dimension
		return nil
	}
	if e.dimension != dimension {
		return e.schemaMismatchError(1, "%s changed embedding dimension from %d to %d", openAICompatibleEmbedderRuntime, e.dimension, dimension)
	}
	return nil
}

func (e *openAICompatibleEmbedder) embeddingFailureDetails(inputCount int) openaicompat.FailureDetails {
	details := e.client.RequestFailureDetails("embeddings")
	details.BatchSize = inputCount
	details.InputCount = inputCount
	return details
}

func (e *openAICompatibleEmbedder) schemaMismatchError(inputCount int, format string, args ...any) *openaicompat.DependencyUnavailableError {
	details := e.embeddingFailureDetails(inputCount)
	details.FailureClass = openaicompat.FailureClassSchemaMismatch
	return openaicompat.NewDependencyUnavailableWithDetails(details, format, args...)
}
