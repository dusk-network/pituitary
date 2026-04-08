package ast

import (
	"testing"
)

func TestExtractRationale_TaggedComments(t *testing.T) {
	t.Parallel()
	src := []byte(`package main

// WHY: We use a fixed window here because sliding window causes race conditions under high concurrency.
func RateLimit() {}

// RATIONALE: Chose etcd over consul for consistency guarantees.
func ConfigStore() {}

# HACK: Temporary workaround until upstream fixes the parser bug.
# TODO: Remove this after v2.0 release.
`)
	symbols := []Symbol{
		{Name: "RateLimit", Kind: SymbolFunction},
		{Name: "ConfigStore", Kind: SymbolFunction},
	}

	rationales := ExtractRationale(src, symbols, LangGo)
	if len(rationales) < 3 {
		t.Fatalf("expected at least 3 rationales, got %d: %+v", len(rationales), rationales)
	}

	// Check WHY tag.
	found := false
	for _, r := range rationales {
		if r.Kind == RationaleWhy && r.Line == 3 {
			found = true
			if r.NearestSymbol != "RateLimit" {
				t.Errorf("WHY nearest_symbol = %q, want RateLimit", r.NearestSymbol)
			}
			if r.Text == "" {
				t.Error("WHY text is empty")
			}
		}
	}
	if !found {
		t.Error("WHY rationale not found at line 3")
	}

	// Check RATIONALE tag.
	found = false
	for _, r := range rationales {
		if r.Kind == RationaleRationale {
			found = true
			if r.NearestSymbol != "ConfigStore" {
				t.Errorf("RATIONALE nearest_symbol = %q, want ConfigStore", r.NearestSymbol)
			}
		}
	}
	if !found {
		t.Error("RATIONALE tag not found")
	}
}

func TestExtractRationale_DecisionLanguage(t *testing.T) {
	t.Parallel()
	src := []byte(`package main

// We chose fixed-window instead of sliding-window due to the race condition.
func RateLimit() {}
`)
	symbols := []Symbol{{Name: "RateLimit", Kind: SymbolFunction}}

	rationales := ExtractRationale(src, symbols, LangGo)

	found := false
	for _, r := range rationales {
		if r.Kind == RationaleDecision {
			found = true
			if r.NearestSymbol != "RateLimit" {
				t.Errorf("decision nearest_symbol = %q, want RateLimit", r.NearestSymbol)
			}
		}
	}
	if !found {
		t.Error("decision-language rationale not found")
	}
}

func TestExtractRationale_PythonComments(t *testing.T) {
	t.Parallel()
	src := []byte(`# WHY: Using subprocess for improved security
def run_command():
    pass

# HACK: Monkey-patch to fix upstream bug
def patch_library():
    pass
`)
	symbols := []Symbol{
		{Name: "run_command", Kind: SymbolFunction},
		{Name: "patch_library", Kind: SymbolFunction},
	}

	rationales := ExtractRationale(src, symbols, LangPython)
	if len(rationales) < 2 {
		t.Fatalf("expected at least 2 rationales, got %d", len(rationales))
	}

	kinds := map[RationaleKind]bool{}
	for _, r := range rationales {
		kinds[r.Kind] = true
	}
	if !kinds[RationaleWhy] {
		t.Error("WHY rationale not found")
	}
	if !kinds[RationaleHack] {
		t.Error("HACK rationale not found")
	}
}

func TestExtractRationale_NoRationale(t *testing.T) {
	t.Parallel()
	src := []byte(`package main

func Hello() {
    fmt.Println("hello")
}
`)
	rationales := ExtractRationale(src, nil, LangGo)
	if len(rationales) != 0 {
		t.Fatalf("expected 0 rationales, got %d", len(rationales))
	}
}

func TestExtractRationale_NearestSymbolLinking(t *testing.T) {
	t.Parallel()
	src := []byte(`package main

func Alpha() {}

// WHY: This is about Beta, not Alpha.
func Beta() {}

func Gamma() {}
`)
	symbols := []Symbol{
		{Name: "Alpha", Kind: SymbolFunction},
		{Name: "Beta", Kind: SymbolFunction},
		{Name: "Gamma", Kind: SymbolFunction},
	}

	rationales := ExtractRationale(src, symbols, LangGo)
	if len(rationales) != 1 {
		t.Fatalf("expected 1 rationale, got %d", len(rationales))
	}
	if rationales[0].NearestSymbol != "Beta" {
		t.Errorf("nearest_symbol = %q, want Beta", rationales[0].NearestSymbol)
	}
}
