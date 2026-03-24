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
	description := commandDescription(command)
	if description == "" {
		return fmt.Errorf("unknown command %q", command)
	}

	fmt.Fprintf(w, "pituitary %s: %s\n", command, description)

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
	case *analysis.ReviewResult:
		renderReviewResult(w, typed)
	default:
		return fmt.Errorf("unsupported result type %T", result)
	}

	return nil
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
	fmt.Fprintf(w, "workspace: %s\n", result.WorkspaceRoot)
	fmt.Fprintf(w, "config path: %s\n", result.ConfigPath)
	fmt.Fprintf(w, "config action: %s\n", result.ConfigAction)
	if result.Discover != nil {
		fmt.Fprintf(w, "discovered sources: %d\n", len(result.Discover.Sources))
	}
	if result.Index != nil {
		fmt.Fprintf(w, "index: %d artifact(s), %d chunk(s), %d edge(s)\n", result.Index.ArtifactCount, result.Index.ChunkCount, result.Index.EdgeCount)
	}
	if result.Status != nil {
		status := "unknown"
		if result.Status.Freshness != nil && result.Status.Freshness.State != "" {
			status = result.Status.Freshness.State
		}
		fmt.Fprintf(w, "status: %s | specs: %d | docs: %d | chunks: %d\n", status, result.Status.SpecCount, result.Status.DocCount, result.Status.ChunkCount)
	}
	if result.ConfigAction == "preview" {
		fmt.Fprintln(w, "next: run `pituitary init` without --dry-run to write the config and build the index")
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
	if result.WorkspaceRoot != "" {
		fmt.Fprintf(w, "workspace: %s\n", result.WorkspaceRoot)
	}
	fmt.Fprintf(w, "config: %s\n", result.ConfigPath)
	if result.ConfigResolution != nil {
		if result.ConfigResolution.Reason != "" {
			fmt.Fprintf(w, "config resolution: %s\n", result.ConfigResolution.Reason)
		}
		if len(result.ConfigResolution.Candidates) > 0 {
			fmt.Fprintln(w, "config candidates:")
			for _, candidate := range result.ConfigResolution.Candidates {
				fmt.Fprintf(w, "  %d. %s | %s", candidate.Precedence, configSourceLabel(candidate.Source), candidate.Status)
				if candidate.Path != "" {
					fmt.Fprintf(w, " | %s", candidate.Path)
				}
				fmt.Fprintln(w)
				if candidate.Detail != "" {
					fmt.Fprintf(w, "     %s\n", candidate.Detail)
				}
			}
		}
	}
	fmt.Fprintf(w, "index path: %s\n", result.IndexPath)
	if result.IndexExists {
		fmt.Fprintln(w, "index: present")
	} else {
		fmt.Fprintln(w, "index: missing")
	}
	if result.Freshness != nil {
		fmt.Fprintf(w, "index freshness: %s\n", result.Freshness.State)
		for _, issue := range result.Freshness.Issues {
			fmt.Fprintf(w, "freshness: %s\n", issue.Message)
		}
		if result.Freshness.Action != "" {
			fmt.Fprintf(w, "freshness action: %s\n", result.Freshness.Action)
		}
	}
	fmt.Fprintf(w, "indexed specs: %d\n", result.SpecCount)
	fmt.Fprintf(w, "indexed docs: %d\n", result.DocCount)
	fmt.Fprintf(w, "indexed chunks: %d\n", result.ChunkCount)
	if result.RelationGraph != nil {
		fmt.Fprintf(w, "relation graph: %s\n", result.RelationGraph.State)
		for _, finding := range result.RelationGraph.Findings {
			fmt.Fprintf(w, "relation issue: %s\n", finding.Message)
		}
	}
	if result.ArtifactLocations != nil {
		fmt.Fprintf(w, "artifact index dir: %s\n", result.ArtifactLocations.IndexDir)
		fmt.Fprintf(w, "artifact discover --write default: %s\n", result.ArtifactLocations.DiscoverConfigPath)
		fmt.Fprintf(w, "artifact canonicalize default: %s\n", result.ArtifactLocations.CanonicalizeBundleRoot)
		if len(result.ArtifactLocations.IgnorePatterns) > 0 {
			fmt.Fprintf(w, "artifact ignore patterns: %s\n", strings.Join(result.ArtifactLocations.IgnorePatterns, ", "))
		}
		for _, hint := range result.ArtifactLocations.RelocationHints {
			fmt.Fprintf(w, "artifact relocation: %s\n", hint)
		}
	}
	if result.Runtime != nil {
		fmt.Fprintf(w, "runtime probe: %s\n", result.Runtime.Scope)
		for _, check := range result.Runtime.Checks {
			fmt.Fprintf(w, "runtime: %s | %s | provider: %s", check.Name, check.Status, check.Provider)
			if check.Model != "" {
				fmt.Fprintf(w, " | model: %s", check.Model)
			}
			if check.Endpoint != "" {
				fmt.Fprintf(w, " | endpoint: %s", check.Endpoint)
			}
			fmt.Fprintln(w)
			if check.Message != "" {
				fmt.Fprintf(w, "runtime note: %s\n", check.Message)
			}
		}
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
	fmt.Fprintf(w, "candidate: %s | %s\n", result.Candidate.Ref, result.Candidate.Title)
	if len(result.Overlaps) == 0 {
		fmt.Fprintln(w, "no overlaps")
		renderOverlapRecommendation(w, result.Recommendation)
		return
	}
	for i, overlap := range result.Overlaps {
		fmt.Fprintf(w, "%d. %s | %s | %.3f | %s | %s | %s\n", i+1, overlap.Ref, overlap.Title, overlap.Score, overlap.OverlapDegree, overlap.Relationship, humanizeOverlapGuidance(overlap.Guidance))
	}
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
		if item.Traceability != "" {
			fmt.Fprintf(w, "  traceability: %s\n", item.Traceability)
		}
		if item.LimitingFactor != "" {
			fmt.Fprintf(w, "  limiting factor: %s\n", item.LimitingFactor)
		}
		if item.Suggestion != "" {
			fmt.Fprintf(w, "  suggestion: %s\n", item.Suggestion)
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
	if len(result.DriftItems) == 0 && len(result.Assessments) == 0 {
		fmt.Fprintln(w, "no drift items")
		return
	}

	assessments := result.Assessments
	if len(assessments) == 0 {
		assessments = driftAssessmentsFromItems(result.DriftItems)
	}
	driftItems := driftItemsByDocRef(result.DriftItems)
	remediation := remediationItemsByDocRef(result.Remediation)
	for i, assessment := range assessments {
		fmt.Fprintf(w, "%d. %s | %s | status: %s", i+1, assessment.DocRef, assessment.Title, assessment.Status)
		if assessment.Confidence != nil && assessment.Confidence.Level != "" {
			fmt.Fprintf(w, " | confidence: %s", assessment.Confidence.Level)
			if assessment.Confidence.Score > 0 {
				fmt.Fprintf(w, " (%.3f)", assessment.Confidence.Score)
			}
		}
		if item, ok := driftItems[assessment.DocRef]; ok {
			fmt.Fprintf(w, " | findings: %d", len(item.Findings))
		}
		if suggestions := remediation[assessment.DocRef]; len(suggestions) > 0 {
			fmt.Fprintf(w, " | remediation: %d", len(suggestions))
		}
		fmt.Fprintln(w)
		if assessment.Rationale != "" {
			fmt.Fprintf(w, "   rationale: %s\n", assessment.Rationale)
		}
		if assessment.Evidence != nil {
			renderDriftEvidence(w, assessment.Evidence, "   ")
		}
		if assessment.Confidence != nil && assessment.Confidence.Basis != "" {
			fmt.Fprintf(w, "   confidence basis: %s\n", assessment.Confidence.Basis)
		}
		if item, ok := driftItems[assessment.DocRef]; ok {
			for _, finding := range item.Findings {
				fmt.Fprintf(w, "   finding: %s", finding.Code)
				if finding.Artifact != "" {
					fmt.Fprintf(w, " | artifact: %s", finding.Artifact)
				}
				if finding.Confidence != nil && finding.Confidence.Level != "" {
					fmt.Fprintf(w, " | confidence: %s", finding.Confidence.Level)
				}
				fmt.Fprintln(w)
				fmt.Fprintf(w, "     message: %s\n", finding.Message)
				if finding.Rationale != "" {
					fmt.Fprintf(w, "     rationale: %s\n", finding.Rationale)
				}
				if finding.Expected != "" || finding.Observed != "" {
					fmt.Fprintf(w, "     expected: %s\n", finding.Expected)
					fmt.Fprintf(w, "     observed: %s\n", finding.Observed)
				}
				if finding.Evidence != nil {
					renderDriftEvidence(w, finding.Evidence, "     ")
				}
				if finding.Confidence != nil && finding.Confidence.Basis != "" {
					fmt.Fprintf(w, "     confidence basis: %s\n", finding.Confidence.Basis)
				}
			}
		}
		for _, suggestion := range remediation[assessment.DocRef] {
			fmt.Fprintf(w, "   remediation: %s | %s\n", suggestion.SpecRef, suggestion.Summary)
			if suggestion.SuggestedEdit.Replace != "" || suggestion.SuggestedEdit.With != "" {
				fmt.Fprintf(w, "   suggested edit: replace %q with %q\n", suggestion.SuggestedEdit.Replace, suggestion.SuggestedEdit.With)
			} else if suggestion.SuggestedEdit.Note != "" {
				fmt.Fprintf(w, "   suggested edit: %s\n", suggestion.SuggestedEdit.Note)
			}
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

func renderReviewResult(w io.Writer, result *analysis.ReviewResult) {
	fmt.Fprintf(w, "spec: %s\n", result.SpecRef)

	if result.Overlap != nil {
		fmt.Fprintf(w, "overlaps: %d | recommendation: %s", len(result.Overlap.Overlaps), result.Overlap.Recommendation)
		if detail := humanizeOverlapRecommendation(result.Overlap.Recommendation); detail != "" {
			fmt.Fprintf(w, " | %s", detail)
		}
		fmt.Fprintln(w)
		if len(result.Overlap.Overlaps) > 0 {
			top := result.Overlap.Overlaps[0]
			fmt.Fprintf(w, "top overlap: %s | %s | %.3f | %s\n", top.Ref, top.Relationship, top.Score, humanizeOverlapGuidance(top.Guidance))
		}
	}
	if result.Comparison != nil {
		fmt.Fprintf(w, "comparison: %s\n", result.Comparison.Comparison.Recommendation)
	} else {
		fmt.Fprintln(w, "comparison: none")
	}
	if result.Impact != nil {
		fmt.Fprintf(w, "impact: %d spec(s), %d ref(s), %d doc(s)\n", len(result.Impact.AffectedSpecs), len(result.Impact.AffectedRefs), len(result.Impact.AffectedDocs))
		renderReviewImpactSummary(w, result.Impact)
	} else {
		fmt.Fprintln(w, "impact: none")
	}
	if result.DocDrift != nil {
		fmt.Fprintf(w, "doc drift: %d item(s)\n", len(result.DocDrift.DriftItems))
	} else {
		fmt.Fprintln(w, "doc drift: none")
	}
	if result.DocRemediation != nil {
		fmt.Fprintf(w, "doc remediation: %d item(s)\n", len(result.DocRemediation.Items))
	} else {
		fmt.Fprintln(w, "doc remediation: none")
	}
}

func renderReviewMarkdown(w io.Writer, result *analysis.ReviewResult) {
	fmt.Fprintf(w, "# Review Spec Report\n\n")
	fmt.Fprintf(w, "## Spec\n\n")
	fmt.Fprintf(w, "- Ref: `%s`\n", result.SpecRef)

	fmt.Fprintf(w, "\n## Overlap\n\n")
	if result.Overlap == nil {
		fmt.Fprintln(w, "No overlap analysis.")
	} else {
		fmt.Fprintf(w, "- Recommendation: `%s`", result.Overlap.Recommendation)
		if detail := humanizeOverlapRecommendation(result.Overlap.Recommendation); detail != "" {
			fmt.Fprintf(w, " (%s)", detail)
		}
		fmt.Fprintln(w)
		if len(result.Overlap.Overlaps) == 0 {
			fmt.Fprintln(w, "- No overlapping specs detected.")
		} else {
			for _, item := range result.Overlap.Overlaps {
				fmt.Fprintf(w, "- `%s` %s (%s, %.3f, %s)\n", item.Ref, item.Title, item.Relationship, item.Score, humanizeOverlapGuidance(item.Guidance))
			}
		}
	}

	fmt.Fprintf(w, "\n## Comparison\n\n")
	if result.Comparison == nil {
		fmt.Fprintln(w, "No comparison generated.")
	} else {
		fmt.Fprintf(w, "- Recommendation: `%s`\n", result.Comparison.Comparison.Recommendation)
		for _, tradeoff := range result.Comparison.Comparison.Tradeoffs {
			fmt.Fprintf(w, "- %s: %s\n", tradeoff.Topic, tradeoff.Summary)
		}
	}

	fmt.Fprintf(w, "\n## Impact\n\n")
	if result.Impact == nil {
		fmt.Fprintln(w, "No impact analysis generated.")
	} else {
		if len(result.Impact.AffectedSpecs) == 0 {
			fmt.Fprintln(w, "- Affected specs: none")
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
			fmt.Fprintln(w, "- Affected docs: none")
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
	if result.DocDrift == nil || len(result.DocDrift.DriftItems) == 0 {
		fmt.Fprintln(w, "No drifting docs detected.")
	} else {
		for _, item := range result.DocDrift.DriftItems {
			fmt.Fprintf(w, "### `%s`\n\n", item.DocRef)
			for _, finding := range item.Findings {
				fmt.Fprintf(w, "- `%s` from `%s`: %s", finding.Code, finding.SpecRef, finding.Message)
				if finding.Expected != "" || finding.Observed != "" {
					fmt.Fprintf(w, " (expected `%s`, observed `%s`)", finding.Expected, finding.Observed)
				}
				fmt.Fprintln(w)
			}
			fmt.Fprintln(w)
		}
	}

	fmt.Fprintf(w, "## Doc Remediation\n\n")
	if result.DocRemediation == nil || len(result.DocRemediation.Items) == 0 {
		fmt.Fprintln(w, "No remediation guidance.")
		return
	}
	for _, item := range result.DocRemediation.Items {
		fmt.Fprintf(w, "### `%s`\n\n", item.DocRef)
		for _, suggestion := range item.Suggestions {
			fmt.Fprintf(w, "- `%s` from `%s`: %s\n", suggestion.Code, suggestion.SpecRef, suggestion.Summary)
			if suggestion.Evidence.SpecExcerpt != "" {
				fmt.Fprintf(w, "  Evidence: spec says %q", suggestion.Evidence.SpecExcerpt)
				if suggestion.Evidence.DocExcerpt != "" {
					fmt.Fprintf(w, "; doc says %q", suggestion.Evidence.DocExcerpt)
				}
				fmt.Fprintln(w)
			}
			switch {
			case suggestion.SuggestedEdit.Replace != "" || suggestion.SuggestedEdit.With != "":
				fmt.Fprintf(w, "  Suggested edit: replace %q with %q\n", suggestion.SuggestedEdit.Replace, suggestion.SuggestedEdit.With)
			case suggestion.SuggestedEdit.Note != "":
				fmt.Fprintf(w, "  Suggested edit: %s\n", suggestion.SuggestedEdit.Note)
			}
		}
		fmt.Fprintln(w)
	}
}

func renderOverlapRecommendation(w io.Writer, recommendation string) {
	fmt.Fprintf(w, "recommendation: %s", recommendation)
	if detail := humanizeOverlapRecommendation(recommendation); detail != "" {
		fmt.Fprintf(w, " | %s", detail)
	}
	fmt.Fprintln(w)
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
