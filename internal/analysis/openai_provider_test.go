package analysis

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
