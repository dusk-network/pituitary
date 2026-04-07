package ast

import (
	"path/filepath"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// LangID identifies a supported programming language.
type LangID string

const (
	LangGo         LangID = "go"
	LangPython     LangID = "python"
	LangJavaScript LangID = "javascript"
	LangTypeScript LangID = "typescript"
	LangTSX        LangID = "tsx"
	LangRust       LangID = "rust"
)

var extToLang = map[string]LangID{
	".go":  LangGo,
	".py":  LangPython,
	".pyw": LangPython,
	".js":  LangJavaScript,
	".mjs": LangJavaScript,
	".cjs": LangJavaScript,
	".ts":  LangTypeScript,
	".tsx": LangTSX,
	".rs":  LangRust,
}

// DetectLanguage returns the language ID for a file path, or "" if unsupported.
func DetectLanguage(path string) LangID {
	ext := strings.ToLower(filepath.Ext(path))
	return extToLang[ext]
}

// SupportedLanguages returns all language IDs that have a grammar registered.
func SupportedLanguages() []LangID {
	return []LangID{LangGo, LangPython, LangJavaScript, LangTypeScript, LangTSX, LangRust}
}

// GrammarFor returns the gotreesitter Language for a LangID, or nil if unknown.
func GrammarFor(lang LangID) *gotreesitter.Language {
	switch lang {
	case LangGo:
		return grammars.GoLanguage()
	case LangPython:
		return grammars.PythonLanguage()
	case LangJavaScript:
		return grammars.JavascriptLanguage()
	case LangTypeScript:
		return grammars.TypescriptLanguage()
	case LangTSX:
		return grammars.TsxLanguage()
	case LangRust:
		return grammars.RustLanguage()
	default:
		return nil
	}
}
