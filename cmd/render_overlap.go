package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
)

func renderOverlapResult(w io.Writer, result *analysis.OverlapResult) {
	p := presentationForWriter(w)
	fmt.Fprintln(w, p.headerLine("check-overlap", " · "+p.cyan(result.Candidate.Ref)))
	if strings.TrimSpace(result.Candidate.Title) != "" {
		fmt.Fprintf(w, "    %s\n", p.dim(result.Candidate.Title))
	}
	fmt.Fprintln(w)

	if len(result.Overlaps) == 0 {
		fmt.Fprintf(w, "  %s no overlaps\n", p.check())
		renderOverlapRecommendation(w, result.Recommendation)
		return
	}
	for _, overlap := range result.Overlaps {
		fmt.Fprintf(
			w,
			"  %s %s  %s  %s  %s\n",
			overlapBlock(p, overlap),
			p.cyan(overlap.Ref),
			p.bold(fmt.Sprintf("%.3f", overlap.Score)),
			p.dim(overlap.Title),
			overlapDisplaySummary(overlap),
		)
	}
	fmt.Fprintln(w)
	renderOverlapRecommendation(w, result.Recommendation)
}

func renderCompareResult(w io.Writer, result *analysis.CompareResult) {
	fmt.Fprintf(w, "specs: %s\n", strings.Join(result.SpecRefs, ", "))
	fmt.Fprintf(w, "recommendation: %s\n", result.Comparison.Recommendation)
	for _, tradeoff := range result.Comparison.Tradeoffs {
		fmt.Fprintf(w, "%s: %s\n", tradeoff.Topic, tradeoff.Summary)
	}
}

func overlapBlock(p renderPresentation, item analysis.OverlapItem) string {
	if item.Guidance == "merge_candidate" || item.Score >= 0.85 {
		return p.blockHigh()
	}
	return p.blockMedium()
}

func overlapDisplaySummary(item analysis.OverlapItem) string {
	guidance := humanizeOverlapGuidance(item.Guidance)
	if item.Relationship == "" {
		return guidance
	}
	if guidance == "" || guidance == item.Relationship {
		return item.Relationship
	}
	return guidance
}

func renderOverlapRecommendation(w io.Writer, recommendation string) {
	p := presentationForWriter(w)
	switch recommendation {
	case "proceed_with_supersedes":
		fmt.Fprintf(w, "  %s %s\n", p.check(), "candidate already declares the replacement path — no action needed")
	case "merge_into_existing":
		fmt.Fprintf(w, "  %s %s\n", p.arrow(), "strong merge candidate — merge into the existing spec")
	case "review_boundaries":
		fmt.Fprintf(w, "  %s %s\n", p.arrow(), "real overlap detected — review scope boundaries before merging")
	default:
		fmt.Fprintf(w, "  %s %s\n", p.arrow(), recommendation)
	}
}

func humanizeOverlapRecommendation(recommendation string) string {
	switch recommendation {
	case "review_boundaries":
		return "real overlap, but clarify boundaries before merging"
	case "merge_into_existing":
		return "strong merge candidate"
	case "proceed_with_supersedes":
		return "candidate already declares the replacement path"
	default:
		return ""
	}
}

func humanizeOverlapGuidance(guidance string) string {
	switch guidance {
	case "merge_candidate":
		return "merge candidate"
	case "boundary_review":
		return "boundary review"
	default:
		return guidance
	}
}
