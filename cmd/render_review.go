package cmd

import (
	"fmt"
	htemplate "html/template"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
)

const reviewHTMLStyles = `<style>
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
</style>`

func renderReviewMarkdownSpecSection(w io.Writer, result *analysis.ReviewResult) {
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
}

func renderReviewMarkdownSummarySection(w io.Writer, result *analysis.ReviewResult) {
	fmt.Fprintf(w, "\n## Summary\n\n")
	for _, line := range reviewMarkdownSummary(result) {
		fmt.Fprintf(w, "- %s\n", line)
	}
}

func renderReviewMarkdownActionsSection(w io.Writer, result *analysis.ReviewResult) {
	fmt.Fprintf(w, "\n## Recommended Next Actions\n\n")
	actions := reviewMarkdownActions(result)
	if len(actions) == 0 {
		fmt.Fprintln(w, "- No immediate follow-up identified from the current review.")
		return
	}
	for i, action := range actions {
		fmt.Fprintf(w, "%d. %s\n", i+1, action)
	}
}

func renderReviewMarkdownOverlapSection(w io.Writer, result *analysis.ReviewResult) {
	fmt.Fprintf(w, "\n## Overlap\n\n")
	if result.Overlap == nil {
		fmt.Fprintln(w, "No overlap analysis was generated.")
		return
	}

	fmt.Fprintf(w, "- Posture: `%s`", result.Overlap.Recommendation)
	if detail := humanizeOverlapRecommendation(result.Overlap.Recommendation); detail != "" {
		fmt.Fprintf(w, " (%s)", detail)
	}
	fmt.Fprintln(w)
	if len(result.Overlap.Overlaps) == 0 {
		fmt.Fprintln(w, "- No overlapping specs detected.")
		return
	}

	for i, item := range result.Overlap.Overlaps {
		label := "Related overlap"
		if i == 0 {
			label = "Primary overlap"
		}
		fmt.Fprintf(w, "- %s: `%s` %s (%s, %.3f, %s)\n", label, item.Ref, item.Title, item.Relationship, item.Score, humanizeOverlapGuidance(item.Guidance))
	}
}

func renderReviewMarkdownComparisonSection(w io.Writer, result *analysis.ReviewResult) {
	fmt.Fprintf(w, "\n## Comparison\n\n")
	if result.Comparison == nil {
		fmt.Fprintln(w, "No comparison was generated because no primary comparison target was shortlisted.")
		return
	}

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

func renderReviewMarkdownImpactSection(w io.Writer, result *analysis.ReviewResult) {
	fmt.Fprintf(w, "\n## Impact\n\n")
	if result.Impact == nil {
		fmt.Fprintln(w, "No impact analysis generated.")
		return
	}

	fmt.Fprintf(w, "- Summary: %d impacted spec(s), %d governed ref(s), %d impacted doc(s)\n", len(result.Impact.AffectedSpecs), len(result.Impact.AffectedRefs), len(result.Impact.AffectedDocs))
	renderReviewMarkdownImpactedSpecs(w, result.Impact)
	renderReviewMarkdownImpactedDocs(w, result.Impact)
}

func renderReviewMarkdownImpactedSpecs(w io.Writer, result *analysis.AnalyzeImpactResult) {
	if len(result.AffectedSpecs) == 0 {
		fmt.Fprintln(w, "- Impacted specs: none")
		return
	}

	fmt.Fprintln(w, "- Top impacted specs:")
	for _, item := range topImpactedSpecs(result.AffectedSpecs, 3) {
		fmt.Fprintf(w, "  - `%s` %s (%s", item.Ref, item.Title, item.Relationship)
		if item.Repo != "" {
			fmt.Fprintf(w, ", repo %s", item.Repo)
		}
		if item.Historical {
			fmt.Fprint(w, ", historical")
		}
		fmt.Fprintln(w, ")")
	}
	if extra := len(result.AffectedSpecs) - minInt(len(result.AffectedSpecs), 3); extra > 0 {
		fmt.Fprintf(w, "  - `%d` more impacted spec(s)\n", extra)
	}
}

func renderReviewMarkdownImpactedDocs(w io.Writer, result *analysis.AnalyzeImpactResult) {
	if len(result.AffectedDocs) == 0 {
		fmt.Fprintln(w, "- Impacted docs: none")
		return
	}

	fmt.Fprintln(w, "- Top impacted docs:")
	for _, item := range topImpactedDocs(result.AffectedDocs, 3) {
		fmt.Fprintf(w, "  - `%s` %s (score %.3f", item.Ref, item.Title, item.Score)
		if item.Classification != "" {
			fmt.Fprintf(w, ", %s", item.Classification)
		}
		if item.Repo != "" {
			fmt.Fprintf(w, ", repo %s", item.Repo)
		}
		if item.SourceRef != "" {
			fmt.Fprintf(w, ", %s", item.SourceRef)
		}
		fmt.Fprintln(w, ")")
		if item.Evidence != nil {
			fmt.Fprintf(
				w,
				"    - Evidence: %s / %s -> %s / %s\n",
				item.Evidence.SpecRef,
				displayDefault(item.Evidence.SpecSection, "(body)"),
				displaySourcePath(item.Evidence.DocSourceRef),
				displayDefault(item.Evidence.DocSection, "(body)"),
			)
		}
		if len(item.SuggestedTargets) > 0 {
			target := item.SuggestedTargets[0]
			fmt.Fprintf(w, "    - Suggested target: %s", target.SourceRef)
			if target.Section != "" {
				fmt.Fprintf(w, " / %s", target.Section)
			}
			fmt.Fprintln(w)
		}
	}
	if extra := len(result.AffectedDocs) - minInt(len(result.AffectedDocs), 3); extra > 0 {
		fmt.Fprintf(w, "  - `%d` more impacted doc(s)\n", extra)
	}
}

func renderReviewMarkdownDocDriftSection(w io.Writer, result *analysis.ReviewResult) {
	fmt.Fprintf(w, "\n## Doc Drift\n\n")
	driftAssessments := reviewDocDriftAssessments(result.DocDrift)
	if len(driftAssessments) == 0 {
		fmt.Fprintln(w, "No drifting docs detected.")
		return
	}

	fmt.Fprintf(w, "- Summary: %d doc(s) need follow-up\n", len(driftAssessments))
	driftItems := driftItemsByDocRef(result.DocDrift.DriftItems)
	for _, assessment := range driftAssessments {
		renderReviewMarkdownDriftAssessment(w, assessment, driftItems[assessment.DocRef])
	}
}

func renderReviewMarkdownDriftAssessment(w io.Writer, assessment analysis.DocDriftAssessment, item analysis.DriftItem) {
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
	for _, finding := range item.Findings {
		renderReviewMarkdownDriftFinding(w, finding)
	}
	fmt.Fprintln(w)
}

func renderReviewMarkdownDriftFinding(w io.Writer, finding analysis.DriftFinding) {
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

func renderReviewMarkdownDocRemediationSection(w io.Writer, result *analysis.ReviewResult) {
	fmt.Fprintf(w, "## Doc Remediation\n\n")
	if result.DocRemediation == nil || len(result.DocRemediation.Items) == 0 {
		fmt.Fprintln(w, "No remediation guidance.")
		return
	}

	fmt.Fprintf(w, "- Summary: %d suggested update(s)\n", reviewRemediationSuggestionCount(result.DocRemediation))
	for _, item := range result.DocRemediation.Items {
		renderReviewMarkdownRemediationItem(w, item)
	}
}

func renderReviewMarkdownRemediationItem(w io.Writer, item analysis.DocRemediationItem) {
	fmt.Fprintf(w, "### `%s`\n\n", item.DocRef)
	if item.Title != "" {
		fmt.Fprintf(w, "- Title: %s\n", item.Title)
	}
	if item.SourceRef != "" {
		fmt.Fprintf(w, "- Source: %s\n", item.SourceRef)
	}
	for _, suggestion := range item.Suggestions {
		fmt.Fprintf(w, "- Update `%s` from `%s`", suggestion.Code, suggestion.SpecRef)
		if suggestion.Classification != "" {
			fmt.Fprintf(w, " [%s]", suggestion.Classification)
		}
		fmt.Fprintf(w, ": %s\n", suggestion.Summary)
		if suggestion.Evidence.SpecExcerpt != "" {
			fmt.Fprintf(w, "  - Evidence: spec says %q", suggestion.Evidence.SpecExcerpt)
			if suggestion.Evidence.DocExcerpt != "" {
				fmt.Fprintf(w, "; doc says %q", suggestion.Evidence.DocExcerpt)
			}
			fmt.Fprintln(w)
		}
		if suggestion.TargetSourceRef != "" || suggestion.TargetSection != "" {
			fmt.Fprintf(w, "  - Target: %s", suggestion.TargetSourceRef)
			if suggestion.TargetSection != "" {
				fmt.Fprintf(w, " / %s", suggestion.TargetSection)
			}
			fmt.Fprintln(w)
		}
		if suggestion.LinkReason != "" {
			fmt.Fprintf(w, "  - Link reason: %s\n", suggestion.LinkReason)
		}
		for _, bullet := range suggestion.SuggestedBullets {
			fmt.Fprintf(w, "  - Next step: %s\n", bullet)
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

func renderReviewHTMLDocumentStart(w io.Writer, result *analysis.ReviewResult, escape func(string) string) {
	fmt.Fprint(w, "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(w, "<title>Pituitary Review Report: %s</title>\n", escape(result.SpecRef))
	fmt.Fprint(w, reviewHTMLStyles+"\n</head>\n<body>\n<main>\n")
}

func renderReviewHTMLHeroSection(w io.Writer, result *analysis.ReviewResult, escape func(string) string) {
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
}

func renderReviewHTMLSummaryGrid(w io.Writer, result *analysis.ReviewResult, escape func(string) string) {
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
}

func renderReviewHTMLStatsSection(w io.Writer, result *analysis.ReviewResult, driftAssessments []analysis.DocDriftAssessment, escape func(string) string) {
	fmt.Fprint(w, "<div class=\"stats\">\n")
	if result.Overlap != nil {
		fmt.Fprintf(w, "<div class=\"stat\"><span class=\"muted\">Overlaps</span><strong>%d</strong><span>%s</span></div>\n", len(result.Overlap.Overlaps), escape(result.Overlap.Recommendation))
	}
	if result.Impact != nil {
		fmt.Fprintf(w, "<div class=\"stat\"><span class=\"muted\">Impacted specs</span><strong>%d</strong><span>%d docs, %d refs</span></div>\n", len(result.Impact.AffectedSpecs), len(result.Impact.AffectedDocs), len(result.Impact.AffectedRefs))
	}
	fmt.Fprintf(w, "<div class=\"stat\"><span class=\"muted\">Docs needing follow-up</span><strong>%d</strong><span>%d remediation item(s)</span></div>\n", len(driftAssessments), reviewRemediationSuggestionCount(result.DocRemediation))
	fmt.Fprint(w, "</div>\n")
}

func renderReviewHTMLOverlapSection(w io.Writer, result *analysis.ReviewResult, escape func(string) string) {
	fmt.Fprint(w, "<section class=\"section\"><h2>Overlap</h2>\n")
	if result.Overlap == nil {
		fmt.Fprint(w, "<p class=\"muted\">No overlap analysis was generated.</p>\n")
		fmt.Fprint(w, "</section>\n")
		return
	}
	if len(result.Overlap.Overlaps) == 0 {
		fmt.Fprintf(w, "<p>No overlapping specs detected. Review posture: <code>%s</code>.</p>\n", escape(result.Overlap.Recommendation))
		fmt.Fprint(w, "</section>\n")
		return
	}

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
	fmt.Fprint(w, "</ul>\n</section>\n")
}

func renderReviewHTMLComparisonSection(w io.Writer, result *analysis.ReviewResult, escape func(string) string) {
	fmt.Fprint(w, "<section class=\"section\"><h2>Comparison</h2>\n")
	if result.Comparison == nil {
		fmt.Fprint(w, "<p class=\"muted\">No primary comparison target was shortlisted.</p>\n")
		fmt.Fprint(w, "</section>\n")
		return
	}

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
	fmt.Fprint(w, "</section>\n")
}

func renderReviewHTMLImpactSection(w io.Writer, result *analysis.ReviewResult, escape func(string) string) {
	fmt.Fprint(w, "<section class=\"section\"><h2>Impact</h2>\n")
	if result.Impact == nil {
		fmt.Fprint(w, "<p class=\"muted\">No impact analysis generated.</p>\n")
		fmt.Fprint(w, "</section>\n")
		return
	}

	fmt.Fprintf(w, "<p>Summary: %d impacted spec(s), %d governed ref(s), %d impacted doc(s).</p>\n", len(result.Impact.AffectedSpecs), len(result.Impact.AffectedRefs), len(result.Impact.AffectedDocs))
	fmt.Fprint(w, "<div class=\"grid two\">")
	renderReviewHTMLImpactedSpecs(w, result.Impact, escape)
	renderReviewHTMLImpactedDocs(w, result.Impact, escape)
	fmt.Fprint(w, "</div></section>\n")
}

func renderReviewHTMLImpactedSpecs(w io.Writer, result *analysis.AnalyzeImpactResult, escape func(string) string) {
	fmt.Fprint(w, "<div><h3>Top impacted specs</h3>")
	specs := topImpactedSpecs(result.AffectedSpecs, 5)
	if len(specs) == 0 {
		fmt.Fprint(w, "<p class=\"muted\">None.</p>")
		fmt.Fprint(w, "</div>")
		return
	}

	fmt.Fprint(w, "<ul class=\"compact-list\">")
	for _, item := range specs {
		fmt.Fprintf(w, "<li><strong><code>%s</code></strong> %s <span class=\"muted\">(%s", escape(item.Ref), escape(item.Title), escape(item.Relationship))
		if item.Historical {
			fmt.Fprint(w, ", historical")
		}
		fmt.Fprint(w, ")</span></li>")
	}
	fmt.Fprint(w, "</ul></div>")
}

func renderReviewHTMLImpactedDocs(w io.Writer, result *analysis.AnalyzeImpactResult, escape func(string) string) {
	fmt.Fprint(w, "<div><h3>Top impacted docs</h3>")
	docs := topImpactedDocs(result.AffectedDocs, 5)
	if len(docs) == 0 {
		fmt.Fprint(w, "<p class=\"muted\">None.</p>")
		fmt.Fprint(w, "</div>")
		return
	}

	fmt.Fprint(w, "<ul class=\"compact-list\">")
	for _, item := range docs {
		fmt.Fprintf(w, "<li><strong><code>%s</code></strong> %s <span class=\"muted\">(score %.3f", escape(item.Ref), escape(item.Title), item.Score)
		if item.Classification != "" {
			fmt.Fprintf(w, ", %s", escape(item.Classification))
		}
		if item.SourceRef != "" {
			fmt.Fprintf(w, ", %s", escape(item.SourceRef))
		}
		fmt.Fprint(w, ")</span>")
		if item.Evidence != nil {
			fmt.Fprintf(w, "<br><span class=\"subtle\">Evidence: %s / %s -> %s / %s</span>",
				escape(item.Evidence.SpecRef),
				escape(displayDefault(item.Evidence.SpecSection, "(body)")),
				escape(item.Evidence.DocSourceRef),
				escape(displayDefault(item.Evidence.DocSection, "(body)")),
			)
		}
		fmt.Fprint(w, "</li>")
	}
	fmt.Fprint(w, "</ul></div>")
}

func renderReviewHTMLDocDriftSection(w io.Writer, result *analysis.ReviewResult, driftAssessments []analysis.DocDriftAssessment, escape func(string) string) {
	fmt.Fprint(w, "<section class=\"section\"><h2>Doc Drift</h2>\n")
	if len(driftAssessments) == 0 {
		fmt.Fprint(w, "<p class=\"muted\">No drifting docs detected.</p>\n")
		fmt.Fprint(w, "</section>\n")
		return
	}

	fmt.Fprintf(w, "<p>%d doc(s) need follow-up.</p>\n", len(driftAssessments))
	driftItems := driftItemsByDocRef(result.DocDrift.DriftItems)
	for _, assessment := range driftAssessments {
		renderReviewHTMLDriftAssessment(w, assessment, driftItems[assessment.DocRef], escape)
	}
	fmt.Fprint(w, "</section>\n")
}

func renderReviewHTMLDriftAssessment(w io.Writer, assessment analysis.DocDriftAssessment, item analysis.DriftItem, escape func(string) string) {
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
	for _, finding := range item.Findings {
		renderReviewHTMLDriftFinding(w, finding, escape)
	}
	fmt.Fprint(w, "</div></details>\n")
}

func renderReviewHTMLDriftFinding(w io.Writer, finding analysis.DriftFinding, escape func(string) string) {
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

func renderReviewHTMLDocRemediationSection(w io.Writer, result *analysis.ReviewResult, escape func(string) string) {
	fmt.Fprint(w, "<section class=\"section\"><h2>Doc Remediation</h2>\n")
	if result.DocRemediation == nil || len(result.DocRemediation.Items) == 0 {
		fmt.Fprint(w, "<p class=\"muted\">No remediation guidance.</p>\n")
		fmt.Fprint(w, "</section>\n")
		return
	}

	fmt.Fprintf(w, "<p>%d suggested update(s).</p>\n", reviewRemediationSuggestionCount(result.DocRemediation))
	for _, item := range result.DocRemediation.Items {
		renderReviewHTMLRemediationItem(w, item, escape)
	}
	fmt.Fprint(w, "</section>\n")
}

func renderReviewHTMLRemediationItem(w io.Writer, item analysis.DocRemediationItem, escape func(string) string) {
	fmt.Fprintf(w, "<details open><summary><code>%s</code>", escape(item.DocRef))
	if item.Title != "" {
		fmt.Fprintf(w, " — %s", escape(item.Title))
	}
	fmt.Fprint(w, "</summary><div class=\"key-value\">")
	if item.SourceRef != "" {
		fmt.Fprintf(w, "<div><strong>Source</strong><br>%s</div>", escape(item.SourceRef))
	}
	for _, suggestion := range item.Suggestions {
		fmt.Fprintf(w, "<div><strong>%s</strong>", escape(suggestion.Code))
		if suggestion.Classification != "" {
			fmt.Fprintf(w, " <span class=\"subtle\">(%s)</span>", escape(suggestion.Classification))
		}
		fmt.Fprintf(w, "<br>%s", escape(suggestion.Summary))
		if suggestion.Evidence.SpecExcerpt != "" || suggestion.Evidence.DocExcerpt != "" {
			fmt.Fprintf(w, "<br>%s", reviewHTMLRemediationEvidence(suggestion.Evidence))
		}
		if suggestion.TargetSourceRef != "" || suggestion.TargetSection != "" {
			fmt.Fprintf(w, "<br><span class=\"subtle\">Target %s", escape(suggestion.TargetSourceRef))
			if suggestion.TargetSection != "" {
				fmt.Fprintf(w, " / %s", escape(suggestion.TargetSection))
			}
			fmt.Fprint(w, "</span>")
		}
		if suggestion.LinkReason != "" {
			fmt.Fprintf(w, "<br><span class=\"subtle\">%s</span>", escape(suggestion.LinkReason))
		}
		if len(suggestion.SuggestedBullets) > 0 {
			fmt.Fprint(w, "<ul class=\"compact-list\">")
			for _, bullet := range suggestion.SuggestedBullets {
				fmt.Fprintf(w, "<li>%s</li>", escape(bullet))
			}
			fmt.Fprint(w, "</ul>")
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

func renderReviewHTMLWarningsSection(w io.Writer, result *analysis.ReviewResult, escape func(string) string) {
	if len(result.Warnings) == 0 {
		return
	}

	fmt.Fprint(w, "<section class=\"section warning\"><h2>Warnings</h2><ul class=\"compact-list\">")
	for _, warning := range result.Warnings {
		fmt.Fprintf(w, "<li><strong>%s</strong>: %s</li>", escape(warning.Code), escape(warning.Message))
	}
	fmt.Fprint(w, "</ul></section>\n")
}

func renderReviewHTMLDocumentEnd(w io.Writer) {
	fmt.Fprint(w, "</main>\n</body>\n</html>\n")
}

var _ = htemplate.HTMLEscapeString
