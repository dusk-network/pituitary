package runtimeprobe

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
)

func TestRunAllReportsReadyEmbedderAndDisabledAnalysis(t *testing.T) {
	t.Parallel()

	result, err := Run(context.Background(), &config.Config{
		Runtime: config.Runtime{
			Embedder: config.RuntimeProvider{
				Provider: config.RuntimeProviderFixture,
				Model:    "fixture-8d",
			},
			Analysis: config.RuntimeProvider{
				Provider: config.RuntimeProviderDisabled,
			},
		},
	}, ScopeAll)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result == nil {
		t.Fatal("Run() result = nil, want runtime probe result")
	}
	if got, want := result.Scope, "all"; got != want {
		t.Fatalf("result.scope = %q, want %q", got, want)
	}
	if len(result.Checks) != 2 {
		t.Fatalf("len(result.checks) = %d, want 2", len(result.Checks))
	}
	if got, want := result.Checks[0].Name, "runtime.embedder"; got != want {
		t.Fatalf("checks[0].name = %q, want %q", got, want)
	}
	if got, want := result.Checks[0].Status, StatusReady; got != want {
		t.Fatalf("checks[0].status = %q, want %q", got, want)
	}
	if got, want := result.Checks[1].Name, "runtime.analysis"; got != want {
		t.Fatalf("checks[1].name = %q, want %q", got, want)
	}
	if got, want := result.Checks[1].Status, StatusDisabled; got != want {
		t.Fatalf("checks[1].status = %q, want %q", got, want)
	}
}

func TestRunAnalysisUsesLightweightProbe(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(request.Messages) != 2 {
			t.Fatalf("messages = %+v, want system and user prompts", request.Messages)
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

	result, err := Run(context.Background(), &config.Config{
		Runtime: config.Runtime{
			Analysis: config.RuntimeProvider{
				Profile:    "local-lm-studio",
				Provider:   config.RuntimeProviderOpenAI,
				Model:      "pituitary-analysis",
				Endpoint:   server.URL,
				TimeoutMS:  1000,
				MaxRetries: 0,
			},
		},
	}, ScopeAnalysis)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result == nil || len(result.Checks) != 1 {
		t.Fatalf("result = %+v, want one runtime check", result)
	}
	if got, want := result.Checks[0].Status, StatusReady; got != want {
		t.Fatalf("checks[0].status = %q, want %q", got, want)
	}
	if got, want := result.Checks[0].Profile, "local-lm-studio"; got != want {
		t.Fatalf("checks[0].profile = %q, want %q", got, want)
	}
	if got, want := result.Checks[0].Timeout, 1000; got != want {
		t.Fatalf("checks[0].timeout_ms = %d, want %d", got, want)
	}
}

func TestRunReturnsDependencyUnavailableForEmbedderFailures(t *testing.T) {
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

	_, err := Run(context.Background(), &config.Config{
		Runtime: config.Runtime{
			Embedder: config.RuntimeProvider{
				Provider:   config.RuntimeProviderOpenAI,
				Model:      "pituitary-embed",
				Endpoint:   server.URL,
				TimeoutMS:  1000,
				MaxRetries: 0,
			},
		},
	}, ScopeEmbedder)
	if err == nil {
		t.Fatal("Run() error = nil, want dependency-unavailable failure")
	}
	if !index.IsDependencyUnavailable(err) {
		t.Fatalf("Run() error = %v, want dependency-unavailable classification", err)
	}
	if got, want := index.DependencyUnavailableRuntime(err), "runtime.embedder"; got != want {
		t.Fatalf("DependencyUnavailableRuntime() = %q, want %q", got, want)
	}
}
