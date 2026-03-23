package index

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"unicode"

	"github.com/dusk-network/pituitary/internal/config"
)

// DependencyUnavailableError indicates that a required runtime dependency
// cannot satisfy the current command.
type DependencyUnavailableError struct {
	Runtime string
	Message string
}

func (e *DependencyUnavailableError) Error() string {
	return e.Message
}

// IsDependencyUnavailable reports whether err wraps a dependency-unavailable failure.
func IsDependencyUnavailable(err error) bool {
	var target *DependencyUnavailableError
	return errors.As(err, &target)
}

// DependencyUnavailableRuntime reports the runtime surface associated with a
// dependency-unavailable failure, if one was recorded.
func DependencyUnavailableRuntime(err error) string {
	var target *DependencyUnavailableError
	if !errors.As(err, &target) {
		return ""
	}
	return strings.TrimSpace(target.Runtime)
}

// Embedder generates embeddings for rebuild and query-time retrieval.
type Embedder interface {
	Fingerprint() string
	Dimension(ctx context.Context) (int, error)
	EmbedDocuments(ctx context.Context, texts []string) ([][]float64, error)
	EmbedQueries(ctx context.Context, texts []string) ([][]float64, error)
}

// NewEmbedder resolves the configured embedder runtime.
func NewEmbedder(provider config.RuntimeProvider) (Embedder, error) {
	switch provider.Provider {
	case "", config.RuntimeProviderFixture:
		dimension, err := fixtureDimension(provider.Model)
		if err != nil {
			return nil, err
		}
		return fixtureEmbedder{dimension: dimension, model: provider.Model}, nil
	case config.RuntimeProviderOpenAI:
		return newOpenAICompatibleEmbedder(provider)
	default:
		return nil, fmt.Errorf(
			"runtime.embedder.provider %q is not supported; supported providers are %q and %q",
			provider.Provider,
			config.RuntimeProviderFixture,
			config.RuntimeProviderOpenAI,
		)
	}
}

func newEmbedder(provider config.RuntimeProvider) (Embedder, error) {
	return NewEmbedder(provider)
}

type fixtureEmbedder struct {
	dimension int
	model     string
}

func (e fixtureEmbedder) Fingerprint() string {
	return embedderFingerprint(config.RuntimeProviderFixture, e.model, "plain_v1")
}

func (e fixtureEmbedder) Dimension(ctx context.Context) (int, error) {
	return e.dimension, nil
}

func (e fixtureEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float64, error) {
	return e.embedTexts(ctx, texts)
}

func (e fixtureEmbedder) EmbedQueries(ctx context.Context, texts []string) ([][]float64, error) {
	return e.embedTexts(ctx, texts)
}

func (e fixtureEmbedder) embedTexts(ctx context.Context, texts []string) ([][]float64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	vectors := make([][]float64, 0, len(texts))
	for _, text := range texts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		vectors = append(vectors, fixtureVector(text, e.dimension))
	}
	return vectors, nil
}

func embedderFingerprint(provider, model, strategy string) string {
	return fmt.Sprintf("%s|%s|%s", strings.TrimSpace(provider), strings.TrimSpace(model), strings.TrimSpace(strategy))
}

func fixtureVector(text string, dimension int) []float64 {
	vector := make([]float64, dimension)
	if dimension <= 0 {
		return vector
	}

	tokens := tokenize(text)
	if len(tokens) == 0 {
		return vector
	}

	for i, token := range tokens {
		vector[tokenBucket(token, dimension)] += 1.0
		if i > 0 {
			bigram := tokens[i-1] + "_" + token
			vector[tokenBucket(bigram, dimension)] += 1.5
		}
	}

	return normalize(vector)
}

func tokenize(text string) []string {
	var builder strings.Builder
	builder.Grow(len(text))
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte(' ')
	}
	return strings.Fields(builder.String())
}

func tokenBucket(token string, dimension int) int {
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(token))
	return int(hasher.Sum32() % uint32(dimension))
}

func normalize(vector []float64) []float64 {
	var norm float64
	for _, value := range vector {
		norm += value * value
	}
	if norm == 0 {
		return vector
	}
	norm = math.Sqrt(norm)
	for i := range vector {
		vector[i] /= norm
	}
	return vector
}
