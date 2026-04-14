package index

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	stembed "github.com/dusk-network/stroma/embed"
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
	contextualEmbedder, ok := embedder.(stembed.ContextualEmbedder)
	if !ok {
		t.Fatalf("embedder type %T does not implement ContextualEmbedder", embedder)
	}
	chunkVectors, err := contextualEmbedder.EmbedDocumentChunks(context.Background(), "full document", []string{"gamma"})
	if err != nil {
		t.Fatalf("EmbedDocumentChunks() error = %v", err)
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
	if len(chunkVectors) != 1 || len(chunkVectors[0]) != 3 {
		t.Fatalf("chunk vectors = %+v, want one 3d vector", chunkVectors)
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
	if !slices.Equal(inputs[2], []string{"search_document: gamma"}) {
		t.Fatalf("contextual chunk input = %v, want search_document prefix", inputs[2])
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
	details := DependencyUnavailableDetails(err)
	if got, want := details["runtime"], "runtime.embedder"; got != want {
		t.Fatalf("details.runtime = %#v, want %q", got, want)
	}
	if got, want := details["provider"], config.RuntimeProviderOpenAI; got != want {
		t.Fatalf("details.provider = %#v, want %q", got, want)
	}
	if got, want := details["failure_class"], "auth"; got != want {
		t.Fatalf("details.failure_class = %#v, want %q", got, want)
	}
}

func TestOpenAICompatibleEmbedderParsesStringErrorBodies(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"error": "Model unloaded..",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	embedder, err := NewEmbedder(config.RuntimeProvider{
		Provider:  config.RuntimeProviderOpenAI,
		Model:     "pituitary-embed",
		Endpoint:  server.URL + "/v1",
		TimeoutMS: 1000,
	})
	if err != nil {
		t.Fatalf("NewEmbedder() error = %v", err)
	}

	_, err = embedder.EmbedQueries(context.Background(), []string{"ping"})
	if err == nil {
		t.Fatal("EmbedQueries() error = nil, want dependency-unavailable failure")
	}
	if !IsDependencyUnavailable(err) {
		t.Fatalf("EmbedQueries() error = %v, want dependency-unavailable classification", err)
	}
	if !strings.Contains(err.Error(), "Model unloaded..") {
		t.Fatalf("EmbedQueries() error = %q, want parsed model-unloaded detail", err)
	}
	if strings.Contains(err.Error(), `{"error":"Model unloaded.."}`) {
		t.Fatalf("EmbedQueries() error = %q, want parsed message instead of raw JSON", err)
	}
	details := DependencyUnavailableDetails(err)
	if got, want := details["request_type"], "embeddings"; got != want {
		t.Fatalf("details.request_type = %#v, want %q", got, want)
	}
	if got, want := details["batch_size"], 1; got != want {
		t.Fatalf("details.batch_size = %#v, want %d", got, want)
	}
	if got, want := details["input_count"], 1; got != want {
		t.Fatalf("details.input_count = %#v, want %d", got, want)
	}
	if got, want := details["http_status"], http.StatusBadRequest; got != want {
		t.Fatalf("details.http_status = %#v, want %d", got, want)
	}
	if got, want := details["failure_class"], "dependency_unavailable"; got != want {
		t.Fatalf("details.failure_class = %#v, want %q", got, want)
	}
}

func TestOpenAICompatibleEmbedderBatchesLargeRequests(t *testing.T) {
	t.Parallel()

	var calls [][]string
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
		calls = append(calls, append([]string(nil), request.Input...))

		response := map[string]any{
			"data": make([]map[string]any, 0, len(request.Input)),
		}
		for i, raw := range request.Input {
			idx, err := strconv.Atoi(strings.TrimPrefix(raw, "text-"))
			if err != nil {
				t.Fatalf("parse input index from %q: %v", raw, err)
			}
			response["data"] = append(response["data"].([]map[string]any), map[string]any{
				"index":     i,
				"embedding": []float64{float64(idx)},
			})
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	embedder, err := NewEmbedder(config.RuntimeProvider{
		Provider:  config.RuntimeProviderOpenAI,
		Model:     "pituitary-embed",
		Endpoint:  server.URL + "/v1",
		TimeoutMS: 1000,
	})
	if err != nil {
		t.Fatalf("NewEmbedder() error = %v", err)
	}

	texts := make([]string, openAICompatibleEmbeddingBatchSize+3)
	for i := range texts {
		texts[i] = fmt.Sprintf("text-%d", i)
	}

	vectors, err := embedder.EmbedDocuments(context.Background(), texts)
	if err != nil {
		t.Fatalf("EmbedDocuments() error = %v", err)
	}

	if len(calls) != 2 {
		t.Fatalf("embedding request count = %d, want 2", len(calls))
	}
	if len(calls[0]) != openAICompatibleEmbeddingBatchSize {
		t.Fatalf("first batch size = %d, want %d", len(calls[0]), openAICompatibleEmbeddingBatchSize)
	}
	if len(calls[1]) != 3 {
		t.Fatalf("second batch size = %d, want 3", len(calls[1]))
	}
	if len(vectors) != len(texts) {
		t.Fatalf("vector count = %d, want %d", len(vectors), len(texts))
	}
	for i, vector := range vectors {
		if len(vector) != 1 || vector[0] != float64(i) {
			t.Fatalf("vector[%d] = %v, want [%d]", i, vector, i)
		}
	}
}

func TestOpenAICompatibleEmbedderFallsBackToSmallerBatchesOnDependencyFailure(t *testing.T) {
	t.Parallel()

	var calls []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		calls = append(calls, len(request.Input))
		if len(request.Input) > 1 {
			http.Error(w, `{"error":"llama_decode returned -1"}`, http.StatusInternalServerError)
			return
		}

		idx, err := strconv.Atoi(strings.TrimPrefix(request.Input[0], "text-"))
		if err != nil {
			t.Fatalf("parse input index from %q: %v", request.Input[0], err)
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"index":     0,
				"embedding": []float64{float64(idx)},
			}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	embedder, err := NewEmbedder(config.RuntimeProvider{
		Provider:  config.RuntimeProviderOpenAI,
		Model:     "pituitary-embed",
		Endpoint:  server.URL + "/v1",
		TimeoutMS: 1000,
	})
	if err != nil {
		t.Fatalf("NewEmbedder() error = %v", err)
	}

	vectors, err := embedder.EmbedDocuments(context.Background(), []string{"text-0", "text-1", "text-2"})
	if err != nil {
		t.Fatalf("EmbedDocuments() error = %v", err)
	}

	if len(vectors) != 3 {
		t.Fatalf("vector count = %d, want 3", len(vectors))
	}
	for i, vector := range vectors {
		if len(vector) != 1 || vector[0] != float64(i) {
			t.Fatalf("vector[%d] = %v, want [%d]", i, vector, i)
		}
	}
	if len(calls) < 4 {
		t.Fatalf("call sizes = %v, want recursive fallback requests", calls)
	}
	if calls[0] != 3 {
		t.Fatalf("first call batch size = %d, want 3", calls[0])
	}
}

func TestOpenAICompatibleEmbedderDoesNotSplitClientErrors(t *testing.T) {
	t.Parallel()

	var calls []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		calls = append(calls, len(request.Input))
		http.Error(w, `{"error":"unknown model"}`, http.StatusBadRequest)
	}))
	defer server.Close()

	embedder, err := NewEmbedder(config.RuntimeProvider{
		Provider:  config.RuntimeProviderOpenAI,
		Model:     "pituitary-embed",
		Endpoint:  server.URL + "/v1",
		TimeoutMS: 1000,
	})
	if err != nil {
		t.Fatalf("NewEmbedder() error = %v", err)
	}

	_, err = embedder.EmbedDocuments(context.Background(), []string{"text-0", "text-1", "text-2"})
	if err == nil {
		t.Fatal("EmbedDocuments() error = nil, want dependency-unavailable failure")
	}
	if !IsDependencyUnavailable(err) {
		t.Fatalf("EmbedDocuments() error = %v, want dependency-unavailable classification", err)
	}
	if len(calls) != 1 || calls[0] != 3 {
		t.Fatalf("call sizes = %v, want one unsplit client-error request", calls)
	}
}
