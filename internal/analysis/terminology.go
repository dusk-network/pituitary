package analysis

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
)

const terminologyEvidenceThreshold = 0.15

var terminologyCompatibilityPattern = regexp.MustCompile(`(?i)\b(compatibility|compatible|debug|projection|alias|shim|fallback|migration|migrating|transitional)\b`)

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
	Section    string               `json:"section"`
	Terms      []string             `json:"terms"`
	Excerpt    string               `json:"excerpt"`
	Assessment string               `json:"assessment,omitempty"`
	Evidence   *TerminologyEvidence `json:"evidence,omitempty"`
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
	Scope          TerminologyAuditScope   `json:"scope"`
	Terms          []string                `json:"terms"`
	CanonicalTerms []string                `json:"canonical_terms,omitempty"`
	AnchorSpecs    []TerminologyAnchorSpec `json:"anchor_specs,omitempty"`
	Findings       []TerminologyFinding    `json:"findings"`
	Warnings       []Warning               `json:"warnings,omitempty"`
}

type terminologyArtifact struct {
	Ref       string
	Kind      string
	Title     string
	SourceRef string
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

type terminologyTextMatch struct {
	Terms      []string
	Excerpt    string
	Assessment string
	Suppressed bool
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

	normalized, err := normalizeTerminologyAuditRequest(request)
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

	artifacts, err := loadTerminologyArtifacts(repo, normalized, anchor)
	if err != nil {
		return nil, err
	}

	anchors, evidenceSections, warnings, err := loadTerminologyAnchors(repo, normalized, anchor)
	if err != nil {
		return nil, err
	}

	matchers := compileTerminologyMatchers(normalized.Terms)
	findings := auditTerminologyArtifacts(artifacts, matchers, evidenceSections)

	warningSpecs := make([]specDocument, 0, len(anchors))
	warningSpecs = append(warningSpecs, anchors...)
	warnings = append(warnings, buildSpecInferenceWarnings("terminology audit", warningSpecs...)...)

	return &TerminologyAuditResult{
		Scope: TerminologyAuditScope{
			Mode:          terminologyAuditMode(normalized),
			ArtifactKinds: terminologyArtifactKinds(normalized.Scope),
			SpecRef:       normalized.SpecRef,
		},
		Terms:          normalized.Terms,
		CanonicalTerms: normalized.CanonicalTerms,
		AnchorSpecs:    terminologyAnchorSpecs(anchors),
		Findings:       findings,
		Warnings:       uniqueWarnings(warnings),
	}, nil
}

func normalizeTerminologyAuditRequest(request TerminologyAuditRequest) (TerminologyAuditRequest, error) {
	request.SpecRef = stringsTrimSpace(request.SpecRef)
	request.Terms = uniqueStrings(request.Terms)
	request.CanonicalTerms = uniqueStrings(request.CanonicalTerms)
	request.Scope = defaultString(stringsTrimSpace(request.Scope), "all")

	if len(request.Terms) == 0 {
		return TerminologyAuditRequest{}, fmt.Errorf("at least one term is required")
	}

	switch request.Scope {
	case "all", "docs", "specs":
		return request, nil
	default:
		return TerminologyAuditRequest{}, fmt.Errorf("scope %q is invalid", request.Scope)
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
			SourceRef: spec.Record.SourceRef,
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

func auditTerminologyArtifacts(artifacts []terminologyArtifact, matchers []terminologyMatcher, evidenceSections []terminologyEvidenceSection) []TerminologyFinding {
	findings := make([]TerminologyFinding, 0, len(artifacts))
	for _, artifact := range artifacts {
		finding := auditTerminologyArtifact(artifact, matchers, evidenceSections)
		if finding == nil {
			continue
		}
		findings = append(findings, *finding)
	}

	sort.Slice(findings, func(i, j int) bool {
		switch {
		case findings[i].Score != findings[j].Score:
			return findings[i].Score > findings[j].Score
		case len(findings[i].Sections) != len(findings[j].Sections):
			return len(findings[i].Sections) > len(findings[j].Sections)
		default:
			return findings[i].Ref < findings[j].Ref
		}
	})
	return findings
}

func auditTerminologyArtifact(artifact terminologyArtifact, matchers []terminologyMatcher, evidenceSections []terminologyEvidenceSection) *TerminologyFinding {
	sections := make([]TerminologySectionFinding, 0, len(artifact.Sections))
	matchedTerms := make([]string, 0, len(matchers))
	bestScore := 0.0

	for _, section := range artifact.Sections {
		terms, excerpt, assessment := terminologySectionTerms(section, matchers)
		if len(terms) == 0 {
			continue
		}

		sectionFinding := TerminologySectionFinding{
			Section:    defaultString(stringsTrimSpace(section.Heading), "(body)"),
			Terms:      terms,
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
	}

	if len(sections) == 0 {
		return nil
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
	}
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
		if match.Suppressed {
			continue
		}
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
		matches = append(matches, terminologyTextMatch{
			Terms:      terms,
			Excerpt:    trimmed,
			Assessment: "exact match in body text without compatibility-only markers",
			Suppressed: isCompatibilityOnlyTerminologyReference(trimmed),
		})
	}
	if heading := stringsTrimSpace(section.Heading); heading != "" {
		if terms := matchedTerminologyTerms(heading, matchers); len(terms) > 0 {
			matches = append(matches, terminologyTextMatch{
				Terms:      terms,
				Excerpt:    heading,
				Assessment: "exact match in section heading",
				Suppressed: isCompatibilityOnlyTerminologyReference(heading),
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
