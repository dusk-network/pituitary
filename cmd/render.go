package cmd

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

func renderCommandResult(w io.Writer, command string, result any) error {
	description, ok := commands[command]
	if !ok {
		return fmt.Errorf("unknown command %q", command)
	}

	fmt.Fprintf(w, "pituitary %s: %s\n", command, description)

	switch typed := result.(type) {
	case *index.RebuildResult:
		renderIndexResult(w, typed)
	case *statusResult:
		renderStatusResult(w, typed)
	case *versionResult:
		renderVersionResult(w, typed)
	case *source.PreviewResult:
		renderPreviewSourcesResult(w, typed)
	case *index.SearchSpecResult:
		renderSearchSpecsResult(w, typed)
	case *analysis.OverlapResult:
		renderOverlapResult(w, typed)
	case *analysis.CompareResult:
		renderCompareResult(w, typed)
	case *analysis.AnalyzeImpactResult:
		renderAnalyzeImpactResult(w, typed)
	case *analysis.ComplianceResult:
		renderComplianceResult(w, typed)
	case *analysis.DocDriftResult:
		renderDocDriftResult(w, typed)
	case *analysis.ReviewResult:
		renderReviewResult(w, typed)
	default:
		return fmt.Errorf("unsupported result type %T", result)
	}

	return nil
}

func renderCommandTable(w io.Writer, command string, result any) error {
	description, ok := commands[command]
	if !ok {
		return fmt.Errorf("unknown command %q", command)
	}

	fmt.Fprintf(w, "pituitary %s: %s\n", command, description)

	switch typed := result.(type) {
	case *index.SearchSpecResult:
		renderSearchSpecsTable(w, typed)
		return nil
	default:
		return fmt.Errorf("format %q is only supported for search-specs", "table")
	}
}

func renderIndexResult(w io.Writer, result *index.RebuildResult) {
	if result.DryRun {
		fmt.Fprintf(w, "dry run validated %d artifact(s), %d chunk(s), and %d edge(s)\n", result.ArtifactCount, result.ChunkCount, result.EdgeCount)
		fmt.Fprintf(w, "index path: %s\n", result.IndexPath)
		renderIndexSourceSummaries(w, result.Sources)
		fmt.Fprintln(w, "database write: skipped")
		return
	}
	fmt.Fprintf(w, "indexed %d artifact(s), %d chunk(s), and %d edge(s)\n", result.ArtifactCount, result.ChunkCount, result.EdgeCount)
	fmt.Fprintf(w, "database: %s\n", result.IndexPath)
	renderIndexSourceSummaries(w, result.Sources)
}

func renderIndexSourceSummaries(w io.Writer, sources []source.LoadSourceSummary) {
	for _, summary := range sources {
		fmt.Fprintf(w, "source: %s | %s | root: %s | items: %d", summary.Name, summary.Kind, summary.Path, summary.ItemCount)
		if summary.SpecCount > 0 {
			fmt.Fprintf(w, " | specs: %d", summary.SpecCount)
		}
		if summary.DocCount > 0 {
			fmt.Fprintf(w, " | docs: %d", summary.DocCount)
		}
		fmt.Fprintln(w)
	}
}

func renderStatusResult(w io.Writer, result *statusResult) {
	fmt.Fprintf(w, "config: %s\n", result.ConfigPath)
	fmt.Fprintf(w, "index path: %s\n", result.IndexPath)
	if result.IndexExists {
		fmt.Fprintln(w, "index: present")
	} else {
		fmt.Fprintln(w, "index: missing")
	}
	fmt.Fprintf(w, "indexed specs: %d\n", result.SpecCount)
	fmt.Fprintf(w, "indexed docs: %d\n", result.DocCount)
	fmt.Fprintf(w, "indexed chunks: %d\n", result.ChunkCount)
}

func renderVersionResult(w io.Writer, result *versionResult) {
	fmt.Fprintf(w, "version: %s\n", result.Version)
	fmt.Fprintf(w, "go version: %s\n", result.GoVersion)
	if result.Commit != "" {
		fmt.Fprintf(w, "commit: %s\n", result.Commit)
	}
	if result.BuildDate != "" {
		fmt.Fprintf(w, "build date: %s\n", result.BuildDate)
	}
}

func renderPreviewSourcesResult(w io.Writer, result *source.PreviewResult) {
	if len(result.Sources) == 0 {
		fmt.Fprintln(w, "no sources")
		return
	}

	for i, preview := range result.Sources {
		fmt.Fprintf(w, "source: %s | %s | root: %s | items: %d\n", preview.Name, preview.Kind, preview.Path, preview.ItemCount)
		if len(preview.Files) > 0 {
			fmt.Fprintf(w, "files: %s\n", strings.Join(preview.Files, ", "))
		}
		if len(preview.Include) > 0 {
			fmt.Fprintf(w, "include: %s\n", strings.Join(preview.Include, ", "))
		}
		if len(preview.Exclude) > 0 {
			fmt.Fprintf(w, "exclude: %s\n", strings.Join(preview.Exclude, ", "))
		}
		if len(preview.Items) == 0 {
			fmt.Fprintln(w, "no matching items")
		} else {
			for _, item := range preview.Items {
				fmt.Fprintf(w, "- %s | %s\n", item.ArtifactKind, item.Path)
			}
		}
		if i < len(result.Sources)-1 {
			fmt.Fprintln(w)
		}
	}
}

func renderSearchSpecsResult(w io.Writer, result *index.SearchSpecResult) {
	if len(result.Matches) == 0 {
		fmt.Fprintln(w, "no matches")
		return
	}

	for i, match := range result.Matches {
		fmt.Fprintf(w, "%d. %s | %s | %.3f\n", i+1, match.Ref, match.SectionHeading, match.Score)
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

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "REF\tTITLE\tSECTION\tSCORE")
	for _, match := range result.Matches {
		fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%.3f\n",
			renderTableValue(match.Ref, 12),
			renderTableValue(match.Title, 28),
			renderTableValue(match.SectionHeading, 36),
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

func renderOverlapResult(w io.Writer, result *analysis.OverlapResult) {
	fmt.Fprintf(w, "candidate: %s | %s\n", result.Candidate.Ref, result.Candidate.Title)
	if len(result.Overlaps) == 0 {
		fmt.Fprintln(w, "no overlaps")
		fmt.Fprintf(w, "recommendation: %s\n", result.Recommendation)
		return
	}
	for i, overlap := range result.Overlaps {
		fmt.Fprintf(w, "%d. %s | %s | %.3f | %s | %s\n", i+1, overlap.Ref, overlap.Title, overlap.Score, overlap.OverlapDegree, overlap.Relationship)
	}
	fmt.Fprintf(w, "recommendation: %s\n", result.Recommendation)
}

func renderCompareResult(w io.Writer, result *analysis.CompareResult) {
	fmt.Fprintf(w, "specs: %s\n", strings.Join(result.SpecRefs, ", "))
	fmt.Fprintf(w, "recommendation: %s\n", result.Comparison.Recommendation)
	for _, tradeoff := range result.Comparison.Tradeoffs {
		fmt.Fprintf(w, "%s: %s\n", tradeoff.Topic, tradeoff.Summary)
	}
}

func renderAnalyzeImpactResult(w io.Writer, result *analysis.AnalyzeImpactResult) {
	fmt.Fprintf(w, "spec: %s | change_type: %s\n", result.SpecRef, result.ChangeType)
	fmt.Fprintf(w, "affected specs: %d\n", len(result.AffectedSpecs))
	fmt.Fprintf(w, "affected refs: %d\n", len(result.AffectedRefs))
	fmt.Fprintf(w, "affected docs: %d\n", len(result.AffectedDocs))
}

func renderComplianceResult(w io.Writer, result *analysis.ComplianceResult) {
	fmt.Fprintf(w, "paths: %s\n", strings.Join(result.Paths, ", "))
	fmt.Fprintf(w, "relevant specs: %d\n", len(result.RelevantSpecs))
	renderComplianceFindingGroup(w, "conflicts", result.Conflicts)
	renderComplianceFindingGroup(w, "compliant", result.Compliant)
	renderComplianceFindingGroup(w, "unspecified", result.Unspecified)
}

func renderComplianceFindingGroup(w io.Writer, label string, findings []analysis.ComplianceFinding) {
	if len(findings) == 0 {
		fmt.Fprintf(w, "%s: none\n", label)
		return
	}

	fmt.Fprintf(w, "%s: %d\n", label, len(findings))
	for _, item := range findings {
		fmt.Fprintf(w, "- %s", item.Path)
		if item.SpecRef != "" {
			fmt.Fprintf(w, " | %s", item.SpecRef)
		}
		if item.SectionHeading != "" {
			fmt.Fprintf(w, " | %s", item.SectionHeading)
		}
		fmt.Fprintf(w, " | %s\n", item.Message)
	}
}

func renderDocDriftResult(w io.Writer, result *analysis.DocDriftResult) {
	if len(result.DriftItems) == 0 {
		fmt.Fprintln(w, "no drift items")
		return
	}
	for i, item := range result.DriftItems {
		fmt.Fprintf(w, "%d. %s | %s | findings: %d\n", i+1, item.DocRef, item.Title, len(item.Findings))
	}
}

func renderReviewResult(w io.Writer, result *analysis.ReviewResult) {
	fmt.Fprintf(w, "spec: %s\n", result.SpecRef)

	if result.Overlap != nil {
		fmt.Fprintf(w, "overlaps: %d | recommendation: %s\n", len(result.Overlap.Overlaps), result.Overlap.Recommendation)
		if len(result.Overlap.Overlaps) > 0 {
			top := result.Overlap.Overlaps[0]
			fmt.Fprintf(w, "top overlap: %s | %s | %.3f\n", top.Ref, top.Relationship, top.Score)
		}
	}
	if result.Comparison != nil {
		fmt.Fprintf(w, "comparison: %s\n", result.Comparison.Comparison.Recommendation)
	} else {
		fmt.Fprintln(w, "comparison: none")
	}
	if result.Impact != nil {
		fmt.Fprintf(w, "impact: %d spec(s), %d ref(s), %d doc(s)\n", len(result.Impact.AffectedSpecs), len(result.Impact.AffectedRefs), len(result.Impact.AffectedDocs))
	} else {
		fmt.Fprintln(w, "impact: none")
	}
	if result.DocDrift != nil {
		fmt.Fprintf(w, "doc drift: %d item(s)\n", len(result.DocDrift.DriftItems))
	} else {
		fmt.Fprintln(w, "doc drift: none")
	}
}
