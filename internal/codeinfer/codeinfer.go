package codeinfer

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const DefaultInfererName = "ast"

// SymbolKind classifies an extracted code symbol.
type SymbolKind string

const (
	SymbolFunction SymbolKind = "function"
	SymbolMethod   SymbolKind = "method"
	SymbolType     SymbolKind = "type"
	SymbolImport   SymbolKind = "import"
)

// Symbol is a named code element extracted from a source file.
type Symbol struct {
	Name string     `json:"name"`
	Kind SymbolKind `json:"kind"`
}

// RationaleKind classifies the type of rationale comment.
type RationaleKind string

const (
	RationaleWhy       RationaleKind = "why"
	RationaleRationale RationaleKind = "rationale"
	RationaleNote      RationaleKind = "note"
	RationaleHack      RationaleKind = "hack"
	RationaleFixme     RationaleKind = "fixme"
	RationaleTodo      RationaleKind = "todo"
	RationaleDecision  RationaleKind = "decision"
)

// Rationale is a structured comment extracted from a source file that
// documents a deliberate decision or known deviation.
type Rationale struct {
	Kind          RationaleKind `json:"kind"`
	Text          string        `json:"text"`
	Line          int           `json:"line"`
	NearestSymbol string        `json:"nearest_symbol,omitempty"`
}

// SpecInput is the minimal spec data needed by an applies_to inferer.
type SpecInput struct {
	Ref             string
	BodyText        string
	ManualAppliesTo []string
}

// CacheEntry is one code-scan cache row exchanged between the kernel and an
// applies_to inferer.
type CacheEntry struct {
	ContentHash string
	Path        string
	Symbols     []Symbol
	Rationale   []Rationale
}

// InferredEdge is one inferred applies_to link from a spec to a code file.
type InferredEdge struct {
	SpecRef   string   `json:"spec_ref"`
	FilePath  string   `json:"file_path"`
	MatchedOn []string `json:"matched_on"`
}

// Limits bounds inference work. Zero values ask the inferer to use its own
// conservative defaults.
type Limits struct {
	MaxFileSizeBytes int64
}

// Request is the kernel-owned input contract for applies_to inference.
type Request struct {
	WorkspaceRoot string
	Specs         []SpecInput
	PreviousCache []CacheEntry
	Limits        Limits
}

// Result is the structured output returned by an applies_to inferer. The
// kernel owns all persistence of these values.
type Result struct {
	CacheEntries []CacheEntry
	Edges        []InferredEdge
}

// AppliesToInferer generates inferred applies_to edges and refreshed code-scan
// cache entries.
type AppliesToInferer interface {
	Name() string
	InferAppliesTo(ctx context.Context, req Request) (*Result, error)
}

// Factory creates an applies_to inferer.
type Factory func() AppliesToInferer

var (
	mu       sync.RWMutex
	inferers = map[string]Factory{}
)

// Register registers an applies_to inferer factory. It is safe to call from
// init.
func Register(name string, factory Factory) {
	name = strings.TrimSpace(name)
	if name == "" {
		panic("codeinfer: empty inferer name")
	}
	if factory == nil {
		panic(fmt.Sprintf("codeinfer: nil factory for %q", name))
	}

	mu.Lock()
	defer mu.Unlock()
	if _, exists := inferers[name]; exists {
		panic(fmt.Sprintf("codeinfer: inferer %q already registered", name))
	}
	inferers[name] = factory
}

// Lookup returns a new inferer instance by name.
func Lookup(name string) (AppliesToInferer, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false
	}

	mu.RLock()
	factory, ok := inferers[name]
	mu.RUnlock()
	if !ok {
		return nil, false
	}
	return factory(), true
}

// Registered reports whether an inferer factory is registered by name without
// constructing an inferer instance.
func Registered(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}

	mu.RLock()
	_, ok := inferers[name]
	mu.RUnlock()
	return ok
}

// RegisteredNames returns registered inferer names in stable order.
func RegisteredNames() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(inferers))
	for name := range inferers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ReplaceForTest swaps one registry entry and returns a restore function. It is
// intended for kernel tests that need to exercise inference without importing a
// concrete extension package.
func ReplaceForTest(name string, factory Factory) func() {
	name = strings.TrimSpace(name)
	if name == "" {
		panic("codeinfer: empty test inferer name")
	}

	mu.Lock()
	previous, hadPrevious := inferers[name]
	if factory == nil {
		delete(inferers, name)
	} else {
		inferers[name] = factory
	}
	mu.Unlock()

	return func() {
		mu.Lock()
		defer mu.Unlock()
		if hadPrevious {
			inferers[name] = previous
			return
		}
		delete(inferers, name)
	}
}
