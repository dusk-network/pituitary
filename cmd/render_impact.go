package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
)

func renderAnalyzeImpactResult(w io.Writer, result *analysis.AnalyzeImpactResult) {
	fmt.Fprintf(w, "spec: %s | change_type: %s\n", result.SpecRef, result.ChangeType)
	if len(result.RankedSummary) > 0 {
		fmt.Fprintf(w, "ranked summary: %d\n", len(result.RankedSummary))
		for _, item := range result.RankedSummary {
			fmt.Fprintf(w, "%d. %s %s", item.Rank, item.Kind, item.Ref)
			if item.Repo != "" {
				fmt.Fprintf(w, " | repo: %s", item.Repo)
			}
			if item.SourceRef != "" {
				fmt.Fprintf(w, " | source: %s", displaySourcePath(item.SourceRef))
			}
			if item.Score > 0 {
				fmt.Fprintf(w, " | %.3f", item.Score)
			}
			if item.Title != "" {
				fmt.Fprintf(w, " | %s", item.Title)
			}
			if item.Why != "" {
				fmt.Fprintf(w, " | why: %s", item.Why)
			}
			if item.ReviewFirst != "" {
				fmt.Fprintf(w, " | review first: %s", displayImpactSummaryTarget(item.ReviewFirst))
			}
			fmt.Fprintln(w)
		}
	}
	if result.SummaryOnly {
		return
	}
	fmt.Fprintf(w, "affected specs: %d\n", len(result.AffectedSpecs))
	for _, item := range result.AffectedSpecs {
		fmt.Fprintf(w, "- %s", item.Ref)
		if item.Repo != "" {
			fmt.Fprintf(w, " | repo: %s", item.Repo)
		}
		fmt.Fprintf(w, " | %s", item.Relationship)
		if item.Historical {
			fmt.Fprint(w, " | historical")
		}
		if item.Title != "" {
			fmt.Fprintf(w, " | %s", item.Title)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "affected refs: %d\n", len(result.AffectedRefs))
	for _, item := range result.AffectedRefs {
		fmt.Fprintf(w, "- %s | %s\n", item.Ref, item.Kind)
	}
	fmt.Fprintf(w, "affected docs: %d\n", len(result.AffectedDocs))
	for _, item := range result.AffectedDocs {
		fmt.Fprintf(w, "- %s", item.Ref)
		if item.Repo != "" {
			fmt.Fprintf(w, " | repo: %s", item.Repo)
		}
		if item.SourceRef != "" {
			fmt.Fprintf(w, " | source: %s", displaySourcePath(item.SourceRef))
		}
		fmt.Fprintf(w, " | %.3f", item.Score)
		if item.Classification != "" {
			fmt.Fprintf(w, " | %s", item.Classification)
		}
		if item.Title != "" {
			fmt.Fprintf(w, " | %s", item.Title)
		}
		fmt.Fprintln(w)
		if len(item.Reasons) > 0 {
			fmt.Fprintf(w, "  reason: %s\n", item.Reasons[0])
		}
		if item.Evidence != nil {
			fmt.Fprintf(
				w,
				"  evidence: %s / %s -> %s / %s\n",
				displaySourcePath(item.Evidence.SpecSourceRef),
				displayDefault(item.Evidence.SpecSection, "(body)"),
				displaySourcePath(item.Evidence.DocSourceRef),
				displayDefault(item.Evidence.DocSection, "(body)"),
			)
			if strings.TrimSpace(item.Evidence.LinkReason) != "" {
				fmt.Fprintf(w, "  why: %s\n", item.Evidence.LinkReason)
			}
		}
		if len(item.SuggestedTargets) > 0 {
			target := item.SuggestedTargets[0]
			fmt.Fprintf(w, "  target: %s", displaySourcePath(target.SourceRef))
			if strings.TrimSpace(target.Section) != "" {
				fmt.Fprintf(w, " / %s", target.Section)
			}
			fmt.Fprintln(w)
		}
	}
}

func displayImpactSummaryTarget(value string) string {
	head, tail, ok := strings.Cut(value, " / ")
	if !ok {
		return displaySourcePath(value)
	}
	return displaySourcePath(head) + " / " + tail
}

func reviewImpactLines(result *analysis.AnalyzeImpactResult) []string {
	if result == nil {
		return nil
	}
	lines := make([]string, 0, 6)
	for _, item := range topImpactedSpecs(result.AffectedSpecs, 2) {
		line := fmt.Sprintf("%s  %s · %s", item.Ref, item.Title, item.Relationship)
		if item.Repo != "" {
			line += " · repo: " + item.Repo
		}
		if item.Historical {
			line += " · historical"
		}
		lines = append(lines, line)
	}
	for _, item := range topImpactedDocs(result.AffectedDocs, 2) {
		line := fmt.Sprintf("%s  %.3f", item.Ref, item.Score)
		if item.Classification != "" {
			line += " · " + item.Classification
		}
		if item.Repo != "" {
			line += " · repo: " + item.Repo
		}
		if item.SourceRef != "" {
			line += " · " + displaySourcePath(item.SourceRef)
		}
		if len(item.SuggestedTargets) > 0 && item.SuggestedTargets[0].Section != "" {
			line += " · target: " + item.SuggestedTargets[0].Section
		}
		lines = append(lines, line)
	}
	return lines
}

func topImpactedSpecs(items []analysis.ImpactedSpec, limit int) []analysis.ImpactedSpec {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	if len(items) <= limit {
		return append([]analysis.ImpactedSpec(nil), items...)
	}
	return append([]analysis.ImpactedSpec(nil), items[:limit]...)
}

func topImpactedDocs(items []analysis.ImpactedDoc, limit int) []analysis.ImpactedDoc {
	if len(items) == 0 || limit <= 0 {
		return nil
	}
	if len(items) <= limit {
		return append([]analysis.ImpactedDoc(nil), items...)
	}
	return append([]analysis.ImpactedDoc(nil), items[:limit]...)
}
