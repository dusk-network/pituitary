package index

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
)

func TestOpenAICompatibleEmbedderUsesNomicSearchPrefixes(t *testing.T) {
	t.Parallel()

	var inputs [][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("request path = %q, want %q", r.URL.Path, "/v1/embeddings")
		}
		var request struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		inputs = append(inputs, append([]string(nil), request.Input...))

		response := map[string]any{
			"data": []map[string]any{},
		}
		for i := range request.Input {
			response["data"] = append(response["data"].([]map[string]any), map[string]any{
				"index":     i,
				"embedding": []float64{float64(i + 1), float64(i + 2), float64(i + 3)},
			})
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	embedder, err := NewEmbedder(config.RuntimeProvider{
		Provider:  config.RuntimeProviderOpenAI,
		Model:     "nomic-embed-text-v1.5",
		Endpoint:  server.URL + "/v1",
		TimeoutMS: 1000,
	})
	if err != nil {
		t.Fatalf("NewEmbedder() error = %v", err)
	}

	documentVectors, err := embedder.EmbedDocuments(context.Background(), []string{"alpha"})
	if err != nil {
		t.Fatalf("EmbedDocuments() error = %v", err)
	}
	queryVectors, err := embedder.EmbedQueries(context.Background(), []string{"beta"})
	if err != nil {
		t.Fatalf("EmbedQueries() error = %v", err)
	}
	dimension, err := embedder.Dimension(context.Background())
	if err != nil {
		t.Fatalf("Dimension() error = %v", err)
	}

	if len(documentVectors) != 1 || len(documentVectors[0]) != 3 {
		t.Fatalf("document vectors = %+v, want one 3d vector", documentVectors)
	}
	if len(queryVectors) != 1 || len(queryVectors[0]) != 3 {
		t.Fatalf("query vectors = %+v, want one 3d vector", queryVectors)
	}
	if dimension != 3 {
		t.Fatalf("Dimension() = %d, want 3", dimension)
	}
	if !slices.Equal(inputs[0], []string{"search_document: alpha"}) {
		t.Fatalf("document input = %v, want search_document prefix", inputs[0])
	}
	if !slices.Equal(inputs[1], []string{"search_query: beta"}) {
		t.Fatalf("query input = %v, want search_query prefix", inputs[1])
	}
	if got, want := embedder.Fingerprint(), "openai_compatible|nomic-embed-text-v1.5|nomic_search_prefix_v1"; got != want {
		t.Fatalf("Fingerprint() = %q, want %q", got, want)
	}
}

func TestOpenAICompatibleEmbedderRequiresConfiguredAPIKey(t *testing.T) {
	t.Parallel()

	const envVar = "PITUITARY_TEST_OPENAI_API_KEY"
	if err := os.Unsetenv(envVar); err != nil {
		t.Fatalf("Unsetenv(%s): %v", envVar, err)
	}

	_, err := NewEmbedder(config.RuntimeProvider{
		Provider:  config.RuntimeProviderOpenAI,
		Model:     "pituitary-embed",
		Endpoint:  "http://127.0.0.1:1234/v1",
		APIKeyEnv: envVar,
	})
	if err == nil {
		t.Fatal("NewEmbedder() error = nil, want missing-API-key failure")
	}
	if !IsDependencyUnavailable(err) {
		t.Fatalf("NewEmbedder() error = %v, want dependency-unavailable classification", err)
	}
	if !strings.Contains(err.Error(), "missing API key for runtime.embedder") {
		t.Fatalf("NewEmbedder() error = %q, want missing-API-key detail", err)
	}
}
