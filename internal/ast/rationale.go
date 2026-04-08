package ast

import (
	"regexp"
	"sort"
	"strings"
)

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

// tagPatterns match explicit rationale tags at the start of a comment.
var tagPatterns = []struct {
	re   *regexp.Regexp
	kind RationaleKind
}{
	{regexp.MustCompile(`(?i)^\s*(?://|#|/\*+|\*)\s*WHY:\s*(.+)`), RationaleWhy},
	{regexp.MustCompile(`(?i)^\s*(?://|#|/\*+|\*)\s*RATIONALE:\s*(.+)`), RationaleRationale},
	{regexp.MustCompile(`(?i)^\s*(?://|#|/\*+|\*)\s*NOTE:\s*(.+)`), RationaleNote},
	{regexp.MustCompile(`(?i)^\s*(?://|#|/\*+|\*)\s*HACK:\s*(.+)`), RationaleHack},
	{regexp.MustCompile(`(?i)^\s*(?://|#|/\*+|\*)\s*FIXME:\s*(.+)`), RationaleFixme},
	{regexp.MustCompile(`(?i)^\s*(?://|#|/\*+|\*)\s*TODO:\s*(.+)`), RationaleTodo},
}

// decisionLanguagePattern matches comments containing decision language.
var decisionLanguagePattern = regexp.MustCompile(`(?i)\b(because|instead of|chose|trade-?off|deliberately|intentionally|workaround|deviation)\b`)

// commentLinePattern detects single-line comments.
var commentLinePattern = regexp.MustCompile(`^\s*(?://|#|/\*|\*)`)

// ExtractRationale scans source code for rationale comments and returns them
// with line numbers and nearest symbol associations.
func ExtractRationale(src []byte, symbols []Symbol, lang LangID) []Rationale {
	lines := strings.Split(string(src), "\n")

	// Build a map of symbol positions for nearest-symbol linking.
	// We use a simple heuristic: find the first occurrence of each symbol
	// name in the source to estimate its line number.
	symbolLines := buildSymbolLineMap(lines, symbols)

	var rationales []Rationale

	for lineIdx, line := range lines {
		lineNum := lineIdx + 1

		// Check explicit tags first.
		if r, ok := matchTaggedRationale(line, lineNum); ok {
			r.NearestSymbol = findNearestSymbol(lineNum, symbolLines)
			rationales = append(rationales, r)
			continue
		}

		// Check for decision language in comments.
		if isCommentLine(line) && decisionLanguagePattern.MatchString(line) {
			text := cleanCommentText(line)
			if len(text) > 10 { // skip trivially short matches
				rationales = append(rationales, Rationale{
					Kind:          RationaleDecision,
					Text:          text,
					Line:          lineNum,
					NearestSymbol: findNearestSymbol(lineNum, symbolLines),
				})
			}
		}
	}

	return rationales
}

func matchTaggedRationale(line string, lineNum int) (Rationale, bool) {
	for _, pat := range tagPatterns {
		if m := pat.re.FindStringSubmatch(line); m != nil {
			text := strings.TrimSpace(m[1])
			// Strip trailing */ for block comments.
			text = strings.TrimSuffix(text, "*/")
			text = strings.TrimSpace(text)
			return Rationale{Kind: pat.kind, Text: text, Line: lineNum}, true
		}
	}
	return Rationale{}, false
}

func isCommentLine(line string) bool {
	return commentLinePattern.MatchString(line)
}

func cleanCommentText(line string) string {
	// Strip common comment prefixes.
	line = strings.TrimSpace(line)
	for _, prefix := range []string{"//", "# ", "#", "/*", "*/", "* ", "*"} {
		line = strings.TrimPrefix(line, prefix)
	}
	// Strip trailing */ for single-line block comments.
	line = strings.TrimSuffix(line, "*/")
	return strings.TrimSpace(line)
}

type symbolLine struct {
	name string
	line int
}

func buildSymbolLineMap(lines []string, symbols []Symbol) []symbolLine {
	var result []symbolLine
	seen := make(map[string]bool)
	for _, sym := range symbols {
		if sym.Kind == SymbolImport || seen[sym.Name] {
			continue
		}
		seen[sym.Name] = true
		for i, line := range lines {
			if strings.Contains(line, sym.Name) {
				result = append(result, symbolLine{name: sym.Name, line: i + 1})
				break
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].line < result[j].line })
	return result
}

func findNearestSymbol(lineNum int, symbolLines []symbolLine) string {
	if len(symbolLines) == 0 {
		return ""
	}
	// Prefer the first symbol that is at or after the rationale line.
	// If there is no such symbol, fall back to the nearest symbol before it.
	beforeName := ""

	for _, sl := range symbolLines {
		if sl.line >= lineNum {
			// symbolLines is sorted by line, so the first symbol at or after
			// the comment is the preferred match.
			return sl.name
		}
		// Keep the latest symbol before the comment as the fallback.
		beforeName = sl.name
	}

	return beforeName
}
