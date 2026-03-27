package analysis

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
)

var requestsPerMinutePattern = regexp.MustCompile(`(?i)(\d+)\s+requests per minute`)
var artifactReferencePattern = regexp.MustCompile(`(?i)[a-z0-9][a-z0-9._-]*\.(?:db|json|md|toml|yaml|yml)`)

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

// DriftEvidence reports the concrete spec/doc sections that support one drift judgment.
type DriftEvidence struct {
	SpecRef     string `json:"spec_ref,omitempty"`
	SpecTitle   string `json:"spec_title,omitempty"`
	SpecSection string `json:"spec_section,omitempty"`
	SpecExcerpt string `json:"spec_excerpt,omitempty"`
	DocSection  string `json:"doc_section,omitempty"`
	DocExcerpt  string `json:"doc_excerpt,omitempty"`
}

// DriftConfidence reports how certain the analysis is about one judgment.
type DriftConfidence struct {
	Level string  `json:"level"`
	Score float64 `json:"score,omitempty"`
	Basis string  `json:"basis,omitempty"`
}

// DriftFinding reports one contradiction between a doc and a spec.
type DriftFinding struct {
	SpecRef    string           `json:"spec_ref"`
	Artifact   string           `json:"artifact,omitempty"`
	Code       string           `json:"code"`
	Message    string           `json:"message"`
	Rationale  string           `json:"rationale,omitempty"`
	Expected   string           `json:"expected,omitempty"`
	Observed   string           `json:"observed,omitempty"`
	Evidence   *DriftEvidence   `json:"evidence,omitempty"`
	Confidence *DriftConfidence `json:"confidence,omitempty"`
}

// DriftItem reports one doc that drifts from accepted specs.
type DriftItem struct {
	DocRef    string         `json:"doc_ref"`
	Title     string         `json:"title"`
	SourceRef string         `json:"source_ref"`
	SpecRefs  []string       `json:"spec_refs"`
	Findings  []DriftFinding `json:"findings"`
}

// DocDriftAssessment reports how one reviewed doc was judged, even when no deterministic drift was proven.
type DocDriftAssessment struct {
	DocRef     string           `json:"doc_ref"`
	Title      string           `json:"title"`
	SourceRef  string           `json:"source_ref"`
	Status     string           `json:"status"`
	SpecRefs   []string         `json:"spec_refs,omitempty"`
	Rationale  string           `json:"rationale,omitempty"`
	Evidence   *DriftEvidence   `json:"evidence,omitempty"`
	Confidence *DriftConfidence `json:"confidence,omitempty"`
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
	Scope          DocDriftScope         `json:"scope"`
	DriftItems     []DriftItem           `json:"drift_items"`
	Assessments    []DocDriftAssessment  `json:"assessments,omitempty"`
	SpecInferences []SpecInference       `json:"spec_inferences,omitempty"`
	Remediation    *DocRemediationResult `json:"remediation"`
	Warnings       []Warning             `json:"warnings,omitempty"`
}

type normalizedClaims struct {
	Window          string
	Subject         string
	DefaultLimit    int
	HasDefaultLimit bool
	Overrides       *bool
}

type artifactMention struct {
	Artifact string
	Active   bool
	Aligned  bool
}

type artifactConstraint struct {
	Artifact string
	Kind     string
	Expected string
}

type alignedAssessmentCandidate struct {
	spec            *specDocument
	evidence        *DriftEvidence
	score           float64
	docMentionsSpec bool
	headingOverlap  bool
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

	analyzer, err := newQualitativeAnalyzer(cfg.Runtime.Analysis)
	if err != nil {
		return nil, err
	}

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

	return buildDocDriftResult(ctx, analyzer, scope, selectedDocs, specs, repo.loadAllSpecs)
}

func buildDocDriftResult(ctx context.Context, analyzer qualitativeAnalyzer, scope DocDriftScope, selectedDocs map[string]docDocument, specs map[string]specDocument, loadAllSpecs func() (map[string]specDocument, error)) (*DocDriftResult, error) {
	driftItems := make([]DriftItem, 0, len(selectedDocs))
	assessments := make([]DocDriftAssessment, 0, len(selectedDocs))
	remediationItems := make([]DocRemediationItem, 0, len(selectedDocs))
	relevantSpecRefs := make([]string, 0, len(specs))
	warningSpecs := make([]specDocument, 0, len(specs))
	inferenceSpecs := specs
	var allSpecs map[string]specDocument
	ensureAllSpecs := func() (map[string]specDocument, error) {
		if allSpecs != nil || loadAllSpecs == nil {
			return allSpecs, nil
		}
		loaded, err := loadAllSpecs()
		if err != nil {
			return nil, err
		}
		allSpecs = loaded
		inferenceSpecs = loaded
		return allSpecs, nil
	}
	for _, ref := range sortedDocRefs(selectedDocs) {
		doc := selectedDocs[ref]
		relevant := relevantAcceptedSpecs(doc, specs)
		item, remediation := driftAgainstAcceptedSpecs(doc, relevant)
		if item != nil && analyzer != nil {
			relevantByRef := make(map[string]specDocument, len(relevant))
			for _, spec := range relevant {
				relevantByRef[spec.Record.Ref] = spec
			}
			refinedItem, refinedRemediation, err := analyzer.RefineDocDrift(ctx, doc, relevantByRef, *item, remediation)
			if err != nil {
				return nil, err
			}
			if refinedItem != nil {
				item = refinedItem
			}
			if refinedRemediation != nil {
				remediation = refinedRemediation
			}
		}
		if assessment := assessDocDrift(doc, relevant, item); assessment != nil {
			assessments = append(assessments, *assessment)
			relevantSpecRefs = append(relevantSpecRefs, assessment.SpecRefs...)
			for _, specRef := range assessment.SpecRefs {
				if spec, ok := specs[specRef]; ok {
					warningSpecs = append(warningSpecs, spec)
				}
			}
		} else if item == nil {
			allSpecs, err := ensureAllSpecs()
			if err != nil {
				return nil, err
			}
			if assessment := possibleDriftAssessment(doc, allSpecs); assessment != nil {
				assessments = append(assessments, *assessment)
				relevantSpecRefs = append(relevantSpecRefs, assessment.SpecRefs...)
				for _, specRef := range assessment.SpecRefs {
					if spec, ok := allSpecs[specRef]; ok {
						warningSpecs = append(warningSpecs, spec)
					}
				}
			}
		}
		if item == nil {
			continue
		}
		driftItems = append(driftItems, *item)
		if remediation != nil {
			remediationItems = append(remediationItems, *remediation)
		}
		for _, specRef := range item.SpecRefs {
			if spec, ok := specs[specRef]; ok {
				relevantSpecRefs = append(relevantSpecRefs, specRef)
				warningSpecs = append(warningSpecs, spec)
			}
		}
	}
	relevantSpecRefs = uniqueStrings(relevantSpecRefs)

	return &DocDriftResult{
		Scope:          scope,
		DriftItems:     driftItems,
		Assessments:    assessments,
		SpecInferences: buildSpecInferences(inferenceSpecs, relevantSpecRefs),
		Remediation: &DocRemediationResult{
			Items: remediationItems,
		},
		Warnings: buildSpecInferenceWarnings("doc-drift analysis", warningSpecs...),
	}, nil
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
	docArtifacts := artifactMentionsFromSections(doc.Sections)
	var (
		specRefs []string
		findings []DriftFinding
		byRef    = make(map[string]specDocument, len(relevant))
	)
	for _, spec := range relevant {
		byRef[spec.Record.Ref] = spec
		specFindings := contradictingFindings(docClaims, claimsFromText(joinDocumentText(spec.Sections)), spec.Record.Ref)
		specFindings = append(specFindings, artifactDriftFindings(docArtifacts, spec)...)
		if len(specFindings) == 0 {
			continue
		}
		specRefs = append(specRefs, spec.Record.Ref)
		findings = append(findings, specFindings...)
	}
	findings = uniqueDriftFindings(findings)
	if len(findings) == 0 {
		return nil, nil
	}
	for i := range findings {
		spec, ok := byRef[findings[i].SpecRef]
		if !ok {
			continue
		}
		findings[i] = enrichDriftFinding(doc, spec, findings[i])
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

func enrichDriftFinding(doc docDocument, spec specDocument, finding DriftFinding) DriftFinding {
	evidence, score := driftEvidence(doc, spec, finding)
	finding.Evidence = evidence
	finding.Rationale = rationaleForFinding(finding)
	finding.Confidence = confidenceForDriftFinding(finding, score)
	return finding
}

func assessDocDrift(doc docDocument, relevant []specDocument, item *DriftItem) *DocDriftAssessment {
	if item != nil {
		assessment := &DocDriftAssessment{
			DocRef:    item.DocRef,
			Title:     item.Title,
			SourceRef: item.SourceRef,
			Status:    "drift",
			SpecRefs:  append([]string(nil), item.SpecRefs...),
		}
		if finding := topDriftFinding(item.Findings); finding != nil {
			assessment.Rationale = defaultString(stringsTrimSpace(finding.Rationale), finding.Message)
			assessment.Evidence = cloneDriftEvidence(finding.Evidence)
			assessment.Confidence = cloneDriftConfidence(finding.Confidence)
		}
		if assessment.Confidence == nil {
			assessment.Confidence = &DriftConfidence{
				Level: "medium",
				Basis: "deterministic drift findings were emitted for this doc",
			}
		}
		return assessment
	}
	if len(relevant) == 0 {
		return nil
	}
	candidate := bestAlignedAssessmentCandidateForDocs(doc, relevant)
	if !shouldEmitAlignedAssessment(candidate) {
		return nil
	}
	return &DocDriftAssessment{
		DocRef:     doc.Record.Ref,
		Title:      doc.Record.Title,
		SourceRef:  doc.Record.SourceRef,
		Status:     "aligned",
		SpecRefs:   []string{candidate.spec.Record.Ref},
		Rationale:  fmt.Sprintf("matched accepted spec %s and found no deterministic contradiction in the reviewed sections", candidate.spec.Record.Ref),
		Evidence:   candidate.evidence,
		Confidence: &DriftConfidence{Level: alignmentConfidenceLevel(candidate.score), Score: roundScore(candidate.score), Basis: "nearest accepted spec and doc sections agree semantically, but no explicit contradiction was detected"},
	}
}

func possibleDriftAssessment(doc docDocument, specs map[string]specDocument) *DocDriftAssessment {
	candidates := assessmentFallbackSpecs(doc, specs)
	if len(candidates) == 0 {
		return nil
	}
	bestSpec, evidence, score := bestAlignedAssessmentEvidence(doc, candidates)
	if bestSpec == nil || evidence == nil {
		return nil
	}
	return &DocDriftAssessment{
		DocRef:     doc.Record.Ref,
		Title:      doc.Record.Title,
		SourceRef:  doc.Record.SourceRef,
		Status:     "possible_drift",
		SpecRefs:   []string{bestSpec.Record.Ref},
		Rationale:  fmt.Sprintf("doc is semantically or lexically close to accepted spec %s, but the current deterministic doc-drift rules could not prove contradiction; inspect it manually", bestSpec.Record.Ref),
		Evidence:   evidence,
		Confidence: &DriftConfidence{Level: "low", Score: roundScore(score), Basis: "nearest accepted spec is suggestive, but this is not a deterministic contradiction"},
	}
}

func assessmentFallbackSpecs(doc docDocument, specs map[string]specDocument) []specDocument {
	type scoredSpec struct {
		spec  specDocument
		score float64
	}

	var scored []scoredSpec
	for _, spec := range specs {
		if spec.Record.Status != model.StatusAccepted {
			continue
		}
		score := assessmentSimilarity(doc.Sections, spec.Sections)
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
	if len(scored) == 0 {
		return nil
	}

	limit := minInt(len(scored), 2)
	result := make([]specDocument, 0, limit)
	for _, item := range scored[:limit] {
		result = append(result, item.spec)
	}
	return result
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
	docArtifacts := artifactMentionSet(doc.Sections)
	var scored []scoredSpec
	for _, spec := range specs {
		if spec.Record.Status != model.StatusAccepted {
			continue
		}
		score := documentSimilarity(doc.Sections, spec.Sections)
		if hasArtifactConstraintOverlap(docArtifacts, spec) {
			score += 0.4
		}
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

func artifactDriftFindings(docArtifacts map[string][]artifactMention, spec specDocument) []DriftFinding {
	constraints := artifactConstraintsFromSections(spec.Sections)
	if len(constraints) == 0 || len(docArtifacts) == 0 {
		return nil
	}

	findings := make([]DriftFinding, 0, len(constraints))
	for _, constraint := range constraints {
		mentions := docArtifacts[constraint.Artifact]
		if len(mentions) == 0 {
			continue
		}
		for _, mention := range mentions {
			if mention.Aligned {
				continue
			}

			code := "artifact_contract_mismatch"
			message := fmt.Sprintf("document still presents `%s` as part of the active runtime contract", constraint.Artifact)
			observed := "documented as active runtime state"
			if constraint.Kind == "runtime_input" && mention.Active {
				code = "artifact_runtime_input_mismatch"
				message = fmt.Sprintf("document still treats `%s` as canonical runtime input", constraint.Artifact)
				observed = "documented as an active runtime input"
			}

			findings = append(findings, DriftFinding{
				SpecRef:  spec.Record.Ref,
				Artifact: constraint.Artifact,
				Code:     code,
				Message:  message,
				Expected: constraint.Expected,
				Observed: observed,
			})
			break
		}
	}
	return findings
}

func artifactMentionSet(sections []embeddedSection) map[string]struct{} {
	mentions := artifactMentionsFromSections(sections)
	result := make(map[string]struct{}, len(mentions))
	for artifact := range mentions {
		result[artifact] = struct{}{}
	}
	return result
}

func artifactMentionsFromSections(sections []embeddedSection) map[string][]artifactMention {
	mentions := map[string][]artifactMention{}
	for _, section := range sections {
		for _, line := range sectionContentLines(section.Content) {
			artifacts := artifactRefsFromText(line)
			if len(artifacts) == 0 {
				continue
			}
			lower := strings.ToLower(line)
			active := containsAny(lower,
				" read ", " reads ", " load ", " loads ",
				" write ", " writes ", " uses ", " use ",
				" cache", " cached", " storing ", " stored ",
				" refresh", " refreshes ", " startup", " start ",
			)
			aligned := containsAny(lower,
				"optional", "derived", "historical", "history", "archive",
				"legacy", "not part of", "not required", "must not",
				"implementation detail", "safe to discard",
			)
			for _, artifact := range artifacts {
				mentions[artifact] = append(mentions[artifact], artifactMention{
					Artifact: artifact,
					Active:   active,
					Aligned:  aligned,
				})
			}
		}
	}
	return mentions
}

func hasArtifactConstraintOverlap(docArtifacts map[string]struct{}, spec specDocument) bool {
	if len(docArtifacts) == 0 {
		return false
	}
	for _, constraint := range artifactConstraintsFromSections(spec.Sections) {
		if _, ok := docArtifacts[constraint.Artifact]; ok {
			return true
		}
	}
	return false
}

func artifactConstraintsFromSections(sections []embeddedSection) []artifactConstraint {
	constraints := map[string]artifactConstraint{}
	for _, section := range sections {
		for _, line := range sectionContentLines(section.Content) {
			artifacts := artifactRefsFromText(line)
			if len(artifacts) == 0 {
				continue
			}
			for _, artifact := range artifacts {
				kind, expected, ok := classifyArtifactConstraint(line, artifact)
				if !ok {
					continue
				}
				next := artifactConstraint{
					Artifact: artifact,
					Kind:     kind,
					Expected: expected,
				}
				current, exists := constraints[artifact]
				if !exists || artifactConstraintPriority(next.Kind) > artifactConstraintPriority(current.Kind) {
					constraints[artifact] = next
				}
			}
		}
	}

	artifacts := make([]string, 0, len(constraints))
	for artifact := range constraints {
		artifacts = append(artifacts, artifact)
	}
	sort.Strings(artifacts)

	result := make([]artifactConstraint, 0, len(artifacts))
	for _, artifact := range artifacts {
		result = append(result, constraints[artifact])
	}
	return result
}

func classifyArtifactConstraint(line, artifact string) (string, string, bool) {
	lower := strings.ToLower(line)
	artifact = strings.ToLower(artifact)
	local := lower
	runtimeLocal := lower
	if idx := strings.Index(lower, artifact); idx >= 0 {
		start := idx
		if start > 96 {
			start -= 96
		} else {
			start = 0
		}
		end := idx + len(artifact) + 128
		if end > len(lower) {
			end = len(lower)
		}
		local = lower[start:end]

		runtimeStart := idx
		if runtimeStart > 48 {
			runtimeStart -= 48
		} else {
			runtimeStart = 0
		}
		runtimeEnd := idx + len(artifact) + 32
		if runtimeEnd > len(lower) {
			runtimeEnd = len(lower)
		}
		runtimeLocal = lower[runtimeStart:runtimeEnd]
	}

	switch {
	case containsAny(runtimeLocal,
		"must not read", "must not load", "must not parse",
		"must not reparse", "must not treat",
	):
		return "runtime_input", "not a canonical runtime input", true
	case containsAny(local,
		artifact+"` is not a required artifact",
		artifact+" is not a required artifact",
		artifact+"` is not part of the accepted runtime contract",
		artifact+" is not part of the accepted runtime contract",
		artifact+"` is not part of the persisted runtime contract",
		artifact+" is not part of the persisted runtime contract",
	):
		return "contract", "not part of the accepted runtime contract", true
	case containsAny(lower,
		"legacy derived files",
	) && containsAny(lower,
		"not part of the accepted runtime contract",
		"not part of the persisted runtime contract",
	):
		return "contract", "not part of the accepted runtime contract", true
	default:
		return "", "", false
	}
}

func artifactConstraintPriority(kind string) int {
	switch kind {
	case "runtime_input":
		return 2
	case "contract":
		return 1
	default:
		return 0
	}
}

func uniqueDriftFindings(findings []DriftFinding) []DriftFinding {
	seen := map[string]struct{}{}
	result := make([]DriftFinding, 0, len(findings))
	for _, finding := range findings {
		key := strings.Join([]string{
			finding.SpecRef,
			finding.Artifact,
			finding.Code,
			finding.Message,
			finding.Expected,
			finding.Observed,
		}, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, finding)
	}
	return result
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
	case "artifact_runtime_input_mismatch":
		if finding.Artifact != "" {
			return fmt.Sprintf("rewrite the `%s` guidance so the doc stops treating it as canonical runtime input", finding.Artifact)
		}
		return "rewrite the stale artifact guidance so the doc stops treating it as canonical runtime input"
	case "artifact_contract_mismatch":
		if finding.Artifact != "" {
			return fmt.Sprintf("remove or qualify the stale `%s` reference so it is no longer presented as active runtime state", finding.Artifact)
		}
		return "remove or qualify the stale artifact reference so it is no longer presented as active runtime state"
	default:
		return finding.Message
	}
}

func remediationEvidence(doc docDocument, spec specDocument, finding DriftFinding) DocRemediationEvidence {
	evidence, _ := driftEvidence(doc, spec, finding)
	if evidence == nil {
		return DocRemediationEvidence{
			Expected: humanizedDriftValue(finding.Code, finding.Expected),
			Observed: humanizedDriftValue(finding.Code, finding.Observed),
		}
	}
	return DocRemediationEvidence{
		SpecSection: evidence.SpecSection,
		SpecExcerpt: evidence.SpecExcerpt,
		DocSection:  evidence.DocSection,
		DocExcerpt:  evidence.DocExcerpt,
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
	case "artifact_runtime_input_mismatch":
		note := "Rewrite the section so the artifact is treated as derived output rather than canonical runtime input."
		if finding.Artifact != "" {
			note = fmt.Sprintf("Rewrite the `%s` references so the section treats it as derived output rather than canonical runtime input.", finding.Artifact)
		}
		return DocSuggestedEdit{
			Action: "update_section",
			Note:   note,
		}
	case "artifact_contract_mismatch":
		note := "Rewrite the section so the artifact is described as derived, optional, or historical rather than active runtime state."
		if finding.Artifact != "" {
			note = fmt.Sprintf("Rewrite the `%s` references so the section does not present it as active runtime state.", finding.Artifact)
		}
		return DocSuggestedEdit{
			Action: "update_section",
			Note:   note,
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
	case "artifact_runtime_input_mismatch", "artifact_contract_mismatch":
		if finding.Artifact != "" {
			return []string{finding.Artifact}
		}
		return []string{finding.Expected, finding.Observed}
	default:
		return []string{finding.Expected, finding.Observed}
	}
}

func artifactRefsFromText(text string) []string {
	raw := artifactReferencePattern.FindAllString(text, -1)
	if len(raw) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	artifacts := make([]string, 0, len(raw))
	for _, match := range raw {
		artifact := strings.ToLower(strings.TrimSpace(match))
		if artifact == "" {
			continue
		}
		if _, ok := seen[artifact]; ok {
			continue
		}
		seen[artifact] = struct{}{}
		artifacts = append(artifacts, artifact)
	}
	sort.Strings(artifacts)
	return artifacts
}

func sectionContentLines(content string) []string {
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := stringsTrimSpace(strings.TrimPrefix(line, "- "))
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if value != "" && strings.Contains(text, value) {
			return true
		}
	}
	return false
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

func rationaleForFinding(finding DriftFinding) string {
	switch finding.Code {
	case "window_mismatch":
		return fmt.Sprintf("accepted spec expects %s, but the doc still describes %s", humanizedDriftValue(finding.Code, finding.Expected), humanizedDriftValue(finding.Code, finding.Observed))
	case "subject_mismatch":
		return fmt.Sprintf("accepted spec expects %s, but the doc still targets %s", humanizedDriftValue(finding.Code, finding.Expected), humanizedDriftValue(finding.Code, finding.Observed))
	case "default_limit_mismatch":
		return fmt.Sprintf("accepted spec sets %s, but the doc still states %s", humanizedDriftValue(finding.Code, finding.Expected), humanizedDriftValue(finding.Code, finding.Observed))
	case "override_support_mismatch":
		return fmt.Sprintf("accepted spec says %s, but the doc still says %s", humanizedDriftValue(finding.Code, finding.Expected), humanizedDriftValue(finding.Code, finding.Observed))
	case "artifact_runtime_input_mismatch":
		if finding.Artifact != "" {
			return fmt.Sprintf("accepted spec says `%s` is not a canonical runtime input, but the doc still presents it as active startup input", finding.Artifact)
		}
	case "artifact_contract_mismatch":
		if finding.Artifact != "" {
			return fmt.Sprintf("accepted spec says `%s` is not part of the active runtime contract, but the doc still presents it as active runtime state", finding.Artifact)
		}
	}
	return finding.Message
}

func confidenceForDriftFinding(finding DriftFinding, score float64) *DriftConfidence {
	level := "medium"
	basis := "finding is backed by deterministic evidence excerpts from the doc and accepted spec"
	if finding.Evidence != nil && finding.Evidence.DocExcerpt != "" && finding.Evidence.SpecExcerpt != "" {
		level = "high"
		if score > 0 {
			basis = "finding is backed by explicit doc/spec excerpts that also align semantically"
		}
	} else if finding.Evidence == nil || (finding.Evidence.DocExcerpt == "" && finding.Evidence.SpecExcerpt == "") {
		level = "low"
		basis = "finding is deterministic, but the supporting doc/spec excerpts could not be localized precisely"
	}
	confidence := &DriftConfidence{
		Level: level,
		Basis: basis,
	}
	if score > 0 {
		confidence.Score = roundScore(score)
	}
	return confidence
}

func driftEvidence(doc docDocument, spec specDocument, finding DriftFinding) (*DriftEvidence, float64) {
	keywords := evidenceKeywordsForFinding(finding)
	docSection := bestSectionForKeywords(doc.Sections, keywords)
	specSection := bestSectionForKeywords(spec.Sections, keywords)
	if docSection == nil && specSection == nil {
		return nil, 0
	}

	score := 0.0
	if docSection != nil && specSection != nil {
		score = cosineSimilarity(docSection.Embedding, specSection.Embedding)
	}

	evidence := &DriftEvidence{
		SpecRef:   spec.Record.Ref,
		SpecTitle: spec.Record.Title,
	}
	if specSection != nil {
		evidence.SpecSection = defaultString(stringsTrimSpace(specSection.Heading), "(body)")
		evidence.SpecExcerpt = defaultString(sectionExcerpt(*specSection), stringsTrimSpace(spec.Record.Title))
	}
	if docSection != nil {
		evidence.DocSection = defaultString(stringsTrimSpace(docSection.Heading), "(body)")
		evidence.DocExcerpt = sectionExcerpt(*docSection)
	}
	return evidence, score
}

func bestSectionForKeywords(sections []embeddedSection, keywords []string) *embeddedSection {
	for _, section := range sections {
		if _, ok := sectionExcerptForKeywords(section, keywords); ok {
			candidate := section
			return &candidate
		}
	}
	if len(sections) == 0 {
		return nil
	}

	var (
		best      embeddedSection
		bestScore float64
		found     bool
	)
	for _, section := range sections {
		excerpt := sectionExcerpt(section)
		if excerpt == "" {
			continue
		}
		score := keywordDensity(excerpt, keywords) + keywordDensity(section.Heading, keywords)
		if !found || score > bestScore {
			best = section
			bestScore = score
			found = true
		}
	}
	if !found {
		return nil
	}
	return &best
}

func keywordDensity(text string, keywords []string) float64 {
	if stringsTrimSpace(text) == "" || len(keywords) == 0 {
		return 0
	}
	lower := strings.ToLower(text)
	var score float64
	for _, keyword := range keywords {
		keyword = strings.ToLower(stringsTrimSpace(keyword))
		if keyword != "" && strings.Contains(lower, keyword) {
			score++
		}
	}
	return score
}

func bestAlignedAssessmentEvidence(doc docDocument, relevant []specDocument) (*specDocument, *DriftEvidence, float64) {
	candidate := bestAlignedAssessmentCandidateForDocs(doc, relevant)
	if candidate == nil {
		return nil, nil, 0
	}
	return candidate.spec, candidate.evidence, candidate.score
}

func bestAlignedAssessmentCandidateForDocs(doc docDocument, relevant []specDocument) *alignedAssessmentCandidate {
	docText := strings.ToLower(joinDocumentText(doc.Sections))
	var best *alignedAssessmentCandidate
	for i := range relevant {
		spec := &relevant[i]
		for j := range doc.Sections {
			docSection := &doc.Sections[j]
			for k := range spec.Sections {
				specSection := &spec.Sections[k]
				score := sectionAssessmentScore(*docSection, *specSection)
				if best != nil && score <= best.score {
					continue
				}
				best = &alignedAssessmentCandidate{
					spec: spec,
					evidence: &DriftEvidence{
						SpecRef:     spec.Record.Ref,
						SpecTitle:   spec.Record.Title,
						SpecSection: defaultString(stringsTrimSpace(specSection.Heading), "(body)"),
						SpecExcerpt: defaultString(sectionExcerpt(*specSection), stringsTrimSpace(spec.Record.Title)),
						DocSection:  defaultString(stringsTrimSpace(docSection.Heading), "(body)"),
						DocExcerpt:  sectionExcerpt(*docSection),
					},
					score:           score,
					docMentionsSpec: strings.Contains(docText, strings.ToLower(spec.Record.Ref)),
					headingOverlap:  lexicalOverlapScore(docSection.Heading, specSection.Heading) >= 0.8,
				}
			}
		}
	}
	return best
}

func shouldEmitAlignedAssessment(candidate *alignedAssessmentCandidate) bool {
	if candidate == nil {
		return false
	}
	if candidate.score < 0.7 {
		return false
	}
	return candidate.docMentionsSpec || candidate.headingOverlap
}

func alignmentConfidenceLevel(score float64) string {
	switch {
	case score >= 0.82:
		return "high"
	case score >= 0.65:
		return "medium"
	default:
		return "low"
	}
}

func topDriftFinding(findings []DriftFinding) *DriftFinding {
	if len(findings) == 0 {
		return nil
	}
	bestIndex := 0
	for i := 1; i < len(findings); i++ {
		if compareDriftFindings(findings[i], findings[bestIndex]) < 0 {
			bestIndex = i
		}
	}
	return &findings[bestIndex]
}

func compareDriftFindings(left, right DriftFinding) int {
	leftRank := driftConfidenceRank(left.Confidence)
	rightRank := driftConfidenceRank(right.Confidence)
	switch {
	case leftRank != rightRank:
		return rightRank - leftRank
	case len(left.Rationale) != len(right.Rationale):
		if len(left.Rationale) > len(right.Rationale) {
			return -1
		}
		return 1
	case left.Code < right.Code:
		return -1
	case left.Code > right.Code:
		return 1
	default:
		return 0
	}
}

func driftConfidenceRank(confidence *DriftConfidence) int {
	if confidence == nil {
		return 0
	}
	switch confidence.Level {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func cloneDriftEvidence(evidence *DriftEvidence) *DriftEvidence {
	if evidence == nil {
		return nil
	}
	clone := *evidence
	return &clone
}

func cloneDriftConfidence(confidence *DriftConfidence) *DriftConfidence {
	if confidence == nil {
		return nil
	}
	clone := *confidence
	return &clone
}

func assessmentSimilarity(left, right []embeddedSection) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}

	best := 0.0
	for _, leftSection := range left {
		for _, rightSection := range right {
			score := sectionAssessmentScore(leftSection, rightSection)
			if score > best {
				best = score
			}
		}
	}
	return best
}

func sectionAssessmentScore(left, right embeddedSection) float64 {
	semantic := cosineSimilarity(left.Embedding, right.Embedding)
	lexical := lexicalOverlapScore(
		textForEmbedding("", left.Heading, left.Content),
		textForEmbedding("", right.Heading, right.Content),
	)
	if semantic == 0 {
		return lexical
	}
	if lexical == 0 {
		return semantic
	}
	return (semantic * 0.8) + (lexical * 0.2)
}

func lexicalOverlapScore(left, right string) float64 {
	leftTokens := normalizedLexicalTokens(left)
	rightTokens := normalizedLexicalTokens(right)
	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return 0
	}
	shared := sharedStrings(leftTokens, rightTokens)
	if len(shared) == 0 {
		return 0
	}
	return float64(len(shared)) / float64(minInt(len(leftTokens), len(rightTokens)))
}

func normalizedLexicalTokens(text string) []string {
	var builder strings.Builder
	builder.Grow(len(text))
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte(' ')
	}
	return uniqueStrings(strings.Fields(builder.String()))
}
