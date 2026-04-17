package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
)

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
	if suggestions := complianceTopSuggestions(result); len(suggestions) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %s %d\n", p.white("TOP SUGGESTIONS:"), len(suggestions))
		for _, suggestion := range suggestions {
			fmt.Fprintf(w, "     %s %s\n", p.arrow(), truncateRenderLine(suggestion, 160))
		}
	}
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
			fmt.Fprintf(w, "     %s %s\n", p.dim("limiting factor"), humanizeComplianceLimitingFactor(item.LimitingFactor))
		}
		if item.Suggestion != "" {
			fmt.Fprintf(w, "     %s %s\n", p.arrow(), item.Suggestion)
		}
	}
}

func complianceTopSuggestions(result *analysis.ComplianceResult) []string {
	if result == nil {
		return nil
	}
	if len(result.TopSuggestions) > 0 {
		return result.TopSuggestions
	}

	seen := make(map[string]struct{})
	suggestions := make([]string, 0, 3)
	appendSuggestions := func(findings []analysis.ComplianceFinding) {
		for _, finding := range findings {
			suggestion := strings.TrimSpace(finding.Suggestion)
			if suggestion == "" {
				continue
			}
			if _, ok := seen[suggestion]; ok {
				continue
			}
			seen[suggestion] = struct{}{}
			suggestions = append(suggestions, suggestion)
			if len(suggestions) == 3 {
				return
			}
		}
	}

	appendSuggestions(result.Conflicts)
	if len(suggestions) < 3 {
		appendSuggestions(result.Unspecified)
	}
	if len(suggestions) < 3 {
		appendSuggestions(result.Compliant)
	}
	return suggestions
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

func humanizeComplianceLimitingFactor(factor string) string {
	switch factor {
	case "spec_metadata_gap":
		return "accepted spec metadata is missing explicit applies_to coverage"
	case "code_evidence_gap":
		return "the file or diff does not expose enough literal code evidence to confirm compliance"
	default:
		return factor
	}
}
