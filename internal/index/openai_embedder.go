package index

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/runtimeerr"
	stembed "github.com/dusk-network/stroma/v2/embed"
)

const (
	embeddingStrategyPlain             = "plain_v1"
	embeddingStrategyNomicSearchPrefix = "nomic_search_prefix_v1"
	openAICompatibleEmbedderRuntime    = "runtime.embedder"
	// Conservative batch cap for self-hosted gateways, notably LM Studio
	// serving nomic-embed-text, that destabilise on larger embedding batches
	// even when single-item requests succeed. The outer batch loop feeds
	// stroma one 8-item slice at a time so the adaptive split on 413/5xx
	// can halve a rejected batch in-place rather than fail the whole call.
	openAICompatibleEmbeddingBatchSize = 8
)

type openAICompatibleEmbedder struct {
	runtime    string
	provider   string
	model      string
	strategy   string
	endpoint   string
	timeoutMS  int
	maxRetries int
	client     *stembed.OpenAI

	mu        sync.Mutex
	dimension int
}

var _ Embedder = (*openAICompatibleEmbedder)(nil)
var _ stembed.ContextualEmbedder = (*openAICompatibleEmbedder)(nil)

func newOpenAICompatibleEmbedder(cfg config.RuntimeProvider) (Embedder, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	token := ""
	if envVar := strings.TrimSpace(cfg.APIKeyEnv); envVar != "" {
		token = strings.TrimSpace(os.Getenv(envVar))
		if token == "" {
			return nil, runtimeerr.NewDependencyUnavailableWithDetails(runtimeerr.FailureDetails{
				Runtime:      openAICompatibleEmbedderRuntime,
				Provider:     config.RuntimeProviderOpenAI,
				Model:        strings.TrimSpace(cfg.Model),
				Endpoint:     endpoint,
				FailureClass: runtimeerr.FailureClassAuth,
				TimeoutMS:    cfg.TimeoutMS,
				MaxRetries:   cfg.MaxRetries,
			}, "missing API key for %s", openAICompatibleEmbedderRuntime)
		}
	}

	var timeout time.Duration
	if cfg.TimeoutMS > 0 {
		timeout = time.Duration(cfg.TimeoutMS) * time.Millisecond
	}

	// Transient-failure retries (429/5xx/transport) are delegated to stroma
	// via MaxRetries. The outer batch loop here still owns the adaptive
	// 413/5xx split that halves a rejected batch — a behavior stroma does
	// not provide at the substrate level.
	client := stembed.NewOpenAI(stembed.OpenAIConfig{
		BaseURL:      endpoint,
		Model:        strings.TrimSpace(cfg.Model),
		APIToken:     token,
		Timeout:      timeout,
		MaxRetries:   cfg.MaxRetries,
		MaxBatchSize: openAICompatibleEmbeddingBatchSize,
	})

	return &openAICompatibleEmbedder{
		runtime:    openAICompatibleEmbedderRuntime,
		provider:   config.RuntimeProviderOpenAI,
		model:      strings.TrimSpace(cfg.Model),
		strategy:   embeddingStrategyForModel(cfg.Model),
		endpoint:   endpoint,
		timeoutMS:  cfg.TimeoutMS,
		maxRetries: cfg.MaxRetries,
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
		return 0, e.schemaMismatchError(1, "%s returned no embedding dimensions", openAICompatibleEmbedderRuntime)
	}
	return len(vectors[0]), nil
}

func (e *openAICompatibleEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float64, error) {
	return e.embedTexts(ctx, false, texts)
}

func (e *openAICompatibleEmbedder) EmbedQueries(ctx context.Context, texts []string) ([][]float64, error) {
	return e.embedTexts(ctx, true, texts)
}

func (e *openAICompatibleEmbedder) EmbedDocumentChunks(ctx context.Context, _ string, chunks []string) ([][]float64, error) {
	// Current OpenAI-compatible providers embed each chunk independently, so
	// document context is intentionally unused.
	return e.embedTexts(ctx, false, chunks)
}

func (e *openAICompatibleEmbedder) embedTexts(ctx context.Context, isQuery bool, texts []string) ([][]float64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	vectors := make([][]float64, 0, len(texts))
	for start := 0; start < len(texts); start += openAICompatibleEmbeddingBatchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := min(start+openAICompatibleEmbeddingBatchSize, len(texts))
		batchVectors, err := e.embedBatchAdaptive(ctx, isQuery, texts[start:end])
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, batchVectors...)
	}
	return vectors, nil
}

func (e *openAICompatibleEmbedder) embedBatchAdaptive(ctx context.Context, isQuery bool, batch []string) ([][]float64, error) {
	vectors, err := e.embedBatch(ctx, isQuery, batch)
	if err == nil || !shouldSplitEmbeddingBatch(err, len(batch)) {
		return vectors, err
	}

	mid := len(batch) / 2
	left, err := e.embedBatchAdaptive(ctx, isQuery, batch[:mid])
	if err != nil {
		return nil, err
	}
	right, err := e.embedBatchAdaptive(ctx, isQuery, batch[mid:])
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

func (e *openAICompatibleEmbedder) embedBatch(ctx context.Context, isQuery bool, batch []string) ([][]float64, error) {
	var (
		vectors [][]float64
		err     error
	)
	if isQuery {
		vectors, err = e.client.EmbedQueries(ctx, batch)
	} else {
		vectors, err = e.client.EmbedDocuments(ctx, batch)
	}
	if err != nil {
		return nil, runtimeerr.FromProviderError(err, e.embeddingFailureLabels(len(batch)))
	}

	if len(vectors) != len(batch) {
		return nil, e.schemaMismatchError(len(batch), "%s returned %d embedding(s) for %d input(s)", openAICompatibleEmbedderRuntime, len(vectors), len(batch))
	}
	for i, vector := range vectors {
		if len(vector) == 0 {
			return nil, e.schemaMismatchError(len(batch), "%s omitted embedding for input %d", openAICompatibleEmbedderRuntime, i)
		}
		if err := e.cacheDimension(len(vector)); err != nil {
			return nil, err
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

func (e *openAICompatibleEmbedder) embeddingFailureLabels(inputCount int) runtimeerr.FailureDetails {
	return runtimeerr.FailureDetails{
		Runtime:     e.runtime,
		Provider:    e.provider,
		Model:       e.model,
		Endpoint:    e.endpoint,
		RequestType: "embeddings",
		TimeoutMS:   e.timeoutMS,
		MaxRetries:  e.maxRetries,
		BatchSize:   inputCount,
		InputCount:  inputCount,
	}
}

func (e *openAICompatibleEmbedder) schemaMismatchError(inputCount int, format string, args ...any) *runtimeerr.DependencyUnavailableError {
	details := e.embeddingFailureLabels(inputCount)
	details.FailureClass = runtimeerr.FailureClassSchemaMismatch
	return runtimeerr.NewDependencyUnavailableWithDetails(details, format, args...)
}
