package ast

import (
	"fmt"
	"strings"
	"sync"

	"github.com/dusk-network/pituitary/internal/codeinfer"
	"github.com/odvcencio/gotreesitter"
)

// parserMu serializes tree-sitter parsing. The gotreesitter Language objects
// contain shared mutable state (DFA token source pool) that is not
// goroutine-safe, even through ParserPool.
var parserMu sync.Mutex

type SymbolKind = codeinfer.SymbolKind

const (
	SymbolFunction = codeinfer.SymbolFunction
	SymbolMethod   = codeinfer.SymbolMethod
	SymbolType     = codeinfer.SymbolType
	SymbolImport   = codeinfer.SymbolImport
)

type Symbol = codeinfer.Symbol

// queryPatterns maps each language to its tree-sitter query for symbol extraction.
var queryPatterns = map[LangID]string{
	LangGo: `
		(function_declaration name: (identifier) @func_name)
		(method_declaration name: (field_identifier) @method_name)
		(type_spec name: (type_identifier) @type_name)
		(import_spec path: (interpreted_string_literal) @import_path)
	`,
	LangPython: `
		(function_definition name: (identifier) @func_name)
		(class_definition name: (identifier) @type_name)
		(import_statement name: (dotted_name) @import_name)
		(import_from_statement module_name: (dotted_name) @import_name)
	`,
	LangJavaScript: `
		(function_declaration name: (identifier) @func_name)
		(class_declaration name: (identifier) @type_name)
		(method_definition name: (property_identifier) @method_name)
		(import_statement source: (string) @import_path)
	`,
	LangTypeScript: `
		(function_declaration name: (identifier) @func_name)
		(class_declaration name: (type_identifier) @type_name)
		(method_definition name: (property_identifier) @method_name)
		(interface_declaration name: (type_identifier) @type_name)
		(import_statement source: (string) @import_path)
	`,
	LangTSX: `
		(function_declaration name: (identifier) @func_name)
		(class_declaration name: (type_identifier) @type_name)
		(method_definition name: (property_identifier) @method_name)
		(interface_declaration name: (type_identifier) @type_name)
		(import_statement source: (string) @import_path)
	`,
	LangRust: `
		(function_item name: (identifier) @func_name)
		(struct_item name: (type_identifier) @type_name)
		(enum_item name: (type_identifier) @type_name)
		(trait_item name: (type_identifier) @type_name)
		(use_declaration argument: (_) @import_path)
	`,
}

// captureKind maps tree-sitter capture names to SymbolKind.
var captureKind = map[string]SymbolKind{
	"func_name":   SymbolFunction,
	"method_name": SymbolMethod,
	"type_name":   SymbolType,
	"import_path": SymbolImport,
	"import_name": SymbolImport,
}

// ExtractSymbols parses source code with tree-sitter and returns the defined symbols.
// If the parser panics on malformed or complex input, the error is recovered and
// returned rather than crashing the process.
func ExtractSymbols(src []byte, lang LangID) (symbols []Symbol, err error) {
	defer func() {
		if r := recover(); r != nil {
			symbols = nil
			err = fmt.Errorf("tree-sitter panic on %s source: %v", lang, r)
		}
	}()
	return extractSymbols(src, lang)
}

func extractSymbols(src []byte, lang LangID) ([]Symbol, error) {
	parserMu.Lock()
	defer parserMu.Unlock()

	grammar := GrammarFor(lang)
	if grammar == nil {
		return nil, fmt.Errorf("unsupported language: %q", lang)
	}
	pattern, ok := queryPatterns[lang]
	if !ok {
		return nil, fmt.Errorf("no query pattern for language: %q", lang)
	}

	parser := gotreesitter.NewParser(grammar)
	tree, err := parser.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parse %q source: %w", lang, err)
	}
	root := tree.RootNode()

	query, err := gotreesitter.NewQuery(pattern, grammar)
	if err != nil {
		return nil, fmt.Errorf("compile query for %q: %w", lang, err)
	}

	seen := make(map[string]bool)
	var symbols []Symbol

	cursor := query.Exec(root, grammar, src)
	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}
		for _, capture := range match.Captures {
			kind, ok := captureKind[capture.Name]
			if !ok {
				continue
			}
			name := cleanSymbolName(capture.Node.Text(src))
			if name == "" {
				continue
			}

			key := name + "|" + string(kind)
			if seen[key] {
				continue
			}
			seen[key] = true
			symbols = append(symbols, Symbol{Name: name, Kind: kind})
		}
	}

	// Python: reclassify functions inside class bodies as methods.
	if lang == LangPython {
		symbols = classifyNestedMethods(root, grammar, src, symbols, "class_definition", "function_definition")
	}
	// Rust: reclassify functions inside impl blocks as methods.
	if lang == LangRust {
		symbols = classifyNestedMethods(root, grammar, src, symbols, "impl_item", "function_item")
	}

	return symbols, nil
}

// cleanSymbolName strips quotes and trims whitespace from extracted symbol text.
func cleanSymbolName(name string) string {
	name = strings.Trim(name, "\"'`")
	return strings.TrimSpace(name)
}

// classifyNestedMethods walks the tree to find function nodes nested inside
// container nodes (class or impl block) and reclassifies them from
// SymbolFunction to SymbolMethod.
func classifyNestedMethods(root *gotreesitter.Node, lang *gotreesitter.Language, src []byte, symbols []Symbol, containerType, funcType string) []Symbol {
	nested := make(map[string]bool)
	walkForNested(root, lang, src, nested, containerType, funcType, false)

	for i, sym := range symbols {
		if sym.Kind == SymbolFunction && nested[sym.Name] {
			symbols[i].Kind = SymbolMethod
		}
	}
	return symbols
}

func walkForNested(node *gotreesitter.Node, lang *gotreesitter.Language, src []byte, names map[string]bool, containerType, funcType string, inContainer bool) {
	nodeType := node.Type(lang)
	if nodeType == containerType {
		inContainer = true
	}
	if inContainer && nodeType == funcType {
		// Find the name child (identifier or field_identifier).
		for i := 0; i < node.NamedChildCount(); i++ {
			child := node.NamedChild(i)
			ct := child.Type(lang)
			if ct == "identifier" || ct == "field_identifier" {
				name := child.Text(src)
				if name != "__init__" && name != "__new__" {
					names[name] = true
				}
				break
			}
		}
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		walkForNested(node.NamedChild(i), lang, src, names, containerType, funcType, inContainer)
	}
}
