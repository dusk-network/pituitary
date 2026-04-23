package analysis

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/ranking"
	"github.com/dusk-network/pituitary/internal/runtimeerr"
	stchat "github.com/dusk-network/stroma/v2/chat"
)

type qualitativeAnalyzer interface {
	Probe(ctx context.Context) error
	Compare(ctx context.Context, orderedRefs []string, specs map[string]specDocument, base Comparison) (Comparison, error)
	RefineDocDrift(ctx context.Context, doc docDocument, specs map[string]specDocument, item DriftItem, remediation *DocRemediationItem) (*DriftItem, *DocRemediationItem, error)
}

type openAICompatibleAnalysisProvider struct {
	runtime           string
	provider          string
	model             string
	endpoint          string
	timeoutMS         int
	maxRetries        int
	maxResponseTokens int
	client            *stchat.OpenAI
}

// openAICompatibleChatRequest mirrors the OpenAI-compatible chat request
// wire shape so tests can decode captured request bodies without depending
// on stroma internals.
type openAICompatibleChatRequest struct {
	Model       string                        `json:"model"`
	Messages    []openAICompatibleChatMessage `json:"messages"`
	Temperature float64                       `json:"temperature"`
	MaxTokens   int                           `json:"max_tokens,omitempty"`
}

type openAICompatibleChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type compareAnalysisPrompt struct {
	Command     string               `json:"command"`
	OrderedRefs []string             `json:"ordered_refs"`
	Specs       []analysisSpecPrompt `json:"specs"`
	Baseline    Comparison           `json:"baseline"`
}

type docDriftAnalysisPrompt struct {
	Command                  string                     `json:"command"`
	Doc                      analysisDocPrompt          `json:"doc"`
	RelevantSpecs            []analysisSpecPrompt       `json:"relevant_specs"`
	DeterministicFindings    []DriftFinding             `json:"deterministic_findings"`
	DeterministicSuggestions []DocRemediationSuggestion `json:"deterministic_suggestions,omitempty"`
}

type docDriftAnalysisResponse struct {
	Findings    []DriftFinding             `json:"findings"`
	Suggestions []DocRemediationSuggestion `json:"suggestions"`
}

type analysisSpecPrompt struct {
	Ref       string                   `json:"ref"`
	Title     string                   `json:"title"`
	Status    string                   `json:"status,omitempty"`
	Domain    string                   `json:"domain,omitempty"`
	AppliesTo []string                 `json:"applies_to,omitempty"`
	Relations []analysisRelationPrompt `json:"relations,omitempty"`
	Sections  []analysisSectionPrompt  `json:"sections,omitempty"`
}

type analysisDocPrompt struct {
	Ref       string                  `json:"ref"`
	Title     string                  `json:"title"`
	SourceRef string                  `json:"source_ref"`
	Sections  []analysisSectionPrompt `json:"sections,omitempty"`
}

type analysisRelationPrompt struct {
	Type string `json:"type"`
	Ref  string `json:"ref"`
}

type analysisSectionPrompt struct {
	Heading string `json:"heading,omitempty"`
	Content string `json:"content"`
}

const (
	openAICompatibleProbeSystemPrompt    = "You are Pituitary's runtime probe. Return only one JSON object with key ok set to true."
	openAICompatibleCompareSystemPrompt  = "You are Pituitary's compare-specs adjudicator. Use only the provided spec evidence. Return only one JSON object with keys shared_scope, differences, tradeoffs, compatibility, and recommendation. Preserve spec refs exactly. Keep every difference items array concise and limited to concrete design choices from the provided specs."
	openAICompatibleDocDriftSystemPrompt = "You are Pituitary's doc-drift adjudicator. Use only the provided deterministic findings and cited spec/doc evidence. Return only one JSON object with keys findings and suggestions. Do not invent new finding codes or spec refs. Findings must correspond to the provided deterministic findings, and suggestions must stay actionable and bounded to the same contradictions."
	analysisPromptSectionLimit           = 4
	analysisPromptSectionContentLimit    = 500
	openAICompatibleAnalysisRuntime      = "runtime.analysis"
)

func newQualitativeAnalyzer(provider config.RuntimeProvider) (qualitativeAnalyzer, error) {
	switch strings.TrimSpace(provider.Provider) {
	case "", config.RuntimeProviderDisabled:
		return nil, nil
	case config.RuntimeProviderOpenAI:
		return newOpenAICompatibleAnalysisProvider(provider)
	default:
		return nil, fmt.Errorf(
			"runtime.analysis.provider %q is not supported; supported providers are %q and %q",
			provider.Provider,
			config.RuntimeProviderDisabled,
			config.RuntimeProviderOpenAI,
		)
	}
}

func newOpenAICompatibleAnalysisProvider(cfg config.RuntimeProvider) (qualitativeAnalyzer, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	token := ""
	if envVar := strings.TrimSpace(cfg.APIKeyEnv); envVar != "" {
		token = strings.TrimSpace(os.Getenv(envVar))
		if token == "" {
			return nil, runtimeerr.NewDependencyUnavailableWithDetails(runtimeerr.FailureDetails{
				Runtime:      openAICompatibleAnalysisRuntime,
				Provider:     config.RuntimeProviderOpenAI,
				Model:        strings.TrimSpace(cfg.Model),
				Endpoint:     endpoint,
				FailureClass: runtimeerr.FailureClassAuth,
				TimeoutMS:    cfg.TimeoutMS,
				MaxRetries:   cfg.MaxRetries,
			}, "missing API key for %s", openAICompatibleAnalysisRuntime)
		}
	}

	var timeout time.Duration
	if cfg.TimeoutMS > 0 {
		timeout = time.Duration(cfg.TimeoutMS) * time.Millisecond
	}

	client := stchat.NewOpenAI(stchat.OpenAIConfig{
		BaseURL:    endpoint,
		Model:      strings.TrimSpace(cfg.Model),
		APIToken:   token,
		Timeout:    timeout,
		MaxRetries: cfg.MaxRetries,
	})

	return &openAICompatibleAnalysisProvider{
		runtime:           openAICompatibleAnalysisRuntime,
		provider:          config.RuntimeProviderOpenAI,
		model:             strings.TrimSpace(cfg.Model),
		endpoint:          endpoint,
		timeoutMS:         cfg.TimeoutMS,
		maxRetries:        cfg.MaxRetries,
		maxResponseTokens: cfg.MaxResponseTokens,
		client:            client,
	}, nil
}

func ProbeProviderContext(ctx context.Context, provider config.RuntimeProvider) error {
	analyzer, err := newQualitativeAnalyzer(provider)
	if err != nil {
		return err
	}
	if analyzer == nil {
		return nil
	}
	return analyzer.Probe(ctx)
}

func (p *openAICompatibleAnalysisProvider) Probe(ctx context.Context) error {
	var response struct {
		OK bool `json:"ok"`
	}
	if err := p.completeJSON(ctx, "runtime-probe", openAICompatibleProbeSystemPrompt, map[string]any{
		"command": "runtime-probe",
	}, &response); err != nil {
		return err
	}
	if !response.OK {
		return p.dependencyError(runtimeerr.FailureClassDependency, "%s probe returned ok=false", openAICompatibleAnalysisRuntime)
	}
	return nil
}

func (p *openAICompatibleAnalysisProvider) Compare(ctx context.Context, orderedRefs []string, specs map[string]specDocument, base Comparison) (Comparison, error) {
	payload := compareAnalysisPrompt{
		Command:     "compare-specs",
		OrderedRefs: append([]string(nil), orderedRefs...),
		Specs:       analysisSpecsForComparison(specs, orderedRefs),
		Baseline:    base,
	}

	var provided Comparison
	if err := p.completeJSON(ctx, payload.Command, openAICompatibleCompareSystemPrompt, payload, &provided); err != nil {
		return Comparison{}, err
	}
	return normalizeProvidedComparison(base, provided, orderedRefs, specs), nil
}

func (p *openAICompatibleAnalysisProvider) RefineDocDrift(ctx context.Context, doc docDocument, specs map[string]specDocument, item DriftItem, remediation *DocRemediationItem) (*DriftItem, *DocRemediationItem, error) {
	prompt := docDriftAnalysisPrompt{
		Command:               "check-doc-drift",
		Doc:                   analysisDocFromDocument(doc, flattenedSectionsFromSpecs(specs)),
		RelevantSpecs:         analysisSpecsForDocDrift(specs, doc),
		DeterministicFindings: append([]DriftFinding(nil), item.Findings...),
	}
	if remediation != nil {
		prompt.DeterministicSuggestions = append([]DocRemediationSuggestion(nil), remediation.Suggestions...)
	}

	var provided docDriftAnalysisResponse
	if err := p.completeJSON(ctx, prompt.Command, openAICompatibleDocDriftSystemPrompt, prompt, &provided); err != nil {
		return nil, nil, err
	}

	refinedItem := item
	refinedItem.Findings = normalizeProvidedDriftFindings(item.Findings, provided.Findings)

	refinedRemediation := remediation
	if refined := normalizeProvidedRemediation(remediation, provided.Suggestions); refined != nil {
		refinedRemediation = refined
	}

	return &refinedItem, refinedRemediation, nil
}

func (p *openAICompatibleAnalysisProvider) completeJSON(ctx context.Context, command string, systemPrompt string, input any, target any) error {
	if ctx == nil {
		ctx = context.Background()
	}

	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode runtime.analysis prompt: %w", err)
	}

	responseBody, err := p.requestChatCompletion(ctx, command, []stchat.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: string(body)},
	})
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(responseBody), target); err != nil {
		return p.dependencyError(runtimeerr.FailureClassSchemaMismatch, "decode %s response as JSON object: %v", openAICompatibleAnalysisRuntime, err)
	}
	return nil
}

func (p *openAICompatibleAnalysisProvider) requestChatCompletion(ctx context.Context, command string, messages []stchat.Message) (string, error) {
	text, err := p.client.ChatCompletionText(ctx, messages, 0, analysisResponseTokenLimit(command, p.maxResponseTokens))
	if err != nil {
		return "", runtimeerr.FromProviderError(err, p.analysisFailureLabels())
	}
	// stroma.chat already rejects empty or unrecognised-shape responses as
	// schema_mismatch, so a non-nil err with empty text is not reachable.
	// The remaining Pituitary concern is whether the returned text is a
	// JSON object matching the prompt contract.
	text = normalizeJSONResponseText(text)
	if text == "" {
		return "", p.dependencyError(runtimeerr.FailureClassSchemaMismatch, "%s returned no JSON object", openAICompatibleAnalysisRuntime)
	}
	return text, nil
}

func normalizeJSONResponseText(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSuffix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
	}
	if looksLikeJSONObject(trimmed) && json.Valid([]byte(trimmed)) {
		return trimmed
	}

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		candidate := strings.TrimSpace(trimmed[start : end+1])
		if looksLikeJSONObject(candidate) && json.Valid([]byte(candidate)) {
			return candidate
		}
	}
	return ""
}

func (p *openAICompatibleAnalysisProvider) analysisFailureLabels() runtimeerr.FailureDetails {
	return runtimeerr.FailureDetails{
		Runtime:     p.runtime,
		Provider:    p.provider,
		Model:       p.model,
		Endpoint:    p.endpoint,
		RequestType: "analysis",
		TimeoutMS:   p.timeoutMS,
		MaxRetries:  p.maxRetries,
	}
}

func (p *openAICompatibleAnalysisProvider) dependencyError(failureClass, format string, args ...any) *runtimeerr.DependencyUnavailableError {
	details := p.analysisFailureLabels()
	details.FailureClass = strings.TrimSpace(failureClass)
	return runtimeerr.NewDependencyUnavailableWithDetails(details, format, args...)
}

func analysisSpecsFromMap(specs map[string]specDocument, orderedRefs []string, sectionSelector func(specDocument) []analysisSectionPrompt) []analysisSpecPrompt {
	result := make([]analysisSpecPrompt, 0, len(orderedRefs))
	for _, ref := range orderedRefs {
		spec, ok := specs[ref]
		if !ok {
			continue
		}
		result = append(result, analysisSpecFromDocument(spec, sectionSelector(spec)))
	}
	return result
}

func analysisSpecsFromSlice(specs map[string]specDocument, sectionSelector func(specDocument) []analysisSectionPrompt) []analysisSpecPrompt {
	refs := make([]string, 0, len(specs))
	for ref := range specs {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return analysisSpecsFromMap(specs, refs, sectionSelector)
}

func analysisSpecsForComparison(specs map[string]specDocument, orderedRefs []string) []analysisSpecPrompt {
	counterparts := make(map[string][]embeddedSection, len(orderedRefs))
	for _, ref := range orderedRefs {
		for _, otherRef := range orderedRefs {
			if otherRef == ref {
				continue
			}
			counterparts[ref] = append(counterparts[ref], specs[otherRef].Sections...)
		}
	}
	return analysisSpecsFromMap(specs, orderedRefs, func(spec specDocument) []analysisSectionPrompt {
		return analysisSectionsFromEmbedded(spec.Sections, counterparts[spec.Record.Ref])
	})
}

func analysisSpecsForDocDrift(specs map[string]specDocument, doc docDocument) []analysisSpecPrompt {
	return analysisSpecsFromSlice(specs, func(spec specDocument) []analysisSectionPrompt {
		return analysisSectionsFromEmbedded(spec.Sections, doc.Sections)
	})
}

func analysisSpecFromDocument(spec specDocument, sections ...[]analysisSectionPrompt) analysisSpecPrompt {
	selectedSections := analysisSectionsFromEmbedded(spec.Sections)
	if len(sections) > 0 {
		selectedSections = sections[0]
	}

	relations := make([]analysisRelationPrompt, 0, len(spec.Record.Relations))
	for _, relation := range spec.Record.Relations {
		relations = append(relations, analysisRelationPrompt{
			Type: string(relation.Type),
			Ref:  relation.Ref,
		})
	}
	return analysisSpecPrompt{
		Ref:       spec.Record.Ref,
		Title:     spec.Record.Title,
		Status:    spec.Record.Status,
		Domain:    spec.Record.Domain,
		AppliesTo: append([]string(nil), spec.Record.AppliesTo...),
		Relations: relations,
		Sections:  selectedSections,
	}
}

func analysisDocFromDocument(doc docDocument, counterpartSections []embeddedSection) analysisDocPrompt {
	return analysisDocPrompt{
		Ref:       doc.Record.Ref,
		Title:     doc.Record.Title,
		SourceRef: doc.Record.SourceRef,
		Sections:  analysisSectionsFromEmbedded(doc.Sections, counterpartSections),
	}
}

func flattenedSectionsFromSpecs(specs map[string]specDocument) []embeddedSection {
	refs := make([]string, 0, len(specs))
	for ref := range specs {
		refs = append(refs, ref)
	}
	sort.Strings(refs)

	sections := make([]embeddedSection, 0)
	for _, ref := range refs {
		sections = append(sections, specs[ref].Sections...)
	}
	return sections
}

type scoredAnalysisSection struct {
	section    embeddedSection
	score      float64
	index      int
	comparable bool
}

func analysisSectionsFromEmbedded(sections []embeddedSection, counterpartSections ...[]embeddedSection) []analysisSectionPrompt {
	var counterparts []embeddedSection
	if len(counterpartSections) > 0 {
		counterparts = counterpartSections[0]
	}

	scored := make([]scoredAnalysisSection, 0, len(sections))
	anyComparable := false
	for index, section := range sections {
		content := strings.TrimSpace(section.Content)
		if content == "" {
			continue
		}
		score, comparable := analysisSectionRelevance(section, counterparts)
		anyComparable = anyComparable || comparable
		scored = append(scored, scoredAnalysisSection{
			section:    section,
			score:      score,
			index:      index,
			comparable: comparable,
		})
	}
	if anyComparable {
		sort.SliceStable(scored, func(i, j int) bool {
			switch {
			case scored[i].score != scored[j].score:
				return scored[i].score > scored[j].score
			case scored[i].comparable != scored[j].comparable:
				return scored[i].comparable
			default:
				return scored[i].index < scored[j].index
			}
		})
	}
	if len(scored) > analysisPromptSectionLimit {
		scored = scored[:analysisPromptSectionLimit]
	}

	result := make([]analysisSectionPrompt, 0, len(scored))
	for _, candidate := range scored {
		result = append(result, analysisSectionPrompt{
			Heading: strings.TrimSpace(candidate.section.Heading),
			Content: truncateForAnalysisPrompt(strings.TrimSpace(candidate.section.Content), analysisPromptSectionContentLimit),
		})
	}
	return result
}

func analysisSectionRelevance(section embeddedSection, counterpartSections []embeddedSection) (float64, bool) {
	if len(section.Embedding) == 0 {
		return 0, false
	}

	var (
		bestScore float64
		found     bool
	)
	for _, counterpart := range counterpartSections {
		if len(counterpart.Embedding) == 0 {
			continue
		}
		score := cosineSimilarity(section.Embedding, counterpart.Embedding)
		score = ranking.AdjustHistoricalSectionScore(score, section.Heading, false)
		if !found || score > bestScore {
			bestScore = score
			found = true
		}
	}
	return bestScore, found
}

func analysisResponseTokenLimit(command string, configured int) int {
	if configured > 0 {
		return configured
	}
	switch strings.TrimSpace(command) {
	case "check-doc-drift", "check-compliance-adjudicate":
		return 2048
	case "compare-specs", "analyze-impact-severity":
		return 1024
	case "runtime-probe":
		return 64
	default:
		return 1024
	}
}

func truncateForAnalysisPrompt(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	if limit <= 3 {
		return safePromptPrefix(text, limit)
	}
	prefix := safePromptPrefix(text, limit-3)
	if prefix == "" {
		return ""
	}
	return prefix + "..."
}

func safePromptPrefix(text string, byteLimit int) string {
	if byteLimit <= 0 {
		return ""
	}
	if len(text) <= byteLimit {
		return text
	}

	lastSafe := 0
	for index := range text {
		if index > byteLimit {
			break
		}
		lastSafe = index
	}
	if lastSafe == 0 && byteLimit > 0 {
		return text[:0]
	}
	return text[:lastSafe]
}

func looksLikeJSONObject(text string) bool {
	text = strings.TrimSpace(text)
	return strings.HasPrefix(text, "{") && strings.HasSuffix(text, "}")
}

func normalizeProvidedComparison(base, provided Comparison, orderedRefs []string, specs map[string]specDocument) Comparison {
	result := base

	if shared := normalizeStringList(provided.SharedScope, 8); len(shared) > 0 {
		result.SharedScope = shared
	}
	if differences := normalizeProvidedDifferences(base.Differences, provided.Differences, orderedRefs, specs); len(differences) > 0 {
		result.Differences = differences
	}
	if tradeoffs := normalizeProvidedTradeoffs(provided.Tradeoffs); len(tradeoffs) > 0 {
		result.Tradeoffs = tradeoffs
	}
	if compatibility := normalizeProvidedCompatibility(provided.Compatibility); compatibility.Level != "" || compatibility.Summary != "" {
		result.Compatibility = compatibility
	}
	if recommendation := strings.TrimSpace(provided.Recommendation); recommendation != "" {
		result.Recommendation = recommendation
	}
	return result
}

func normalizeProvidedDifferences(base, provided []ComparisonDifference, orderedRefs []string, specs map[string]specDocument) []ComparisonDifference {
	providedByRef := make(map[string]ComparisonDifference, len(provided))
	for index, diff := range provided {
		ref := strings.TrimSpace(diff.SpecRef)
		if ref == "" && index < len(orderedRefs) {
			ref = orderedRefs[index]
		}
		if ref == "" {
			continue
		}
		diff.SpecRef = ref
		providedByRef[ref] = diff
	}

	baseByRef := make(map[string]ComparisonDifference, len(base))
	for _, diff := range base {
		baseByRef[diff.SpecRef] = diff
	}

	result := make([]ComparisonDifference, 0, len(orderedRefs))
	for _, ref := range orderedRefs {
		diff := baseByRef[ref]
		if provided, ok := providedByRef[ref]; ok {
			if title := strings.TrimSpace(provided.Title); title != "" {
				diff.Title = title
			} else if spec, ok := specs[ref]; ok {
				diff.Title = spec.Record.Title
			}
			if items := normalizeStringList(provided.Items, 4); len(items) > 0 {
				diff.Items = items
			}
		}
		if diff.Title == "" {
			if spec, ok := specs[ref]; ok {
				diff.Title = spec.Record.Title
			}
		}
		diff.SpecRef = ref
		result = append(result, diff)
	}
	return result
}

func normalizeProvidedTradeoffs(provided []ComparisonTradeoff) []ComparisonTradeoff {
	result := make([]ComparisonTradeoff, 0, len(provided))
	for _, tradeoff := range provided {
		topic := strings.TrimSpace(tradeoff.Topic)
		summary := strings.TrimSpace(tradeoff.Summary)
		if topic == "" || summary == "" {
			continue
		}
		result = append(result, ComparisonTradeoff{Topic: topic, Summary: summary})
		if len(result) == 4 {
			break
		}
	}
	return result
}

func normalizeProvidedCompatibility(provided ComparisonCompatibility) ComparisonCompatibility {
	level := strings.TrimSpace(provided.Level)
	summary := strings.TrimSpace(provided.Summary)
	switch level {
	case "superseding", "partial", "independent":
	default:
		level = ""
	}
	if level == "" && summary == "" {
		return ComparisonCompatibility{}
	}
	return ComparisonCompatibility{Level: level, Summary: summary}
}

func normalizeProvidedDriftFindings(base, provided []DriftFinding) []DriftFinding {
	providedByKey := make(map[string]DriftFinding, len(provided))
	for _, finding := range provided {
		key := driftFindingKey(finding)
		if key == "" {
			continue
		}
		providedByKey[key] = finding
	}

	result := make([]DriftFinding, 0, len(base))
	for _, finding := range base {
		refined := finding
		if provided, ok := providedByKey[driftFindingKey(finding)]; ok {
			if message := strings.TrimSpace(provided.Message); message != "" {
				refined.Message = message
			}
			if expected := strings.TrimSpace(provided.Expected); expected != "" {
				refined.Expected = expected
			}
			if observed := strings.TrimSpace(provided.Observed); observed != "" {
				refined.Observed = observed
			}
		}
		result = append(result, refined)
	}
	return result
}

func normalizeProvidedRemediation(base *DocRemediationItem, provided []DocRemediationSuggestion) *DocRemediationItem {
	if base == nil {
		return nil
	}
	if len(provided) == 0 {
		return base
	}

	baseByKey := make(map[string]DocRemediationSuggestion, len(base.Suggestions))
	for _, suggestion := range base.Suggestions {
		baseByKey[docSuggestionKey(suggestion)] = suggestion
	}

	result := make([]DocRemediationSuggestion, 0, len(provided))
	for _, suggestion := range provided {
		key := docSuggestionKey(suggestion)
		baseSuggestion, ok := baseByKey[key]
		if !ok {
			continue
		}
		normalized := baseSuggestion
		if summary := strings.TrimSpace(suggestion.Summary); summary != "" {
			normalized.Summary = summary
		}
		if linkReason := strings.TrimSpace(suggestion.LinkReason); linkReason != "" {
			normalized.LinkReason = linkReason
		}
		if bullets := normalizeStringList(suggestion.SuggestedBullets, 4); len(bullets) > 0 {
			normalized.SuggestedBullets = bullets
		}
		if action := strings.TrimSpace(suggestion.SuggestedEdit.Action); action != "" {
			normalized.SuggestedEdit.Action = action
		}
		if replace := strings.TrimSpace(suggestion.SuggestedEdit.Replace); replace != "" {
			normalized.SuggestedEdit.Replace = replace
		}
		if with := strings.TrimSpace(suggestion.SuggestedEdit.With); with != "" {
			normalized.SuggestedEdit.With = with
		}
		if note := strings.TrimSpace(suggestion.SuggestedEdit.Note); note != "" {
			normalized.SuggestedEdit.Note = note
		}
		if evidence := normalizeProvidedEvidence(baseSuggestion.Evidence, suggestion.Evidence); evidence != (DocRemediationEvidence{}) {
			normalized.Evidence = evidence
		}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return base
	}

	return &DocRemediationItem{
		DocRef:      base.DocRef,
		Title:       base.Title,
		Repo:        base.Repo,
		SourceRef:   base.SourceRef,
		Suggestions: result,
	}
}

func normalizeProvidedEvidence(base, provided DocRemediationEvidence) DocRemediationEvidence {
	result := base
	if value := strings.TrimSpace(provided.SpecSourceRef); value != "" {
		result.SpecSourceRef = value
	}
	if value := strings.TrimSpace(provided.SpecSection); value != "" {
		result.SpecSection = value
	}
	if value := strings.TrimSpace(provided.SpecExcerpt); value != "" {
		result.SpecExcerpt = value
	}
	if value := strings.TrimSpace(provided.DocSourceRef); value != "" {
		result.DocSourceRef = value
	}
	if value := strings.TrimSpace(provided.DocSection); value != "" {
		result.DocSection = value
	}
	if value := strings.TrimSpace(provided.DocExcerpt); value != "" {
		result.DocExcerpt = value
	}
	if value := strings.TrimSpace(provided.Expected); value != "" {
		result.Expected = value
	}
	if value := strings.TrimSpace(provided.Observed); value != "" {
		result.Observed = value
	}
	if value := strings.TrimSpace(provided.LinkReason); value != "" {
		result.LinkReason = value
	}
	return result
}

func normalizeStringList(values []string, limit int) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if limit > 0 && len(result) == limit {
			break
		}
	}
	return result
}

func driftFindingKey(finding DriftFinding) string {
	specRef := strings.TrimSpace(finding.SpecRef)
	code := strings.TrimSpace(finding.Code)
	if specRef == "" || code == "" {
		return ""
	}
	return specRef + "\x00" + strings.TrimSpace(finding.Artifact) + "\x00" + code
}

func docSuggestionKey(suggestion DocRemediationSuggestion) string {
	specRef := strings.TrimSpace(suggestion.SpecRef)
	code := strings.TrimSpace(suggestion.Code)
	if specRef == "" || code == "" {
		return ""
	}
	return specRef + "\x00" + code
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
