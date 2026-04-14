package index

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	stembed "github.com/dusk-network/stroma/embed"
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

func (e *DependencyUnavailableError) RuntimeName() string {
	return strings.TrimSpace(e.Runtime)
}

func (e *DependencyUnavailableError) DiagnosticFields() map[string]any {
	values := map[string]any{
		"failure_class": "dependency_unavailable",
	}
	if runtime := strings.TrimSpace(e.Runtime); runtime != "" {
		values["runtime"] = runtime
	}
	return values
}

// IsDependencyUnavailable reports whether err wraps a dependency-unavailable failure.
func IsDependencyUnavailable(err error) bool {
	var target interface {
		error
		RuntimeName() string
	}
	return errors.As(err, &target)
}

// DependencyUnavailableRuntime reports the runtime surface associated with a
// dependency-unavailable failure, if one was recorded.
func DependencyUnavailableRuntime(err error) string {
	var target interface {
		error
		RuntimeName() string
	}
	if !errors.As(err, &target) {
		return ""
	}
	return strings.TrimSpace(target.RuntimeName())
}

// DependencyUnavailableDetails reports structured diagnostic fields associated
// with a dependency-unavailable failure, if the wrapped error exposes any.
func DependencyUnavailableDetails(err error) map[string]any {
	var target interface {
		error
		DiagnosticFields() map[string]any
	}
	if !errors.As(err, &target) {
		return nil
	}
	values := target.DiagnosticFields()
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

// Embedder generates embeddings for rebuild and query-time retrieval.
type Embedder = stembed.Embedder

// NewEmbedder resolves the configured embedder runtime.
func NewEmbedder(provider config.RuntimeProvider) (Embedder, error) {
	switch provider.Provider {
	case "", config.RuntimeProviderFixture:
		dimension, err := fixtureDimension(provider.Model)
		if err != nil {
			return nil, err
		}
		base, err := stembed.NewFixture(provider.Model, dimension)
		if err != nil {
			return nil, err
		}
		return &fixtureEmbedder{base: base, model: provider.Model}, nil
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
	base  *stembed.Fixture
	model string
}

var _ stembed.ContextualEmbedder = (*fixtureEmbedder)(nil)

func (e *fixtureEmbedder) Fingerprint() string {
	return embedderFingerprint(config.RuntimeProviderFixture, e.model, "plain_v1")
}

func (e *fixtureEmbedder) Dimension(ctx context.Context) (int, error) {
	return e.base.Dimension(ctx)
}

func (e *fixtureEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float64, error) {
	return e.base.EmbedDocuments(ctx, texts)
}

func (e *fixtureEmbedder) EmbedQueries(ctx context.Context, texts []string) ([][]float64, error) {
	return e.base.EmbedQueries(ctx, texts)
}

func (e *fixtureEmbedder) EmbedDocumentChunks(ctx context.Context, _ string, chunks []string) ([][]float64, error) {
	return e.base.EmbedDocuments(ctx, chunks)
}

func embedderFingerprint(provider, model, strategy string) string {
	return fmt.Sprintf("%s|%s|%s", strings.TrimSpace(provider), strings.TrimSpace(model), strings.TrimSpace(strategy))
}
