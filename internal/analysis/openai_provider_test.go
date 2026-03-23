package analysis

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"unicode/utf8"

	"github.com/dusk-network/pituitary/internal/config"
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
