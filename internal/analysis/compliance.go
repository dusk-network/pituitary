package analysis

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/resultmeta"
	"golang.org/x/sync/errgroup"
)

const (
	complianceSemanticSuggestionLimit     = 4
	complianceSemanticSuggestionThreshold = 0.45
	complianceWeakSuggestionThreshold     = 0.25
	complianceFactorSpecMetadataGap       = "spec_metadata_gap"
	complianceFactorCodeEvidenceGap       = "code_evidence_gap"
	adjudicationConcurrency               = 4
)

var complianceRequestsPerMinutePattern = regexp.MustCompile(`(?i)\b(\d+)\s+requests?\s+per\s+minute\b`)

var compliancePhraseFamilies = []phraseFamily{
	{
		Canonical: "per tenant",
		Variants:  []string{"per tenant", "per-tenant", "tenant scoped", "tenant-scoped", "tenant specific", "tenant-specific"},
	},
	{
		Canonical: "api key",
		Variants:  []string{"per api key", "per-api-key", "api key", "api-key"},
	},
	{
		Canonical: "sliding window",
		Variants:  []string{"sliding window", "sliding-window"},
	},
	{
		Canonical: "fixed window",
		Variants:  []string{"fixed window", "fixed-window"},
	},
	{
		Canonical: "tenant-specific overrides",
		Variants:  []string{"tenant-specific overrides", "tenant specific overrides", "tenant overrides", "override", "overrides"},
	},
	{
		Canonical: "short bursts",
		Variants:  []string{"short bursts", "burst budget", "burst capacity", "allow short bursts", "burst"},
	},
}

var complianceExclusiveFamilies = []exclusivePhraseFamily{
	{
		Expected: compliancePhraseFamilies[0],
		Observed: compliancePhraseFamilies[1],
	},
	{
		Expected: compliancePhraseFamilies[2],
		Observed: compliancePhraseFamilies[3],
	},
}

var complianceStopwords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {},
	"be": {}, "by": {}, "for": {}, "from": {}, "in": {}, "into": {},
	"is": {}, "it": {}, "of": {}, "on": {}, "or": {}, "that": {},
	"the": {}, "their": {}, "this": {}, "through": {}, "to": {}, "up": {},
	"use": {}, "with": {}, "once": {}, "both": {}, "same": {}, "rather": {},
	"than": {}, "while": {}, "where": {}, "when": {},
}

type phraseFamily struct {
	Canonical string
	Variants  []string
}

type exclusivePhraseFamily struct {
	Expected phraseFamily
	Observed phraseFamily
}

// ComplianceRequest is the normalized compliance input.
type ComplianceRequest struct {
	Paths         []string `json:"paths,omitempty"`
	DiffFile      string   `json:"diff_file,omitempty"`
	DiffText      string   `json:"diff_text,omitempty"`
	AtDate        string   `json:"at_date,omitempty"`
	MinConfidence string   `json:"min_confidence,omitempty"`
}

// ComplianceRelevantSpec reports one accepted spec considered during evaluation.
type ComplianceRelevantSpec struct {
	SpecRef string   `json:"spec_ref"`
	Title   string   `json:"title"`
	Paths   []string `json:"paths,omitempty"`
	Basis   []string `json:"basis,omitempty"`
}

// ComplianceFinding reports one compliant, conflicting, or unspecified item.
type ComplianceFinding struct {
	Path            string  `json:"path"`
	SpecRef         string  `json:"spec_ref,omitempty"`
	Title           string  `json:"title,omitempty"`
	SectionHeading  string  `json:"section_heading,omitempty"`
	Code            string  `json:"code"`
	Message         string  `json:"message"`
	Traceability    string  `json:"traceability,omitempty"`
	LimitingFactor  string  `json:"limiting_factor,omitempty"`
	Suggestion      string  `json:"suggestion,omitempty"`
	Expected        string  `json:"expected,omitempty"`
	Observed        string  `json:"observed,omitempty"`
	Provenance      string  `json:"provenance,omitempty"`
	Confidence      float64 `json:"confidence,omitempty"`
	Classification  string  `json:"classification,omitempty"`
	RationaleText   string  `json:"rationale_text,omitempty"`
	RationaleKind   string  `json:"rationale_kind,omitempty"`
	RationaleSymbol string  `json:"rationale_symbol,omitempty"`
}

// ComplianceResult is the structured compliance output.
type ComplianceResult struct {
	Paths              []string                      `json:"paths"`
	RelevantSpecs      []ComplianceRelevantSpec      `json:"relevant_specs,omitempty"`
	Compliant          []ComplianceFinding           `json:"compliant"`
	Conflicts          []ComplianceFinding           `json:"conflicts"`
	Unspecified        []ComplianceFinding           `json:"unspecified"`
	UnspecifiedSummary *ComplianceUnspecifiedSummary `json:"unspecified_summary,omitempty"`
	Relations          []ComplianceRelation          `json:"relations"`
	RelationSummary    ComplianceRelationSummary     `json:"relation_summary"`
	Discovery          ComplianceDiscovery           `json:"discovery"`
	TopSuggestions     []string                      `json:"top_suggestions,omitempty"`
	Runtime            *CommandRuntime               `json:"runtime,omitempty"`
	ContentTrust       *resultmeta.ContentTrust      `json:"content_trust,omitempty"`
}

type complianceTarget struct {
	Path         string
	Content      string
	Embedding    []float64
	DuplicateKey string
	RemovedOnly  bool
}

type complianceEvaluationTarget struct {
	Target             complianceTarget
	ExplicitRefs       []string
	ExplicitTargetRefs map[string]string
}

type complianceAssessment struct {
	Kind    string
	Finding ComplianceFinding
	Score   float64
}

type complianceRelevantAccumulator struct {
	paths map[string]struct{}
	basis map[string]struct{}
}

type parsedDiffTarget struct {
	Path    string
	Added   []string
	Context []string
	Removed []string
}

// ComplianceUnspecifiedSummary breaks unspecified findings into actionable
// categories so consumers can distinguish missing governance from already-
// governed-but-underexercised surfaces.
type ComplianceUnspecifiedSummary struct {
	Total                     int `json:"total"`
	MissingGovernanceEdge     int `json:"missing_governance_edge"`
	ExplicitButUnderexercised int `json:"explicit_but_underexercised"`
}

// ComplianceRelationEndpoint identifies one endpoint in an explicit
// spec-to-target compliance relation.
type ComplianceRelationEndpoint struct {
	NodeKind string `json:"node_kind"`
	Ref      string `json:"ref"`
}

// ComplianceRelation reports the state of one explicit accepted spec relation
// already present in the index.
type ComplianceRelation struct {
	ID              string                       `json:"id"`
	Type            string                       `json:"type"`
	DeclaredBy      string                       `json:"declared_by"`
	Verifier        string                       `json:"verifier"`
	State           string                       `json:"state"`
	Endpoints       []ComplianceRelationEndpoint `json:"endpoints"`
	Path            string                       `json:"path"`
	SectionHeading  string                       `json:"section_heading,omitempty"`
	Code            string                       `json:"code,omitempty"`
	Message         string                       `json:"message,omitempty"`
	Traceability    string                       `json:"traceability,omitempty"`
	LimitingFactor  string                       `json:"limiting_factor,omitempty"`
	Suggestion      string                       `json:"suggestion,omitempty"`
	Expected        string                       `json:"expected,omitempty"`
	Observed        string                       `json:"observed,omitempty"`
	Provenance      string                       `json:"provenance,omitempty"`
	Confidence      float64                      `json:"confidence,omitempty"`
	Classification  string                       `json:"classification,omitempty"`
	RationaleText   string                       `json:"rationale_text,omitempty"`
	RationaleKind   string                       `json:"rationale_kind,omitempty"`
	RationaleSymbol string                       `json:"rationale_symbol,omitempty"`
}

// ComplianceRelationSummary counts explicit relations by status.
type ComplianceRelationSummary struct {
	Total               int `json:"total"`
	Verified            int `json:"verified"`
	Drifted             int `json:"drifted"`
	UnverifiableInScope int `json:"unverifiable_in_scope"`
}

// ComplianceDiscovery reports changed paths that lack any explicit accepted
// spec relation and therefore need governance authoring attention.
type ComplianceDiscovery struct {
	FilesWithZeroRelations []ComplianceDiscoveryItem `json:"files_with_zero_relations"`
}

// ComplianceDiscoveryItem preserves the existing operator guidance for paths
// that have no explicit accepted governing relation.
type ComplianceDiscoveryItem struct {
	Path           string `json:"path"`
	SpecRef        string `json:"spec_ref,omitempty"`
	Title          string `json:"title,omitempty"`
	Code           string `json:"code"`
	Message        string `json:"message"`
	Traceability   string `json:"traceability,omitempty"`
	LimitingFactor string `json:"limiting_factor,omitempty"`
	Suggestion     string `json:"suggestion,omitempty"`
}

type complianceAdjudicationCandidate struct {
	spec    specDocument
	targets []complianceTarget
}

// CheckCompliance determines whether provided code or diffs align with accepted specs.
func CheckCompliance(cfg *config.Config, request ComplianceRequest) (*ComplianceResult, error) {
	return CheckComplianceContext(context.Background(), cfg, request)
}

// CheckComplianceContext determines whether provided code or diffs align with accepted specs.
func CheckComplianceContext(ctx context.Context, cfg *config.Config, request ComplianceRequest) (*ComplianceResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	request, err := normalizeComplianceRequest(request)
	if err != nil {
		return nil, err
	}

	repo, err := openAnalysisRepositoryContext(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer repo.Close()
	repo.atDate = strings.TrimSpace(request.AtDate)
	repo.minConfidence = strings.TrimSpace(request.MinConfidence)

	targets, err := loadComplianceTargetsContext(ctx, cfg, request)
	if err != nil {
		return nil, err
	}
	evaluationTargets, err := prepareComplianceEvaluationTargetsContext(ctx, repo, cfg, targets)
	if err != nil {
		return nil, err
	}
	evaluationTargets = collapseComplianceDuplicateEvaluationTargets(evaluationTargets)

	result := newComplianceResult(cfg, targets)
	relevant := map[string]*complianceRelevantAccumulator{}

	adjudicationCandidates, err := evaluateComplianceDeterministically(repo, evaluationTargets, result, relevant)
	if err != nil {
		return nil, err
	}
	if err := refineComplianceSemantically(ctx, cfg, repo, adjudicationCandidates, result); err != nil {
		return nil, err
	}
	if err := finalizeComplianceResult(ctx, cfg, repo, evaluationTargets, relevant, result); err != nil {
		return nil, err
	}
	return result, nil
}

func newComplianceResult(cfg *config.Config, targets []complianceTarget) *ComplianceResult {
	analysisRuntime := newAnalysisRuntimeUsage(cfg.Runtime.Analysis)
	var runtime *CommandRuntime
	if analysisRuntime != nil {
		runtime = &CommandRuntime{Analysis: analysisRuntime}
	}

	return &ComplianceResult{
		Paths:        complianceTargetPaths(targets),
		Compliant:    []ComplianceFinding{},
		Conflicts:    []ComplianceFinding{},
		Unspecified:  []ComplianceFinding{},
		Relations:    []ComplianceRelation{},
		Discovery:    ComplianceDiscovery{FilesWithZeroRelations: []ComplianceDiscoveryItem{}},
		Runtime:      runtime,
		ContentTrust: resultmeta.UntrustedWorkspaceText(),
	}
}

func evaluateComplianceDeterministically(repo *analysisRepository, evaluationTargets []complianceEvaluationTarget, result *ComplianceResult, relevant map[string]*complianceRelevantAccumulator) (map[string]*complianceAdjudicationCandidate, error) {
	adjudicationCandidates := map[string]*complianceAdjudicationCandidate{}

	for _, item := range evaluationTargets {
		target := item.Target
		if len(item.ExplicitRefs) == 0 {
			suggestions, err := repo.complianceSemanticSuggestions(target.Embedding)
			if err != nil {
				return nil, err
			}
			finding, specRef, title, basis := complianceNoSpecFinding(repo, target, suggestions)
			if specRef != "" {
				addComplianceRelevantSpec(relevant, specRef, target.Path, basis)
				finding.SpecRef = specRef
				finding.Title = title
			}
			result.Unspecified = append(result.Unspecified, finding)
			continue
		}

		specs, err := repo.loadSelectedSpecs(item.ExplicitRefs)
		if err != nil {
			return nil, err
		}
		for _, ref := range item.ExplicitRefs {
			spec, ok := specs[ref]
			if !ok {
				continue
			}
			addComplianceRelevantSpec(relevant, ref, target.Path, "applies_to")
			assessment := assessComplianceSpec(spec, target)
			assessment.Finding.Provenance = ProvenanceLiteral
			appendComplianceFinding(result, assessment.Finding, assessment.Kind)
			if assessment.Kind == "compliant" {
				continue
			}
			if _, ok := adjudicationCandidates[ref]; !ok {
				adjudicationCandidates[ref] = &complianceAdjudicationCandidate{spec: spec}
			}
			adjudicationCandidates[ref].targets = append(adjudicationCandidates[ref].targets, target)
		}
	}

	return adjudicationCandidates, nil
}

func appendComplianceFinding(result *ComplianceResult, finding ComplianceFinding, kind string) {
	switch kind {
	case "conflict":
		result.Conflicts = append(result.Conflicts, finding)
	case "compliant":
		result.Compliant = append(result.Compliant, finding)
	default:
		result.Unspecified = append(result.Unspecified, finding)
	}
}

func refineComplianceSemantically(ctx context.Context, cfg *config.Config, repo *analysisRepository, adjudicationCandidates map[string]*complianceAdjudicationCandidate, result *ComplianceResult) error {
	analyzer, err := newQualitativeAnalyzer(cfg.Runtime.Analysis)
	if err != nil {
		return err
	}
	analyzer = qualitativeAnalyzerWithTimings(ctx, analyzer)
	adjudicator, ok := analyzer.(complianceAdjudicator)
	if !ok || len(adjudicationCandidates) == 0 {
		return nil
	}

	adjudicator = complianceAdjudicatorWithTimings(ctx, adjudicator)
	if result.Runtime != nil && result.Runtime.Analysis != nil {
		result.Runtime.Analysis.Used = true
	}
	adjFindings, err := runComplianceAdjudication(ctx, adjudicator, repo, adjudicationCandidates, result)
	if err != nil {
		return err
	}
	result.Conflicts = append(result.Conflicts, adjFindings...)
	return nil
}

func finalizeComplianceResult(ctx context.Context, cfg *config.Config, repo *analysisRepository, evaluationTargets []complianceEvaluationTarget, relevant map[string]*complianceRelevantAccumulator, result *ComplianceResult) error {
	allSpecRefs := complianceRelevantSpecRefs(relevant)
	if len(allSpecRefs) > 0 {
		specs, err := repo.loadSelectedSpecs(allSpecRefs)
		if err != nil {
			return err
		}
		result.RelevantSpecs = buildComplianceRelevantSpecs(specs, relevant)
	}

	classifyComplianceConflicts(ctx, cfg, result)
	sortComplianceFindings(result.Compliant)
	sortComplianceFindings(result.Conflicts)
	sortComplianceFindings(result.Unspecified)
	result.UnspecifiedSummary = buildComplianceUnspecifiedSummary(result.Unspecified)
	explicitRelationTargets := complianceExplicitRelationTargets(evaluationTargets)
	result.Relations = buildComplianceRelations(result, explicitRelationTargets)
	result.RelationSummary = buildComplianceRelationSummary(result.Relations)
	result.Discovery = buildComplianceDiscovery(result.Unspecified, explicitRelationTargets)
	result.TopSuggestions = buildComplianceTopSuggestions(result)
	return nil
}

// runComplianceAdjudication sends narrowed targets to the analysis model for
// semantic adjudication. Only findings that are genuinely new (not already
// reported by deterministic evaluation) are returned.
func runComplianceAdjudication(ctx context.Context, adjudicator complianceAdjudicator, repo *analysisRepository, candidates map[string]*complianceAdjudicationCandidate, existing *ComplianceResult) ([]ComplianceFinding, error) {
	existingConflicts := make(map[string]struct{})
	for _, finding := range existing.Conflicts {
		existingConflicts[finding.Path+"\x00"+finding.SpecRef] = struct{}{}
	}

	type adjudicationResult struct {
		ref       string
		candidate *complianceAdjudicationCandidate
		response  *complianceAdjudicationResponse
	}

	var (
		mu      sync.Mutex
		results []adjudicationResult
	)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(adjudicationConcurrency)

	for ref, candidate := range candidates {
		g.Go(func() error {
			specPrompt := analysisSpecFromDocument(candidate.spec)

			targets := make([]adjudicationTarget, 0, len(candidate.targets))
			for _, target := range candidate.targets {
				specSections := bestSpecSectionsForTarget(candidate.spec, target)
				targets = append(targets, adjudicationTarget{
					Path:     target.Path,
					Content:  target.Content,
					Sections: specSections,
				})
			}

			response, err := adjudicator.AdjudicateCompliance(gctx, complianceAdjudicationRequest{
				Spec:    specPrompt,
				Targets: targets,
			})
			if err != nil {
				return fmt.Errorf("adjudicate compliance for %s: %w", ref, err)
			}

			mu.Lock()
			results = append(results, adjudicationResult{ref: ref, candidate: candidate, response: response})
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	var newFindings []ComplianceFinding
	for _, r := range results {
		for _, adj := range r.response.Adjudications {
			if adj.Classification != "conflict" {
				continue
			}
			key := adj.Path + "\x00" + r.ref
			if _, exists := existingConflicts[key]; exists {
				continue
			}
			existingConflicts[key] = struct{}{}
			newFindings = append(newFindings, ComplianceFinding{
				Path:           adj.Path,
				SpecRef:        r.ref,
				Title:          r.candidate.spec.Record.Title,
				SectionHeading: adj.ViolatedSection,
				Code:           "semantic_conflict",
				Message:        adj.Message,
				Expected:       adj.Expected,
				Observed:       adj.Observed,
				Provenance:     ProvenanceModelAdjudication,
				Confidence:     adj.Confidence,
			})
		}
	}
	return newFindings, nil
}

// bestSpecSectionsForTarget selects the spec sections most relevant to a
// compliance target by embedding similarity, falling back to all sections.
func bestSpecSectionsForTarget(spec specDocument, target complianceTarget) []analysisSectionPrompt {
	if len(target.Embedding) == 0 || len(spec.Sections) == 0 {
		return analysisSectionsFromEmbedded(spec.Sections)
	}

	type scored struct {
		section embeddedSection
		score   float64
	}
	candidates := make([]scored, 0, len(spec.Sections))
	for _, section := range spec.Sections {
		if len(section.Embedding) == 0 {
			candidates = append(candidates, scored{section: section, score: 0})
			continue
		}
		candidates = append(candidates, scored{
			section: section,
			score:   cosineSimilarity(target.Embedding, section.Embedding),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	limit := minInt(len(candidates), analysisPromptSectionLimit)
	sections := make([]embeddedSection, 0, limit)
	for i := 0; i < limit; i++ {
		sections = append(sections, candidates[i].section)
	}
	return analysisSectionsFromEmbedded(sections)
}

func normalizeComplianceRequest(request ComplianceRequest) (ComplianceRequest, error) {
	request.Paths = uniqueStrings(request.Paths)
	request.DiffFile = stringsTrimSpace(request.DiffFile)

	hasPaths := len(request.Paths) > 0
	hasDiff := stringsTrimSpace(request.DiffText) != ""
	switch {
	case hasPaths && hasDiff:
		return ComplianceRequest{}, fmt.Errorf("exactly one of paths or diff_text is allowed")
	case !hasPaths && !hasDiff:
		return ComplianceRequest{}, fmt.Errorf("one of paths or diff_text is required")
	default:
		return request, nil
	}
}

func loadComplianceTargetsContext(ctx context.Context, cfg *config.Config, request ComplianceRequest) ([]complianceTarget, error) {
	if len(request.Paths) > 0 {
		return loadPathComplianceTargetsContext(ctx, cfg, request.Paths)
	}
	return loadDiffComplianceTargetsContext(ctx, cfg, request.DiffText)
}

func loadPathComplianceTargetsContext(ctx context.Context, cfg *config.Config, paths []string) ([]complianceTarget, error) {
	targets := make([]complianceTarget, 0, len(paths))

	for _, rawPath := range paths {
		relPath, absPath, err := resolveWorkspaceFilePath(cfg.Workspace.RootPath, rawPath)
		if err != nil {
			return nil, err
		}

		info, err := os.Stat(absPath)
		if err != nil {
			return nil, fmt.Errorf("read path %q: %w", rawPath, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("path %q is a directory; --path expects a file", rawPath)
		}

		// #nosec G304 -- absPath is resolved under the workspace root by resolveWorkspaceFilePath.
		data, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("read path %q: %w", rawPath, err)
		}

		targets = append(targets, complianceTarget{
			Path:         relPath,
			Content:      string(data),
			DuplicateKey: complianceContentDigest(string(data)),
		})
	}
	return targets, nil
}

func loadDiffComplianceTargetsContext(ctx context.Context, cfg *config.Config, diffText string) ([]complianceTarget, error) {
	parsed, err := parseDiffTargets(diffText)
	if err != nil {
		return nil, err
	}
	return loadParsedDiffComplianceTargetsContext(ctx, cfg, parsed)
}

func loadParsedDiffComplianceTargetsContext(ctx context.Context, cfg *config.Config, parsed []parsedDiffTarget) ([]complianceTarget, error) {
	targets := make([]complianceTarget, 0, len(parsed))
	for _, item := range parsed {
		content, removedOnly := parsedDiffTargetContent(item)
		if stringsTrimSpace(content) == "" && !removedOnly {
			continue
		}
		targets = append(targets, complianceTarget{
			Path:         item.Path,
			Content:      content,
			DuplicateKey: complianceDuplicateKey(cfg.Workspace.RootPath, item.Path, content),
			RemovedOnly:  removedOnly,
		})
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("diff_text does not contain any changed paths with readable content")
	}
	return targets, nil
}

func prepareComplianceEvaluationTargetsContext(ctx context.Context, repo *analysisRepository, cfg *config.Config, targets []complianceTarget) ([]complianceEvaluationTarget, error) {
	prepared := make([]complianceEvaluationTarget, 0, len(targets))
	fallbackIndexes := make([]int, 0, len(targets))
	for _, target := range targets {
		governedRefs, err := index.ResolveGovernedRefsForPathContext(ctx, repo.db, target.Path)
		if err != nil {
			return nil, err
		}
		explicitTargetRefs, err := repo.explicitTargetRefsForGovernedRefs(target.Path, governedRefs)
		if err != nil {
			return nil, err
		}
		explicitRefs := sortedComplianceExplicitRefs(explicitTargetRefs)
		prepared = append(prepared, complianceEvaluationTarget{
			Target:             target,
			ExplicitRefs:       explicitRefs,
			ExplicitTargetRefs: explicitTargetRefs,
		})
		if len(explicitRefs) == 0 {
			fallbackIndexes = append(fallbackIndexes, len(prepared)-1)
		}
	}
	if len(fallbackIndexes) == 0 {
		return prepared, nil
	}
	if err := embedComplianceFallbackTargetsContext(ctx, cfg, prepared, fallbackIndexes); err != nil {
		return nil, err
	}
	return prepared, nil
}

func collapseComplianceDuplicateEvaluationTargets(targets []complianceEvaluationTarget) []complianceEvaluationTarget {
	if len(targets) < 2 {
		return targets
	}

	groups := make(map[string][]int)
	for i, item := range targets {
		key := stringsTrimSpace(item.Target.DuplicateKey)
		if key == "" || item.Target.RemovedOnly {
			continue
		}
		groups[key] = append(groups[key], i)
	}
	if len(groups) == 0 {
		return targets
	}

	keep := make([]bool, len(targets))
	for i := range keep {
		keep[i] = true
	}

	for _, indexes := range groups {
		if len(indexes) < 2 {
			continue
		}
		rep, ok := complianceDuplicateRepresentativeIndex(targets, indexes)
		if !ok {
			continue
		}
		for _, idx := range indexes {
			if idx != rep {
				keep[idx] = false
			}
		}
	}

	result := make([]complianceEvaluationTarget, 0, len(targets))
	for i, item := range targets {
		if keep[i] {
			result = append(result, item)
		}
	}
	return result
}

func complianceDuplicateRepresentativeIndex(targets []complianceEvaluationTarget, indexes []int) (int, bool) {
	explicitSignatures := map[string]struct{}{}
	explicitIndexes := make([]int, 0, len(indexes))
	for _, idx := range indexes {
		signature := complianceExplicitRefSignature(targets[idx].ExplicitRefs)
		if signature == "" {
			continue
		}
		explicitSignatures[signature] = struct{}{}
		explicitIndexes = append(explicitIndexes, idx)
	}
	if len(explicitSignatures) > 1 {
		return 0, false
	}

	candidates := indexes
	if len(explicitIndexes) > 0 {
		candidates = explicitIndexes
	}
	best := candidates[0]
	for _, idx := range candidates[1:] {
		if complianceRepresentativePathLess(targets[idx].Target.Path, targets[best].Target.Path) {
			best = idx
		}
	}
	return best, true
}

func complianceExplicitRefSignature(refs []string) string {
	if len(refs) == 0 {
		return ""
	}
	sorted := append([]string(nil), refs...)
	sort.Strings(sorted)
	return strings.Join(sorted, "\x00")
}

func complianceRepresentativePathLess(left, right string) bool {
	leftHidden, leftDepth := compliancePathPreference(left)
	rightHidden, rightDepth := compliancePathPreference(right)
	switch {
	case leftHidden != rightHidden:
		return leftHidden < rightHidden
	case leftDepth != rightDepth:
		return leftDepth < rightDepth
	case len(left) != len(right):
		return len(left) < len(right)
	default:
		return left < right
	}
}

func compliancePathPreference(path string) (hiddenSegments int, depth int) {
	for _, segment := range strings.Split(path, "/") {
		if segment == "" {
			continue
		}
		depth++
		if strings.HasPrefix(segment, ".") {
			hiddenSegments++
		}
	}
	return hiddenSegments, depth
}

func complianceDuplicateKey(workspaceRoot, relPath, fallbackContent string) string {
	if workspaceRoot != "" && relPath != "" {
		_, absPath, err := resolveWorkspaceFilePath(workspaceRoot, relPath)
		if err == nil {
			if data, readErr := os.ReadFile(absPath); readErr == nil {
				return complianceContentDigest(string(data))
			}
		}
	}
	if stringsTrimSpace(fallbackContent) == "" {
		return ""
	}
	return complianceContentDigest(fallbackContent)
}

func complianceContentDigest(content string) string {
	if content == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum[:])
}

func embedComplianceFallbackTargetsContext(ctx context.Context, cfg *config.Config, targets []complianceEvaluationTarget, indexes []int) error {
	embedder, err := index.NewEmbedder(cfg.Runtime.Embedder)
	if err != nil {
		return err
	}
	embedder = embedderWithTimings(ctx, embedder)

	texts := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		target := targets[idx].Target
		texts = append(texts, textForEmbedding(target.Path, target.Path, target.Content))
	}

	vectors, err := embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return fmt.Errorf("embed compliance fallback targets: %w", err)
	}
	for i, idx := range indexes {
		targets[idx].Target.Embedding = vectors[i]
	}
	return nil
}

func resolveWorkspaceFilePath(rootPath, rawPath string) (string, string, error) {
	rawPath = stringsTrimSpace(rawPath)
	if rawPath == "" {
		return "", "", fmt.Errorf("path values must not be empty")
	}

	absPath := rawPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(rootPath, rawPath)
	}
	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve path %q: %w", rawPath, err)
	}

	relPath, err := filepath.Rel(rootPath, absPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve path %q relative to workspace: %w", rawPath, err)
	}
	if relPath == ".." || stringsHasPrefix(relPath, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path %q is outside workspace root %s", rawPath, rootPath)
	}

	return normalizeCompliancePath(relPath), absPath, nil
}

func normalizeCompliancePath(path string) string {
	return pathpkg.Clean(filepath.ToSlash(stringsTrimSpace(path)))
}

func parseDiffTargets(diffText string) ([]parsedDiffTarget, error) {
	lines := strings.Split(strings.ReplaceAll(diffText, "\r\n", "\n"), "\n")

	var (
		current *parsedDiffTarget
		result  []parsedDiffTarget
	)
	flush := func() {
		if current == nil || stringsTrimSpace(current.Path) == "" {
			return
		}
		result = append(result, *current)
		current = nil
	}

	for _, line := range lines {
		switch {
		case stringsHasPrefix(line, "diff --git "):
			flush()
			current = &parsedDiffTarget{}
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				if path := parseDiffPathToken(fields[3]); path != "" {
					current.Path = path
				} else {
					current.Path = parseDiffPathToken(fields[2])
				}
			}
		case stringsHasPrefix(line, "+++ "):
			if current == nil {
				current = &parsedDiffTarget{}
			}
			if path := parseDiffPathToken(stringsTrimSpace(strings.TrimPrefix(line, "+++ "))); path != "" {
				current.Path = path
			}
		case stringsHasPrefix(line, "--- "):
			if current == nil {
				current = &parsedDiffTarget{}
			}
			if current.Path == "" {
				if path := parseDiffPathToken(stringsTrimSpace(strings.TrimPrefix(line, "--- "))); path != "" {
					current.Path = path
				}
			}
		case current != nil && stringsHasPrefix(line, "+") && !stringsHasPrefix(line, "+++"):
			current.Added = append(current.Added, strings.TrimPrefix(line, "+"))
		case current != nil && stringsHasPrefix(line, " "):
			current.Context = append(current.Context, strings.TrimPrefix(line, " "))
		case current != nil && stringsHasPrefix(line, "-") && !stringsHasPrefix(line, "---"):
			current.Removed = append(current.Removed, strings.TrimPrefix(line, "-"))
		}
	}
	flush()

	if len(result) == 0 {
		return nil, fmt.Errorf("diff_text does not contain any changed paths")
	}

	deduped := make(map[string]parsedDiffTarget, len(result))
	for _, item := range result {
		existing := deduped[item.Path]
		existing.Path = item.Path
		existing.Added = append(existing.Added, item.Added...)
		existing.Context = append(existing.Context, item.Context...)
		existing.Removed = append(existing.Removed, item.Removed...)
		deduped[item.Path] = existing
	}

	paths := make([]string, 0, len(deduped))
	for path := range deduped {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	merged := make([]parsedDiffTarget, 0, len(paths))
	for _, path := range paths {
		merged = append(merged, deduped[path])
	}
	return merged, nil
}

func parseDiffPathToken(token string) string {
	token = stringsTrimSpace(token)
	token = strings.TrimPrefix(token, "a/")
	token = strings.TrimPrefix(token, "b/")
	if token == "" || token == os.DevNull {
		return ""
	}
	return normalizeCompliancePath(token)
}

func parsedDiffTargetContent(target parsedDiffTarget) (string, bool) {
	switch {
	case len(target.Added) > 0:
		return strings.Join(target.Added, "\n"), false
	case len(target.Context) > 0:
		return strings.Join(target.Context, "\n"), false
	case len(target.Removed) > 0:
		return "", true
	default:
		return "", false
	}
}

func complianceTargetPaths(targets []complianceTarget) []string {
	paths := make([]string, 0, len(targets))
	for _, target := range targets {
		paths = append(paths, target.Path)
	}
	return uniqueStrings(paths)
}

func (r *analysisRepository) explicitTargetRefsForGovernedRefs(path string, refs []string) (map[string]string, error) {
	refs = uniqueStrings(refs)
	if len(refs) == 0 {
		return map[string]string{}, nil
	}

	var builder strings.Builder
	args := make([]any, 0, 2+len(refs))
	builder.WriteString(`
SELECT DISTINCT a.ref, e.to_ref
FROM edges e
JOIN artifacts a ON a.ref = e.from_ref
WHERE a.kind = ?
  AND a.status = ?
  AND e.edge_type = 'applies_to'
  AND e.to_ref IN (`)
	args = append(args, model.ArtifactKindSpec, model.StatusAccepted)
	for i, ref := range refs {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString("?")
		args = append(args, ref)
	}
	builder.WriteString(")\nORDER BY a.ref, e.to_ref")

	rows, err := r.db.QueryContext(r.ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("query governing spec relations: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var (
			specRef   string
			targetRef string
		)
		if err := rows.Scan(&specRef, &targetRef); err != nil {
			return nil, fmt.Errorf("scan governing spec relation: %w", err)
		}
		current, ok := result[specRef]
		if !ok || complianceRelationTargetLess(path, targetRef, current) {
			result[specRef] = targetRef
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate governing spec relations: %w", err)
	}
	return result, nil
}

func sortedComplianceExplicitRefs(refs map[string]string) []string {
	if len(refs) == 0 {
		return nil
	}
	result := make([]string, 0, len(refs))
	for ref := range refs {
		result = append(result, ref)
	}
	sort.Strings(result)
	return result
}

func complianceExplicitRelationTargets(targets []complianceEvaluationTarget) map[string]string {
	result := make(map[string]string)
	for _, target := range targets {
		for specRef, targetRef := range target.ExplicitTargetRefs {
			key := complianceRelationKey(target.Target.Path, specRef)
			current, ok := result[key]
			if !ok || complianceRelationTargetLess(target.Target.Path, targetRef, current) {
				result[key] = targetRef
			}
		}
	}
	return result
}

func complianceRelationKey(path, specRef string) string {
	return normalizeCompliancePath(path) + "\x00" + stringsTrimSpace(specRef)
}

func complianceRelationTargetLess(path, left, right string) bool {
	leftRank := complianceRelationTargetRank(path, left)
	rightRank := complianceRelationTargetRank(path, right)
	switch {
	case leftRank != rightRank:
		return leftRank < rightRank
	case len(left) != len(right):
		return len(left) < len(right)
	default:
		return left < right
	}
}

func complianceRelationTargetRank(path, ref string) int {
	ref = strings.TrimSpace(ref)
	switch {
	case stringsHasPrefix(ref, "doc://"):
		return 0
	case compliancePathLooksConfig(path) && stringsHasPrefix(ref, "config://"):
		return 1
	case stringsHasPrefix(ref, "code://"):
		return 2
	case stringsHasPrefix(ref, "config://"):
		return 3
	default:
		return 4
	}
}

func compliancePathLooksConfig(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml", ".json", ".toml", ".ini", ".cfg", ".conf":
		return true
	default:
		return false
	}
}

func (r *analysisRepository) complianceSemanticSuggestions(embedding []float64) ([]scoredArtifactRef, error) {
	scores, err := r.shortlistScoresForEmbedding(embedding, artifactShortlistQuery{
		Kind:     model.ArtifactKindSpec,
		Statuses: []string{model.StatusAccepted},
		Limit:    complianceSemanticSuggestionLimit,
	})
	if err != nil {
		return nil, err
	}

	result := make([]scoredArtifactRef, 0, len(scores))
	for ref, score := range scores {
		result = append(result, scoredArtifactRef{Ref: ref, Score: score})
	}
	sort.Slice(result, func(i, j int) bool {
		switch {
		case result[i].Score != result[j].Score:
			return result[i].Score > result[j].Score
		default:
			return result[i].Ref < result[j].Ref
		}
	})
	return result, nil
}

func complianceNoSpecFinding(repo *analysisRepository, target complianceTarget, suggestions []scoredArtifactRef) (ComplianceFinding, string, string, string) {
	finding := ComplianceFinding{
		Path:           target.Path,
		Code:           "no_governing_spec",
		Traceability:   "missing_applies_to",
		LimitingFactor: complianceFactorSpecMetadataGap,
		Message:        fmt.Sprintf("%s is not explicitly governed by any accepted applies_to ref in the current index; the limiting factor is accepted spec metadata, not indexing", target.Path),
		Suggestion:     complianceAppliesToSuggestion(target.Path, ""),
	}
	if len(suggestions) == 0 {
		finding.Message = fmt.Sprintf("%s is not explicitly governed by any accepted applies_to ref, and semantic retrieval did not find a strong accepted spec match; the limiting factor is accepted spec metadata, not indexing", target.Path)
		return finding, "", "", ""
	}

	refs := make([]string, 0, len(suggestions))
	for _, suggestion := range suggestions {
		refs = append(refs, suggestion.Ref)
	}
	specs, err := repo.loadSelectedSpecs(refs)
	if err != nil {
		return finding, "", "", ""
	}

	var (
		bestRef     string
		bestTitle   string
		bestHeading string
		bestScore   float64
		bestSection embeddedSection
	)
	for _, suggestion := range suggestions {
		spec, ok := specs[suggestion.Ref]
		if !ok {
			continue
		}
		section, score, ok := strongestComplianceSectionDetail(spec, target)
		if !ok {
			continue
		}
		heading := section.Heading
		if score > bestScore {
			bestRef = spec.Record.Ref
			bestTitle = spec.Record.Title
			bestHeading = heading
			bestScore = score
			bestSection = section
		}
	}
	if bestRef == "" || bestScore < complianceSemanticSuggestionThreshold || !complianceSupportsTraceability(bestSection.Content, target.Content) {
		if bestRef != "" && bestScore >= complianceWeakSuggestionThreshold {
			finding.Code = "weak_traceability"
			finding.Traceability = "weak_semantic_retrieval"
			finding.LimitingFactor = complianceFactorSpecMetadataGap
			finding.SpecRef = bestRef
			finding.Title = bestTitle
			finding.SectionHeading = bestHeading
			finding.Message = fmt.Sprintf("%s is not explicitly governed by any accepted applies_to ref; nearest accepted match %s was too weak to trust as the governing spec, so the limiting factor is still accepted spec metadata", target.Path, bestRef)
			finding.Suggestion = complianceAppliesToSuggestion(target.Path, bestRef)
			return finding, bestRef, bestTitle, "semantic"
		}
		finding.Message = fmt.Sprintf("%s is not explicitly governed by any accepted applies_to ref, and semantic retrieval only found low-confidence accepted matches; the limiting factor is accepted spec metadata, not indexing", target.Path)
		return finding, "", "", ""
	}

	finding.SpecRef = bestRef
	finding.Title = bestTitle
	finding.SectionHeading = bestHeading
	finding.Code = "traceability_gap"
	finding.Traceability = "semantic_neighbor_without_applies_to"
	finding.LimitingFactor = complianceFactorSpecMetadataGap
	finding.Message = fmt.Sprintf("%s is not explicitly governed by any accepted applies_to ref; nearest accepted match is %s, so the limiting factor is accepted spec metadata rather than indexing", target.Path, bestRef)
	finding.Suggestion = complianceAppliesToSuggestion(target.Path, bestRef)
	return finding, bestRef, bestTitle, "semantic"
}

func assessComplianceSpec(spec specDocument, target complianceTarget) complianceAssessment {
	if target.RemovedOnly {
		heading, score := strongestComplianceSection(spec, target)
		return complianceAssessment{
			Kind: "unspecified",
			Finding: ComplianceFinding{
				Path:           target.Path,
				SpecRef:        spec.Record.Ref,
				Title:          spec.Record.Title,
				SectionHeading: heading,
				Code:           "removed_content",
				Message:        fmt.Sprintf("%s removes code governed by %s; deleted lines are not treated as active evidence, so compliance cannot be confirmed from the diff alone and the limiting factor is diff evidence rather than governance metadata", target.Path, spec.Record.Ref),
				Traceability:   "explicit_applies_to",
				LimitingFactor: complianceFactorCodeEvidenceGap,
				Suggestion:     fmt.Sprintf("%s already governs %s via applies_to. Review the surrounding spec change with analyze-impact or review-spec before treating the deletion as compliant.", spec.Record.Ref, target.Path),
			},
			Score: score,
		}
	}

	var (
		bestSupport  *complianceAssessment
		bestConflict *complianceAssessment
	)

	for _, section := range spec.Sections {
		score := cosineSimilarity(target.Embedding, section.Embedding)
		statements := complianceStatements(section.Content)
		if len(statements) == 0 {
			statements = []string{stringsTrimSpace(section.Content)}
		}
		for _, statement := range statements {
			if statement == "" {
				continue
			}
			if finding, ok := conflictingComplianceFinding(spec, target, section, statement); ok {
				candidate := &complianceAssessment{Kind: "conflict", Finding: finding, Score: score}
				if bestConflict == nil || candidate.Score > bestConflict.Score {
					bestConflict = candidate
				}
				continue
			}
			if finding, ok := supportiveComplianceFinding(spec, target, section, statement); ok {
				candidate := &complianceAssessment{Kind: "compliant", Finding: finding, Score: score}
				if bestSupport == nil || candidate.Score > bestSupport.Score {
					bestSupport = candidate
				}
			}
		}
	}

	if bestConflict != nil {
		return *bestConflict
	}
	if bestSupport != nil {
		return *bestSupport
	}

	heading, score := strongestComplianceSection(spec, target)
	return complianceAssessment{
		Kind: "unspecified",
		Finding: ComplianceFinding{
			Path:           target.Path,
			SpecRef:        spec.Record.Ref,
			Title:          spec.Record.Title,
			SectionHeading: heading,
			Code:           "insufficient_evidence",
			Message:        fmt.Sprintf("%s governs %s but the provided code or diff does not contain enough deterministic evidence to confirm compliance; the limiting factor is literal code evidence rather than applies_to coverage", spec.Record.Ref, target.Path),
			Traceability:   "explicit_applies_to",
			LimitingFactor: complianceFactorCodeEvidenceGap,
			Suggestion:     fmt.Sprintf("%s already governs %s via applies_to. Strengthen the accepted requirement wording or the changed code surface with more literal evidence, then rerun check-compliance.", spec.Record.Ref, target.Path),
		},
		Score: score,
	}
}

func strongestComplianceSection(spec specDocument, target complianceTarget) (string, float64) {
	section, score, ok := strongestComplianceSectionDetail(spec, target)
	if !ok {
		return "", 0
	}
	return section.Heading, score
}

func strongestComplianceSectionDetail(spec specDocument, target complianceTarget) (embeddedSection, float64, bool) {
	var (
		bestSection embeddedSection
		bestScore   float64
		found       bool
	)
	for _, section := range spec.Sections {
		score := cosineSimilarity(target.Embedding, section.Embedding)
		if !found || score > bestScore {
			bestScore = score
			bestSection = section
			found = true
		}
	}
	return bestSection, bestScore, found
}

func complianceSupportsTraceability(specContent, targetContent string) bool {
	for _, statement := range complianceStatements(specContent) {
		if _, _, ok := complianceRequestsPerMinuteMatch(statement, targetContent); ok {
			return true
		}
		if _, _, ok := compliancePhraseMatch(statement, targetContent); ok {
			return true
		}
		shared, ratio := complianceLexicalOverlap(statement, targetContent)
		if ratio >= 0.55 && len(shared) >= 3 {
			return true
		}
	}
	return false
}

func complianceStatements(content string) []string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	statements := make([]string, 0, len(lines))
	for _, line := range lines {
		line = stringsTrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = stringsTrimSpace(line)
		if line == "" {
			continue
		}
		statements = append(statements, line)
	}
	return statements
}

func conflictingComplianceFinding(spec specDocument, target complianceTarget, section embeddedSection, statement string) (ComplianceFinding, bool) {
	expected, observed, ok := complianceRequestsPerMinuteConflict(statement, target.Content)
	if ok {
		return ComplianceFinding{
			Path:           target.Path,
			SpecRef:        spec.Record.Ref,
			Title:          spec.Record.Title,
			SectionHeading: section.Heading,
			Code:           "numeric_mismatch",
			Message:        fmt.Sprintf("%s conflicts with %s: observed %s where %s requires %s", target.Path, spec.Record.Ref, observed, spec.Record.Ref, expected),
			Traceability:   "explicit_applies_to",
			Expected:       expected,
			Observed:       observed,
		}, true
	}

	expected, observed, ok = compliancePhraseConflict(statement, target.Content)
	if ok {
		return ComplianceFinding{
			Path:           target.Path,
			SpecRef:        spec.Record.Ref,
			Title:          spec.Record.Title,
			SectionHeading: section.Heading,
			Code:           "phrase_mismatch",
			Message:        fmt.Sprintf("%s conflicts with %s: observed %s where %s expects %s", target.Path, spec.Record.Ref, observed, spec.Record.Ref, expected),
			Traceability:   "explicit_applies_to",
			Expected:       expected,
			Observed:       observed,
		}, true
	}

	return ComplianceFinding{}, false
}

func supportiveComplianceFinding(spec specDocument, target complianceTarget, section embeddedSection, statement string) (ComplianceFinding, bool) {
	expected, observed, ok := complianceRequestsPerMinuteMatch(statement, target.Content)
	if ok {
		return ComplianceFinding{
			Path:           target.Path,
			SpecRef:        spec.Record.Ref,
			Title:          spec.Record.Title,
			SectionHeading: section.Heading,
			Code:           "matching_claim",
			Message:        fmt.Sprintf("%s aligns with %s", target.Path, spec.Record.Ref),
			Traceability:   "explicit_applies_to",
			Expected:       expected,
			Observed:       observed,
		}, true
	}

	expected, observed, ok = compliancePhraseMatch(statement, target.Content)
	if ok {
		return ComplianceFinding{
			Path:           target.Path,
			SpecRef:        spec.Record.Ref,
			Title:          spec.Record.Title,
			SectionHeading: section.Heading,
			Code:           "matching_claim",
			Message:        fmt.Sprintf("%s aligns with %s", target.Path, spec.Record.Ref),
			Traceability:   "explicit_applies_to",
			Expected:       expected,
			Observed:       observed,
		}, true
	}

	shared, ratio := complianceLexicalOverlap(statement, target.Content)
	if ratio >= 0.55 && len(shared) >= 3 {
		return ComplianceFinding{
			Path:           target.Path,
			SpecRef:        spec.Record.Ref,
			Title:          spec.Record.Title,
			SectionHeading: section.Heading,
			Code:           "matching_terms",
			Message:        fmt.Sprintf("%s shares deterministic requirement terms with %s", target.Path, spec.Record.Ref),
			Traceability:   "explicit_applies_to",
			Observed:       strings.Join(shared, ", "),
		}, true
	}

	return ComplianceFinding{}, false
}

func complianceAppliesToSuggestion(path, specRef string) string {
	ref := primaryGovernedRefForPath(path)
	if specRef != "" {
		return fmt.Sprintf("If %s governs %s, add applies_to = [\"%s\"] to that accepted spec and rebuild the index.", specRef, path, ref)
	}
	return fmt.Sprintf("If an accepted spec governs %s, add applies_to = [\"%s\"] to that spec and rebuild the index.", path, ref)
}

func primaryGovernedRefForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml", ".json", ".toml", ".ini", ".cfg", ".conf":
		return "config://" + normalizeCompliancePath(path)
	default:
		return "code://" + normalizeCompliancePath(path)
	}
}

func buildComplianceUnspecifiedSummary(findings []ComplianceFinding) *ComplianceUnspecifiedSummary {
	if len(findings) == 0 {
		return nil
	}

	summary := &ComplianceUnspecifiedSummary{Total: len(findings)}
	for _, finding := range findings {
		if finding.Traceability == "explicit_applies_to" {
			summary.ExplicitButUnderexercised++
			continue
		}
		summary.MissingGovernanceEdge++
	}
	return summary
}

func buildComplianceRelations(result *ComplianceResult, explicitTargetRefs map[string]string) []ComplianceRelation {
	if result == nil || len(explicitTargetRefs) == 0 {
		return []ComplianceRelation{}
	}

	type candidate struct {
		relation ComplianceRelation
		rank     int
	}

	relations := make(map[string]candidate)
	add := func(findings []ComplianceFinding, state string, rank int) {
		for _, finding := range findings {
			if stringsTrimSpace(finding.SpecRef) == "" {
				continue
			}
			targetRef, ok := explicitTargetRefs[complianceRelationKey(finding.Path, finding.SpecRef)]
			if !ok {
				continue
			}
			relation := complianceRelationFromFinding(finding, targetRef, state)
			key := complianceRelationKey(finding.Path, finding.SpecRef)
			current, exists := relations[key]
			if !exists || rank > current.rank {
				relations[key] = candidate{relation: relation, rank: rank}
			}
		}
	}

	add(result.Unspecified, "unverifiable_in_scope", 1)
	add(result.Compliant, "verified", 2)
	add(result.Conflicts, "drifted", 3)

	if len(relations) == 0 {
		return []ComplianceRelation{}
	}

	items := make([]ComplianceRelation, 0, len(relations))
	for _, item := range relations {
		items = append(items, item.relation)
	}
	sort.Slice(items, func(i, j int) bool {
		switch {
		case items[i].Path != items[j].Path:
			return items[i].Path < items[j].Path
		case items[i].Endpoints[0].Ref != items[j].Endpoints[0].Ref:
			return items[i].Endpoints[0].Ref < items[j].Endpoints[0].Ref
		default:
			return items[i].ID < items[j].ID
		}
	})
	return items
}

func complianceRelationFromFinding(finding ComplianceFinding, targetRef, state string) ComplianceRelation {
	relationType, nodeKind := complianceRelationTypeAndNodeKind(targetRef)
	traceability := finding.Traceability
	if traceability == "" {
		traceability = "explicit_applies_to"
	}
	return ComplianceRelation{
		ID:         fmt.Sprintf("rel:%s:%s->%s", relationType, finding.SpecRef, targetRef),
		Type:       relationType,
		DeclaredBy: finding.SpecRef + "#applies_to",
		Verifier:   complianceRelationVerifier(finding),
		State:      state,
		Endpoints: []ComplianceRelationEndpoint{
			{NodeKind: "spec", Ref: finding.SpecRef},
			{NodeKind: nodeKind, Ref: targetRef},
		},
		Path:            finding.Path,
		SectionHeading:  finding.SectionHeading,
		Code:            finding.Code,
		Message:         finding.Message,
		Traceability:    traceability,
		LimitingFactor:  finding.LimitingFactor,
		Suggestion:      finding.Suggestion,
		Expected:        finding.Expected,
		Observed:        finding.Observed,
		Provenance:      finding.Provenance,
		Confidence:      finding.Confidence,
		Classification:  finding.Classification,
		RationaleText:   finding.RationaleText,
		RationaleKind:   finding.RationaleKind,
		RationaleSymbol: finding.RationaleSymbol,
	}
}

func complianceRelationTypeAndNodeKind(targetRef string) (string, string) {
	switch {
	case stringsHasPrefix(targetRef, "doc://"):
		return "doc_reflects_spec", "doc"
	case stringsHasPrefix(targetRef, "config://"):
		return "config_reflects_spec", "config"
	case stringsHasPrefix(targetRef, "code://"):
		return "code_reflects_spec", "code"
	default:
		return "artifact_reflects_spec", "artifact"
	}
}

func complianceRelationVerifier(finding ComplianceFinding) string {
	switch finding.Provenance {
	case ProvenanceModelAdjudication:
		return "semantic_adjudication"
	default:
		return "deterministic_traceability"
	}
}

func buildComplianceRelationSummary(relations []ComplianceRelation) ComplianceRelationSummary {
	summary := ComplianceRelationSummary{Total: len(relations)}
	for _, relation := range relations {
		switch relation.State {
		case "verified":
			summary.Verified++
		case "drifted":
			summary.Drifted++
		case "unverifiable_in_scope":
			summary.UnverifiableInScope++
		}
	}
	return summary
}

func buildComplianceDiscovery(findings []ComplianceFinding, explicitTargetRefs map[string]string) ComplianceDiscovery {
	items := make([]ComplianceDiscoveryItem, 0)
	seen := make(map[string]struct{})
	for _, finding := range findings {
		if stringsTrimSpace(finding.SpecRef) != "" {
			if _, ok := explicitTargetRefs[complianceRelationKey(finding.Path, finding.SpecRef)]; ok {
				continue
			}
		}
		if _, ok := seen[finding.Path]; ok {
			continue
		}
		seen[finding.Path] = struct{}{}
		items = append(items, ComplianceDiscoveryItem{
			Path:           finding.Path,
			SpecRef:        finding.SpecRef,
			Title:          finding.Title,
			Code:           finding.Code,
			Message:        finding.Message,
			Traceability:   finding.Traceability,
			LimitingFactor: finding.LimitingFactor,
			Suggestion:     finding.Suggestion,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		switch {
		case items[i].Path != items[j].Path:
			return items[i].Path < items[j].Path
		case items[i].SpecRef != items[j].SpecRef:
			return items[i].SpecRef < items[j].SpecRef
		default:
			return items[i].Code < items[j].Code
		}
	})
	return ComplianceDiscovery{FilesWithZeroRelations: items}
}

func buildComplianceTopSuggestions(result *ComplianceResult) []string {
	if result == nil {
		return nil
	}

	seen := make(map[string]struct{})
	suggestions := make([]string, 0, 3)
	appendSuggestions := func(findings []ComplianceFinding) {
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

func complianceRequestsPerMinuteConflict(statement, content string) (string, string, bool) {
	expected := complianceRequestsPerMinutePattern.FindString(statement)
	if expected == "" {
		return "", "", false
	}
	observed := complianceRequestsPerMinutePattern.FindString(content)
	if observed == "" || strings.EqualFold(expected, observed) {
		return "", "", false
	}
	return strings.ToLower(expected), strings.ToLower(observed), true
}

func complianceRequestsPerMinuteMatch(statement, content string) (string, string, bool) {
	expected := complianceRequestsPerMinutePattern.FindString(statement)
	if expected == "" {
		return "", "", false
	}
	observed := complianceRequestsPerMinutePattern.FindString(content)
	if observed == "" || !strings.EqualFold(expected, observed) {
		return "", "", false
	}
	expected = strings.ToLower(expected)
	return expected, expected, true
}

func compliancePhraseConflict(statement, content string) (string, string, bool) {
	statement = strings.ToLower(statement)
	content = strings.ToLower(content)
	for _, family := range complianceExclusiveFamilies {
		expectedMatches := family.Expected.matches(statement)
		observedMatches := family.Observed.matches(content)
		if !expectedMatches || !observedMatches {
			continue
		}
		if family.Expected.matches(content) {
			continue
		}
		return family.Expected.Canonical, family.Observed.Canonical, true
	}
	return "", "", false
}

func compliancePhraseMatch(statement, content string) (string, string, bool) {
	statement = strings.ToLower(statement)
	content = strings.ToLower(content)
	for _, family := range compliancePhraseFamilies {
		if !family.matches(statement) || !family.matches(content) {
			continue
		}
		return family.Canonical, family.Canonical, true
	}
	return "", "", false
}

func (f phraseFamily) matches(text string) bool {
	for _, variant := range f.Variants {
		if strings.Contains(text, variant) {
			return true
		}
	}
	return false
}

func complianceLexicalOverlap(statement, content string) ([]string, float64) {
	statementTokens := complianceContentTokens(statement)
	contentTokens := complianceContentTokens(content)
	if len(statementTokens) == 0 || len(contentTokens) == 0 {
		return nil, 0
	}

	contentSet := make(map[string]struct{}, len(contentTokens))
	for _, token := range contentTokens {
		contentSet[token] = struct{}{}
	}

	shared := make([]string, 0, len(statementTokens))
	seen := map[string]struct{}{}
	for _, token := range statementTokens {
		if _, ok := contentSet[token]; !ok {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		shared = append(shared, token)
	}
	sort.Strings(shared)
	return shared, float64(len(shared)) / float64(len(statementTokens))
}

func complianceContentTokens(text string) []string {
	raw := complianceTokenize(text)
	tokens := make([]string, 0, len(raw))
	for _, token := range raw {
		if len(token) < 3 {
			continue
		}
		if _, ok := complianceStopwords[token]; ok {
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens
}

func complianceTokenize(text string) []string {
	var builder strings.Builder
	builder.Grow(len(text))
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte(' ')
	}
	return strings.Fields(builder.String())
}

func addComplianceRelevantSpec(destination map[string]*complianceRelevantAccumulator, ref, path, basis string) {
	entry := destination[ref]
	if entry == nil {
		entry = &complianceRelevantAccumulator{
			paths: map[string]struct{}{},
			basis: map[string]struct{}{},
		}
		destination[ref] = entry
	}
	if path != "" {
		entry.paths[path] = struct{}{}
	}
	if basis != "" {
		entry.basis[basis] = struct{}{}
	}
}

func complianceRelevantSpecRefs(relevant map[string]*complianceRelevantAccumulator) []string {
	refs := make([]string, 0, len(relevant))
	for ref := range relevant {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

func buildComplianceRelevantSpecs(specs map[string]specDocument, relevant map[string]*complianceRelevantAccumulator) []ComplianceRelevantSpec {
	refs := complianceRelevantSpecRefs(relevant)
	result := make([]ComplianceRelevantSpec, 0, len(refs))
	for _, ref := range refs {
		spec, ok := specs[ref]
		if !ok {
			continue
		}
		entry := relevant[ref]
		item := ComplianceRelevantSpec{
			SpecRef: ref,
			Title:   spec.Record.Title,
		}
		for path := range entry.paths {
			item.Paths = append(item.Paths, path)
		}
		for basis := range entry.basis {
			item.Basis = append(item.Basis, basis)
		}
		sort.Strings(item.Paths)
		sort.Strings(item.Basis)
		result = append(result, item)
	}
	return result
}

// classifyComplianceConflicts annotates conflict findings with rationale from
// the ast_cache. Conflicts with associated rationale are classified as
// deliberate_deviation; those without are classified as unintentional_drift.
func classifyComplianceConflicts(ctx context.Context, cfg *config.Config, result *ComplianceResult) {
	if len(result.Conflicts) == 0 {
		return
	}

	// Collect unique paths from conflict findings.
	pathSet := make(map[string]bool)
	for _, f := range result.Conflicts {
		if f.Path != "" {
			pathSet[f.Path] = true
		}
	}
	paths := make([]string, 0, len(pathSet))
	for p := range pathSet {
		paths = append(paths, p)
	}

	// Query rationale from the index.
	rationaleEntries, err := index.QueryRationaleContext(ctx, cfg.Workspace.ResolvedIndexPath, paths)
	if err != nil || len(rationaleEntries) == 0 {
		// If rationale query fails or returns nothing, classify all as unintentional_drift.
		for i := range result.Conflicts {
			if result.Conflicts[i].Classification == "" {
				result.Conflicts[i].Classification = "unintentional_drift"
			}
		}
		return
	}

	// Build lookup from path to rationale entries.
	rationaleByPath := make(map[string]index.FileRationale)
	for _, fr := range rationaleEntries {
		rationaleByPath[fr.Path] = fr
	}

	for i := range result.Conflicts {
		if result.Conflicts[i].Classification != "" {
			continue
		}
		path := result.Conflicts[i].Path
		normalizedPath := strings.TrimPrefix(strings.TrimPrefix(path, "code://"), "config://")

		fr, ok := rationaleByPath[normalizedPath]
		if ok && len(fr.Rationale) > 0 {
			r := fr.Rationale[0]
			result.Conflicts[i].Classification = "deliberate_deviation"
			result.Conflicts[i].RationaleText = r.Text
			result.Conflicts[i].RationaleKind = string(r.Kind)
			result.Conflicts[i].RationaleSymbol = r.NearestSymbol
			result.Conflicts[i].Suggestion = fmt.Sprintf(
				"Documented rationale found (%s). Update the spec to reflect this decision, or update the code to match the spec.",
				r.Kind,
			)
		} else {
			result.Conflicts[i].Classification = "unintentional_drift"
		}
	}
}

func sortComplianceFindings(findings []ComplianceFinding) {
	sort.Slice(findings, func(i, j int) bool {
		switch {
		case findings[i].Path != findings[j].Path:
			return findings[i].Path < findings[j].Path
		case findings[i].SpecRef != findings[j].SpecRef:
			return findings[i].SpecRef < findings[j].SpecRef
		case findings[i].SectionHeading != findings[j].SectionHeading:
			return findings[i].SectionHeading < findings[j].SectionHeading
		default:
			return findings[i].Code < findings[j].Code
		}
	})
}
