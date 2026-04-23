package cmd

import (
	"fmt"
	"io"

	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

func renderIndexResult(w io.Writer, result *index.RebuildResult) {
	if result.DryRun {
		fmt.Fprintf(w, "dry run validated %d artifact(s), %d chunk(s), and %d edge(s)\n", result.ArtifactCount, result.ChunkCount, result.EdgeCount)
		fmt.Fprintf(w, "index path: %s\n", result.IndexPath)
		renderIndexReuseSummary(w, result)
		renderIndexRepoCoverage(w, result.Repos)
		renderIndexSourceSummaries(w, result.Sources)
		fmt.Fprintln(w, "database write: skipped")
		return
	}
	if result.Update {
		fmt.Fprintf(w, "updated %d artifact(s): %d added, %d updated, %d removed, %d unchanged\n",
			result.ArtifactCount, result.AddedCount, result.UpdatedCount, result.RemovedCount, result.UnchangedCount)
		fmt.Fprintf(w, "chunks: %d total, %d reused, %d embedded\n", result.ChunkCount, result.ReusedChunkCount, result.EmbeddedChunkCount)
		fmt.Fprintf(w, "edges: %d\n", result.EdgeCount)
		fmt.Fprintf(w, "database: %s\n", result.IndexPath)
		renderIndexRepoCoverage(w, result.Repos)
		renderIndexSourceSummaries(w, result.Sources)
		renderGovernanceDelta(w, result.Delta)
		return
	}
	fmt.Fprintf(w, "indexed %d artifact(s), %d chunk(s), and %d edge(s)\n", result.ArtifactCount, result.ChunkCount, result.EdgeCount)
	fmt.Fprintf(w, "database: %s\n", result.IndexPath)
	renderIndexInferenceSummary(w, result)
	renderIndexReuseSummary(w, result)
	renderIndexRepoCoverage(w, result.Repos)
	renderIndexSourceSummaries(w, result.Sources)
}

func renderIndexInferenceSummary(w io.Writer, result *index.RebuildResult) {
	if result.InferAppliesToEnabled {
		fmt.Fprintf(w, "inference: enabled (%d inferred edge(s))\n", result.InferredEdgeCount)
		return
	}
	fmt.Fprintln(w, "inference: disabled (set workspace.infer_applies_to = true to enable)")
}

func renderIndexReuseSummary(w io.Writer, result *index.RebuildResult) {
	if result.FullRebuild {
		fmt.Fprintln(w, "rebuild mode: full")
	} else {
		fmt.Fprintln(w, "rebuild mode: incremental")
	}
	fmt.Fprintf(w, "reused artifacts: %d\n", result.ReusedArtifactCount)
	fmt.Fprintf(w, "reused chunks: %d\n", result.ReusedChunkCount)
	fmt.Fprintf(w, "embedded chunks: %d\n", result.EmbeddedChunkCount)
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
		if summary.Repo != "" {
			fmt.Fprintf(w, " | repo: %s", summary.Repo)
		}
		fmt.Fprintln(w)
	}
}

func renderIndexRepoCoverage(w io.Writer, repos []index.RepoCoverage) {
	for _, repo := range repos {
		fmt.Fprintf(w, "repo: %s\n", renderRepoCoverageLine(repo))
	}
}

func renderRepoCoverageLine(repo index.RepoCoverage) string {
	line := fmt.Sprintf("%s | items: %d", repo.Repo, repo.ItemCount)
	if repo.SpecCount > 0 {
		line += fmt.Sprintf(" | specs: %d", repo.SpecCount)
	}
	if repo.DocCount > 0 {
		line += fmt.Sprintf(" | docs: %d", repo.DocCount)
	}
	return line
}

func renderGovernanceDelta(w io.Writer, delta *index.GovernanceDelta) {
	if delta == nil {
		return
	}
	fmt.Fprintf(w, "\nGovernance delta since last rebuild:\n")
	for _, s := range delta.AddedSpecs {
		line := fmt.Sprintf("  + %s added", s.Ref)
		if s.Status != "" {
			line += fmt.Sprintf(" (status: %s", s.Status)
			if s.Domain != "" {
				line += fmt.Sprintf(", domain: %s", s.Domain)
			}
			line += ")"
		}
		fmt.Fprintln(w, line)
	}
	for _, s := range delta.RemovedSpecs {
		fmt.Fprintf(w, "  - %s removed\n", s.Ref)
	}
	for _, s := range delta.UpdatedSpecs {
		line := fmt.Sprintf("  ~ %s updated", s.Ref)
		if s.Status != "" {
			line += fmt.Sprintf(" (status: %s)", s.Status)
		}
		fmt.Fprintln(w, line)
	}
	for _, e := range delta.AddedEdges {
		fmt.Fprintf(w, "  + %s %s %s (%s)\n", e.FromRef, e.EdgeType, e.ToRef, e.EdgeSource)
	}
	for _, e := range delta.RemovedEdges {
		fmt.Fprintf(w, "  - %s %s %s (%s)\n", e.FromRef, e.EdgeType, e.ToRef, e.EdgeSource)
	}
	for _, e := range delta.UpdatedEdges {
		fmt.Fprintf(w, "  ~ %s %s %s (%s → %s)\n", e.FromRef, e.EdgeType, e.ToRef, e.EdgeSource, e.Confidence)
	}
	fmt.Fprintf(w, "  summary: %s\n", delta.Summary)
}
