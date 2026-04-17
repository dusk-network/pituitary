package cmd

import (
	"fmt"
	htemplate "html/template"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

func renderCommandResult(w io.Writer, command string, result any) error {
	description := commandDescription(command)
	if description == "" {
		return fmt.Errorf("unknown command %q", command)
	}

	if !usesSemanticTextRendering(command) {
		fmt.Fprintf(w, "pituitary %s: %s\n", command, description)
	}

	switch typed := result.(type) {
	case *source.CanonicalizeResult:
		renderCanonicalizeResult(w, typed)
	case *source.NewSpecBundleResult:
		renderNewSpecBundleResult(w, typed)
	case *source.DiscoverResult:
		renderDiscoverResult(w, typed)
	case *initResult:
		renderInitResult(w, typed)
	case *migrateConfigResult:
		renderMigrateConfigResult(w, typed)
	case *index.RebuildResult:
		renderIndexResult(w, typed)
	case *statusResult:
		renderStatusResult(w, typed)
	case *versionResult:
		renderVersionResult(w, typed)
	case *source.PreviewResult:
		renderPreviewSourcesResult(w, typed)
	case *source.ExplainFileResult:
		renderExplainFileResult(w, typed)
	case *index.SearchSpecResult:
		renderSearchSpecsResult(w, typed)
	case *analysis.OverlapResult:
		renderOverlapResult(w, typed)
	case *analysis.CompareResult:
		renderCompareResult(w, typed)
	case *analysis.AnalyzeImpactResult:
		renderAnalyzeImpactResult(w, typed)
	case *analysis.TerminologyAuditResult:
		renderTerminologyAuditResult(w, typed)
	case *analysis.ComplianceResult:
		renderComplianceResult(w, typed)
	case *analysis.DocDriftResult:
		renderDocDriftResult(w, typed)
	case *analysis.FreshnessResult:
		renderFreshnessResult(w, typed)
	case *app.FixResult:
		renderFixResult(w, typed)
	case *app.CompileResult:
		renderCompileResult(w, typed)
	case *analysis.ReviewResult:
		renderReviewResult(w, typed)
	case *schemaCatalogResult:
		renderSchemaCatalogResult(w, typed)
	case *schemaCommandResult:
		renderSchemaCommandResult(w, typed)
	default:
		return fmt.Errorf("unsupported result type %T", result)
	}

	return nil
}

func usesSemanticTextRendering(command string) bool {
	switch command {
	case "check-doc-drift", "check-overlap", "review-spec", "check-compliance", "check-spec-freshness", "status", "init", "fix", "compile":
		return true
	default:
		return false
	}
}

func renderCommandTable(w io.Writer, command string, result any) error {
	description := commandDescription(command)
	if description == "" {
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

func renderCommandMarkdown(w io.Writer, command string, result any) error {
	switch typed := result.(type) {
	case *analysis.ReviewResult:
		if command != "review-spec" {
			return fmt.Errorf("format %q is only supported for review-spec", "markdown")
		}
		renderReviewMarkdown(w, typed)
		return nil
	default:
		return fmt.Errorf("format %q is only supported for review-spec", "markdown")
	}
}

func renderCommandHTML(w io.Writer, command string, result any) error {
	switch typed := result.(type) {
	case *analysis.ReviewResult:
		if command != "review-spec" {
			return fmt.Errorf("format %q is only supported for review-spec", "html")
		}
		renderReviewHTML(w, typed)
		return nil
	default:
		return fmt.Errorf("format %q is only supported for review-spec", "html")
	}
}

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
	renderIndexReuseSummary(w, result)
	renderIndexRepoCoverage(w, result.Repos)
	renderIndexSourceSummaries(w, result.Sources)
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

func renderDiscoverResult(w io.Writer, result *source.DiscoverResult) {
	fmt.Fprintf(w, "workspace: %s\n", result.WorkspaceRoot)
	fmt.Fprintf(w, "config path: %s\n", result.ConfigPath)
	if result.WroteConfig {
		fmt.Fprintln(w, "config write: wrote local config")
	} else {
		fmt.Fprintln(w, "config write: skipped")
	}

	for _, discovered := range result.Sources {
		fmt.Fprintf(w, "source: %s | %s | root: %s | items: %d | confidence: %s\n", discovered.Name, discovered.Kind, discovered.Path, discovered.ItemCount, discovered.Confidence)
		for _, reason := range discovered.Rationale {
			fmt.Fprintf(w, "rationale: %s\n", reason)
		}
		for _, item := range discovered.Items {
			fmt.Fprintf(w, "- %s | %s\n", item.Path, item.Confidence)
		}
	}

	if result.Preview != nil {
		fmt.Fprintf(w, "preview sources: %d\n", len(result.Preview.Sources))
	}

	fmt.Fprintln(w, "generated config:")
	fmt.Fprint(w, result.Config)
}

func renderCanonicalizeResult(w io.Writer, result *source.CanonicalizeResult) {
	fmt.Fprintf(w, "workspace: %s\n", result.WorkspaceRoot)
	fmt.Fprintf(w, "source: %s\n", result.SourcePath)
	fmt.Fprintf(w, "bundle dir: %s\n", result.BundleDir)
	if result.WroteBundle {
		fmt.Fprintln(w, "bundle write: wrote generated bundle")
	} else {
		fmt.Fprintln(w, "bundle write: skipped")
	}
	fmt.Fprintf(w, "spec ref: %s\n", result.Spec.Ref)
	fmt.Fprintf(w, "title: %s\n", result.Spec.Title)
	if result.Spec.Inference != nil {
		fmt.Fprintf(w, "inference: %s (%.2f)\n", result.Spec.Inference.Level, result.Spec.Inference.Score)
	}
	fmt.Fprintf(w, "provenance: %s\n", result.Provenance.SourceRef)
	for _, file := range result.Files {
		fmt.Fprintf(w, "generated file: %s\n", file.Path)
		fmt.Fprint(w, file.Content)
		if !strings.HasSuffix(file.Content, "\n") {
			fmt.Fprintln(w)
		}
	}
}

func renderNewSpecBundleResult(w io.Writer, result *source.NewSpecBundleResult) {
	fmt.Fprintf(w, "workspace: %s\n", result.WorkspaceRoot)
	if result.ConfigPath != "" {
		fmt.Fprintf(w, "config path: %s\n", result.ConfigPath)
	}
	fmt.Fprintf(w, "spec root: %s\n", result.SpecRoot)
	fmt.Fprintf(w, "bundle dir: %s\n", result.BundleDir)
	if result.WroteBundle {
		fmt.Fprintln(w, "bundle write: wrote draft bundle")
	}
	fmt.Fprintf(w, "spec ref: %s\n", result.Spec.Ref)
	fmt.Fprintf(w, "title: %s\n", result.Spec.Title)
	fmt.Fprintf(w, "status: %s\n", result.Spec.Status)
	fmt.Fprintf(w, "domain: %s\n", result.Spec.Domain)
	for _, file := range result.Files {
		fmt.Fprintf(w, "generated file: %s\n", file.Path)
	}
}

func renderInitResult(w io.Writer, result *initResult) {
	p := presentationForWriter(w)
	fmt.Fprintln(w, p.headerLine("init", ""))
	fmt.Fprintln(w)

	sourceCount := 0
	if result.Discover != nil {
		sourceCount = len(result.Discover.Sources)
	}
	artifactCount := 0
	chunkCount := 0
	if result.Index != nil {
		artifactCount = result.Index.ArtifactCount
		chunkCount = result.Index.ChunkCount
	}
	specCount := 0
	docCount := 0
	freshness := "unknown"
	embedderProvider := ""
	if result.Status != nil {
		specCount = result.Status.SpecCount
		docCount = result.Status.DocCount
		if result.Status.Freshness != nil && result.Status.Freshness.State != "" {
			freshness = result.Status.Freshness.State
		}
		embedderProvider = result.Status.EmbedderProvider
	}
	fmt.Fprintf(
		w,
		"  %s sources  %s artifacts  %s chunks  %s  %s\n",
		p.bold(fmt.Sprintf("%d", sourceCount)),
		p.bold(fmt.Sprintf("%d", artifactCount)),
		p.bold(fmt.Sprintf("%d", chunkCount)),
		renderFreshnessLabel(p, freshness),
		p.dim(statusEmbedderSummary(embedderProvider)),
	)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s\n", p.dim("workspace:"), result.WorkspaceRoot)
	fmt.Fprintf(w, "  %s %s\n", p.dim("config:"), result.ConfigPath)
	fmt.Fprintf(w, "  %s %s\n", p.dim("action:"), result.ConfigAction)
	fmt.Fprintf(w, "  %s %d specs · %d docs\n", p.dim("index:"), specCount, docCount)
	if result.Status != nil {
		for _, guidance := range result.Status.Guidance {
			fmt.Fprintf(w, "  %s %s\n", p.arrow(), guidance)
		}
	}
	if result.ConfigAction == "preview" {
		fmt.Fprintf(w, "  %s %s\n", p.arrow(), "run `pituitary init` without --dry-run to write the config and build the index")
	} else if specCount > 0 && docCount > 0 {
		fmt.Fprintf(w, "\n  %s run `pituitary check-doc-drift --scope all` to see findings\n", p.arrow())
	}
}

func renderMigrateConfigResult(w io.Writer, result *migrateConfigResult) {
	fmt.Fprintf(w, "config path: %s\n", result.ConfigPath)
	fmt.Fprintf(w, "detected schema: %s\n", result.DetectedSchema)
	fmt.Fprintf(w, "target schema_version: %d\n", result.TargetSchemaVersion)
	if result.WroteConfig {
		fmt.Fprintln(w, "config write: wrote migrated config")
	} else {
		fmt.Fprintln(w, "config write: skipped")
	}
	for _, note := range result.Notes {
		fmt.Fprintf(w, "note: %s\n", note)
	}
	fmt.Fprintln(w, "migrated config:")
	fmt.Fprint(w, result.Config)
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
		fmt.Fprintf(w, "source: %s | %s | path: %s | items: %d\n", preview.Name, preview.Kind, preview.Path, preview.ItemCount)
		if preview.ResolvedPath != "" {
			fmt.Fprintf(w, "resolved path: %s\n", preview.ResolvedPath)
		}
		if len(preview.Files) > 0 {
			fmt.Fprintf(w, "files: %s\n", strings.Join(preview.Files, ", "))
		}
		if len(preview.Include) > 0 {
			fmt.Fprintf(w, "include: %s\n", strings.Join(preview.Include, ", "))
		}
		if len(preview.Exclude) > 0 {
			fmt.Fprintf(w, "exclude: %s\n", strings.Join(preview.Exclude, ", "))
		}
		if preview.CandidateCount > 0 {
			fmt.Fprintf(w, "candidate files: %d\n", preview.CandidateCount)
		}
		if len(preview.Items) == 0 {
			fmt.Fprint(w, "no matching items")
			if preview.CandidateCount > 0 {
				fmt.Fprint(w, " (candidate files were found under the source root, but the selectors excluded them)")
			}
			fmt.Fprintln(w)
		} else {
			for _, item := range preview.Items {
				fmt.Fprintf(w, "- %s | %s\n", item.ArtifactKind, item.Path)
				if len(item.FilesMatched) > 0 {
					fmt.Fprintf(w, "  files matched: %s\n", strings.Join(item.FilesMatched, ", "))
				}
				if len(item.IncludeMatches) > 0 {
					fmt.Fprintf(w, "  include matches: %s\n", strings.Join(item.IncludeMatches, ", "))
				}
			}
		}
		if len(preview.RejectedItems) > 0 {
			fmt.Fprintln(w, "rejected candidates:")
			for _, item := range preview.RejectedItems {
				fmt.Fprintf(w, "- %s | %s\n", item.Path, humanizePreviewRejectionReason(item.Reason))
				if len(item.FilesMatched) > 0 {
					fmt.Fprintf(w, "  files matched: %s\n", strings.Join(item.FilesMatched, ", "))
				}
				if len(item.IncludeMatches) > 0 {
					fmt.Fprintf(w, "  include matches: %s\n", strings.Join(item.IncludeMatches, ", "))
				}
				if len(item.ExcludeMatches) > 0 {
					fmt.Fprintf(w, "  exclude matches: %s\n", strings.Join(item.ExcludeMatches, ", "))
				}
			}
		}
		if i < len(result.Sources)-1 {
			fmt.Fprintln(w)
		}
	}
}

func humanizePreviewRejectionReason(reason string) string {
	switch reason {
	case "not_listed_in_files":
		return "not listed in files selectors"
	case "not_matched_by_include":
		return "not matched by include selectors"
	case "excluded_by_selector":
		return "excluded by exclude selectors"
	default:
		return reason
	}
}

func renderExplainFileResult(w io.Writer, result *source.ExplainFileResult) {
	fmt.Fprintf(w, "file: %s\n", result.AbsolutePath)
	if result.WorkspacePath != "" {
		fmt.Fprintf(w, "workspace path: %s\n", result.WorkspacePath)
	}
	fmt.Fprintf(w, "summary: %s\n", result.Summary.Status)
	if len(result.Summary.IndexedBy) > 0 {
		fmt.Fprintf(w, "indexed by: %s\n", strings.Join(result.Summary.IndexedBy, ", "))
	}

	for i, explanation := range result.Sources {
		fmt.Fprintf(w, "%d. %s | %s | root: %s | reason: %s\n", i+1, explanation.Name, explanation.Kind, explanation.Path, explanation.Reason)
		if explanation.UnderSourceRoot {
			fmt.Fprintf(w, "   relative path: %s\n", explanation.RelativePath)
			fmt.Fprintf(w, "   selected: %t\n", explanation.Selected)
		} else {
			fmt.Fprintln(w, "   outside source root")
		}
		if explanation.ArtifactKind != "" {
			fmt.Fprintf(w, "   artifact: %s\n", explanation.ArtifactKind)
		}
		if len(explanation.Files) > 0 {
			fmt.Fprintf(w, "   files: %s\n", strings.Join(explanation.Files, ", "))
		}
		if len(explanation.FilesMatched) > 0 {
			fmt.Fprintf(w, "   files matched: %s\n", strings.Join(explanation.FilesMatched, ", "))
		}
		if len(explanation.Include) > 0 {
			fmt.Fprintf(w, "   include: %s\n", strings.Join(explanation.Include, ", "))
		}
		if len(explanation.IncludeMatches) > 0 {
			fmt.Fprintf(w, "   include matches: %s\n", strings.Join(explanation.IncludeMatches, ", "))
		}
		if len(explanation.Exclude) > 0 {
			fmt.Fprintf(w, "   exclude: %s\n", strings.Join(explanation.Exclude, ", "))
		}
		if len(explanation.ExcludeMatches) > 0 {
			fmt.Fprintf(w, "   exclude matches: %s\n", strings.Join(explanation.ExcludeMatches, ", "))
		}
		if explanation.BundlePath != "" {
			fmt.Fprintf(w, "   bundle: %s\n", explanation.BundlePath)
		}
		if explanation.ConflictsWith != "" {
			fmt.Fprintf(w, "   conflicts with: %s\n", explanation.ConflictsWith)
		}
		if explanation.InferredSpec != nil {
			fmt.Fprintf(w, "   inferred ref: %s\n", explanation.InferredSpec.Ref)
			fmt.Fprintf(w, "   inferred title: %s\n", explanation.InferredSpec.Title)
			fmt.Fprintf(w, "   inferred status: %s\n", explanation.InferredSpec.Status)
			if explanation.InferredSpec.Domain != "" {
				fmt.Fprintf(w, "   inferred domain: %s\n", explanation.InferredSpec.Domain)
			}
			if len(explanation.InferredSpec.DependsOn) > 0 {
				fmt.Fprintf(w, "   inferred depends_on: %s\n", strings.Join(explanation.InferredSpec.DependsOn, ", "))
			}
			if len(explanation.InferredSpec.Supersedes) > 0 {
				fmt.Fprintf(w, "   inferred supersedes: %s\n", strings.Join(explanation.InferredSpec.Supersedes, ", "))
			}
			if len(explanation.InferredSpec.AppliesTo) > 0 {
				fmt.Fprintf(w, "   inferred applies_to: %s\n", strings.Join(explanation.InferredSpec.AppliesTo, ", "))
			}
			if explanation.InferredSpec.Inference != nil {
				fmt.Fprintf(w, "   inference: %s (%.2f)\n", explanation.InferredSpec.Inference.Level, explanation.InferredSpec.Inference.Score)
			}
		}
	}
}

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

func truncateRenderLine(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func renderTerminologyAuditResult(w io.Writer, result *analysis.TerminologyAuditResult) {
	fmt.Fprintf(w, "scope: %s\n", result.Scope.Mode)
	if len(result.Scope.ArtifactKinds) > 0 {
		fmt.Fprintf(w, "artifact kinds: %s\n", strings.Join(result.Scope.ArtifactKinds, ", "))
	}
	if result.Scope.SpecRef != "" {
		fmt.Fprintf(w, "anchor spec: %s\n", result.Scope.SpecRef)
	}
	fmt.Fprintf(w, "terms: %s\n", strings.Join(result.Terms, ", "))
	if len(result.CanonicalTerms) > 0 {
		fmt.Fprintf(w, "canonical terms: %s\n", strings.Join(result.CanonicalTerms, ", "))
	}
	if len(result.AnchorSpecs) > 0 {
		refs := make([]string, 0, len(result.AnchorSpecs))
		for _, anchor := range result.AnchorSpecs {
			refs = append(refs, anchor.Ref)
		}
		fmt.Fprintf(w, "evidence specs: %s\n", strings.Join(refs, ", "))
	}
	fmt.Fprintf(w, "findings: %d artifact(s)\n", len(result.Findings))
	if len(result.Tolerated) > 0 {
		fmt.Fprintf(w, "tolerated historical uses: %d artifact(s)\n", len(result.Tolerated))
	}
	if len(result.Findings) == 0 && len(result.Tolerated) == 0 {
		return
	}

	for i, finding := range result.Findings {
		renderTerminologyFinding(w, fmt.Sprintf("%d.", i+1), finding)
	}
	if len(result.Tolerated) == 0 {
		return
	}
	for i, finding := range result.Tolerated {
		renderTerminologyFinding(w, fmt.Sprintf("t%d.", i+1), finding)
	}
}

func renderTerminologyFinding(w io.Writer, label string, finding analysis.TerminologyFinding) {
	fmt.Fprintf(w, "%s %s | %s | %s | terms: %s\n", label, finding.Ref, finding.Kind, finding.Title, strings.Join(finding.Terms, ", "))
	if finding.SourceRef != "" {
		fmt.Fprintf(w, "   source: %s\n", finding.SourceRef)
	}
	for _, section := range finding.Sections {
		fmt.Fprintf(w, "   - %s | terms: %s\n", section.Section, strings.Join(section.Terms, ", "))
		if section.Excerpt != "" {
			fmt.Fprintf(w, "     excerpt: %s\n", section.Excerpt)
		}
		if section.Assessment != "" {
			fmt.Fprintf(w, "     assessment: %s\n", section.Assessment)
		}
		for _, match := range section.Matches {
			fmt.Fprintf(w, "     match: %s | class: %s | context: %s | severity: %s", match.Term, match.Classification, match.Context, match.Severity)
			if match.Tolerated {
				fmt.Fprint(w, " | tolerated")
			}
			if match.Replacement != "" {
				fmt.Fprintf(w, " | replace with: %s", match.Replacement)
			}
			fmt.Fprintln(w)
		}
		if section.Evidence != nil {
			fmt.Fprintf(w, "     evidence: %s | %s | %.3f\n", section.Evidence.SpecRef, section.Evidence.Section, section.Evidence.Score)
			if section.Evidence.Excerpt != "" {
				fmt.Fprintf(w, "     expected: %s\n", section.Evidence.Excerpt)
			}
		}
	}
}

func renderFixResult(w io.Writer, result *app.FixResult) {
	p := presentationForWriter(w)
	suffix := ""
	if strings.TrimSpace(result.Selector) != "" {
		suffix = " " + p.dim(result.Selector)
	}
	fmt.Fprintln(w, p.headerLine("fix", suffix))
	fmt.Fprintln(w)

	if len(result.Files) == 0 {
		fmt.Fprintf(w, "  %s no deterministic doc-drift edits available\n", p.info())
		for _, guidance := range result.Guidance {
			fmt.Fprintf(w, "  %s %s\n", p.arrow(), guidance)
		}
		return
	}

	for i, file := range result.Files {
		if i > 0 {
			fmt.Fprintln(w)
		}
		renderFixPromptFile(w, result.Selector, file)
		if file.Status == "applied" {
			fmt.Fprintf(w, "    %s applied %d edit%s\n", p.check(), len(file.Edits), pluralSuffix(len(file.Edits)))
		}
		if file.Reason != "" {
			fmt.Fprintf(w, "    %s %s\n", p.arrow(), file.Reason)
		}
		for _, warning := range file.Warnings {
			fmt.Fprintf(w, "    %s %s\n", p.arrow(), warning)
		}
	}
	if len(result.Guidance) > 0 {
		fmt.Fprintln(w)
		for _, guidance := range result.Guidance {
			fmt.Fprintf(w, "  %s %s\n", p.arrow(), guidance)
		}
	}
}

func renderCompileResult(w io.Writer, result *app.CompileResult) {
	p := presentationForWriter(w)
	suffix := ""
	if strings.TrimSpace(result.Scope) != "" {
		suffix = " " + p.dim(result.Scope)
	}
	fmt.Fprintln(w, p.headerLine("compile", suffix))
	fmt.Fprintln(w)

	if len(result.Files) == 0 {
		fmt.Fprintf(w, "  %s no actionable terminology edits found\n", p.info())
		for _, guidance := range result.Guidance {
			fmt.Fprintf(w, "  %s %s\n", p.arrow(), guidance)
		}
		return
	}

	for i, file := range result.Files {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "  %s\n\n", p.dim(file.Path))
		if len(file.Edits) == 0 {
			fmt.Fprintf(w, "    %s %s\n", p.info(), "no unambiguous terminology edits available")
			continue
		}
		for _, edit := range file.Edits {
			fmt.Fprintf(w, "    %s %s\n", p.red("-"), edit.Before)
			fmt.Fprintf(w, "    %s %s\n", p.green("+"), edit.After)
			fmt.Fprintln(w)
		}
		if file.Status == "applied" {
			fmt.Fprintf(w, "    %s applied %d edit%s\n", p.check(), len(file.Edits), pluralSuffix(len(file.Edits)))
		}
		if file.Reason != "" {
			fmt.Fprintf(w, "    %s %s\n", p.arrow(), file.Reason)
		}
		for _, warning := range file.Warnings {
			fmt.Fprintf(w, "    %s %s\n", p.arrow(), warning)
		}
	}
	if len(result.Guidance) > 0 {
		fmt.Fprintln(w)
		for _, guidance := range result.Guidance {
			fmt.Fprintf(w, "  %s %s\n", p.arrow(), guidance)
		}
	}
}

func renderFixPromptFile(w io.Writer, selector string, file app.FixFileResult) {
	p := presentationForWriter(w)
	fmt.Fprintf(w, "  %s\n\n", p.dim(file.Path))
	if len(file.Edits) == 0 {
		fmt.Fprintf(w, "    %s %s\n", p.info(), "no deterministic replace-claim edits available")
		return
	}
	for _, edit := range file.Edits {
		fmt.Fprintf(w, "    %s %s\n", p.red("-"), edit.Before)
		fmt.Fprintf(w, "    %s %s\n", p.green("+"), edit.After)
		if edit.Summary != "" {
			fmt.Fprintf(w, "      %s\n", p.dim(edit.Summary))
		}
		fmt.Fprintln(w)
	}
}

func renderReviewResult(w io.Writer, result *analysis.ReviewResult) {
	p := presentationForWriter(w)
	headerSuffix := " · " + p.cyan(result.SpecRef)
	fmt.Fprintln(w, p.headerLine("review-spec", headerSuffix))
	if result.Overlap != nil && strings.TrimSpace(result.Overlap.Candidate.Title) != "" {
		fmt.Fprintf(w, "    %s\n", p.dim(result.Overlap.Candidate.Title))
	}
	fmt.Fprintln(w)

	if result.Overlap != nil {
		fmt.Fprintf(w, "  %s   %d specs · recommendation: %s\n", p.white("OVERLAP"), len(result.Overlap.Overlaps), result.Overlap.Recommendation)
		if len(result.Overlap.Overlaps) == 0 {
			fmt.Fprintf(w, "  %s %s\n", p.treeLast(), p.dim("no overlapping specs detected"))
		} else {
			for i, overlap := range result.Overlap.Overlaps {
				fmt.Fprintf(w, "  %s %s  %s  %s\n", p.treeBranch(i == len(result.Overlap.Overlaps)-1), p.cyan(overlap.Ref), fmt.Sprintf("%.3f", overlap.Score), overlap.Relationship)
			}
		}
		fmt.Fprintln(w)
	}

	if result.Impact != nil {
		fmt.Fprintf(w, "  %s    %d specs · %d refs · %d docs\n", p.white("IMPACT"), len(result.Impact.AffectedSpecs), len(result.Impact.AffectedRefs), len(result.Impact.AffectedDocs))
		items := reviewImpactLines(result.Impact)
		for i, item := range items {
			fmt.Fprintf(w, "  %s %s\n", p.treeBranch(i == len(items)-1), item)
		}
		fmt.Fprintln(w)
	}

	driftAssessments := reviewDocDriftAssessments(result.DocDrift)
	fmt.Fprintf(w, "  %s %d item%s · %d remediation%s\n", p.white("DOC DRIFT"), len(driftAssessments), pluralSuffix(len(driftAssessments)), reviewRemediationSuggestionCount(result.DocRemediation), pluralSuffix(reviewRemediationSuggestionCount(result.DocRemediation)))
	if len(driftAssessments) == 0 {
		fmt.Fprintf(w, "  %s %s\n", p.treeLast(), p.dim("no drifting docs detected"))
	} else {
		for i, assessment := range driftAssessments {
			fmt.Fprintf(w, "  %s %s  %s\n", p.treeBranch(i == len(driftAssessments)-1), p.cyan(repoPathLabel(assessment.Repo, preferredDocLabel(assessment.DocRef, assessment.SourceRef))), driftAssessmentBadge(p, assessment.Status))
			if suggestions := remediationItemsByDocRef(result.DocRemediation)[assessment.DocRef]; len(suggestions) > 0 {
				fmt.Fprintf(w, "     %s %d suggested edits %s\n", p.arrow(), len(suggestions), p.dim("(see check-doc-drift for detail)"))
			}
		}
	}
	fmt.Fprintln(w)

	if result.Comparison != nil {
		fmt.Fprintf(w, "  %s  prefer %s as the primary reference\n", p.white("COMPARISON"), p.cyan(result.SpecRef))
	} else {
		fmt.Fprintf(w, "  %s  %s\n", p.white("COMPARISON"), p.dim("none"))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s  run review-spec --format html for the full evidence report\n", p.info())
}

func renderReviewMarkdown(w io.Writer, result *analysis.ReviewResult) {
	renderReviewMarkdownSpecSection(w, result)
	renderReviewMarkdownSummarySection(w, result)
	renderReviewMarkdownActionsSection(w, result)
	renderReviewMarkdownOverlapSection(w, result)
	renderReviewMarkdownComparisonSection(w, result)
	renderReviewMarkdownImpactSection(w, result)
	renderReviewMarkdownDocDriftSection(w, result)
	renderReviewMarkdownDocRemediationSection(w, result)
}

func renderReviewHTML(w io.Writer, result *analysis.ReviewResult) {
	escape := htemplate.HTMLEscapeString
	driftAssessments := reviewDocDriftAssessments(result.DocDrift)

	renderReviewHTMLDocumentStart(w, result, escape)
	renderReviewHTMLHeroSection(w, result, escape)
	renderReviewHTMLSummaryGrid(w, result, escape)
	renderReviewHTMLStatsSection(w, result, driftAssessments, escape)
	renderReviewHTMLOverlapSection(w, result, escape)
	renderReviewHTMLComparisonSection(w, result, escape)
	renderReviewHTMLImpactSection(w, result, escape)
	renderReviewHTMLDocDriftSection(w, result, driftAssessments, escape)
	renderReviewHTMLDocRemediationSection(w, result, escape)
	renderReviewHTMLWarningsSection(w, result, escape)
	renderReviewHTMLDocumentEnd(w)
}

func reviewHTMLDriftEvidence(evidence *analysis.DriftEvidence) string {
	if evidence == nil {
		return ""
	}
	escape := htemplate.HTMLEscapeString
	parts := make([]string, 0, 4)
	if evidence.SpecRef != "" {
		spec := "<strong>Spec</strong> " + escape(evidence.SpecRef)
		if evidence.SpecSection != "" {
			spec += " | " + escape(evidence.SpecSection)
		}
		if evidence.SpecExcerpt != "" {
			spec += "<br><span class=\"subtle\">" + escape(evidence.SpecExcerpt) + "</span>"
		}
		parts = append(parts, spec)
	}
	if evidence.DocSection != "" || evidence.DocExcerpt != "" {
		doc := "<strong>Doc</strong> "
		if evidence.DocSection != "" {
			doc += escape(evidence.DocSection)
		} else {
			doc += "matching section"
		}
		if evidence.DocExcerpt != "" {
			doc += "<br><span class=\"subtle\">" + escape(evidence.DocExcerpt) + "</span>"
		}
		parts = append(parts, doc)
	}
	return strings.Join(parts, "<br>")
}

func reviewHTMLRemediationEvidence(evidence analysis.DocRemediationEvidence) string {
	escape := htemplate.HTMLEscapeString
	parts := make([]string, 0, 3)
	if evidence.SpecSection != "" || evidence.SpecExcerpt != "" {
		spec := "<strong>Spec</strong> "
		if evidence.SpecSection != "" {
			spec += escape(evidence.SpecSection)
		} else {
			spec += "matched section"
		}
		if evidence.SpecSourceRef != "" {
			spec += "<br><span class=\"subtle\">" + escape(evidence.SpecSourceRef) + "</span>"
		}
		if evidence.SpecExcerpt != "" {
			spec += "<br><span class=\"subtle\">" + escape(evidence.SpecExcerpt) + "</span>"
		}
		parts = append(parts, spec)
	}
	if evidence.DocSection != "" || evidence.DocExcerpt != "" {
		doc := "<strong>Doc</strong> "
		if evidence.DocSection != "" {
			doc += escape(evidence.DocSection)
		} else {
			doc += "matched section"
		}
		if evidence.DocSourceRef != "" {
			doc += "<br><span class=\"subtle\">" + escape(evidence.DocSourceRef) + "</span>"
		}
		if evidence.DocExcerpt != "" {
			doc += "<br><span class=\"subtle\">" + escape(evidence.DocExcerpt) + "</span>"
		}
		parts = append(parts, doc)
	}
	if evidence.LinkReason != "" {
		parts = append(parts, "<strong>Link</strong><br><span class=\"subtle\">"+escape(evidence.LinkReason)+"</span>")
	}
	return strings.Join(parts, "<br>")
}

func reviewMarkdownSummary(result *analysis.ReviewResult) []string {
	lines := []string{fmt.Sprintf("Spec under review: `%s`.", result.SpecRef)}
	if result.Overlap == nil {
		lines = append(lines, "Overlap posture: no overlap analysis generated.")
	} else if len(result.Overlap.Overlaps) == 0 {
		lines = append(lines, "Overlap posture: no competing accepted spec was shortlisted.")
	} else {
		primary := result.Overlap.Overlaps[0]
		posture := result.Overlap.Recommendation
		if detail := humanizeOverlapRecommendation(result.Overlap.Recommendation); detail != "" {
			posture += " (" + detail + ")"
		}
		lines = append(lines, fmt.Sprintf("Overlap posture: `%s`; closest neighbor is `%s` %s at %.3f.", posture, primary.Ref, primary.Title, primary.Score))
	}
	if result.Comparison == nil {
		lines = append(lines, "Comparison posture: no primary comparison target was generated.")
	} else {
		lines = append(lines, fmt.Sprintf("Comparison posture: `%s`.", result.Comparison.Comparison.Recommendation))
	}
	if result.Impact == nil {
		lines = append(lines, "Impact footprint: no impact analysis generated.")
	} else {
		lines = append(lines, fmt.Sprintf("Impact footprint: %d impacted spec(s), %d governed ref(s), %d impacted doc(s).", len(result.Impact.AffectedSpecs), len(result.Impact.AffectedRefs), len(result.Impact.AffectedDocs)))
	}
	driftAssessments := reviewDocDriftAssessments(result.DocDrift)
	switch {
	case len(driftAssessments) == 0:
		lines = append(lines, "Documentation posture: no drift follow-up identified.")
	case reviewRemediationSuggestionCount(result.DocRemediation) > 0:
		lines = append(lines, fmt.Sprintf("Documentation posture: %d doc(s) need follow-up with %d suggested remediation edit(s).", len(driftAssessments), reviewRemediationSuggestionCount(result.DocRemediation)))
	default:
		lines = append(lines, fmt.Sprintf("Documentation posture: %d doc(s) need follow-up.", len(driftAssessments)))
	}
	if len(result.Warnings) > 0 {
		lines = append(lines, fmt.Sprintf("Warnings: %d warning(s) require manual judgment.", len(result.Warnings)))
	}
	return lines
}

func reviewMarkdownActions(result *analysis.ReviewResult) []string {
	actions := make([]string, 0, 4)
	if result.Overlap != nil && len(result.Overlap.Overlaps) > 0 {
		primary := result.Overlap.Overlaps[0]
		switch result.Overlap.Recommendation {
		case "proceed_with_supersedes":
			actions = append(actions, fmt.Sprintf("Proceed with the supersedes path against `%s`, and keep the replacement scope explicit in the spec body.", primary.Ref))
		case "merge_into_existing":
			actions = append(actions, fmt.Sprintf("Treat `%s` as the primary merge target before accepting further downstream changes.", primary.Ref))
		case "review_boundaries":
			actions = append(actions, fmt.Sprintf("Review the boundary with `%s` before accepting wording or scope changes.", primary.Ref))
		}
	}
	if result.Impact != nil && len(result.Impact.AffectedSpecs) > 0 {
		refs := make([]string, 0, minInt(len(result.Impact.AffectedSpecs), 3))
		for _, item := range topImpactedSpecs(result.Impact.AffectedSpecs, 3) {
			refs = append(refs, "`"+item.Ref+"`")
		}
		actions = append(actions, fmt.Sprintf("Review downstream spec impact first: %s.", strings.Join(refs, ", ")))
	}
	driftAssessments := reviewDocDriftAssessments(result.DocDrift)
	if len(driftAssessments) > 0 {
		docRefs := make([]string, 0, minInt(len(driftAssessments), 3))
		for _, item := range driftAssessments[:minInt(len(driftAssessments), 3)] {
			docRefs = append(docRefs, "`"+item.DocRef+"`")
		}
		actions = append(actions, fmt.Sprintf("Update documentation that still needs follow-up: %s.", strings.Join(docRefs, ", ")))
	}
	if count := reviewRemediationSuggestionCount(result.DocRemediation); count > 0 {
		actions = append(actions, fmt.Sprintf("Apply or adapt the %d suggested remediation edit(s) before treating the review as complete.", count))
	}
	return actions
}

func renderFreshnessLabel(p renderPresentation, state string) string {
	switch state {
	case "fresh":
		return p.green(state)
	case "stale", "incompatible":
		return p.red(state)
	default:
		return p.yellow(state)
	}
}

func statusEmbedderSummary(provider string) string {
	if strings.TrimSpace(provider) == "" {
		return "embedder unknown"
	}
	return provider + " embedder"
}

func preferredDocLabel(docRef, sourceRef string) string {
	if strings.TrimSpace(sourceRef) != "" {
		return displaySourcePath(sourceRef)
	}
	return docRef
}

func displaySourcePath(sourceRef string) string {
	sourceRef = strings.TrimSpace(sourceRef)
	if sourceRef == "" {
		return ""
	}
	return strings.TrimPrefix(sourceRef, "file://")
}

func displayDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func repoPathLabel(repo, label string) string {
	repo = strings.TrimSpace(repo)
	label = strings.TrimSpace(label)
	switch {
	case repo == "":
		return label
	case label == "":
		return repo
	default:
		return fmt.Sprintf("[%s] %s", repo, label)
	}
}

func isNonPrimaryRepoDoc(docRef, repo string) bool {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(docRef), "doc://"+repo+"/")
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

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func humanizeSymbol(value string) string {
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	return value
}

func (p renderPresentation) treeBranch(last bool) string {
	if last {
		return p.treeLast()
	}
	return p.treeMid()
}

func (p renderPresentation) treeItem(last bool) string {
	return p.treeBranch(last)
}

func renderReviewMarkdownDriftEvidence(w io.Writer, evidence *analysis.DriftEvidence) {
	if evidence == nil {
		return
	}
	if evidence.SpecRef != "" || evidence.SpecSection != "" || evidence.SpecExcerpt != "" {
		fmt.Fprintf(w, "  - Spec evidence: `%s`", evidence.SpecRef)
		if evidence.SpecSection != "" {
			fmt.Fprintf(w, " | %s", evidence.SpecSection)
		}
		fmt.Fprintln(w)
		if evidence.SpecExcerpt != "" {
			fmt.Fprintf(w, "    - %s\n", evidence.SpecExcerpt)
		}
	}
	if evidence.DocSection != "" || evidence.DocExcerpt != "" {
		docSection := evidence.DocSection
		if docSection == "" {
			docSection = "matching section"
		}
		fmt.Fprintf(w, "  - Doc evidence: %s\n", docSection)
		if evidence.DocExcerpt != "" {
			fmt.Fprintf(w, "    - %s\n", evidence.DocExcerpt)
		}
	}
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

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

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
