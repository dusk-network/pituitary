package analysis

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
)

var requestsPerMinutePattern = regexp.MustCompile(`(?i)(\d+)\s+requests per minute`)

type docDocument struct {
	Record   model.DocRecord
	Sections []embeddedSection
}

// DocDriftRequest is the normalized doc-drift input.
type DocDriftRequest struct {
	DocRef  string   `json:"doc_ref,omitempty"`
	DocRefs []string `json:"doc_refs,omitempty"`
	Scope   string   `json:"scope,omitempty"`
}

// DocDriftScope reports the normalized selector.
type DocDriftScope struct {
	Mode    string   `json:"mode"`
	DocRefs []string `json:"doc_refs"`
}

// DriftFinding reports one contradiction between a doc and a spec.
type DriftFinding struct {
	SpecRef  string `json:"spec_ref"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Expected string `json:"expected,omitempty"`
	Observed string `json:"observed,omitempty"`
}

// DriftItem reports one doc that drifts from accepted specs.
type DriftItem struct {
	DocRef    string         `json:"doc_ref"`
	Title     string         `json:"title"`
	SourceRef string         `json:"source_ref"`
	SpecRefs  []string       `json:"spec_refs"`
	Findings  []DriftFinding `json:"findings"`
}

// DocRemediationEvidence separates the observed contradiction from the accepted spec evidence.
type DocRemediationEvidence struct {
	SpecSection string `json:"spec_section,omitempty"`
	SpecExcerpt string `json:"spec_excerpt,omitempty"`
	DocSection  string `json:"doc_section,omitempty"`
	DocExcerpt  string `json:"doc_excerpt,omitempty"`
	Expected    string `json:"expected,omitempty"`
	Observed    string `json:"observed,omitempty"`
}

// DocSuggestedEdit is one actionable update recommendation.
type DocSuggestedEdit struct {
	Action  string `json:"action"`
	Replace string `json:"replace,omitempty"`
	With    string `json:"with,omitempty"`
	Note    string `json:"note,omitempty"`
}

// DocRemediationSuggestion is one actionable guidance item derived from a drift finding.
type DocRemediationSuggestion struct {
	SpecRef       string                 `json:"spec_ref"`
	Code          string                 `json:"code"`
	Summary       string                 `json:"summary"`
	Evidence      DocRemediationEvidence `json:"evidence"`
	SuggestedEdit DocSuggestedEdit       `json:"suggested_edit"`
}

// DocRemediationItem groups all remediation suggestions for one drifting doc.
type DocRemediationItem struct {
	DocRef      string                     `json:"doc_ref"`
	Title       string                     `json:"title"`
	SourceRef   string                     `json:"source_ref"`
	Suggestions []DocRemediationSuggestion `json:"suggestions"`
}

// DocRemediationResult is the structured remediation payload shared by doc-drift and review-spec.
type DocRemediationResult struct {
	Items []DocRemediationItem `json:"items"`
}

// DocDriftResult is the structured doc-drift response.
type DocDriftResult struct {
	Scope       DocDriftScope         `json:"scope"`
	DriftItems  []DriftItem           `json:"drift_items"`
	Remediation *DocRemediationResult `json:"remediation"`
}

type normalizedClaims struct {
	Window          string
	Subject         string
	DefaultLimit    int
	HasDefaultLimit bool
	Overrides       *bool
}

// CheckDocDrift detects contradictory docs within a target scope.
func CheckDocDrift(cfg *config.Config, request DocDriftRequest) (*DocDriftResult, error) {
	return CheckDocDriftContext(context.Background(), cfg, request)
}

// CheckDocDriftContext detects contradictory docs within a target scope.
func CheckDocDriftContext(ctx context.Context, cfg *config.Config, request DocDriftRequest) (*DocDriftResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	scope, err := normalizeDocDriftScope(request)
	if err != nil {
		return nil, err
	}

	repo, err := openAnalysisRepositoryContext(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer repo.Close()

	selectedDocs, err := repo.loadDocs(scope.DocRefs)
	if err != nil {
		return nil, err
	}
	if len(scope.DocRefs) > 0 {
		for _, ref := range scope.DocRefs {
			if _, ok := selectedDocs[ref]; !ok {
				return nil, newDocRefNotFoundError(ref)
			}
		}
	}

	specRefs, err := repo.relevantDocDriftSpecRefs(selectedDocs)
	if err != nil {
		return nil, err
	}
	specs, err := repo.loadSelectedSpecs(specRefs)
	if err != nil {
		return nil, err
	}

	return buildDocDriftResult(scope, selectedDocs, specs), nil
}

func buildDocDriftResult(scope DocDriftScope, selectedDocs map[string]docDocument, specs map[string]specDocument) *DocDriftResult {
	driftItems := make([]DriftItem, 0, len(selectedDocs))
	remediationItems := make([]DocRemediationItem, 0, len(selectedDocs))
	for _, ref := range sortedDocRefs(selectedDocs) {
		doc := selectedDocs[ref]
		relevant := relevantAcceptedSpecs(doc, specs)
		item, remediation := driftAgainstAcceptedSpecs(doc, relevant)
		if item == nil {
			continue
		}
		driftItems = append(driftItems, *item)
		if remediation != nil {
			remediationItems = append(remediationItems, *remediation)
		}
	}

	return &DocDriftResult{
		Scope:      scope,
		DriftItems: driftItems,
		Remediation: &DocRemediationResult{
			Items: remediationItems,
		},
	}
}

func normalizeDocDriftScope(request DocDriftRequest) (DocDriftScope, error) {
	hasDocRef := stringsTrimSpace(request.DocRef) != ""
	docRefs := uniqueStrings(request.DocRefs)
	hasDocRefs := len(docRefs) > 0
	hasScope := stringsTrimSpace(request.Scope) != ""

	count := 0
	if hasDocRef {
		count++
	}
	if hasDocRefs {
		count++
	}
	if hasScope {
		count++
	}
	if count != 1 {
		return DocDriftScope{}, fmt.Errorf("exactly one of doc_ref, doc_refs, or scope is required")
	}

	switch {
	case hasDocRef:
		return DocDriftScope{Mode: "doc_ref", DocRefs: []string{stringsTrimSpace(request.DocRef)}}, nil
	case hasDocRefs:
		return DocDriftScope{Mode: "doc_refs", DocRefs: docRefs}, nil
	default:
		if stringsTrimSpace(request.Scope) != "all" {
			return DocDriftScope{}, fmt.Errorf("scope %q is invalid", request.Scope)
		}
		return DocDriftScope{Mode: "all", DocRefs: nil}, nil
	}
}

func loadIndexedDocs(db *sql.DB, refs []string) (map[string]docDocument, error) {
	return loadIndexedDocsContext(context.Background(), db, refs)
}

func loadIndexedDocsContext(ctx context.Context, db *sql.DB, refs []string) (map[string]docDocument, error) {
	rows, err := loadIndexedDocRowsContext(ctx, db, refs)
	if err != nil {
		return nil, err
	}

	docs := make(map[string]docDocument, len(rows))
	for _, row := range rows {
		docs[row.Ref] = docDocument{
			Record: model.DocRecord{
				Ref:        row.Ref,
				Kind:       model.ArtifactKindDoc,
				Title:      row.Title,
				SourceRef:  row.SourceRef,
				BodyFormat: model.BodyFormatMarkdown,
			},
		}
	}
	if err := loadDocSectionsContext(ctx, db, docs); err != nil {
		return nil, err
	}
	return docs, nil
}

func loadIndexedDocRows(db *sql.DB, refs []string) ([]indexedArtifactRow, error) {
	return loadIndexedDocRowsContext(context.Background(), db, refs)
}

func loadIndexedDocRowsContext(ctx context.Context, db *sql.DB, refs []string) ([]indexedArtifactRow, error) {
	var builder strings.Builder
	args := make([]any, 0, len(refs)+1)
	builder.WriteString(`
SELECT ref, title, '', '', source_ref
FROM artifacts
WHERE kind = ?`)
	args = append(args, model.ArtifactKindDoc)
	if len(refs) > 0 {
		builder.WriteString(" AND ref IN (")
		for i, ref := range refs {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString("?")
			args = append(args, ref)
		}
		builder.WriteString(")")
	}

	rows, err := db.QueryContext(ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query indexed docs: %w", err)
	}
	defer rows.Close()

	var result []indexedArtifactRow
	for rows.Next() {
		var row indexedArtifactRow
		if err := rows.Scan(&row.Ref, &row.Title, &row.Status, &row.Domain, &row.SourceRef); err != nil {
			return nil, fmt.Errorf("scan indexed doc: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexed docs: %w", err)
	}
	return result, nil
}

func loadDocSections(db *sql.DB, docs map[string]docDocument) error {
	return loadDocSectionsContext(context.Background(), db, docs)
}

func loadDocSectionsContext(ctx context.Context, db *sql.DB, docs map[string]docDocument) error {
	refs := sortedDocRefs(docs)
	if len(refs) == 0 {
		return nil
	}

	var builder strings.Builder
	args := make([]any, 0, 1+len(refs))
	builder.WriteString(`
SELECT c.artifact_ref, c.section, c.content, cv.embedding
FROM chunks c
JOIN chunks_vec cv ON cv.chunk_id = c.id
JOIN artifacts a ON a.ref = c.artifact_ref
WHERE a.kind = ?`)
	args = append(args, model.ArtifactKindDoc)
	appendRefFilterClause(&builder, &args, "c.artifact_ref", refs)
	builder.WriteString(`
ORDER BY c.id`)

	rows, err := db.QueryContext(ctx, builder.String(), args...)
	if err != nil {
		return fmt.Errorf("query doc sections: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			ref           string
			heading       string
			content       string
			embeddingBlob []byte
		)
		if err := rows.Scan(&ref, &heading, &content, &embeddingBlob); err != nil {
			return fmt.Errorf("scan doc section: %w", err)
		}
		document, ok := docs[ref]
		if !ok {
			continue
		}
		embedding, err := index.DecodeVectorBlob(embeddingBlob)
		if err != nil {
			return fmt.Errorf("decode doc embedding for %s: %w", ref, err)
		}
		document.Sections = append(document.Sections, embeddedSection{
			Heading:   heading,
			Content:   content,
			Embedding: embedding,
		})
		docs[ref] = document
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate doc sections: %w", err)
	}
	return nil
}

func sortedDocRefs(docs map[string]docDocument) []string {
	refs := make([]string, 0, len(docs))
	for ref := range docs {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

func driftAgainstAcceptedSpecs(doc docDocument, relevant []specDocument) (*DriftItem, *DocRemediationItem) {
	if len(relevant) == 0 {
		return nil, nil
	}

	docClaims := claimsFromText(joinDocumentText(doc.Sections))
	var (
		specRefs []string
		findings []DriftFinding
		byRef    = make(map[string]specDocument, len(relevant))
	)
	for _, spec := range relevant {
		byRef[spec.Record.Ref] = spec
		specRefs = append(specRefs, spec.Record.Ref)
		findings = append(findings, contradictingFindings(docClaims, claimsFromText(joinDocumentText(spec.Sections)), spec.Record.Ref)...)
	}
	if len(findings) == 0 {
		return nil, nil
	}

	item := &DriftItem{
		DocRef:    doc.Record.Ref,
		Title:     doc.Record.Title,
		SourceRef: doc.Record.SourceRef,
		SpecRefs:  uniqueStrings(specRefs),
		Findings:  findings,
	}
	return item, buildDocRemediationItem(doc, byRef, findings)
}

func buildDocRemediationItem(doc docDocument, specs map[string]specDocument, findings []DriftFinding) *DocRemediationItem {
	suggestions := make([]DocRemediationSuggestion, 0, len(findings))
	for _, finding := range findings {
		spec, ok := specs[finding.SpecRef]
		if !ok {
			continue
		}
		suggestions = append(suggestions, DocRemediationSuggestion{
			SpecRef:       finding.SpecRef,
			Code:          finding.Code,
			Summary:       remediationSummaryForFinding(finding),
			Evidence:      remediationEvidence(doc, spec, finding),
			SuggestedEdit: suggestedEditForFinding(finding),
		})
	}
	if len(suggestions) == 0 {
		return nil
	}
	return &DocRemediationItem{
		DocRef:      doc.Record.Ref,
		Title:       doc.Record.Title,
		SourceRef:   doc.Record.SourceRef,
		Suggestions: suggestions,
	}
}

func relevantAcceptedSpecs(doc docDocument, specs map[string]specDocument) []specDocument {
	type scoredSpec struct {
		spec  specDocument
		score float64
	}
	var scored []scoredSpec
	for _, spec := range specs {
		if spec.Record.Status != model.StatusAccepted {
			continue
		}
		score := documentSimilarity(doc.Sections, spec.Sections)
		if score < 0.35 {
			continue
		}
		scored = append(scored, scoredSpec{spec: spec, score: score})
	}
	sort.Slice(scored, func(i, j int) bool {
		switch {
		case scored[i].score != scored[j].score:
			return scored[i].score > scored[j].score
		default:
			return scored[i].spec.Record.Ref < scored[j].spec.Record.Ref
		}
	})

	result := make([]specDocument, 0, len(scored))
	for _, item := range scored {
		result = append(result, item.spec)
	}
	return result
}

func joinDocumentText(sections []embeddedSection) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		if trimmed := strings.TrimSpace(section.Content); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, "\n")
}

func claimsFromText(text string) normalizedClaims {
	claims := normalizedClaims{}
	lower := strings.ToLower(text)

	switch {
	case strings.Contains(lower, "sliding-window"):
		claims.Window = "sliding-window"
	case strings.Contains(lower, "fixed-window"):
		claims.Window = "fixed-window"
	}

	switch {
	case strings.Contains(lower, "per tenant"), strings.Contains(lower, "tenant-scoped"), strings.Contains(lower, "tenant-specific"):
		claims.Subject = "tenant"
	case strings.Contains(lower, "per api key"), strings.Contains(lower, "api key"):
		claims.Subject = "api_key"
	}

	if matches := requestsPerMinutePattern.FindStringSubmatch(lower); len(matches) == 2 {
		if value, err := strconv.Atoi(matches[1]); err == nil {
			claims.DefaultLimit = value
			claims.HasDefaultLimit = true
		}
	}

	switch {
	case strings.Contains(lower, "tenant-specific overrides are not supported"),
		strings.Contains(lower, "all tenants share the same rate-limit configuration"):
		value := false
		claims.Overrides = &value
	case strings.Contains(lower, "tenant-specific overrides through configuration"),
		strings.Contains(lower, "tenant override"),
		strings.Contains(lower, "tenant overrides"):
		value := true
		claims.Overrides = &value
	}

	return claims
}

func contradictingFindings(docClaims, specClaims normalizedClaims, specRef string) []DriftFinding {
	var findings []DriftFinding
	if docClaims.Window != "" && specClaims.Window != "" && docClaims.Window != specClaims.Window {
		findings = append(findings, DriftFinding{
			SpecRef:  specRef,
			Code:     "window_mismatch",
			Message:  "document describes a different rate-limiter window",
			Expected: specClaims.Window,
			Observed: docClaims.Window,
		})
	}
	if docClaims.Subject != "" && specClaims.Subject != "" && docClaims.Subject != specClaims.Subject {
		findings = append(findings, DriftFinding{
			SpecRef:  specRef,
			Code:     "subject_mismatch",
			Message:  "document targets a different rate-limit subject",
			Expected: specClaims.Subject,
			Observed: docClaims.Subject,
		})
	}
	if docClaims.HasDefaultLimit && specClaims.HasDefaultLimit && docClaims.DefaultLimit != specClaims.DefaultLimit {
		findings = append(findings, DriftFinding{
			SpecRef:  specRef,
			Code:     "default_limit_mismatch",
			Message:  "document reports a different default limit",
			Expected: strconv.Itoa(specClaims.DefaultLimit),
			Observed: strconv.Itoa(docClaims.DefaultLimit),
		})
	}
	if docClaims.Overrides != nil && specClaims.Overrides != nil && *docClaims.Overrides != *specClaims.Overrides {
		findings = append(findings, DriftFinding{
			SpecRef:  specRef,
			Code:     "override_support_mismatch",
			Message:  "document disagrees on tenant override support",
			Expected: strconv.FormatBool(*specClaims.Overrides),
			Observed: strconv.FormatBool(*docClaims.Overrides),
		})
	}
	return findings
}

func remediationSummaryForFinding(finding DriftFinding) string {
	switch finding.Code {
	case "window_mismatch":
		return "replace the stale limiter-window wording with the accepted window model"
	case "subject_mismatch":
		return "update the subject wording so the doc describes tenant-scoped limits"
	case "default_limit_mismatch":
		return "update the documented default rate limit to the accepted value"
	case "override_support_mismatch":
		return "replace the stale override-support statement with the accepted configuration behavior"
	default:
		return finding.Message
	}
}

func remediationEvidence(doc docDocument, spec specDocument, finding DriftFinding) DocRemediationEvidence {
	docSection, docExcerpt := docEvidenceForFinding(doc, finding)
	specSection, specExcerpt := specEvidenceForFinding(spec, finding)
	return DocRemediationEvidence{
		SpecSection: specSection,
		SpecExcerpt: specExcerpt,
		DocSection:  docSection,
		DocExcerpt:  docExcerpt,
		Expected:    humanizedDriftValue(finding.Code, finding.Expected),
		Observed:    humanizedDriftValue(finding.Code, finding.Observed),
	}
}

func suggestedEditForFinding(finding DriftFinding) DocSuggestedEdit {
	switch finding.Code {
	case "window_mismatch":
		return DocSuggestedEdit{
			Action:  "replace_claim",
			Replace: humanizedDriftValue(finding.Code, finding.Observed),
			With:    humanizedDriftValue(finding.Code, finding.Expected),
			Note:    "Update the limiter-window description to match the accepted design.",
		}
	case "subject_mismatch":
		return DocSuggestedEdit{
			Action:  "replace_claim",
			Replace: humanizedDriftValue(finding.Code, finding.Observed),
			With:    humanizedDriftValue(finding.Code, finding.Expected),
			Note:    "Describe rate limits in terms of tenants, not API keys.",
		}
	case "default_limit_mismatch":
		return DocSuggestedEdit{
			Action:  "replace_claim",
			Replace: humanizedDriftValue(finding.Code, finding.Observed),
			With:    humanizedDriftValue(finding.Code, finding.Expected),
			Note:    "Bring the documented default limit back in line with the accepted spec.",
		}
	case "override_support_mismatch":
		return DocSuggestedEdit{
			Action:  "replace_claim",
			Replace: humanizedDriftValue(finding.Code, finding.Observed),
			With:    humanizedDriftValue(finding.Code, finding.Expected),
			Note:    "Document the accepted override behavior instead of the stale configuration guidance.",
		}
	default:
		return DocSuggestedEdit{
			Action: "update_section",
			Note:   finding.Message,
		}
	}
}

func humanizedDriftValue(code, value string) string {
	switch code {
	case "window_mismatch":
		switch value {
		case "sliding-window":
			return "sliding-window rate limiter"
		case "fixed-window":
			return "fixed-window rate limiter"
		}
	case "subject_mismatch":
		switch value {
		case "tenant":
			return "tenant-scoped limits"
		case "api_key":
			return "API-key-scoped limits"
		}
	case "default_limit_mismatch":
		if value != "" {
			return value + " requests per minute"
		}
	case "override_support_mismatch":
		switch value {
		case "true":
			return "tenant-specific overrides are supported through configuration"
		case "false":
			return "tenant-specific overrides are not supported"
		}
	}
	return value
}

func docEvidenceForFinding(doc docDocument, finding DriftFinding) (string, string) {
	keywords := evidenceKeywordsForFinding(finding)
	for _, section := range doc.Sections {
		if excerpt, ok := sectionExcerptForKeywords(section, keywords); ok {
			return section.Heading, excerpt
		}
	}
	return "", ""
}

func specEvidenceForFinding(spec specDocument, finding DriftFinding) (string, string) {
	keywords := evidenceKeywordsForFinding(finding)
	for _, section := range spec.Sections {
		if excerpt, ok := sectionExcerptForKeywords(section, keywords); ok {
			return section.Heading, excerpt
		}
	}
	return "", ""
}

func evidenceKeywordsForFinding(finding DriftFinding) []string {
	switch finding.Code {
	case "window_mismatch":
		return []string{"window", "sliding-window", "fixed-window", "sliding window", "fixed window"}
	case "subject_mismatch":
		return []string{"tenant", "api key", "tenant-scoped", "api-key"}
	case "default_limit_mismatch":
		return []string{"requests per minute", finding.Expected, finding.Observed}
	case "override_support_mismatch":
		return []string{"override", "overrides", "configuration"}
	default:
		return []string{finding.Expected, finding.Observed}
	}
}

func sectionExcerptForKeywords(section embeddedSection, keywords []string) (string, bool) {
	lines := strings.Split(section.Content, "\n")
	for _, line := range lines {
		trimmed := stringsTrimSpace(strings.TrimPrefix(line, "- "))
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		for _, keyword := range keywords {
			keyword = strings.ToLower(stringsTrimSpace(keyword))
			if keyword == "" {
				continue
			}
			if strings.Contains(lower, keyword) {
				return trimmed, true
			}
		}
	}
	return "", false
}

func stringsTrimSpace(value string) string {
	return strings.TrimSpace(value)
}

func stringsHasPrefix(value, prefix string) bool {
	return strings.HasPrefix(value, prefix)
}
