package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/index"
)

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
	if result.Compact {
		renderCompactStatusDetails(w, result)
		return
	}

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
	if result.InferAppliesToEnabled != nil {
		if *result.InferAppliesToEnabled {
			fmt.Fprintf(w, "  %s %s\n", p.dim("inference:"), p.green("enabled"))
		} else {
			fmt.Fprintf(w, "  %s %s %s\n", p.dim("inference:"), p.red("disabled"), p.dim("(set workspace.infer_applies_to = true to enable)"))
		}
	}
	if len(result.Repos) > 0 {
		fmt.Fprintf(w, "  %s\n", p.white("REPO COVERAGE"))
		for i, repo := range result.Repos {
			fmt.Fprintf(w, "  %s %s\n", p.treeItem(i == len(result.Repos)-1), renderRepoCoverageLine(repo))
		}
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
	renderStatusGovernanceHotspots(w, result.GovernanceHotspots)
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
	if result.RuntimeConfig != nil {
		fmt.Fprintf(w, "  %s\n", p.white("RUNTIME CONFIG"))
		// Providers are never the last tree item anymore — the
		// contextualizer line below is always emitted (enabled or
		// disabled) per #347, so both providers are mid-branches.
		for _, item := range []struct {
			Name     string
			Provider statusRuntimeProvider
		}{
			{Name: "runtime.embedder", Provider: result.RuntimeConfig.Embedder},
			{Name: "runtime.analysis", Provider: result.RuntimeConfig.Analysis},
		} {
			fmt.Fprintf(w, "  %s %s\n", p.treeItem(false), renderRuntimeProviderSummary(item.Name, item.Provider))
		}
		fmt.Fprintf(w, "  %s runtime.chunking.contextualizer: %s\n",
			p.treeItem(true), renderContextualizerSummary(result.RuntimeConfig.Contextualizer))
	}
	if result.Runtime != nil {
		fmt.Fprintf(w, "  %s %s\n", p.dim("runtime probe:"), result.Runtime.Scope)
		for _, check := range result.Runtime.Checks {
			fmt.Fprintf(w, "  %s %s %s | %s | provider: %s", p.dim("runtime:"), runtimeCheckGlyph(p, check.Status), check.Name, check.Status, check.Provider)
			if check.Profile != "" {
				fmt.Fprintf(w, " | profile: %s", check.Profile)
			}
			if check.Model != "" {
				fmt.Fprintf(w, " | model: %s", check.Model)
			}
			if check.Endpoint != "" {
				fmt.Fprintf(w, " | endpoint: %s", check.Endpoint)
			}
			if check.Timeout > 0 {
				fmt.Fprintf(w, " | timeout_ms: %d", check.Timeout)
			}
			if check.Retries > 0 {
				fmt.Fprintf(w, " | max_retries: %d", check.Retries)
			}
			fmt.Fprintln(w)
			if check.Message != "" {
				fmt.Fprintf(w, "     %s %s\n", p.dim("runtime note:"), check.Message)
			}
		}
	}
	renderSpecFamilies(w, result.Families)
	for _, guidance := range result.Guidance {
		fmt.Fprintf(w, "  %s %s\n", p.arrow(), guidance)
	}
}

func renderCompactStatusDetails(w io.Writer, result *statusResult) {
	p := presentationForWriter(w)
	if result.Runtime != nil {
		summaries := make([]string, 0, len(result.Runtime.Checks))
		for _, check := range result.Runtime.Checks {
			summaries = append(summaries, fmt.Sprintf("%s %s", check.Name, check.Status))
		}
		if len(summaries) > 0 {
			fmt.Fprintf(w, "  %s %s | %s\n", p.dim("runtime probe:"), result.Runtime.Scope, strings.Join(summaries, ", "))
		}
	}
	if result.Freshness != nil {
		for _, issue := range result.Freshness.Issues {
			fmt.Fprintf(w, "  %s %s\n", p.cross(), issue.Message)
		}
		if result.Freshness.Action != "" {
			fmt.Fprintf(w, "  %s %s\n", p.arrow(), result.Freshness.Action)
		}
	}
	if result.RelationGraph != nil {
		for _, finding := range result.RelationGraph.Findings {
			fmt.Fprintf(w, "  %s %s\n", p.cross(), finding.Message)
		}
	}
	for _, guidance := range result.Guidance {
		fmt.Fprintf(w, "  %s %s\n", p.arrow(), guidance)
	}
}

func renderStatusGovernanceHotspots(w io.Writer, hotspots *index.GovernanceHotspots) {
	lines := statusGovernanceHotspotLines(hotspots)
	if len(lines) == 0 {
		return
	}

	p := presentationForWriter(w)
	fmt.Fprintf(w, "  %s\n", p.white("GOVERNANCE HOTSPOTS"))
	for i, line := range lines {
		fmt.Fprintf(w, "  %s %s\n", p.treeItem(i == len(lines)-1), line)
	}
}

func statusGovernanceHotspotLines(hotspots *index.GovernanceHotspots) []string {
	if hotspots == nil {
		return nil
	}

	lines := make([]string, 0, len(hotspots.HighFanOutSpecs)+len(hotspots.WeakLinkArtifacts)+len(hotspots.MultiGovernedArtifacts))
	for _, hotspot := range hotspots.HighFanOutSpecs {
		line := fmt.Sprintf("fan-out spec %s", hotspot.Ref)
		if hotspot.Title != "" {
			line += " · " + hotspot.Title
		}
		line += fmt.Sprintf(" | %d applies_to edges", hotspot.AppliesToCount)
		if weakEdges := hotspot.InferredEdgeCount + hotspot.AmbiguousEdgeCount; weakEdges > 0 {
			line += fmt.Sprintf(" | %d weak edges", weakEdges)
		}
		lines = append(lines, line)
	}
	for _, hotspot := range hotspots.WeakLinkArtifacts {
		line := fmt.Sprintf("weak-link artifact %s | %d governing specs", hotspot.Ref, hotspot.GoverningSpecCount)
		if hotspot.SourceRef != "" && hotspot.SourceRef != hotspot.Ref {
			line += " | " + displaySourcePath(hotspot.SourceRef)
		}
		if hotspot.InferredEdgeCount > 0 {
			line += fmt.Sprintf(" | %d inferred", hotspot.InferredEdgeCount)
		}
		if hotspot.AmbiguousEdgeCount > 0 {
			line += fmt.Sprintf(" | %d ambiguous", hotspot.AmbiguousEdgeCount)
		}
		if specs := formatGovernanceHotspotSpecRefs(hotspot.GoverningSpecs, 3); specs != "" {
			line += " | " + specs
		}
		lines = append(lines, line)
	}
	for _, hotspot := range hotspots.MultiGovernedArtifacts {
		line := fmt.Sprintf("multi-governed artifact %s | %d governing specs", hotspot.Ref, hotspot.GoverningSpecCount)
		if hotspot.SourceRef != "" && hotspot.SourceRef != hotspot.Ref {
			line += " | " + displaySourcePath(hotspot.SourceRef)
		}
		if weakEdges := hotspot.InferredEdgeCount + hotspot.AmbiguousEdgeCount; weakEdges > 0 {
			line += fmt.Sprintf(" | %d weak edges", weakEdges)
		}
		if specs := formatGovernanceHotspotSpecRefs(hotspot.GoverningSpecs, 3); specs != "" {
			line += " | " + specs
		}
		lines = append(lines, line)
	}
	return lines
}

func formatGovernanceHotspotSpecRefs(refs []string, limit int) string {
	if len(refs) == 0 {
		return ""
	}
	if limit <= 0 || len(refs) <= limit {
		return strings.Join(refs, ", ")
	}
	return fmt.Sprintf("%s +%d more", strings.Join(refs[:limit], ", "), len(refs)-limit)
}

func renderSpecFamilies(w io.Writer, families *index.FamilyResult) {
	if families == nil || len(families.Families) == 0 {
		return
	}
	p := presentationForWriter(w)
	fmt.Fprintf(w, "  %s\n", p.white("SPEC FAMILIES"))
	for i, family := range families.Families {
		memberList := strings.Join(family.Members, ", ")
		fmt.Fprintf(w, "  %s family %d (%d member(s), cohesion: %.2f): %s\n",
			p.treeItem(i == len(families.Families)-1 && len(families.Ungoverned) == 0),
			family.ID, family.Size, family.Cohesion, memberList)
	}
	if len(families.Ungoverned) > 0 {
		fmt.Fprintf(w, "  %s ungoverned files: %d\n", p.dim("coverage gap:"), len(families.Ungoverned))
		limit := 5
		if len(families.Ungoverned) < limit {
			limit = len(families.Ungoverned)
		}
		for _, path := range families.Ungoverned[:limit] {
			fmt.Fprintf(w, "    %s %s\n", p.cross(), path)
		}
		if len(families.Ungoverned) > 5 {
			fmt.Fprintf(w, "    %s ... and %d more\n", p.dim(""), len(families.Ungoverned)-5)
		}
	}
}

// renderContextualizerSummary formats the contextualizer entry in the
// RUNTIME CONFIG block. Disabled prints "disabled" so an operator can
// confirm the current posture at a glance; enabled prints
// "format=<name>" for symmetry with the " | "-joined provider summary.
func renderContextualizerSummary(format string) string {
	if format == "" {
		return "disabled"
	}
	return "format=" + format
}

func renderRuntimeProviderSummary(name string, provider statusRuntimeProvider) string {
	parts := []string{name}
	if provider.Profile != "" {
		parts = append(parts, "profile: "+provider.Profile)
	}
	if provider.Provider != "" {
		parts = append(parts, "provider: "+provider.Provider)
	}
	if provider.Model != "" {
		parts = append(parts, "model: "+provider.Model)
	}
	if provider.Endpoint != "" {
		parts = append(parts, "endpoint: "+provider.Endpoint)
	}
	if provider.TimeoutMS > 0 {
		parts = append(parts, fmt.Sprintf("timeout_ms: %d", provider.TimeoutMS))
	}
	if provider.MaxRetries > 0 {
		parts = append(parts, fmt.Sprintf("max_retries: %d", provider.MaxRetries))
	}
	return strings.Join(parts, " | ")
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
