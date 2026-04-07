package ast

import "testing"

func TestExtractSymbolsGo(t *testing.T) {
	t.Parallel()
	src := []byte(`package middleware

import "net/http"

type SlidingWindowLimiter struct {
	windowSize int
}

func NewSlidingWindowLimiter(size int) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{windowSize: size}
}

func (l *SlidingWindowLimiter) Allow(r *http.Request) bool {
	return true
}
`)
	symbols, err := ExtractSymbols(src, LangGo)
	if err != nil {
		t.Fatalf("ExtractSymbols error: %v", err)
	}

	want := map[string]SymbolKind{
		"SlidingWindowLimiter":    SymbolType,
		"NewSlidingWindowLimiter": SymbolFunction,
		"Allow":                   SymbolMethod,
	}
	got := make(map[string]SymbolKind)
	for _, s := range symbols {
		got[s.Name] = s.Kind
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Errorf("missing or wrong symbol %q: got %v, want %v", name, got[name], kind)
		}
	}

	var foundImport bool
	for _, s := range symbols {
		if s.Kind == SymbolImport && s.Name == "net/http" {
			foundImport = true
		}
	}
	if !foundImport {
		t.Errorf("expected import \"net/http\" in symbols, got %v", symbols)
	}
}

func TestExtractSymbolsPython(t *testing.T) {
	t.Parallel()
	src := []byte(`import os
from pathlib import Path

class RateLimiter:
    def check(self, key):
        return True

def create_limiter(limit):
    return RateLimiter(limit)
`)
	symbols, err := ExtractSymbols(src, LangPython)
	if err != nil {
		t.Fatalf("ExtractSymbols error: %v", err)
	}

	want := map[string]SymbolKind{
		"RateLimiter":    SymbolType,
		"check":          SymbolMethod,
		"create_limiter": SymbolFunction,
	}
	got := make(map[string]SymbolKind)
	for _, s := range symbols {
		got[s.Name] = s.Kind
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Errorf("missing or wrong symbol %q: got %v, want %v", name, got[name], kind)
		}
	}
}

func TestExtractSymbolsJavaScript(t *testing.T) {
	t.Parallel()
	src := []byte(`import { Router } from 'express';

class ApiGateway {
  route(path) {
    return path;
  }
}

function createGateway(config) {
  return new ApiGateway(config);
}
`)
	symbols, err := ExtractSymbols(src, LangJavaScript)
	if err != nil {
		t.Fatalf("ExtractSymbols error: %v", err)
	}

	want := map[string]SymbolKind{
		"ApiGateway":    SymbolType,
		"route":         SymbolMethod,
		"createGateway": SymbolFunction,
	}
	got := make(map[string]SymbolKind)
	for _, s := range symbols {
		got[s.Name] = s.Kind
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Errorf("missing or wrong symbol %q: got %v, want %v", name, got[name], kind)
		}
	}
}

func TestExtractSymbolsTypeScript(t *testing.T) {
	t.Parallel()
	src := []byte(`import { Request } from 'express';

interface Handler {
  handle(): void;
}

class Router {
  route(path: string): void {}
}

function createRouter(): Router {
  return new Router();
}
`)
	symbols, err := ExtractSymbols(src, LangTypeScript)
	if err != nil {
		t.Fatalf("ExtractSymbols error: %v", err)
	}

	want := map[string]SymbolKind{
		"Handler":      SymbolType,
		"Router":       SymbolType,
		"route":        SymbolMethod,
		"createRouter": SymbolFunction,
	}
	got := make(map[string]SymbolKind)
	for _, s := range symbols {
		got[s.Name] = s.Kind
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Errorf("missing or wrong symbol %q: got %v, want %v", name, got[name], kind)
		}
	}
}

func TestExtractSymbolsRust(t *testing.T) {
	t.Parallel()
	src := []byte(`use std::collections::HashMap;

pub struct TokenBucket {
    capacity: u64,
}

impl TokenBucket {
    pub fn new(capacity: u64) -> Self {
        TokenBucket { capacity }
    }

    pub fn consume(&mut self, tokens: u64) -> bool {
        true
    }
}

pub fn default_bucket() -> TokenBucket {
    TokenBucket { capacity: 100 }
}
`)
	symbols, err := ExtractSymbols(src, LangRust)
	if err != nil {
		t.Fatalf("ExtractSymbols error: %v", err)
	}

	want := map[string]SymbolKind{
		"TokenBucket":    SymbolType,
		"new":            SymbolMethod,
		"consume":        SymbolMethod,
		"default_bucket": SymbolFunction,
	}
	got := make(map[string]SymbolKind)
	for _, s := range symbols {
		got[s.Name] = s.Kind
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Errorf("missing or wrong symbol %q: got %v, want %v", name, got[name], kind)
		}
	}
}

func TestExtractSymbolsUnsupportedLanguage(t *testing.T) {
	t.Parallel()
	_, err := ExtractSymbols([]byte("hello"), "unknown")
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}
