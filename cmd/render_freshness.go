package cmd

import (
	"fmt"
	"io"

	"github.com/dusk-network/pituitary/internal/analysis"
)

func renderFreshnessResult(w io.Writer, result *analysis.FreshnessResult) {
	p := presentationForWriter(w)
	fmt.Fprintln(w, p.headerLine("check-spec-freshness", ""))
	fmt.Fprintln(w)

	if len(result.Items) == 0 {
		fmt.Fprintf(w, "  %s no specs evaluated\n", p.check())
		return
	}

	var staleCount int
	for _, item := range result.Items {
		if item.Verdict != "fresh" {
			staleCount++
		}
	}

	if staleCount == 0 {
		fmt.Fprintf(w, "  %s all %d specs appear fresh\n", p.check(), len(result.Items))
		return
	}

	fmt.Fprintf(w, "  %s %d of %d specs have freshness signals\n\n",
		p.yellow("!"), staleCount, len(result.Items))

	for _, item := range result.Items {
		if item.Verdict == "fresh" {
			continue
		}

		label := item.SpecRef
		if item.Repo != "" {
			label = item.Repo + ":" + label
		}
		fmt.Fprintf(w, "  %s %s\n", p.white(label), p.dim(item.Title))
		fmt.Fprintf(w, "    verdict: %s  confidence: %s  score: %.3f\n",
			freshnessVerdictLabel(p, item.Verdict), item.Confidence, item.Score)

		if item.SourceRef != "" {
			fmt.Fprintf(w, "    source: %s\n", displaySourcePath(item.SourceRef))
		}

		for i, signal := range item.Signals {
			branch := p.treeBranch(i == len(item.Signals)-1)
			fmt.Fprintf(w, "    %s %s %s\n", branch, p.dim("["+signal.Kind+"]"), signal.Summary)
			if signal.Evidence != nil && signal.Evidence.TrailSourceRef != "" {
				indent := "│"
				if i == len(item.Signals)-1 {
					indent = " "
				}
				fmt.Fprintf(w, "    %s   trail: %s",
					indent,
					displaySourcePath(signal.Evidence.TrailSourceRef))
				if signal.Evidence.TrailSection != "" {
					fmt.Fprintf(w, " / %s", signal.Evidence.TrailSection)
				}
				fmt.Fprintln(w)
			}
		}
		fmt.Fprintln(w)
	}
}

func freshnessVerdictLabel(p renderPresentation, verdict string) string {
	switch verdict {
	case "likely_stale":
		return p.yellow("likely_stale")
	case "stale_foundation":
		return p.yellow("stale_foundation")
	case "review_recommended":
		return p.dim("review_recommended")
	default:
		return verdict
	}
}
