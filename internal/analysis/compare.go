package analysis

import (
	"context"
	"fmt"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
)

// CompareRequest is the normalized comparison input.
type CompareRequest struct {
	SpecRefs   []string          `json:"spec_refs,omitempty"`
	SpecRecord *model.SpecRecord `json:"spec_record,omitempty"`
}

// ComparisonDifference captures one spec's distinct choices.
type ComparisonDifference struct {
	SpecRef string   `json:"spec_ref"`
	Title   string   `json:"title"`
	Items   []string `json:"items"`
}

// ComparisonTradeoff captures one tradeoff between specs.
type ComparisonTradeoff struct {
	Topic   string `json:"topic"`
	Summary string `json:"summary"`
}

// ComparisonCompatibility summarizes how the compared specs fit together.
type ComparisonCompatibility struct {
	Level   string `json:"level"`
	Summary string `json:"summary"`
}

// Comparison holds the structured spec comparison payload.
type Comparison struct {
	SharedScope    []string                `json:"shared_scope"`
	Differences    []ComparisonDifference  `json:"differences"`
	Tradeoffs      []ComparisonTradeoff    `json:"tradeoffs"`
	Compatibility  ComparisonCompatibility `json:"compatibility"`
	Recommendation string                  `json:"recommendation"`
}

// CompareResult is the structured compare-specs response.
type CompareResult struct {
	SpecRefs       []string        `json:"spec_refs"`
	SpecInferences []SpecInference `json:"spec_inferences,omitempty"`
	Comparison     Comparison      `json:"comparison"`
}

// CompareSpecs compares exactly two indexed specs or one draft spec against one indexed spec.
func CompareSpecs(cfg *config.Config, request CompareRequest) (*CompareResult, error) {
	return CompareSpecsContext(context.Background(), cfg, request)
}

// CompareSpecsContext compares exactly two indexed specs or one draft spec against one indexed spec.
func CompareSpecsContext(ctx context.Context, cfg *config.Config, request CompareRequest) (*CompareResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	refs, err := normalizeCompareRefs(request.SpecRefs)
	if err != nil {
		return nil, err
	}
	switch {
	case request.SpecRecord == nil && len(refs) != 2:
		return nil, fmt.Errorf("exactly two spec_refs are required")
	case request.SpecRecord != nil && len(refs) != 1:
		return nil, fmt.Errorf("exactly one indexed spec_ref is required when spec_record is provided")
	}
	if request.SpecRecord == nil && refs[0] == refs[1] {
		return nil, fmt.Errorf("spec_refs must refer to two distinct specs")
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

	var candidate *specDocument
	orderedRefs := append([]string{}, refs...)
	if request.SpecRecord != nil {
		candidate, err = loadCandidate(repo, OverlapRequest{SpecRecord: request.SpecRecord}, nil)
		if err != nil {
			return nil, err
		}
		if refs[0] == candidate.Record.Ref {
			return nil, fmt.Errorf("indexed spec_ref must be different from the draft spec_record")
		}
		orderedRefs = []string{candidate.Record.Ref, refs[0]}
	}

	specs, err := repo.loadSpecs(refs)
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		if _, ok := specs[ref]; !ok {
			return nil, newSpecRefNotFoundError(ref)
		}
	}
	return buildCompareResult(ctx, analyzer, candidate, orderedRefs, specs)
}

func buildCompareResult(ctx context.Context, analyzer qualitativeAnalyzer, candidate *specDocument, orderedRefs []string, specs map[string]specDocument) (*CompareResult, error) {
	comparisonSpecs := copySpecDocuments(specs)
	if candidate != nil {
		comparisonSpecs[candidate.Record.Ref] = *candidate
	}

	differences := make([]ComparisonDifference, 0, len(orderedRefs))
	for _, ref := range orderedRefs {
		spec := comparisonSpecs[ref]
		differences = append(differences, ComparisonDifference{
			SpecRef: ref,
			Title:   spec.Record.Title,
			Items:   summarizeSpecChoices(spec),
		})
	}

	result := &CompareResult{
		SpecRefs:       orderedRefs,
		SpecInferences: buildSpecInferences(comparisonSpecs, orderedRefs),
		Comparison: Comparison{
			SharedScope:    sharedScope(comparisonSpecs, orderedRefs),
			Differences:    differences,
			Tradeoffs:      buildTradeoffs(comparisonSpecs, orderedRefs),
			Compatibility:  compareCompatibility(comparisonSpecs, orderedRefs),
			Recommendation: compareRecommendation(comparisonSpecs, orderedRefs),
		},
	}
	if analyzer != nil {
		refined, err := analyzer.Compare(ctx, orderedRefs, comparisonSpecs, result.Comparison)
		if err != nil {
			return nil, err
		}
		result.Comparison = refined
	}
	return result, nil
}

func normalizeCompareRefs(values []string) ([]string, error) {
	refs := make([]string, 0, len(values))
	for _, value := range values {
		ref := strings.TrimSpace(value)
		if ref == "" {
			return nil, fmt.Errorf("spec_refs must not be empty")
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func sharedScope(specs map[string]specDocument, refs []string) []string {
	if len(refs) == 0 {
		return nil
	}

	shared := append([]string{}, specs[refs[0]].Record.AppliesTo...)
	for _, ref := range refs[1:] {
		shared = sharedStrings(shared, specs[ref].Record.AppliesTo)
	}

	if len(refs) > 0 {
		domain := specs[refs[0]].Record.Domain
		if domain != "" {
			allSameDomain := true
			for _, ref := range refs[1:] {
				if specs[ref].Record.Domain != domain {
					allSameDomain = false
					break
				}
			}
			if allSameDomain {
				shared = append([]string{"domain:" + domain}, shared...)
			}
		}
	}

	return shared
}

func summarizeSpecChoices(spec specDocument) []string {
	var items []string
	for _, section := range spec.Sections {
		for _, line := range summarizeSectionLines(section.Content) {
			items = append(items, line)
			if len(items) == 4 {
				return items
			}
		}
	}
	if len(items) == 0 {
		items = append(items, spec.Record.Title)
	}
	return items
}

func summarizeSectionLines(content string) []string {
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		result = append(result, line)
	}
	return result
}

func buildTradeoffs(specs map[string]specDocument, refs []string) []ComparisonTradeoff {
	if len(refs) < 2 {
		return nil
	}

	left := specs[refs[0]]
	right := specs[refs[1]]

	return []ComparisonTradeoff{
		{
			Topic: "scope",
			Summary: fmt.Sprintf("%s focuses on %s, while %s focuses on %s.",
				left.Record.Ref,
				firstChoice(summarizeSpecChoices(left)),
				right.Record.Ref,
				firstChoice(summarizeSpecChoices(right))),
		},
		{
			Topic: "design",
			Summary: fmt.Sprintf("%s and %s keep the same governed refs but make different implementation choices, so teams need to decide whether they want backward-compatible simplicity or the newer behavior encoded in the accepted supersession path.",
				left.Record.Ref,
				right.Record.Ref),
		},
	}
}

func compareCompatibility(specs map[string]specDocument, refs []string) ComparisonCompatibility {
	if len(refs) < 2 {
		return ComparisonCompatibility{}
	}

	left := specs[refs[0]].Record
	right := specs[refs[1]].Record
	switch {
	case relationExists(left.Relations, model.RelationSupersedes, right.Ref):
		return ComparisonCompatibility{
			Level:   "superseding",
			Summary: fmt.Sprintf("%s intentionally replaces %s while preserving the same governed scope.", left.Ref, right.Ref),
		}
	case relationExists(right.Relations, model.RelationSupersedes, left.Ref):
		return ComparisonCompatibility{
			Level:   "superseding",
			Summary: fmt.Sprintf("%s intentionally replaces %s while preserving the same governed scope.", right.Ref, left.Ref),
		}
	case len(sharedStrings(left.AppliesTo, right.AppliesTo)) > 0:
		return ComparisonCompatibility{
			Level:   "partial",
			Summary: fmt.Sprintf("%s and %s govern overlapping refs, so they cannot both be treated as independent designs.", left.Ref, right.Ref),
		}
	default:
		return ComparisonCompatibility{
			Level:   "independent",
			Summary: fmt.Sprintf("%s and %s do not share direct governed scope.", left.Ref, right.Ref),
		}
	}
}

func compareRecommendation(specs map[string]specDocument, refs []string) string {
	if len(refs) < 2 {
		return ""
	}
	best := specs[refs[0]].Record
	for _, ref := range refs[1:] {
		candidate := specs[ref].Record
		if relationExists(candidate.Relations, model.RelationSupersedes, best.Ref) {
			best = candidate
			continue
		}
		if best.Status != model.StatusAccepted && candidate.Status == model.StatusAccepted {
			best = candidate
		}
	}
	return fmt.Sprintf("prefer %s as the primary reference because it is the strongest accepted successor across the compared set", best.Ref)
}

func firstChoice(items []string) string {
	if len(items) == 0 {
		return "its documented scope"
	}
	return items[0]
}
