package analysis

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
)

func configureOpenAIAnalysisProvider(t *testing.T, cfg *config.Config, handler func(t *testing.T, request openAICompatibleChatRequest) string) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
		}

		var request openAICompatibleChatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		response := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": handler(t, request),
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	cfg.Runtime.Analysis = config.RuntimeProvider{
		Provider:   config.RuntimeProviderOpenAI,
		Model:      "pituitary-analysis",
		Endpoint:   server.URL,
		TimeoutMS:  1000,
		MaxRetries: 0,
	}
}

func TestNormalizeJSONResponseTextRejectsNonObjectJSON(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"null", "[]", `"hello"`} {
		if got := normalizeJSONResponseText(input); got != "" {
			t.Fatalf("normalizeJSONResponseText(%q) = %q, want empty string", input, got)
		}
	}
	if got := normalizeJSONResponseText(`{"ok":true}`); got != `{"ok":true}` {
		t.Fatalf("normalizeJSONResponseText(object) = %q, want original object", got)
	}
}

func TestTruncateForAnalysisPromptPreservesUTF8(t *testing.T) {
	t.Parallel()

	got := truncateForAnalysisPrompt("éclair", 5)
	if got != "é..." {
		t.Fatalf("truncateForAnalysisPrompt() = %q, want %q", got, "é...")
	}
	if !utf8.ValidString(got) {
		t.Fatalf("truncateForAnalysisPrompt() returned invalid UTF-8: %q", got)
	}
}

func TestCompleteJSONSendsExplicitTemperatureZero(t *testing.T) {
	t.Parallel()

	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"ok":true}`}},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	rawProvider, err := newOpenAICompatibleAnalysisProvider(config.RuntimeProvider{
		Provider:   config.RuntimeProviderOpenAI,
		Model:      "pituitary-analysis",
		Endpoint:   server.URL,
		TimeoutMS:  1000,
		MaxRetries: 0,
	})
	if err != nil {
		t.Fatalf("newOpenAICompatibleAnalysisProvider() error = %v", err)
	}
	provider := rawProvider.(*openAICompatibleAnalysisProvider)

	var response map[string]any
	if err := provider.completeJSON(context.Background(), "system", map[string]string{"ping": "pong"}, &response); err != nil {
		t.Fatalf("completeJSON() error = %v", err)
	}
	if got, ok := captured["temperature"]; !ok || got.(float64) != 0 {
		t.Fatalf("temperature field = %#v, want explicit 0", captured["temperature"])
	}
}

func TestProbeProviderContextUsesLightweightJSONProbe(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openAICompatibleChatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(request.Messages) != 2 {
			t.Fatalf("messages = %+v, want system and user prompt", request.Messages)
		}
		if !strings.Contains(request.Messages[0].Content, "runtime probe") {
			t.Fatalf("system prompt = %q, want runtime probe guidance", request.Messages[0].Content)
		}
		if !strings.Contains(request.Messages[1].Content, `"command":"runtime-probe"`) {
			t.Fatalf("user prompt = %q, want runtime-probe payload", request.Messages[1].Content)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"ok":true}`}},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	err := ProbeProviderContext(context.Background(), config.RuntimeProvider{
		Provider:   config.RuntimeProviderOpenAI,
		Model:      "pituitary-analysis",
		Endpoint:   server.URL,
		TimeoutMS:  1000,
		MaxRetries: 0,
	})
	if err != nil {
		t.Fatalf("ProbeProviderContext() error = %v, want nil", err)
	}
}

func TestOpenAICompatibleAnalysisProviderParsesStringErrorBodies(t *testing.T) {
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
	t.Cleanup(server.Close)

	rawProvider, err := newOpenAICompatibleAnalysisProvider(config.RuntimeProvider{
		Provider:   config.RuntimeProviderOpenAI,
		Model:      "pituitary-analysis",
		Endpoint:   server.URL,
		TimeoutMS:  1000,
		MaxRetries: 0,
	})
	if err != nil {
		t.Fatalf("newOpenAICompatibleAnalysisProvider() error = %v", err)
	}
	provider := rawProvider.(*openAICompatibleAnalysisProvider)

	var response map[string]any
	err = provider.completeJSON(context.Background(), "system", map[string]string{"ping": "pong"}, &response)
	if err == nil {
		t.Fatal("completeJSON() error = nil, want dependency-unavailable failure")
	}
	if !index.IsDependencyUnavailable(err) {
		t.Fatalf("completeJSON() error = %v, want dependency-unavailable classification", err)
	}
	if !strings.Contains(err.Error(), "Model unloaded..") {
		t.Fatalf("completeJSON() error = %q, want parsed model-unloaded detail", err)
	}
	if strings.Contains(err.Error(), `{"error":"Model unloaded.."}`) {
		t.Fatalf("completeJSON() error = %q, want parsed message instead of raw JSON", err)
	}
}
