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
	case *app.FixResult:
		renderFixResult(w, typed)
	case *analysis.ReviewResult:
		renderReviewResult(w, typed)
	default:
		return fmt.Errorf("unsupported result type %T", result)
	}

	return nil
}

func usesSemanticTextRendering(command string) bool {
	switch command {
	case "check-doc-drift", "check-overlap", "review-spec", "check-compliance", "status", "init", "fix":
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
		renderIndexSourceSummaries(w, result.Sources)
		fmt.Fprintln(w, "database write: skipped")
		return
	}
	fmt.Fprintf(w, "indexed %d artifact(s), %d chunk(s), and %d edge(s)\n", result.ArtifactCount, result.ChunkCount, result.EdgeCount)
	fmt.Fprintf(w, "database: %s\n", result.IndexPath)
	renderIndexReuseSummary(w, result)
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
		fmt.Fprintln(w)
	}
}

func renderStatusResult(w io.Writer, result *statusResult) {
	p := presentationForWriter(w)
	fmt.Fprintln(w, p.headerLine("status", ""))
	fmt.Fprintln(w)

	freshnessState := "unknown"
	if result.Freshness != nil && result.Freshness.State != "" {
		freshnessState = result.Freshness.State
	}
	fmt.Fprintf(
		w,
		"  %s specs  %s docs  %s chunks  %s  %s\n",
		p.bold(fmt.Sprintf("%d", result.SpecCount)),
		p.bold(fmt.Sprintf("%d", result.DocCount)),
		p.bold(fmt.Sprintf("%d", result.ChunkCount)),
		renderFreshnessLabel(p, freshnessState),
		p.dim(statusEmbedderSummary(result.EmbedderProvider)),
	)
	fmt.Fprintln(w)

	if result.WorkspaceRoot != "" {
		fmt.Fprintf(w, "  %s %s\n", p.dim("workspace:"), result.WorkspaceRoot)
	}
	fmt.Fprintf(w, "  %s %s\n", p.dim("config:"), result.ConfigPath)
	if result.ConfigResolution != nil {
		if result.ConfigResolution.Reason != "" {
			fmt.Fprintf(w, "  %s %s\n", p.dim("config resolution:"), result.ConfigResolution.Reason)
		}
		if len(result.ConfigResolution.Candidates) > 0 {
			fmt.Fprintf(w, "  %s\n", p.white("CONFIG CANDIDATES"))
			for _, candidate := range result.ConfigResolution.Candidates {
				fmt.Fprintf(w, "  %s %d. %s | %s", p.treeItem(candidate.Precedence == len(result.ConfigResolution.Candidates)), candidate.Precedence, configSourceLabel(candidate.Source), candidate.Status)
				if candidate.Path != "" {
					fmt.Fprintf(w, " | %s", candidate.Path)
				}
				fmt.Fprintln(w)
				if candidate.Detail != "" {
					fmt.Fprintf(w, "     %s\n", p.dim(candidate.Detail))
				}
			}
		}
	}
	fmt.Fprintf(w, "  %s %s\n", p.dim("index path:"), result.IndexPath)
	if result.IndexExists {
		fmt.Fprintf(w, "  %s %s\n", p.dim("index:"), p.green("present"))
	} else {
		fmt.Fprintf(w, "  %s %s\n", p.dim("index:"), p.red("missing"))
	}
	if result.Freshness != nil {
		fmt.Fprintf(w, "  %s %s\n", p.dim("index freshness:"), renderFreshnessLabel(p, result.Freshness.State))
		for _, issue := range result.Freshness.Issues {
			fmt.Fprintf(w, "  %s %s\n", p.cross(), issue.Message)
		}
		if result.Freshness.Action != "" {
			fmt.Fprintf(w, "  %s %s\n", p.arrow(), result.Freshness.Action)
		}
	}
	if result.RelationGraph != nil {
		fmt.Fprintf(w, "  %s %s\n", p.dim("relation graph:"), result.RelationGraph.State)
		for _, finding := range result.RelationGraph.Findings {
			fmt.Fprintf(w, "  %s %s\n", p.cross(), finding.Message)
		}
	}
	if result.ArtifactLocations != nil {
		fmt.Fprintf(w, "  %s %s\n", p.dim("artifact index dir:"), result.ArtifactLocations.IndexDir)
		fmt.Fprintf(w, "  %s %s\n", p.dim("artifact discover --write default:"), result.ArtifactLocations.DiscoverConfigPath)
		fmt.Fprintf(w, "  %s %s\n", p.dim("artifact canonicalize default:"), result.ArtifactLocations.CanonicalizeBundleRoot)
		if len(result.ArtifactLocations.IgnorePatterns) > 0 {
			fmt.Fprintf(w, "  %s %s\n", p.dim("artifact ignore patterns:"), strings.Join(result.ArtifactLocations.IgnorePatterns, ", "))
		}
		for _, hint := range result.ArtifactLocations.RelocationHints {
			fmt.Fprintf(w, "  %s %s\n", p.arrow(), hint)
		}
	}
	if result.Runtime != nil {
		fmt.Fprintf(w, "  %s %s\n", p.dim("runtime probe:"), result.Runtime.Scope)
		for _, check := range result.Runtime.Checks {
			fmt.Fprintf(w, "  %s %s %s | %s | provider: %s", p.dim("runtime:"), runtimeCheckGlyph(p, check.Status), check.Name, check.Status, check.Provider)
			if check.Model != "" {
				fmt.Fprintf(w, " | model: %s", check.Model)
			}
			if check.Endpoint != "" {
				fmt.Fprintf(w, " | endpoint: %s", check.Endpoint)
			}
			fmt.Fprintln(w)
			if check.Message != "" {
				fmt.Fprintf(w, "     %s %s\n", p.dim("runtime note:"), check.Message)
			}
		}
	}
	for _, guidance := range result.Guidance {
		fmt.Fprintf(w, "  %s %s\n", p.arrow(), guidance)
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

func renderAnalyzeImpactResult(w io.Writer, result *analysis.AnalyzeImpactResult) {
	fmt.Fprintf(w, "spec: %s | change_type: %s\n", result.SpecRef, result.ChangeType)
	fmt.Fprintf(w, "affected specs: %d\n", len(result.AffectedSpecs))
	fmt.Fprintf(w, "affected refs: %d\n", len(result.AffectedRefs))
	fmt.Fprintf(w, "affected docs: %d\n", len(result.AffectedDocs))
}

func renderComplianceResult(w io.Writer, result *analysis.ComplianceResult) {
	p := presentationForWriter(w)
	fmt.Fprintln(w, p.headerLine("check-compliance", ""))
	fmt.Fprintln(w)
	if len(result.Paths) > 0 {
		fmt.Fprintf(w, "  %s %s\n", p.dim("paths:"), strings.Join(result.Paths, ", "))
	}
	fmt.Fprintf(w, "  %s %d\n", p.dim("relevant specs:"), len(result.RelevantSpecs))
	fmt.Fprintln(w)
	renderComplianceFindingGroup(w, "conflicts", result.Conflicts)
	renderComplianceFindingGroup(w, "compliant", result.Compliant)
	renderComplianceFindingGroup(w, "unspecified", result.Unspecified)
}

func renderComplianceFindingGroup(w io.Writer, label string, findings []analysis.ComplianceFinding) {
	p := presentationForWriter(w)
	if len(findings) == 0 {
		fmt.Fprintf(w, "  %s none\n", p.dim(label+":"))
		return
	}

	fmt.Fprintf(w, "  %s %d\n", p.white(strings.ToUpper(label)+":"), len(findings))
	for _, item := range findings {
		fmt.Fprintf(w, "  %s %s", complianceBadge(p, label), item.Path)
		if item.SpecRef != "" {
			fmt.Fprintf(w, " | %s", p.cyan(item.SpecRef))
		}
		if item.SectionHeading != "" {
			fmt.Fprintf(w, " | %s", item.SectionHeading)
		}
		fmt.Fprintf(w, " | %s\n", item.Message)
		if item.Traceability != "" {
			fmt.Fprintf(w, "     %s %s\n", p.dim("traceability"), item.Traceability)
		}
		if item.LimitingFactor != "" {
			fmt.Fprintf(w, "     %s %s\n", p.dim("limiting factor"), item.LimitingFactor)
		}
		if item.Suggestion != "" {
			fmt.Fprintf(w, "     %s %s\n", p.arrow(), item.Suggestion)
		}
	}
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
	if len(result.Findings) == 0 {
		return
	}

	for i, finding := range result.Findings {
		fmt.Fprintf(w, "%d. %s | %s | %s | terms: %s\n", i+1, finding.Ref, finding.Kind, finding.Title, strings.Join(finding.Terms, ", "))
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
			if section.Evidence != nil {
				fmt.Fprintf(w, "     evidence: %s | %s | %.3f\n", section.Evidence.SpecRef, section.Evidence.Section, section.Evidence.Score)
				if section.Evidence.Excerpt != "" {
					fmt.Fprintf(w, "     expected: %s\n", section.Evidence.Excerpt)
				}
			}
		}
	}
}

func renderDocDriftResult(w io.Writer, result *analysis.DocDriftResult) {
	p := presentationForWriter(w)
	fmt.Fprintln(w, p.headerLine("check-doc-drift", ""))
	fmt.Fprintln(w)

	if len(result.DriftItems) == 0 && len(result.Assessments) == 0 {
		fmt.Fprintf(w, "  %s no drift items\n", p.check())
		return
	}

	assessments := result.Assessments
	if len(assessments) == 0 {
		assessments = driftAssessmentsFromItems(result.DriftItems)
	}
	driftItems := driftItemsByDocRef(result.DriftItems)
	remediation := remediationItemsByDocRef(result.Remediation)
	for i, assessment := range assessments {
		if i > 0 {
			fmt.Fprintln(w)
		}
		docLabel := preferredDocLabel(assessment.DocRef, assessment.SourceRef)
		fmt.Fprintf(w, "  %s", p.cyan(docLabel))
		if padding := docDriftPadding(docLabel); padding > 0 {
			fmt.Fprint(w, strings.Repeat(" ", padding))
		} else {
			fmt.Fprint(w, "  ")
		}
		fmt.Fprintln(w, driftAssessmentBadge(p, assessment.Status))
		if item, ok := driftItems[assessment.DocRef]; ok && len(item.Findings) > 0 {
			for _, finding := range item.Findings {
				fmt.Fprintf(w, "\n    %s %s", p.cross(), p.bold(driftFindingSummary(finding)))
				if finding.Expected != "" {
					fmt.Fprintf(w, "  %s %s", p.yellow("expected"), finding.Expected)
				}
				if finding.Observed != "" {
					fmt.Fprintf(w, "  %s %s", p.yellow("got"), finding.Observed)
				}
				fmt.Fprintln(w)
			}
		} else if assessment.Rationale != "" {
			fmt.Fprintf(w, "\n    %s %s\n", p.arrow(), assessment.Rationale)
		}
		if suggestions := remediation[assessment.DocRef]; len(suggestions) > 0 {
			pathArg := docLabel
			if assessment.SourceRef != "" {
				pathArg = assessment.SourceRef
			}
			fmt.Fprintf(w, "\n    %s pituitary fix --path %s %s\n", p.green("fix:"), pathArg, p.dim(fmt.Sprintf("(%d edits)", len(suggestions))))
			fmt.Fprintf(w, "    %s  run `pituitary review-spec --format html --path <spec>` for the full evidence report\n", p.info())
		} else if assessment.Status == "drift" || assessment.Status == "possible_drift" {
			fmt.Fprintf(w, "\n    %s  run `pituitary review-spec --format html --path <spec>` for the full evidence chain (no deterministic fix available)\n", p.info())
		}
	}
}

func renderDriftEvidence(w io.Writer, evidence *analysis.DriftEvidence, prefix string) {
	if evidence == nil {
		return
	}
	if evidence.SpecRef != "" || evidence.SpecSection != "" || evidence.SpecExcerpt != "" {
		fmt.Fprintf(w, "%sspec evidence: %s", prefix, evidence.SpecRef)
		if evidence.SpecSection != "" {
			fmt.Fprintf(w, " | %s", evidence.SpecSection)
		}
		fmt.Fprintln(w)
		if evidence.SpecExcerpt != "" {
			fmt.Fprintf(w, "%s  excerpt: %s\n", prefix, evidence.SpecExcerpt)
		}
	}
	if evidence.DocSection != "" || evidence.DocExcerpt != "" {
		fmt.Fprintf(w, "%sdoc evidence: %s\n", prefix, evidence.DocSection)
		if evidence.DocExcerpt != "" {
			fmt.Fprintf(w, "%s  excerpt: %s\n", prefix, evidence.DocExcerpt)
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
			fmt.Fprintf(w, "  %s %s  %s\n", p.treeBranch(i == len(driftAssessments)-1), p.cyan(preferredDocLabel(assessment.DocRef, assessment.SourceRef)), driftAssessmentBadge(p, assessment.Status))
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
	fmt.Fprintf(w, "# Review Spec Report\n\n")
	fmt.Fprintf(w, "## Spec\n\n")
	fmt.Fprintf(w, "- Ref: `%s`\n", result.SpecRef)
	if result.SpecInference != nil && result.SpecInference.Level != "" {
		fmt.Fprintf(w, "- Inference: `%s`", result.SpecInference.Level)
		if result.SpecInference.Score > 0 {
			fmt.Fprintf(w, " (%.3f)", result.SpecInference.Score)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "\n## Summary\n\n")
	for _, line := range reviewMarkdownSummary(result) {
		fmt.Fprintf(w, "- %s\n", line)
	}

	fmt.Fprintf(w, "\n## Recommended Next Actions\n\n")
	actions := reviewMarkdownActions(result)
	if len(actions) == 0 {
		fmt.Fprintln(w, "- No immediate follow-up identified from the current review.")
	} else {
		for i, action := range actions {
			fmt.Fprintf(w, "%d. %s\n", i+1, action)
		}
	}

	fmt.Fprintf(w, "\n## Overlap\n\n")
	if result.Overlap == nil {
		fmt.Fprintln(w, "No overlap analysis was generated.")
	} else {
		fmt.Fprintf(w, "- Posture: `%s`", result.Overlap.Recommendation)
		if detail := humanizeOverlapRecommendation(result.Overlap.Recommendation); detail != "" {
			fmt.Fprintf(w, " (%s)", detail)
		}
		fmt.Fprintln(w)
		if len(result.Overlap.Overlaps) == 0 {
			fmt.Fprintln(w, "- No overlapping specs detected.")
		} else {
			for i, item := range result.Overlap.Overlaps {
				label := "Related overlap"
				if i == 0 {
					label = "Primary overlap"
				}
				fmt.Fprintf(w, "- %s: `%s` %s (%s, %.3f, %s)\n", label, item.Ref, item.Title, item.Relationship, item.Score, humanizeOverlapGuidance(item.Guidance))
			}
		}
	}

	fmt.Fprintf(w, "\n## Comparison\n\n")
	if result.Comparison == nil {
		fmt.Fprintln(w, "No comparison was generated because no primary comparison target was shortlisted.")
	} else {
		fmt.Fprintf(w, "- Recommendation: `%s`\n", result.Comparison.Comparison.Recommendation)
		if compatibility := result.Comparison.Comparison.Compatibility; compatibility.Level != "" || compatibility.Summary != "" {
			fmt.Fprintf(w, "- Compatibility: `%s`", compatibility.Level)
			if compatibility.Summary != "" {
				fmt.Fprintf(w, " (%s)", compatibility.Summary)
			}
			fmt.Fprintln(w)
		}
		if len(result.Comparison.Comparison.SharedScope) > 0 {
			fmt.Fprintf(w, "- Shared scope: %s\n", strings.Join(result.Comparison.Comparison.SharedScope, ", "))
		}
		for _, tradeoff := range result.Comparison.Comparison.Tradeoffs {
			fmt.Fprintf(w, "- Tradeoff `%s`: %s\n", tradeoff.Topic, tradeoff.Summary)
		}
	}

	fmt.Fprintf(w, "\n## Impact\n\n")
	if result.Impact == nil {
		fmt.Fprintln(w, "No impact analysis generated.")
	} else {
		fmt.Fprintf(w, "- Summary: %d impacted spec(s), %d governed ref(s), %d impacted doc(s)\n", len(result.Impact.AffectedSpecs), len(result.Impact.AffectedRefs), len(result.Impact.AffectedDocs))
		if len(result.Impact.AffectedSpecs) == 0 {
			fmt.Fprintln(w, "- Impacted specs: none")
		} else {
			fmt.Fprintln(w, "- Top impacted specs:")
			for _, item := range topImpactedSpecs(result.Impact.AffectedSpecs, 3) {
				fmt.Fprintf(w, "  - `%s` %s (%s", item.Ref, item.Title, item.Relationship)
				if item.Historical {
					fmt.Fprint(w, ", historical")
				}
				fmt.Fprintln(w, ")")
			}
			if extra := len(result.Impact.AffectedSpecs) - minInt(len(result.Impact.AffectedSpecs), 3); extra > 0 {
				fmt.Fprintf(w, "  - `%d` more impacted spec(s)\n", extra)
			}
		}
		if len(result.Impact.AffectedDocs) == 0 {
			fmt.Fprintln(w, "- Impacted docs: none")
		} else {
			fmt.Fprintln(w, "- Top impacted docs:")
			for _, item := range topImpactedDocs(result.Impact.AffectedDocs, 3) {
				fmt.Fprintf(w, "  - `%s` %s (score %.3f", item.Ref, item.Title, item.Score)
				if item.SourceRef != "" {
					fmt.Fprintf(w, ", %s", item.SourceRef)
				}
				fmt.Fprintln(w, ")")
			}
			if extra := len(result.Impact.AffectedDocs) - minInt(len(result.Impact.AffectedDocs), 3); extra > 0 {
				fmt.Fprintf(w, "  - `%d` more impacted doc(s)\n", extra)
			}
		}
	}

	fmt.Fprintf(w, "\n## Doc Drift\n\n")
	driftAssessments := reviewDocDriftAssessments(result.DocDrift)
	if len(driftAssessments) == 0 {
		fmt.Fprintln(w, "No drifting docs detected.")
	} else {
		fmt.Fprintf(w, "- Summary: %d doc(s) need follow-up\n", len(driftAssessments))
		driftItems := driftItemsByDocRef(result.DocDrift.DriftItems)
		for _, assessment := range driftAssessments {
			fmt.Fprintf(w, "### `%s`\n\n", assessment.DocRef)
			if assessment.Title != "" {
				fmt.Fprintf(w, "- Title: %s\n", assessment.Title)
			}
			if assessment.SourceRef != "" {
				fmt.Fprintf(w, "- Source: %s\n", assessment.SourceRef)
			}
			fmt.Fprintf(w, "- Status: `%s`", assessment.Status)
			if assessment.Confidence != nil && assessment.Confidence.Level != "" {
				fmt.Fprintf(w, " | confidence: %s", assessment.Confidence.Level)
				if assessment.Confidence.Score > 0 {
					fmt.Fprintf(w, " (%.3f)", assessment.Confidence.Score)
				}
			}
			fmt.Fprintln(w)
			if assessment.Rationale != "" {
				fmt.Fprintf(w, "- Why it matters: %s\n", assessment.Rationale)
			}
			if assessment.Evidence != nil {
				renderReviewMarkdownDriftEvidence(w, assessment.Evidence)
			}
			item, ok := driftItems[assessment.DocRef]
			if !ok {
				fmt.Fprintln(w)
				continue
			}
			for _, finding := range item.Findings {
				fmt.Fprintf(w, "- Finding `%s` from `%s`: %s", finding.Code, finding.SpecRef, finding.Message)
				if finding.Expected != "" || finding.Observed != "" {
					fmt.Fprintf(w, " (expected `%s`, observed `%s`)", finding.Expected, finding.Observed)
				}
				fmt.Fprintln(w)
				if finding.Rationale != "" {
					fmt.Fprintf(w, "  - Rationale: %s\n", finding.Rationale)
				}
				if finding.Evidence != nil {
					renderReviewMarkdownDriftEvidence(w, finding.Evidence)
				}
			}
			fmt.Fprintln(w)
		}
	}

	fmt.Fprintf(w, "## Doc Remediation\n\n")
	if result.DocRemediation == nil || len(result.DocRemediation.Items) == 0 {
		fmt.Fprintln(w, "No remediation guidance.")
		return
	}
	fmt.Fprintf(w, "- Summary: %d suggested update(s)\n", reviewRemediationSuggestionCount(result.DocRemediation))
	for _, item := range result.DocRemediation.Items {
		fmt.Fprintf(w, "### `%s`\n\n", item.DocRef)
		if item.Title != "" {
			fmt.Fprintf(w, "- Title: %s\n", item.Title)
		}
		if item.SourceRef != "" {
			fmt.Fprintf(w, "- Source: %s\n", item.SourceRef)
		}
		for _, suggestion := range item.Suggestions {
			fmt.Fprintf(w, "- Update `%s` from `%s`: %s\n", suggestion.Code, suggestion.SpecRef, suggestion.Summary)
			if suggestion.Evidence.SpecExcerpt != "" {
				fmt.Fprintf(w, "  - Evidence: spec says %q", suggestion.Evidence.SpecExcerpt)
				if suggestion.Evidence.DocExcerpt != "" {
					fmt.Fprintf(w, "; doc says %q", suggestion.Evidence.DocExcerpt)
				}
				fmt.Fprintln(w)
			}
			switch {
			case suggestion.SuggestedEdit.Replace != "" || suggestion.SuggestedEdit.With != "":
				fmt.Fprintf(w, "  - Suggested edit: replace %q with %q\n", suggestion.SuggestedEdit.Replace, suggestion.SuggestedEdit.With)
			case suggestion.SuggestedEdit.Note != "":
				fmt.Fprintf(w, "  - Suggested edit: %s\n", suggestion.SuggestedEdit.Note)
			}
		}
		fmt.Fprintln(w)
	}
}

func renderReviewHTML(w io.Writer, result *analysis.ReviewResult) {
	escape := htemplate.HTMLEscapeString

	fmt.Fprint(w, "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(w, "<title>Pituitary Review Report: %s</title>\n", escape(result.SpecRef))
	fmt.Fprint(w, `<style>
:root {
  color-scheme: light;
  --bg: #f5f1e8;
  --paper: #fffdf8;
  --ink: #1f2933;
  --muted: #52606d;
  --accent: #0b6e4f;
  --accent-soft: #dff3ea;
  --line: #d9cbb2;
  --warn: #9c4221;
  --warn-soft: #fef3e6;
  --shadow: rgba(31, 41, 51, 0.08);
}
* { box-sizing: border-box; }
body {
  margin: 0;
  font: 16px/1.55 "Iowan Old Style", "Palatino Linotype", Georgia, serif;
  color: var(--ink);
  background:
    radial-gradient(circle at top right, rgba(11, 110, 79, 0.08), transparent 28rem),
    linear-gradient(180deg, #f9f7f2, var(--bg));
}
main {
  max-width: 72rem;
  margin: 0 auto;
  padding: 2.5rem 1.25rem 4rem;
}
h1, h2, h3 { line-height: 1.2; margin: 0 0 0.75rem; }
h1 { font-size: 2.4rem; letter-spacing: -0.03em; }
h2 { font-size: 1.35rem; margin-top: 0; }
h3 { font-size: 1rem; }
p, ul, ol { margin: 0 0 1rem; }
code {
  font-family: "SFMono-Regular", "Cascadia Code", "Menlo", monospace;
  background: #f3eee4;
  border: 1px solid #eadfca;
  border-radius: 0.35rem;
  padding: 0.08rem 0.35rem;
}
.hero, .section, details {
  background: var(--paper);
  border: 1px solid var(--line);
  border-radius: 1rem;
  box-shadow: 0 10px 30px var(--shadow);
}
.hero {
  padding: 1.5rem;
  margin-bottom: 1.25rem;
}
.meta {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
  margin-top: 1rem;
}
.pill {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  padding: 0.45rem 0.7rem;
  border-radius: 999px;
  background: var(--accent-soft);
  color: var(--accent);
  font: 600 0.92rem/1.2 system-ui, sans-serif;
}
.grid {
  display: grid;
  gap: 1rem;
}
@media (min-width: 60rem) {
  .grid.two { grid-template-columns: 1.05fr 0.95fr; }
}
.section {
  padding: 1.2rem 1.25rem;
}
.muted { color: var(--muted); }
.compact-list li + li { margin-top: 0.45rem; }
.stats {
  display: grid;
  gap: 0.75rem;
}
@media (min-width: 40rem) {
  .stats { grid-template-columns: repeat(3, minmax(0, 1fr)); }
}
.stat {
  padding: 0.9rem 1rem;
  border-radius: 0.8rem;
  background: #f8f4eb;
  border: 1px solid #ebdecb;
}
.stat strong {
  display: block;
  font: 700 1.45rem/1.1 system-ui, sans-serif;
}
details {
  padding: 0.95rem 1rem;
  margin-top: 0.9rem;
}
summary {
  cursor: pointer;
  font-weight: 700;
}
.subtle {
  margin-top: 0.35rem;
  color: var(--muted);
}
.warning {
  border-color: #ebc39c;
  background: var(--warn-soft);
}
.key-value {
  display: grid;
  gap: 0.45rem;
}
.key-value div {
  padding-left: 0.9rem;
  border-left: 3px solid #eadfca;
}
</style>`+"\n</head>\n<body>\n<main>\n")

	fmt.Fprint(w, "<section class=\"hero\">\n")
	fmt.Fprint(w, "<p class=\"muted\">Pituitary review report</p>\n")
	fmt.Fprintf(w, "<h1>%s</h1>\n", escape(result.SpecRef))
	if result.SpecInference != nil && result.SpecInference.Level != "" {
		fmt.Fprintf(w, "<div class=\"meta\"><span class=\"pill\">Inference %s", escape(result.SpecInference.Level))
		if result.SpecInference.Score > 0 {
			fmt.Fprintf(w, " (%.3f)", result.SpecInference.Score)
		}
		fmt.Fprint(w, "</span></div>\n")
	}
	fmt.Fprint(w, "</section>\n")

	fmt.Fprint(w, "<div class=\"grid two\">\n")

	fmt.Fprint(w, "<section class=\"section\"><h2>Summary</h2><ul class=\"compact-list\">\n")
	for _, line := range reviewMarkdownSummary(result) {
		fmt.Fprintf(w, "<li>%s</li>\n", escape(line))
	}
	fmt.Fprint(w, "</ul></section>\n")

	fmt.Fprint(w, "<section class=\"section\"><h2>Recommended Next Actions</h2>\n")
	actions := reviewMarkdownActions(result)
	if len(actions) == 0 {
		fmt.Fprint(w, "<p class=\"muted\">No immediate follow-up identified from the current review.</p>\n")
	} else {
		fmt.Fprint(w, "<ol class=\"compact-list\">\n")
		for _, action := range actions {
			fmt.Fprintf(w, "<li>%s</li>\n", escape(action))
		}
		fmt.Fprint(w, "</ol>\n")
	}
	fmt.Fprint(w, "</section>\n")

	fmt.Fprint(w, "</div>\n")

	fmt.Fprint(w, "<div class=\"stats\">\n")
	if result.Overlap != nil {
		fmt.Fprintf(w, "<div class=\"stat\"><span class=\"muted\">Overlaps</span><strong>%d</strong><span>%s</span></div>\n", len(result.Overlap.Overlaps), escape(result.Overlap.Recommendation))
	}
	if result.Impact != nil {
		fmt.Fprintf(w, "<div class=\"stat\"><span class=\"muted\">Impacted specs</span><strong>%d</strong><span>%d docs, %d refs</span></div>\n", len(result.Impact.AffectedSpecs), len(result.Impact.AffectedDocs), len(result.Impact.AffectedRefs))
	}
	driftAssessments := reviewDocDriftAssessments(result.DocDrift)
	fmt.Fprintf(w, "<div class=\"stat\"><span class=\"muted\">Docs needing follow-up</span><strong>%d</strong><span>%d remediation item(s)</span></div>\n", len(driftAssessments), reviewRemediationSuggestionCount(result.DocRemediation))
	fmt.Fprint(w, "</div>\n")

	fmt.Fprint(w, "<section class=\"section\"><h2>Overlap</h2>\n")
	if result.Overlap == nil {
		fmt.Fprint(w, "<p class=\"muted\">No overlap analysis was generated.</p>\n")
	} else if len(result.Overlap.Overlaps) == 0 {
		fmt.Fprintf(w, "<p>No overlapping specs detected. Review posture: <code>%s</code>.</p>\n", escape(result.Overlap.Recommendation))
	} else {
		fmt.Fprintf(w, "<p>Review posture: <code>%s</code>", escape(result.Overlap.Recommendation))
		if detail := humanizeOverlapRecommendation(result.Overlap.Recommendation); detail != "" {
			fmt.Fprintf(w, " <span class=\"muted\">(%s)</span>", escape(detail))
		}
		fmt.Fprint(w, "</p><ul class=\"compact-list\">\n")
		for _, item := range result.Overlap.Overlaps {
			fmt.Fprintf(w, "<li><strong><code>%s</code></strong> %s <span class=\"muted\">(%s, %.3f, %s)</span></li>\n",
				escape(item.Ref),
				escape(item.Title),
				escape(item.Relationship),
				item.Score,
				escape(humanizeOverlapGuidance(item.Guidance)),
			)
		}
		fmt.Fprint(w, "</ul>\n")
	}
	fmt.Fprint(w, "</section>\n")

	fmt.Fprint(w, "<section class=\"section\"><h2>Comparison</h2>\n")
	if result.Comparison == nil {
		fmt.Fprint(w, "<p class=\"muted\">No primary comparison target was shortlisted.</p>\n")
	} else {
		fmt.Fprintf(w, "<p>Recommendation: <code>%s</code></p>\n", escape(result.Comparison.Comparison.Recommendation))
		if compatibility := result.Comparison.Comparison.Compatibility; compatibility.Level != "" || compatibility.Summary != "" {
			fmt.Fprintf(w, "<p class=\"subtle\">Compatibility: <code>%s</code>", escape(compatibility.Level))
			if compatibility.Summary != "" {
				fmt.Fprintf(w, " (%s)", escape(compatibility.Summary))
			}
			fmt.Fprint(w, "</p>\n")
		}
		if len(result.Comparison.Comparison.SharedScope) > 0 {
			fmt.Fprintf(w, "<p class=\"subtle\">Shared scope: %s</p>\n", escape(strings.Join(result.Comparison.Comparison.SharedScope, ", ")))
		}
		if len(result.Comparison.Comparison.Tradeoffs) > 0 {
			fmt.Fprint(w, "<ul class=\"compact-list\">\n")
			for _, tradeoff := range result.Comparison.Comparison.Tradeoffs {
				fmt.Fprintf(w, "<li><strong>%s:</strong> %s</li>\n", escape(tradeoff.Topic), escape(tradeoff.Summary))
			}
			fmt.Fprint(w, "</ul>\n")
		}
	}
	fmt.Fprint(w, "</section>\n")

	fmt.Fprint(w, "<section class=\"section\"><h2>Impact</h2>\n")
	if result.Impact == nil {
		fmt.Fprint(w, "<p class=\"muted\">No impact analysis generated.</p>\n")
	} else {
		fmt.Fprintf(w, "<p>Summary: %d impacted spec(s), %d governed ref(s), %d impacted doc(s).</p>\n", len(result.Impact.AffectedSpecs), len(result.Impact.AffectedRefs), len(result.Impact.AffectedDocs))
		fmt.Fprint(w, "<div class=\"grid two\">")
		fmt.Fprint(w, "<div><h3>Top impacted specs</h3>")
		specs := topImpactedSpecs(result.Impact.AffectedSpecs, 5)
		if len(specs) == 0 {
			fmt.Fprint(w, "<p class=\"muted\">None.</p>")
		} else {
			fmt.Fprint(w, "<ul class=\"compact-list\">")
			for _, item := range specs {
				fmt.Fprintf(w, "<li><strong><code>%s</code></strong> %s <span class=\"muted\">(%s", escape(item.Ref), escape(item.Title), escape(item.Relationship))
				if item.Historical {
					fmt.Fprint(w, ", historical")
				}
				fmt.Fprint(w, ")</span></li>")
			}
			fmt.Fprint(w, "</ul>")
		}
		fmt.Fprint(w, "</div>")
		fmt.Fprint(w, "<div><h3>Top impacted docs</h3>")
		docs := topImpactedDocs(result.Impact.AffectedDocs, 5)
		if len(docs) == 0 {
			fmt.Fprint(w, "<p class=\"muted\">None.</p>")
		} else {
			fmt.Fprint(w, "<ul class=\"compact-list\">")
			for _, item := range docs {
				fmt.Fprintf(w, "<li><strong><code>%s</code></strong> %s <span class=\"muted\">(score %.3f", escape(item.Ref), escape(item.Title), item.Score)
				if item.SourceRef != "" {
					fmt.Fprintf(w, ", %s", escape(item.SourceRef))
				}
				fmt.Fprint(w, ")</span></li>")
			}
			fmt.Fprint(w, "</ul>")
		}
		fmt.Fprint(w, "</div></div>")
	}
	fmt.Fprint(w, "</section>\n")

	fmt.Fprint(w, "<section class=\"section\"><h2>Doc Drift</h2>\n")
	if len(driftAssessments) == 0 {
		fmt.Fprint(w, "<p class=\"muted\">No drifting docs detected.</p>\n")
	} else {
		fmt.Fprintf(w, "<p>%d doc(s) need follow-up.</p>\n", len(driftAssessments))
		driftItems := driftItemsByDocRef(result.DocDrift.DriftItems)
		for _, assessment := range driftAssessments {
			detailClass := "warning"
			if assessment.Status == "possible_drift" {
				detailClass = ""
			}
			fmt.Fprintf(w, "<details class=\"%s\" open><summary><code>%s</code>", detailClass, escape(assessment.DocRef))
			if assessment.Title != "" {
				fmt.Fprintf(w, " — %s", escape(assessment.Title))
			}
			fmt.Fprintf(w, " <span class=\"muted\">(%s)</span></summary>\n", escape(assessment.Status))
			fmt.Fprint(w, "<div class=\"key-value\">")
			if assessment.SourceRef != "" {
				fmt.Fprintf(w, "<div><strong>Source</strong><br>%s</div>", escape(assessment.SourceRef))
			}
			if assessment.Rationale != "" {
				fmt.Fprintf(w, "<div><strong>Why it matters</strong><br>%s</div>", escape(assessment.Rationale))
			}
			if assessment.Evidence != nil {
				fmt.Fprintf(w, "<div><strong>Evidence</strong><br>%s</div>", reviewHTMLDriftEvidence(assessment.Evidence))
			}
			item, ok := driftItems[assessment.DocRef]
			if ok {
				for _, finding := range item.Findings {
					fmt.Fprintf(w, "<div><strong>%s</strong><br>%s", escape(finding.Code), escape(finding.Message))
					if finding.Expected != "" || finding.Observed != "" {
						fmt.Fprintf(w, "<br><span class=\"subtle\">expected %s | observed %s</span>", escape(finding.Expected), escape(finding.Observed))
					}
					if finding.Rationale != "" {
						fmt.Fprintf(w, "<br><span class=\"subtle\">%s</span>", escape(finding.Rationale))
					}
					if finding.Evidence != nil {
						fmt.Fprintf(w, "<br>%s", reviewHTMLDriftEvidence(finding.Evidence))
					}
					fmt.Fprint(w, "</div>")
				}
			}
			fmt.Fprint(w, "</div></details>\n")
		}
	}
	fmt.Fprint(w, "</section>\n")

	fmt.Fprint(w, "<section class=\"section\"><h2>Doc Remediation</h2>\n")
	if result.DocRemediation == nil || len(result.DocRemediation.Items) == 0 {
		fmt.Fprint(w, "<p class=\"muted\">No remediation guidance.</p>\n")
	} else {
		fmt.Fprintf(w, "<p>%d suggested update(s).</p>\n", reviewRemediationSuggestionCount(result.DocRemediation))
		for _, item := range result.DocRemediation.Items {
			fmt.Fprintf(w, "<details open><summary><code>%s</code>", escape(item.DocRef))
			if item.Title != "" {
				fmt.Fprintf(w, " — %s", escape(item.Title))
			}
			fmt.Fprint(w, "</summary><div class=\"key-value\">")
			if item.SourceRef != "" {
				fmt.Fprintf(w, "<div><strong>Source</strong><br>%s</div>", escape(item.SourceRef))
			}
			for _, suggestion := range item.Suggestions {
				fmt.Fprintf(w, "<div><strong>%s</strong><br>%s", escape(suggestion.Code), escape(suggestion.Summary))
				if suggestion.Evidence.SpecExcerpt != "" || suggestion.Evidence.DocExcerpt != "" {
					fmt.Fprintf(w, "<br>%s", reviewHTMLRemediationEvidence(suggestion.Evidence))
				}
				switch {
				case suggestion.SuggestedEdit.Replace != "" || suggestion.SuggestedEdit.With != "":
					fmt.Fprintf(w, "<br><span class=\"subtle\">Replace %s with %s</span>", escape(suggestion.SuggestedEdit.Replace), escape(suggestion.SuggestedEdit.With))
				case suggestion.SuggestedEdit.Note != "":
					fmt.Fprintf(w, "<br><span class=\"subtle\">%s</span>", escape(suggestion.SuggestedEdit.Note))
				}
				fmt.Fprint(w, "</div>")
			}
			fmt.Fprint(w, "</div></details>\n")
		}
	}
	fmt.Fprint(w, "</section>\n")

	if len(result.Warnings) > 0 {
		fmt.Fprint(w, "<section class=\"section warning\"><h2>Warnings</h2><ul class=\"compact-list\">")
		for _, warning := range result.Warnings {
			fmt.Fprintf(w, "<li><strong>%s</strong>: %s</li>", escape(warning.Code), escape(warning.Message))
		}
		fmt.Fprint(w, "</ul></section>\n")
	}

	fmt.Fprint(w, "</main>\n</body>\n</html>\n")
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
	parts := make([]string, 0, 2)
	if evidence.SpecSection != "" || evidence.SpecExcerpt != "" {
		spec := "<strong>Spec</strong> "
		if evidence.SpecSection != "" {
			spec += escape(evidence.SpecSection)
		} else {
			spec += "matched section"
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
		if evidence.DocExcerpt != "" {
			doc += "<br><span class=\"subtle\">" + escape(evidence.DocExcerpt) + "</span>"
		}
		parts = append(parts, doc)
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

func reviewDocDriftAssessments(result *analysis.DocDriftResult) []analysis.DocDriftAssessment {
	if result == nil {
		return nil
	}
	if len(result.Assessments) > 0 {
		items := make([]analysis.DocDriftAssessment, 0, len(result.Assessments))
		for _, item := range result.Assessments {
			if item.Status == "aligned" {
				continue
			}
			items = append(items, item)
		}
		return items
	}
	return driftAssessmentsFromItems(result.DriftItems)
}

func reviewRemediationSuggestionCount(result *analysis.DocRemediationResult) int {
	if result == nil {
		return 0
	}
	count := 0
	for _, item := range result.Items {
		count += len(item.Suggestions)
	}
	return count
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

func runtimeCheckGlyph(p renderPresentation, status string) string {
	switch status {
	case "ready":
		return p.check()
	case "disabled":
		return p.info()
	default:
		return p.cross()
	}
}

func complianceBadge(p renderPresentation, label string) string {
	switch label {
	case "conflicts":
		return p.conflictBadge()
	case "compliant":
		return p.compliantBadge()
	default:
		return p.unspecifiedBadge()
	}
}

func preferredDocLabel(docRef, sourceRef string) string {
	if strings.TrimSpace(sourceRef) != "" {
		return sourceRef
	}
	return docRef
}

func docDriftPadding(label string) int {
	width := 56 - len([]rune(label))
	if width < 2 {
		return 2
	}
	return width
}

func driftAssessmentBadge(p renderPresentation, status string) string {
	switch status {
	case "aligned":
		return p.okBadge()
	case "possible_drift":
		return p.reviewBadge()
	default:
		return p.driftBadge()
	}
}

func driftFindingSummary(finding analysis.DriftFinding) string {
	if strings.TrimSpace(finding.Code) != "" {
		return humanizeSymbol(finding.Code)
	}
	return strings.TrimSpace(finding.Message)
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

func reviewImpactLines(result *analysis.AnalyzeImpactResult) []string {
	if result == nil {
		return nil
	}
	lines := make([]string, 0, 6)
	for _, item := range topImpactedSpecs(result.AffectedSpecs, 2) {
		line := fmt.Sprintf("%s  %s · %s", item.Ref, item.Title, item.Relationship)
		if item.Historical {
			line += " · historical"
		}
		lines = append(lines, line)
	}
	for _, item := range topImpactedDocs(result.AffectedDocs, 2) {
		line := fmt.Sprintf("%s  %.3f", item.Ref, item.Score)
		lines = append(lines, line)
	}
	return lines
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

func remediationItemsByDocRef(result *analysis.DocRemediationResult) map[string][]analysis.DocRemediationSuggestion {
	if result == nil {
		return map[string][]analysis.DocRemediationSuggestion{}
	}
	items := make(map[string][]analysis.DocRemediationSuggestion, len(result.Items))
	for _, item := range result.Items {
		items[item.DocRef] = item.Suggestions
	}
	return items
}

func driftItemsByDocRef(items []analysis.DriftItem) map[string]analysis.DriftItem {
	result := make(map[string]analysis.DriftItem, len(items))
	for _, item := range items {
		result[item.DocRef] = item
	}
	return result
}

func driftAssessmentsFromItems(items []analysis.DriftItem) []analysis.DocDriftAssessment {
	result := make([]analysis.DocDriftAssessment, 0, len(items))
	for _, item := range items {
		result = append(result, analysis.DocDriftAssessment{
			DocRef:    item.DocRef,
			Title:     item.Title,
			SourceRef: item.SourceRef,
			Status:    "drift",
			SpecRefs:  append([]string(nil), item.SpecRefs...),
		})
	}
	return result
}

func renderReviewImpactSummary(w io.Writer, impact *analysis.AnalyzeImpactResult) {
	if impact == nil {
		return
	}
	specs := topImpactedSpecs(impact.AffectedSpecs, 3)
	if len(specs) == 0 {
		fmt.Fprintln(w, "top impacted specs: none")
	} else {
		fmt.Fprintln(w, "top impacted specs:")
		for _, item := range specs {
			fmt.Fprintf(w, "- %s | %s | %s", item.Ref, item.Title, item.Relationship)
			if item.Historical {
				fmt.Fprint(w, " | historical")
			}
			fmt.Fprintln(w)
		}
		if extra := len(impact.AffectedSpecs) - len(specs); extra > 0 {
			fmt.Fprintf(w, "- %d more impacted spec(s)\n", extra)
		}
	}

	docs := topImpactedDocs(impact.AffectedDocs, 3)
	if len(docs) == 0 {
		fmt.Fprintln(w, "top impacted docs: none")
		return
	}
	fmt.Fprintln(w, "top impacted docs:")
	for _, item := range docs {
		fmt.Fprintf(w, "- %s | %s | %.3f", item.Ref, item.Title, item.Score)
		if item.SourceRef != "" {
			fmt.Fprintf(w, " | %s", item.SourceRef)
		}
		fmt.Fprintln(w)
	}
	if extra := len(impact.AffectedDocs) - len(docs); extra > 0 {
		fmt.Fprintf(w, "- %d more impacted doc(s)\n", extra)
	}
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

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
