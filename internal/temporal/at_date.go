package temporal

import (
	"fmt"
	"strings"
	"time"
)

const atDateLayout = "2006-01-02"

// NormalizeAtDate normalizes point-in-time query input to YYYY-MM-DD.
// It accepts plain dates and timezone-aware timestamps. Timestamps are
// converted to UTC before extracting the comparison date.
func NormalizeAtDate(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if len(trimmed) == len(atDateLayout) {
		parsed, err := time.Parse(atDateLayout, trimmed)
		if err != nil || parsed.Format(atDateLayout) != trimmed {
			return "", invalidAtDateError(value)
		}
		return trimmed, nil
	}

	parsed, err := parseAtDateTimestamp(normalizeTimestampSeparator(trimmed))
	if err != nil {
		return "", invalidAtDateError(value)
	}
	return parsed.UTC().Format(atDateLayout), nil
}

func normalizeTimestampSeparator(value string) string {
	if len(value) <= len(atDateLayout) || value[len(atDateLayout)] != 't' {
		return value
	}
	normalized := []byte(value)
	normalized[len(atDateLayout)] = 'T'
	return string(normalized)
}

func parseAtDateTimestamp(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05 Z07:00",
		"2006-01-02 15:04:05.999999999 Z07:00",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported timestamp")
}

func invalidAtDateError(value string) error {
	return fmt.Errorf("invalid at_date %q: expected YYYY-MM-DD or a timezone-aware timestamp", value)
}
