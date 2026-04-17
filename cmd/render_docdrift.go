package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
)

func renderDocDriftResult(w io.Writer, result *analysis.DocDriftResult) {
	p := presentationForWriter(w)
	fmt.Fprintln(w, p.headerLine("check-doc-drift", ""))
	fmt.Fprintln(w)

	if len(result.DriftItems) == 0 && len(result.Assessments) == 0 && len(result.ChangedFiles) == 0 && len(result.ImplicatedSpecs) == 0 && len(result.ImplicatedDocs) == 0 {
		fmt.Fprintf(w, "  %s no drift items\n", p.check())
		return
	}
	if len(result.ChangedFiles) > 0 {
		fmt.Fprintf(w, "  %s %d\n", p.white("CHANGED FILES"), len(result.ChangedFiles))
		for i, file := range result.ChangedFiles {
			fmt.Fprintf(w, "  %s %s", p.treeBranch(i == len(result.ChangedFiles)-1), file.Path)
			if file.AddedLineCount > 0 || file.RemovedLineCount > 0 {
				fmt.Fprintf(w, " %s", p.dim(fmt.Sprintf("(+%d/-%d)", file.AddedLineCount, file.RemovedLineCount)))
			}
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w)
	}
	if len(result.ImplicatedSpecs) > 0 {
		fmt.Fprintf(w, "  %s %d\n", p.white("IMPLICATED SPECS"), len(result.ImplicatedSpecs))
		for i, item := range result.ImplicatedSpecs {
			line := item.Ref
			if item.Title != "" {
				line += " · " + item.Title
			}
			if item.Repo != "" {
				line += " · repo: " + item.Repo
			}
			if len(item.Reasons) > 0 {
				line += " · " + item.Reasons[0]
			}
			fmt.Fprintf(w, "  %s %s\n", p.treeBranch(i == len(result.ImplicatedSpecs)-1), line)
		}
		fmt.Fprintln(w)
	}
	if len(result.ImplicatedDocs) > 0 {
		fmt.Fprintf(w, "  %s %d\n", p.white("IMPLICATED DOCS"), len(result.ImplicatedDocs))
		for i, item := range result.ImplicatedDocs {
			label := repoPathLabel(item.Repo, preferredDocLabel(item.DocRef, item.SourceRef))
			line := fmt.Sprintf("%s  %.3f", label, item.Score)
			if len(item.Reasons) > 0 {
				line += " · " + item.Reasons[0]
			}
			fmt.Fprintf(w, "  %s %s\n", p.treeBranch(i == len(result.ImplicatedDocs)-1), line)
		}
		fmt.Fprintln(w)
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
		docLabel := repoPathLabel(assessment.Repo, preferredDocLabel(assessment.DocRef, assessment.SourceRef))
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
			pathArg := preferredDocLabel(assessment.DocRef, assessment.SourceRef)
			if assessment.SourceRef != "" {
				pathArg = displaySourcePath(assessment.SourceRef)
			}
			if isNonPrimaryRepoDoc(assessment.DocRef, assessment.Repo) {
				fmt.Fprintf(w, "\n    %s  deterministic remediation is available, but `pituitary fix --path` only targets primary-workspace docs; inspect %s manually\n", p.info(), p.cyan(docLabel))
				fmt.Fprintf(w, "    %s  run `pituitary review-spec --format html --path <spec>` for the full evidence report\n", p.info())
			} else {
				fmt.Fprintf(w, "\n    %s pituitary fix --path %s %s\n", p.green("fix:"), pathArg, p.dim(fmt.Sprintf("(%d edits)", len(suggestions))))
				fmt.Fprintf(w, "    %s  run `pituitary review-spec --format html --path <spec>` for the full evidence report\n", p.info())
			}
		} else if assessment.Status == "drift" || assessment.Status == "possible_drift" {
			fmt.Fprintf(w, "\n    %s  run `pituitary review-spec --format html --path <spec>` for the full evidence chain (no deterministic fix available)\n", p.info())
		}
	}
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
	summary := strings.TrimSpace(finding.Message)
	if strings.TrimSpace(finding.Code) != "" {
		summary = humanizeSymbol(finding.Code)
	}
	if finding.Classification == "role_mismatch" {
		return "role mismatch · " + summary
	}
	return summary
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
			Repo:      item.Repo,
			SourceRef: item.SourceRef,
			Status:    "drift",
			SpecRefs:  append([]string(nil), item.SpecRefs...),
		})
	}
	return result
}
