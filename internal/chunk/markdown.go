package chunk

import (
	"strings"
)

// Section is one heading-aware Markdown chunk.
type Section struct {
	Heading string
	Body    string
}

type heading struct {
	Level int
	Text  string
}

// Markdown splits Markdown into heading-aware sections.
func Markdown(title, body string) []Section {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")

	var (
		sections      []Section
		stack         []heading
		currentLines  []string
		currentHeader string
	)

	flush := func() {
		text := strings.TrimSpace(strings.Join(currentLines, "\n"))
		currentLines = nil
		if text == "" {
			return
		}

		headingText := strings.TrimSpace(currentHeader)
		if headingText == "" {
			headingText = strings.TrimSpace(title)
		}

		sections = append(sections, Section{
			Heading: headingText,
			Body:    text,
		})
	}

	for _, line := range lines {
		level, text, ok := parseHeading(line)
		if ok {
			flush()
			for len(stack) > 0 && stack[len(stack)-1].Level >= level {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, heading{Level: level, Text: text})
			currentHeader = joinHeadings(stack)
			continue
		}

		currentLines = append(currentLines, line)
	}

	flush()

	if len(sections) == 0 {
		text := strings.TrimSpace(body)
		if text == "" {
			return nil
		}
		return []Section{{
			Heading: strings.TrimSpace(title),
			Body:    text,
		}}
	}

	return sections
}

func parseHeading(line string) (int, string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return 0, "", false
	}

	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level > 6 {
		return 0, "", false
	}
	if len(trimmed) == level || trimmed[level] != ' ' {
		return 0, "", false
	}

	text := strings.TrimSpace(trimmed[level+1:])
	if text == "" {
		return 0, "", false
	}
	return level, text, true
}

func joinHeadings(stack []heading) string {
	parts := make([]string, 0, len(stack))
	for _, item := range stack {
		if item.Text == "" {
			continue
		}
		parts = append(parts, item.Text)
	}
	return strings.Join(parts, " / ")
}
