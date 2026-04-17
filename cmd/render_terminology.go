package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
)

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
