package cmd

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/dusk-network/pituitary/internal/index"
)

func renderSearchSpecsResult(w io.Writer, result *index.SearchSpecResult) {
	if len(result.Matches) == 0 {
		fmt.Fprintln(w, "no matches")
		return
	}
	if note := searchSpecScoreNote(result); note != "" {
		fmt.Fprintf(w, "score semantics: %s\n\n", note)
	}

	for i, match := range result.Matches {
		fmt.Fprintf(w, "%d. %s | %s | %.3f\n", i+1, match.Ref, match.SectionHeading, match.Score)
		if details := searchMatchDetailLine(match); details != "" {
			fmt.Fprintf(w, "   %s\n", details)
		}
		if match.Excerpt != "" {
			fmt.Fprintln(w, match.Excerpt)
		}
		if i < len(result.Matches)-1 {
			fmt.Fprintln(w)
		}
	}
}

func renderSearchSpecsTable(w io.Writer, result *index.SearchSpecResult) {
	if len(result.Matches) == 0 {
		fmt.Fprintln(w, "no matches")
		return
	}
	if note := searchSpecScoreNote(result); note != "" {
		fmt.Fprintf(w, "score semantics: %s\n\n", note)
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "REF\tREPO\tTITLE\tSECTION\tSOURCE\t%s\n", index.SearchSpecScoreColumnLabel(result.ScoreKind))
	for _, match := range result.Matches {
		fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%s\t%.3f\n",
			renderTableValue(match.Ref, 12),
			renderTableValue(match.Repo, 16),
			renderTableValue(match.Title, 28),
			renderTableValue(match.SectionHeading, 36),
			renderTableValue(displaySourcePath(match.SourceRef), 40),
			match.Score,
		)
	}
	_ = tw.Flush()
}

func renderTableValue(value string, maxWidth int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	if maxWidth <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxWidth {
		return value
	}
	if maxWidth <= 3 {
		return string(runes[:maxWidth])
	}
	return string(runes[:maxWidth-3]) + "..."
}

func searchMatchDetailLine(match index.SearchSpecMatch) string {
	parts := make([]string, 0, 2)
	if match.Repo != "" {
		parts = append(parts, "repo: "+match.Repo)
	}
	if match.SourceRef != "" {
		parts = append(parts, "source: "+displaySourcePath(match.SourceRef))
	}
	return strings.Join(parts, " | ")
}

func searchSpecScoreNote(result *index.SearchSpecResult) string {
	if result == nil {
		return ""
	}
	if note := strings.TrimSpace(result.ScoreDescription); note != "" {
		return note
	}
	return index.SearchSpecScoreDescription(result.ScoreKind)
}
