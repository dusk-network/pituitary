package ast

import "testing"

func TestDetectLanguage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want LangID
	}{
		{"main.go", LangGo},
		{"server.py", LangPython},
		{"app.js", LangJavaScript},
		{"app.mjs", LangJavaScript},
		{"index.ts", LangTypeScript},
		{"index.tsx", LangTSX},
		{"lib.rs", LangRust},
		{"README.md", ""},
		{"data.json", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := DetectLanguage(tt.path)
			if got != tt.want {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestGrammarForReturnsNilForUnknown(t *testing.T) {
	t.Parallel()
	if g := GrammarFor(""); g != nil {
		t.Fatalf("expected nil grammar for empty language, got %v", g)
	}
}

func TestGrammarForReturnsNonNilForSupported(t *testing.T) {
	t.Parallel()
	for _, lang := range SupportedLanguages() {
		if g := GrammarFor(lang); g == nil {
			t.Errorf("GrammarFor(%q) returned nil", lang)
		}
	}
}
