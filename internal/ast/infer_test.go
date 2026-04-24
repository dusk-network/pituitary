package ast

import "testing"

func TestInferEdges(t *testing.T) {
	t.Parallel()

	fileSymbols := map[string][]Symbol{
		"src/api/middleware/ratelimiter.go": {
			{Name: "SlidingWindowLimiter", Kind: SymbolType},
			{Name: "NewSlidingWindowLimiter", Kind: SymbolFunction},
			{Name: "Allow", Kind: SymbolMethod},
		},
		"src/api/config/loader.go": {
			{Name: "LoadConfig", Kind: SymbolFunction},
			{Name: "ConfigStore", Kind: SymbolType},
		},
	}

	specs := []SpecSummary{
		{
			Ref:             "SPEC-042",
			BodyText:        "This spec governs the SlidingWindowLimiter. Use NewSlidingWindowLimiter to create instances.",
			ManualAppliesTo: []string{"code://src/api/middleware/ratelimiter.go"},
		},
		{
			Ref:      "SPEC-099",
			BodyText: "The ConfigStore handles all runtime configuration loading.",
		},
	}

	edges := InferEdges(fileSymbols, specs)

	// SPEC-042 already has a manual edge to ratelimiter.go — no inferred edge for that file.
	// SPEC-099 should get an inferred edge to loader.go via ConfigStore.
	var found bool
	for _, e := range edges {
		if e.SpecRef == "SPEC-099" && e.FilePath == "src/api/config/loader.go" {
			found = true
		}
		if e.SpecRef == "SPEC-042" && e.FilePath == "src/api/middleware/ratelimiter.go" {
			t.Errorf("should not produce inferred edge that duplicates manual edge")
		}
	}
	if !found {
		t.Errorf("expected inferred edge SPEC-099 -> src/api/config/loader.go, got %v", edges)
	}
}

func TestInferEdgesSkipsShortSymbols(t *testing.T) {
	t.Parallel()

	fileSymbols := map[string][]Symbol{
		"main.go": {
			{Name: "Run", Kind: SymbolFunction},
		},
	}
	specs := []SpecSummary{
		{Ref: "SPEC-001", BodyText: "Run the main loop."},
	}

	edges := InferEdges(fileSymbols, specs)
	if len(edges) != 0 {
		t.Errorf("expected no edges for short symbol name, got %v", edges)
	}
}

func TestInferEdgesMultipleSpecsSameFile(t *testing.T) {
	t.Parallel()

	fileSymbols := map[string][]Symbol{
		"auth/handler.go": {
			{Name: "AuthHandler", Kind: SymbolType},
			{Name: "TokenValidator", Kind: SymbolType},
		},
	}
	specs := []SpecSummary{
		{Ref: "SPEC-A", BodyText: "The AuthHandler validates tokens."},
		{Ref: "SPEC-B", BodyText: "The TokenValidator checks expiry."},
	}

	edges := InferEdges(fileSymbols, specs)
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d: %v", len(edges), edges)
	}
}

func TestContentHash(t *testing.T) {
	t.Parallel()
	h1 := ContentHash([]byte("hello"), "main.go")
	h2 := ContentHash([]byte("hello"), "other.go")
	h3 := ContentHash([]byte("hello"), "main.go")

	if h1 == h2 {
		t.Error("different paths should produce different hashes")
	}
	if h1 != h3 {
		t.Error("same content+path should produce same hash")
	}
	if len(h1) != 64 {
		t.Errorf("expected SHA256 hex length 64, got %d", len(h1))
	}
}
