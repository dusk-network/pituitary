package ranking

import (
	"strings"
	"unicode"
)

const historicalSectionPenaltyFactor = 0.7

var historicalIntentTerms = []string{
	"historical",
	"history",
	"provenance",
	"legacy",
	"archival",
	"archive",
	"superseded",
	"deprecated",
}

var historicalSectionTerms = []string{
	"historical provenance",
	"historical context",
	"change history",
	"migration history",
	"version history",
	"history",
	"provenance",
	"legacy context",
	"archival context",
}

// SearchPrefersHistoricalContext reports whether the caller explicitly asked
// for historical or provenance-focused retrieval.
func SearchPrefersHistoricalContext(query string) bool {
	normalized := normalizeText(query)
	if normalized == "" {
		return false
	}
	for _, term := range historicalIntentTerms {
		if strings.Contains(normalized, term) {
			return true
		}
	}
	return false
}

// IsHistoricalSectionHeading reports whether a chunk heading looks like
// archival provenance/history content rather than active normative material.
func IsHistoricalSectionHeading(heading string) bool {
	normalized := normalizeText(heading)
	if normalized == "" {
		return false
	}
	segments := strings.Split(normalized, "/")
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		for _, term := range historicalSectionTerms {
			if strings.Contains(segment, term) {
				return true
			}
		}
	}
	return false
}

// AdjustHistoricalSectionScore down-ranks historical provenance sections by
// default while keeping them accessible when the query explicitly asks for
// historical context.
func AdjustHistoricalSectionScore(score float64, heading string, preferHistorical bool) float64 {
	if score <= 0 {
		return 0
	}
	if preferHistorical || !IsHistoricalSectionHeading(heading) {
		return score
	}
	return score * historicalSectionPenaltyFactor
}

func normalizeText(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			builder.WriteRune(r)
		case r == '/':
			builder.WriteString(" / ")
		default:
			builder.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}
