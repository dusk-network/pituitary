package ast

import (
	"fmt"
	"regexp"
	"strings"
)

// MinIdentifierLength is the minimum length for a spec-text identifier to be
// considered a candidate for symbol matching. Shorter identifiers produce too
// many false-positive matches.
const MinIdentifierLength = 6

// identifierRE matches PascalCase, camelCase, snake_case, and UPPER_SNAKE
// identifiers that are at least MinIdentifierLength characters.
// The {5,} quantifier is MinIdentifierLength-1 because the first character is
// matched separately by [A-Za-z_].
var identifierRE = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]{` + fmt.Sprintf("%d", MinIdentifierLength-1) + `,})\b`)

// ScanSpecIdentifiers extracts identifier-like tokens from spec body text.
// Only identifiers with length >= MinIdentifierLength are returned.
// Duplicates are removed.
func ScanSpecIdentifiers(body string) []string {
	matches := identifierRE.FindAllString(body, -1)
	seen := make(map[string]bool)
	var result []string
	for _, m := range matches {
		m = strings.TrimSpace(m)
		if len(m) < MinIdentifierLength {
			continue
		}
		if isCommonWord(m) {
			continue
		}
		if !seen[m] {
			seen[m] = true
			result = append(result, m)
		}
	}
	return result
}

// commonWords are prose words that pass the identifier regex but are not code symbols.
var commonWords = map[string]bool{
	"should": true, "between": true, "through": true, "before": true,
	"during": true, "unless": true, "because": true, "rather": true,
	"existing": true, "following": true, "configuration": true,
	"possible": true, "required": true, "specific": true,
	"default": true, "example": true, "section": true,
	"within": true, "without": true, "however": true,
	"overview": true, "requirements": true, "design": true,
	"decisions": true, "implementation": true, "middleware": true,
	"instead": true, "already": true, "another": true,
	"change": true, "changes": true, "current": true,
	"ensure": true, "expect": true, "format": true,
	"include": true, "method": true, "module": true,
	"number": true, "option": true, "output": true,
	"parameter": true, "prefer": true, "process": true,
	"provide": true, "replace": true, "replaces": true,
	"result": true, "return": true, "single": true,
	"string": true, "system": true, "update": true,
	"values": true, "preserve": true, "accept": true,
}

func isCommonWord(word string) bool {
	return commonWords[strings.ToLower(word)]
}
