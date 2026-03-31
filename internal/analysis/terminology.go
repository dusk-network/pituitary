package analysis

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/resultmeta"
)

const terminologyEvidenceThreshold = 0.15

var terminologyCompatibilityPattern = regexp.MustCompile(`(?i)\b(compatibility|compatible|debug|projection|alias|shim|fallback|migration|migrating|transitional)\b`)

const (
	terminologyClassificationDisplacedTerm    = "displaced_term"
	terminologyClassificationHistoricalAlias  = "historical_alias"
	terminologyClassificationDeprecatedTerm   = "deprecated_term"
	terminologyClassificationForbiddenCurrent = "forbidden_current_state"
	terminologyContextCurrentState            = "current_state"
	terminologyContextHistorical              = "historical"
)

// TerminologyAuditRequest is the normalized input for terminology audits.
type TerminologyAuditRequest struct {
	Terms          []string `json:"terms"`
	CanonicalTerms []string `json:"canonical_terms,omitempty"`
	SpecRef        string   `json:"spec_ref,omitempty"`
	Scope          string   `json:"scope,omitempty"`
}

// TerminologyAuditScope reports how the audit selected artifacts.
type TerminologyAuditScope struct {
	Mode          string   `json:"mode"`
	ArtifactKinds []string `json:"artifact_kinds"`
	SpecRef       string   `json:"spec_ref,omitempty"`
}

// TerminologyAnchorSpec reports one canonical spec used to anchor terminology evidence.
type TerminologyAnchorSpec struct {
	Ref       string                     `json:"ref"`
	Title     string                     `json:"title"`
	Status    string                     `json:"status,omitempty"`
	Inference *model.InferenceConfidence `json:"inference,omitempty"`
}

// TerminologyEvidence reports the canonical spec evidence paired with one finding.
type TerminologyEvidence struct {
	SpecRef string  `json:"spec_ref"`
	Title   string  `json:"title"`
	Section string  `json:"section"`
	Excerpt string  `json:"excerpt"`
	Score   float64 `json:"score"`
}

// TerminologySectionFinding reports one section that still uses displaced terms.
type TerminologySectionFinding struct {
	Section    string                 `json:"section"`
	Terms      []string               `json:"terms"`
	Matches    []TerminologyTermMatch `json:"matches,omitempty"`
	Excerpt    string                 `json:"excerpt"`
	Assessment string                 `json:"assessment,omitempty"`
	Evidence   *TerminologyEvidence   `json:"evidence,omitempty"`
}

// TerminologyTermMatch reports the policy treatment for one matched term in a section.
type TerminologyTermMatch struct {
	Term           string `json:"term"`
	PreferredTerm  string `json:"preferred_term,omitempty"`
	Classification string `json:"classification,omitempty"`
	Context        string `json:"context,omitempty"`
	Severity       string `json:"severity,omitempty"`
	Replacement    string `json:"replacement,omitempty"`
	Tolerated      bool   `json:"tolerated,omitempty"`
}

// TerminologyFinding reports one offending doc or spec.
type TerminologyFinding struct {
	Ref       string                      `json:"ref"`
	Kind      string                      `json:"kind"`
	Title     string                      `json:"title"`
	SourceRef string                      `json:"source_ref"`
	Terms     []string                    `json:"terms"`
	Score     float64                     `json:"score,omitempty"`
	Inference *model.InferenceConfidence  `json:"inference,omitempty"`
	Sections  []TerminologySectionFinding `json:"sections"`
}

// TerminologyAuditResult is the structured terminology-audit response.
type TerminologyAuditResult struct {
	Scope          TerminologyAuditScope    `json:"scope"`
	Terms          []string                 `json:"terms"`
	CanonicalTerms []string                 `json:"canonical_terms,omitempty"`
	AnchorSpecs    []TerminologyAnchorSpec  `json:"anchor_specs,omitempty"`
	Findings       []TerminologyFinding     `json:"findings"`
	Tolerated      []TerminologyFinding     `json:"tolerated,omitempty"`
	Warnings       []Warning                `json:"warnings,omitempty"`
	ContentTrust   *resultmeta.ContentTrust `json:"content_trust,omitempty"`
}

type terminologyArtifact struct {
	Ref       string
	Kind      string
	Title     string
	Status    string
	SourceRef string
	Metadata  map[string]string
	Inference *model.InferenceConfidence
	Sections  []embeddedSection
}

type terminologyEvidenceSection struct {
	SpecRef   string
	Title     string
	Section   string
	Excerpt   string
	Embedding []float64
}

type terminologyMatcher struct {
	Term    string
	Pattern *regexp.Regexp
}

type normalizedTerminologyAuditRequest struct {
	TerminologyAuditRequest
	GovernedTerms map[string]terminologyGovernedTerm
}

type terminologyGovernedTerm struct {
	Term           string
	PreferredTerm  string
	Classification string
	DocsSeverity   string
	SpecsSeverity  string
}

type terminologyTextMatch struct {
	Terms             []string
	Excerpt           string
	Assessment        string
	HistoricalContext bool
}

// CheckTerminology audits indexed docs and specs for displaced terminology.
func CheckTerminology(cfg *config.Config, request TerminologyAuditRequest) (*TerminologyAuditResult, error) {
	return CheckTerminologyContext(context.Background(), cfg, request)
}

// CheckTerminologyContext audits indexed docs and specs for displaced terminology.
func CheckTerminologyContext(ctx context.Context, cfg *config.Config, request TerminologyAuditRequest) (*TerminologyAuditResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	normalized, err := normalizeTerminologyAuditRequest(cfg, request)
	if err != nil {
		return nil, err
	}

	repo, err := openAnalysisRepositoryContext(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer repo.Close()

	var anchor *specDocument
	if normalized.SpecRef != "" {
		anchor, err = loadCandidate(repo, OverlapRequest{SpecRef: normalized.SpecRef}, nil)
		if err != nil {
			return nil, err
		}
	}

	artifacts, err := loadTerminologyArtifacts(repo, normalized.TerminologyAuditRequest, anchor)
	if err != nil {
		return nil, err
	}

	anchors, evidenceSections, warnings, err := loadTerminologyAnchors(repo, normalized.TerminologyAuditRequest, anchor)
	if err != nil {
		return nil, err
	}

	matchers := compileTerminologyMatchers(normalized.Terms)
	findings, tolerated := auditTerminologyArtifacts(artifacts, matchers, normalized.GovernedTerms, normalized.CanonicalTerms, evidenceSections)

	warningSpecs := make([]specDocument, 0, len(anchors))
	warningSpecs = append(warningSpecs, anchors...)
	warnings = append(warnings, buildSpecInferenceWarnings("terminology audit", warningSpecs...)...)

	return &TerminologyAuditResult{
		Scope: TerminologyAuditScope{
			Mode:          terminologyAuditMode(normalized.TerminologyAuditRequest),
			ArtifactKinds: terminologyArtifactKinds(normalized.Scope),
			SpecRef:       normalized.SpecRef,
		},
		Terms:          normalized.Terms,
		CanonicalTerms: normalized.CanonicalTerms,
		AnchorSpecs:    terminologyAnchorSpecs(anchors),
		Findings:       findings,
		Tolerated:      tolerated,
		Warnings:       uniqueWarnings(warnings),
		ContentTrust:   resultmeta.UntrustedWorkspaceText(),
	}, nil
}

func normalizeTerminologyAuditRequest(cfg *config.Config, request TerminologyAuditRequest) (normalizedTerminologyAuditRequest, error) {
	request.SpecRef = stringsTrimSpace(request.SpecRef)
	request.Terms = uniqueStrings(request.Terms)
	request.CanonicalTerms = uniqueStrings(request.CanonicalTerms)
	request.Scope = defaultString(stringsTrimSpace(request.Scope), "all")
	governedTerms := terminologyGovernedTerms(cfg)

	if len(request.Terms) == 0 {
		request.Terms = terminologyGovernedAuditTerms(governedTerms)
	}
	if len(request.CanonicalTerms) == 0 {
		request.CanonicalTerms = terminologyGovernedCanonicalTerms(request.Terms, governedTerms)
	}

	if len(request.Terms) == 0 {
		return normalizedTerminologyAuditRequest{}, fmt.Errorf("at least one term or terminology policy is required")
	}

	switch request.Scope {
	case "all", "docs", "specs":
		return normalizedTerminologyAuditRequest{
			TerminologyAuditRequest: request,
			GovernedTerms:           governedTerms,
		}, nil
	default:
		return normalizedTerminologyAuditRequest{}, fmt.Errorf("scope %q is invalid", request.Scope)
	}
}

func terminologyAuditMode(request TerminologyAuditRequest) string {
	if request.SpecRef != "" {
		return "spec_ref"
	}
	return "workspace"
}

func terminologyArtifactKinds(scope string) []string {
	switch scope {
	case "docs":
		return []string{model.ArtifactKindDoc}
	case "specs":
		return []string{model.ArtifactKindSpec}
	default:
		return []string{model.ArtifactKindDoc, model.ArtifactKindSpec}
	}
}

func loadTerminologyArtifacts(repo *analysisRepository, request TerminologyAuditRequest, anchor *specDocument) ([]terminologyArtifact, error) {
	artifacts := make([]terminologyArtifact, 0)
	includeDocs := request.Scope == "all" || request.Scope == "docs"
	includeSpecs := request.Scope == "all" || request.Scope == "specs"

	if anchor == nil {
		if includeDocs {
			docs, err := repo.loadDocs(nil)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, terminologyArtifactsFromDocs(docs)...)
		}
		if includeSpecs {
			specs, err := repo.loadSpecs(nil)
			if err != nil {
				return nil, err
			}
			artifacts = append(artifacts, terminologyArtifactsFromSpecs(specs)...)
		}
		return artifacts, nil
	}

	if includeDocs {
		docRefs, err := repo.impactedDocRefs(*anchor)
		if err != nil {
			return nil, err
		}
		docs, err := repo.loadSelectedDocs(docRefs)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, terminologyArtifactsFromDocs(docs)...)
	}
	if includeSpecs {
		specRefs, err := repo.impactedSpecRefs(anchor.Record)
		if err != nil {
			return nil, err
		}
		specRefs = append(specRefs, anchor.Record.Ref)
		specs, err := repo.loadSelectedSpecs(specRefs)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, terminologyArtifactsFromSpecs(specs)...)
	}

	sort.Slice(artifacts, func(i, j int) bool {
		switch {
		case artifacts[i].Kind != artifacts[j].Kind:
			return artifacts[i].Kind < artifacts[j].Kind
		default:
			return artifacts[i].Ref < artifacts[j].Ref
		}
	})
	return artifacts, nil
}

func terminologyArtifactsFromDocs(docs map[string]docDocument) []terminologyArtifact {
	refs := sortedDocRefs(docs)
	artifacts := make([]terminologyArtifact, 0, len(refs))
	for _, ref := range refs {
		doc := docs[ref]
		artifacts = append(artifacts, terminologyArtifact{
			Ref:       doc.Record.Ref,
			Kind:      model.ArtifactKindDoc,
			Title:     doc.Record.Title,
			SourceRef: doc.Record.SourceRef,
			Metadata:  doc.Record.Metadata,
			Sections:  doc.Sections,
		})
	}
	return artifacts
}

func terminologyArtifactsFromSpecs(specs map[string]specDocument) []terminologyArtifact {
	refs := sortedSpecRefs(specs)
	artifacts := make([]terminologyArtifact, 0, len(refs))
	for _, ref := range refs {
		spec := specs[ref]
		artifacts = append(artifacts, terminologyArtifact{
			Ref:       spec.Record.Ref,
			Kind:      model.ArtifactKindSpec,
			Title:     spec.Record.Title,
			Status:    spec.Record.Status,
			SourceRef: spec.Record.SourceRef,
			Metadata:  spec.Record.Metadata,
			Inference: spec.Record.Inference,
			Sections:  spec.Sections,
		})
	}
	return artifacts
}

func loadTerminologyAnchors(repo *analysisRepository, request TerminologyAuditRequest, anchor *specDocument) ([]specDocument, []terminologyEvidenceSection, []Warning, error) {
	var (
		anchors  []specDocument
		evidence []terminologyEvidenceSection
		warnings []Warning
	)

	if anchor != nil {
		anchors = append(anchors, *anchor)
		selected := terminologyEvidenceSections(*anchor, request.CanonicalTerms)
		if len(selected) == 0 {
			selected = terminologyEvidenceSectionsFromAll(*anchor)
			if len(request.CanonicalTerms) > 0 {
				warnings = append(warnings, Warning{
					Code:    "no_canonical_evidence",
					Ref:     anchor.Record.Ref,
					Message: fmt.Sprintf("anchor spec %s did not contain the supplied canonical terms; evidence fell back to all anchor sections", anchor.Record.Ref),
				})
			}
		}
		return anchors, selected, warnings, nil
	}

	if len(request.CanonicalTerms) == 0 {
		return nil, nil, nil, nil
	}

	specs, err := repo.loadAllSpecs()
	if err != nil {
		return nil, nil, nil, err
	}
	for _, ref := range sortedSpecRefs(specs) {
		spec := specs[ref]
		if spec.Record.Status != model.StatusAccepted {
			continue
		}
		selected := terminologyEvidenceSections(spec, request.CanonicalTerms)
		if len(selected) == 0 {
			continue
		}
		anchors = append(anchors, spec)
		evidence = append(evidence, selected...)
	}

	if len(evidence) == 0 {
		warnings = append(warnings, Warning{
			Code:    "no_canonical_evidence",
			Message: "no accepted spec sections matched the supplied canonical terms; results are lexical only",
		})
	}
	return anchors, evidence, warnings, nil
}

func terminologyEvidenceSections(spec specDocument, canonicalTerms []string) []terminologyEvidenceSection {
	if len(canonicalTerms) == 0 {
		return terminologyEvidenceSectionsFromAll(spec)
	}

	matchers := compileTerminologyMatchers(canonicalTerms)
	evidence := make([]terminologyEvidenceSection, 0, len(spec.Sections))
	for _, section := range spec.Sections {
		terms, excerpt, _ := terminologySectionTerms(section, matchers)
		if len(terms) == 0 {
			continue
		}
		evidence = append(evidence, terminologyEvidenceSection{
			SpecRef:   spec.Record.Ref,
			Title:     spec.Record.Title,
			Section:   defaultString(stringsTrimSpace(section.Heading), "(body)"),
			Excerpt:   excerpt,
			Embedding: section.Embedding,
		})
	}
	return evidence
}

func terminologyEvidenceSectionsFromAll(spec specDocument) []terminologyEvidenceSection {
	evidence := make([]terminologyEvidenceSection, 0, len(spec.Sections))
	for _, section := range spec.Sections {
		evidence = append(evidence, terminologyEvidenceSection{
			SpecRef:   spec.Record.Ref,
			Title:     spec.Record.Title,
			Section:   defaultString(stringsTrimSpace(section.Heading), "(body)"),
			Excerpt:   defaultString(sectionExcerpt(section), spec.Record.Title),
			Embedding: section.Embedding,
		})
	}
	return evidence
}

func terminologyAnchorSpecs(specs []specDocument) []TerminologyAnchorSpec {
	result := make([]TerminologyAnchorSpec, 0, len(specs))
	for _, spec := range specs {
		result = append(result, TerminologyAnchorSpec{
			Ref:       spec.Record.Ref,
			Title:     spec.Record.Title,
			Status:    spec.Record.Status,
			Inference: spec.Record.Inference,
		})
	}
	return result
}

func compileTerminologyMatchers(terms []string) []terminologyMatcher {
	matchers := make([]terminologyMatcher, 0, len(terms))
	for _, term := range uniqueStrings(terms) {
		if stringsTrimSpace(term) == "" {
			continue
		}
		pattern := regexp.MustCompile(`(?i)\b` + strings.ReplaceAll(regexp.QuoteMeta(term), `\ `, `\s+`) + `\b`)
		matchers = append(matchers, terminologyMatcher{
			Term:    term,
			Pattern: pattern,
		})
	}
	return matchers
}

func auditTerminologyArtifacts(artifacts []terminologyArtifact, matchers []terminologyMatcher, governedTerms map[string]terminologyGovernedTerm, canonicalTerms []string, evidenceSections []terminologyEvidenceSection) ([]TerminologyFinding, []TerminologyFinding) {
	findings := make([]TerminologyFinding, 0, len(artifacts))
	tolerated := make([]TerminologyFinding, 0, len(artifacts))
	for _, artifact := range artifacts {
		finding, toleratedOnly := auditTerminologyArtifact(artifact, matchers, governedTerms, canonicalTerms, evidenceSections)
		if finding == nil {
			continue
		}
		if toleratedOnly {
			tolerated = append(tolerated, *finding)
			continue
		}
		findings = append(findings, *finding)
	}

	sortTerminologyFindings(findings)
	sortTerminologyFindings(tolerated)
	return findings, tolerated
}

func auditTerminologyArtifact(artifact terminologyArtifact, matchers []terminologyMatcher, governedTerms map[string]terminologyGovernedTerm, canonicalTerms []string, evidenceSections []terminologyEvidenceSection) (*TerminologyFinding, bool) {
	sections := make([]TerminologySectionFinding, 0, len(artifact.Sections))
	matchedTerms := make([]string, 0, len(matchers))
	bestScore := 0.0
	actionable := false

	for _, section := range artifact.Sections {
		matches, excerpt, assessment := terminologySectionMatches(artifact, section, matchers, governedTerms, canonicalTerms)
		if len(matches) == 0 {
			continue
		}
		terms := terminologyMatchTerms(matches)
		if len(terms) == 0 {
			continue
		}

		sectionFinding := TerminologySectionFinding{
			Section:    defaultString(stringsTrimSpace(section.Heading), "(body)"),
			Terms:      terms,
			Matches:    matches,
			Excerpt:    excerpt,
			Assessment: assessment,
		}
		if evidence := bestTerminologyEvidence(section, evidenceSections); evidence != nil {
			sectionFinding.Evidence = evidence
			if evidence.Score > bestScore {
				bestScore = evidence.Score
			}
		}
		matchedTerms = append(matchedTerms, terms...)
		sections = append(sections, sectionFinding)
		if terminologyMatchesActionable(matches) {
			actionable = true
		}
	}

	if len(sections) == 0 {
		return nil, false
	}

	return &TerminologyFinding{
		Ref:       artifact.Ref,
		Kind:      artifact.Kind,
		Title:     artifact.Title,
		SourceRef: artifact.SourceRef,
		Terms:     uniqueStrings(matchedTerms),
		Score:     roundScore(bestScore),
		Inference: artifact.Inference,
		Sections:  sections,
	}, !actionable
}

func terminologySectionTerms(section embeddedSection, matchers []terminologyMatcher) ([]string, string, string) {
	matches := terminologyExactMatches(section, matchers)
	if len(matches) == 0 {
		return nil, "", ""
	}

	terms := make([]string, 0, len(matchers))
	excerpt := ""
	assessment := ""
	for _, match := range matches {
		terms = append(terms, match.Terms...)
		if excerpt == "" {
			excerpt = match.Excerpt
			assessment = match.Assessment
		}
	}
	terms = uniqueStrings(terms)
	if len(terms) == 0 {
		return nil, "", ""
	}
	return terms, excerpt, assessment
}

func terminologySectionMatches(artifact terminologyArtifact, section embeddedSection, matchers []terminologyMatcher, governedTerms map[string]terminologyGovernedTerm, canonicalTerms []string) ([]TerminologyTermMatch, string, string) {
	rawMatches := terminologyExactMatches(section, matchers)
	if len(rawMatches) == 0 {
		return nil, "", ""
	}

	selected := make(map[string]TerminologyTermMatch)
	excerpt := ""
	assessment := ""
	bestExcerptRank := -1
	for _, raw := range rawMatches {
		lineMatches := make([]TerminologyTermMatch, 0, len(raw.Terms))
		for _, term := range raw.Terms {
			match := classifyTerminologyTermMatch(term, artifact, raw, governedTerms, canonicalTerms)
			if match == nil {
				continue
			}
			lineMatches = append(lineMatches, *match)
			key := strings.ToLower(match.Term)
			if existing, ok := selected[key]; ok {
				if terminologyTermMatchRank(*match) > terminologyTermMatchRank(existing) {
					selected[key] = *match
				}
				continue
			}
			selected[key] = *match
		}
		if len(lineMatches) == 0 {
			continue
		}
		if rank := terminologyLineMatchRank(lineMatches); rank > bestExcerptRank {
			bestExcerptRank = rank
			excerpt = raw.Excerpt
			assessment = raw.Assessment
		}
	}

	if len(selected) == 0 {
		return nil, "", ""
	}

	matches := make([]TerminologyTermMatch, 0, len(selected))
	for _, match := range selected {
		matches = append(matches, match)
	}
	sort.Slice(matches, func(i, j int) bool {
		switch {
		case terminologyTermMatchRank(matches[i]) != terminologyTermMatchRank(matches[j]):
			return terminologyTermMatchRank(matches[i]) > terminologyTermMatchRank(matches[j])
		case matches[i].Classification != matches[j].Classification:
			return matches[i].Classification < matches[j].Classification
		default:
			return matches[i].Term < matches[j].Term
		}
	})
	return matches, excerpt, assessment
}

func terminologyExactMatches(section embeddedSection, matchers []terminologyMatcher) []terminologyTextMatch {
	matches := make([]terminologyTextMatch, 0, len(matchers))
	for _, line := range strings.Split(section.Content, "\n") {
		trimmed := stringsTrimSpace(strings.TrimPrefix(line, "- "))
		if trimmed == "" {
			continue
		}
		terms := matchedTerminologyTerms(trimmed, matchers)
		if len(terms) == 0 {
			continue
		}
		historical := isCompatibilityOnlyTerminologyReference(trimmed)
		assessment := "exact match in body text without compatibility-only markers"
		if historical {
			assessment = "exact match in body text with compatibility or migration markers"
		}
		matches = append(matches, terminologyTextMatch{
			Terms:             terms,
			Excerpt:           trimmed,
			Assessment:        assessment,
			HistoricalContext: historical,
		})
	}
	if heading := stringsTrimSpace(section.Heading); heading != "" {
		if terms := matchedTerminologyTerms(heading, matchers); len(terms) > 0 {
			historical := isCompatibilityOnlyTerminologyReference(heading)
			assessment := "exact match in section heading"
			if historical {
				assessment = "exact match in section heading with compatibility or migration markers"
			}
			matches = append(matches, terminologyTextMatch{
				Terms:             terms,
				Excerpt:           heading,
				Assessment:        assessment,
				HistoricalContext: historical,
			})
		}
	}
	return matches
}

func matchedTerminologyTerms(text string, matchers []terminologyMatcher) []string {
	terms := make([]string, 0, len(matchers))
	for _, matcher := range matchers {
		if matcher.Pattern.MatchString(text) {
			terms = append(terms, matcher.Term)
		}
	}
	return uniqueStrings(terms)
}

func isCompatibilityOnlyTerminologyReference(text string) bool {
	return terminologyCompatibilityPattern.MatchString(text)
}

func sectionExcerpt(section embeddedSection) string {
	for _, line := range strings.Split(section.Content, "\n") {
		trimmed := stringsTrimSpace(strings.TrimPrefix(line, "- "))
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func bestTerminologyEvidence(section embeddedSection, evidenceSections []terminologyEvidenceSection) *TerminologyEvidence {
	if len(evidenceSections) == 0 || len(section.Embedding) == 0 {
		return nil
	}

	var (
		best      terminologyEvidenceSection
		bestScore float64
	)
	for _, candidate := range evidenceSections {
		if len(candidate.Embedding) == 0 {
			continue
		}
		score := cosineSimilarity(section.Embedding, candidate.Embedding)
		if score > bestScore {
			bestScore = score
			best = candidate
		}
	}
	if bestScore < terminologyEvidenceThreshold {
		return nil
	}
	return &TerminologyEvidence{
		SpecRef: best.SpecRef,
		Title:   best.Title,
		Section: best.Section,
		Excerpt: best.Excerpt,
		Score:   roundScore(bestScore),
	}
}

func terminologyGovernedTerms(cfg *config.Config) map[string]terminologyGovernedTerm {
	if cfg == nil || len(cfg.Terminology.Policies) == 0 {
		return nil
	}

	governed := make(map[string]terminologyGovernedTerm)
	for _, policy := range cfg.Terminology.Policies {
		register := func(classification string, values []string) {
			for _, term := range values {
				term = stringsTrimSpace(term)
				if term == "" {
					continue
				}
				governed[strings.ToLower(term)] = terminologyGovernedTerm{
					Term:           term,
					PreferredTerm:  policy.Preferred,
					Classification: classification,
					DocsSeverity:   config.NormalizeTerminologySeverity(policy.DocsSeverity),
					SpecsSeverity:  config.NormalizeTerminologySeverity(policy.SpecsSeverity),
				}
			}
		}
		register(terminologyClassificationHistoricalAlias, policy.HistoricalAliases)
		register(terminologyClassificationDeprecatedTerm, policy.DeprecatedTerms)
		register(terminologyClassificationForbiddenCurrent, policy.ForbiddenCurrent)
	}
	return governed
}

func terminologyGovernedAuditTerms(governed map[string]terminologyGovernedTerm) []string {
	if len(governed) == 0 {
		return nil
	}
	terms := make([]string, 0, len(governed))
	for _, rule := range governed {
		terms = append(terms, rule.Term)
	}
	return uniqueStrings(terms)
}

func terminologyGovernedCanonicalTerms(terms []string, governed map[string]terminologyGovernedTerm) []string {
	if len(terms) == 0 || len(governed) == 0 {
		return nil
	}
	preferred := make([]string, 0, len(terms))
	for _, term := range terms {
		rule, ok := governed[strings.ToLower(stringsTrimSpace(term))]
		if !ok || stringsTrimSpace(rule.PreferredTerm) == "" {
			continue
		}
		preferred = append(preferred, rule.PreferredTerm)
	}
	return uniqueStrings(preferred)
}

func classifyTerminologyTermMatch(term string, artifact terminologyArtifact, textMatch terminologyTextMatch, governedTerms map[string]terminologyGovernedTerm, canonicalTerms []string) *TerminologyTermMatch {
	term = stringsTrimSpace(term)
	if term == "" {
		return nil
	}

	context := terminologyContextForMatch(artifact, textMatch)
	if rule, ok := governedTerms[strings.ToLower(term)]; ok {
		severity := terminologySeverityForArtifact(rule, artifact.Kind)
		tolerated := terminologyGovernedTermTolerated(rule, context)
		if tolerated {
			severity = config.TerminologySeverityIgnore
		}
		if !tolerated && severity == config.TerminologySeverityIgnore {
			return nil
		}

		match := &TerminologyTermMatch{
			Term:           term,
			PreferredTerm:  rule.PreferredTerm,
			Classification: rule.Classification,
			Context:        context,
			Severity:       severity,
			Tolerated:      tolerated,
		}
		if replacement := terminologyReplacementTerm(term, rule.PreferredTerm, tolerated); replacement != "" {
			match.Replacement = replacement
		}
		return match
	}

	tolerated := context == terminologyContextHistorical
	severity := config.TerminologySeverityWarning
	if tolerated {
		severity = config.TerminologySeverityIgnore
	}
	replacement := terminologyFallbackReplacement(term, canonicalTerms, tolerated)
	match := &TerminologyTermMatch{
		Term:           term,
		PreferredTerm:  replacement,
		Classification: terminologyClassificationDisplacedTerm,
		Context:        context,
		Severity:       severity,
		Replacement:    replacement,
		Tolerated:      tolerated,
	}
	if replacement == "" {
		match.PreferredTerm = ""
	}
	return match
}

func terminologyContextForMatch(artifact terminologyArtifact, textMatch terminologyTextMatch) string {
	if textMatch.HistoricalContext {
		return terminologyContextHistorical
	}
	if sourceRoleFromMetadata(artifact.Metadata) == config.SourceRoleHistorical {
		return terminologyContextHistorical
	}
	if artifact.Kind == model.ArtifactKindSpec {
		switch stringsTrimSpace(artifact.Status) {
		case model.StatusSuperseded, model.StatusDeprecated:
			return terminologyContextHistorical
		}
	}
	return terminologyContextCurrentState
}

func terminologySeverityForArtifact(rule terminologyGovernedTerm, kind string) string {
	if kind == model.ArtifactKindSpec {
		return config.NormalizeTerminologySeverity(rule.SpecsSeverity)
	}
	return config.NormalizeTerminologySeverity(rule.DocsSeverity)
}

func terminologyGovernedTermTolerated(rule terminologyGovernedTerm, context string) bool {
	if context != terminologyContextHistorical {
		return false
	}
	switch rule.Classification {
	case terminologyClassificationHistoricalAlias, terminologyClassificationForbiddenCurrent:
		return true
	default:
		return false
	}
}

func terminologyReplacementTerm(term, preferred string, tolerated bool) string {
	if tolerated || stringsTrimSpace(preferred) == "" || strings.EqualFold(term, preferred) {
		return ""
	}
	return preferred
}

func terminologyFallbackReplacement(term string, canonicalTerms []string, tolerated bool) string {
	if tolerated || len(canonicalTerms) != 1 {
		return ""
	}
	if strings.EqualFold(term, canonicalTerms[0]) {
		return ""
	}
	return canonicalTerms[0]
}

func terminologyMatchTerms(matches []TerminologyTermMatch) []string {
	terms := make([]string, 0, len(matches))
	for _, match := range matches {
		terms = append(terms, match.Term)
	}
	return uniqueStrings(terms)
}

func terminologyMatchesActionable(matches []TerminologyTermMatch) bool {
	for _, match := range matches {
		if !match.Tolerated {
			return true
		}
	}
	return false
}

func terminologyLineMatchRank(matches []TerminologyTermMatch) int {
	rank := 0
	for _, match := range matches {
		if termRank := terminologyTermMatchRank(match); termRank > rank {
			rank = termRank
		}
	}
	return rank
}

func terminologyTermMatchRank(match TerminologyTermMatch) int {
	if !match.Tolerated {
		return 10 + terminologySeverityRank(match.Severity)
	}
	return terminologySeverityRank(match.Severity)
}

func terminologySeverityRank(severity string) int {
	switch config.NormalizeTerminologySeverity(severity) {
	case config.TerminologySeverityError:
		return 2
	case config.TerminologySeverityWarning:
		return 1
	default:
		return 0
	}
}

func sortTerminologyFindings(findings []TerminologyFinding) {
	sort.Slice(findings, func(i, j int) bool {
		switch {
		case terminologyFindingRank(findings[i]) != terminologyFindingRank(findings[j]):
			return terminologyFindingRank(findings[i]) > terminologyFindingRank(findings[j])
		case findings[i].Score != findings[j].Score:
			return findings[i].Score > findings[j].Score
		case len(findings[i].Sections) != len(findings[j].Sections):
			return len(findings[i].Sections) > len(findings[j].Sections)
		default:
			return findings[i].Ref < findings[j].Ref
		}
	})
}

func terminologyFindingRank(finding TerminologyFinding) int {
	rank := 0
	for _, section := range finding.Sections {
		for _, match := range section.Matches {
			if termRank := terminologyTermMatchRank(match); termRank > rank {
				rank = termRank
			}
		}
	}
	return rank
}
