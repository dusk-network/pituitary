package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/openaicompat"
)

type qualitativeAnalyzer interface {
	Probe(ctx context.Context) error
	Compare(ctx context.Context, orderedRefs []string, specs map[string]specDocument, base Comparison) (Comparison, error)
	RefineDocDrift(ctx context.Context, doc docDocument, specs map[string]specDocument, item DriftItem, remediation *DocRemediationItem) (*DriftItem, *DocRemediationItem, error)
}

type openAICompatibleAnalysisProvider struct {
	model      string
	endpoint   string
	token      string
	maxRetries int
	client     *http.Client
}

type openAICompatibleChatRequest struct {
	Model       string                        `json:"model"`
	Messages    []openAICompatibleChatMessage `json:"messages"`
	Temperature float64                       `json:"temperature"`
}

type openAICompatibleChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAICompatibleChatResponse struct {
	Choices []openAICompatibleChoice `json:"choices"`
	Err     json.RawMessage          `json:"error,omitempty"`
}

type openAICompatibleChoice struct {
	Message openAICompatibleChoiceMessage `json:"message"`
}

type openAICompatibleChoiceMessage struct {
	Content json.RawMessage `json:"content"`
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
	analysisPromptSectionLimit           = 6
	analysisPromptSectionContentLimit    = 700
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

func newOpenAICompatibleAnalysisProvider(provider config.RuntimeProvider) (qualitativeAnalyzer, error) {
	token := ""
	if envVar := strings.TrimSpace(provider.APIKeyEnv); envVar != "" {
		token = strings.TrimSpace(os.Getenv(envVar))
		if token == "" {
			return nil, analysisDependencyUnavailable("missing API key for %s", openAICompatibleAnalysisRuntime)
		}
	}

	client := &http.Client{}
	if provider.TimeoutMS > 0 {
		client.Timeout = time.Duration(provider.TimeoutMS) * time.Millisecond
	}

	return &openAICompatibleAnalysisProvider{
		model:      strings.TrimSpace(provider.Model),
		endpoint:   strings.TrimRight(strings.TrimSpace(provider.Endpoint), "/"),
		token:      token,
		maxRetries: provider.MaxRetries,
		client:     client,
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
	if err := p.completeJSON(ctx, openAICompatibleProbeSystemPrompt, map[string]any{
		"command": "runtime-probe",
	}, &response); err != nil {
		return err
	}
	if !response.OK {
		return analysisDependencyUnavailable("%s probe returned ok=false", openAICompatibleAnalysisRuntime)
	}
	return nil
}

func (p *openAICompatibleAnalysisProvider) Compare(ctx context.Context, orderedRefs []string, specs map[string]specDocument, base Comparison) (Comparison, error) {
	payload := compareAnalysisPrompt{
		Command:     "compare-specs",
		OrderedRefs: append([]string(nil), orderedRefs...),
		Specs:       analysisSpecsFromMap(specs, orderedRefs),
		Baseline:    base,
	}

	var provided Comparison
	if err := p.completeJSON(ctx, openAICompatibleCompareSystemPrompt, payload, &provided); err != nil {
		return Comparison{}, err
	}
	return normalizeProvidedComparison(base, provided, orderedRefs, specs), nil
}

func (p *openAICompatibleAnalysisProvider) RefineDocDrift(ctx context.Context, doc docDocument, specs map[string]specDocument, item DriftItem, remediation *DocRemediationItem) (*DriftItem, *DocRemediationItem, error) {
	prompt := docDriftAnalysisPrompt{
		Command:               "check-doc-drift",
		Doc:                   analysisDocFromDocument(doc),
		RelevantSpecs:         analysisSpecsFromSlice(specs),
		DeterministicFindings: append([]DriftFinding(nil), item.Findings...),
	}
	if remediation != nil {
		prompt.DeterministicSuggestions = append([]DocRemediationSuggestion(nil), remediation.Suggestions...)
	}

	var provided docDriftAnalysisResponse
	if err := p.completeJSON(ctx, openAICompatibleDocDriftSystemPrompt, prompt, &provided); err != nil {
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

func (p *openAICompatibleAnalysisProvider) completeJSON(ctx context.Context, systemPrompt string, input any, target any) error {
	if ctx == nil {
		ctx = context.Background()
	}

	body, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode runtime.analysis prompt: %w", err)
	}

	requestBody, err := json.Marshal(openAICompatibleChatRequest{
		Model: p.model,
		Messages: []openAICompatibleChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: string(body)},
		},
		Temperature: 0,
	})
	if err != nil {
		return fmt.Errorf("encode runtime.analysis request: %w", err)
	}

	responseBody, err := p.requestChatCompletion(ctx, requestBody)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(responseBody), target); err != nil {
		return &index.DependencyUnavailableError{
			Runtime: openAICompatibleAnalysisRuntime,
			Message: fmt.Sprintf("decode %s response as JSON object: %v", openAICompatibleAnalysisRuntime, err),
		}
	}
	return nil
}

func (p *openAICompatibleAnalysisProvider) requestChatCompletion(ctx context.Context, body []byte) (string, error) {
	var lastErr error

	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("build runtime.analysis request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if p.token != "" {
			req.Header.Set("Authorization", "Bearer "+p.token)
		}

		resp, err := p.client.Do(req)
		if err != nil {
			if errors.Is(err, context.Canceled) || (errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil) {
				return "", err
			}
			lastErr = &index.DependencyUnavailableError{
				Runtime: openAICompatibleAnalysisRuntime,
				Message: fmt.Sprintf("call %s endpoint %s: %v", openAICompatibleAnalysisRuntime, p.endpoint, err),
			}
			if shouldRetryOpenAICompatibleAnalysisRequest(err, 0) && attempt < p.maxRetries {
				if waitErr := waitBeforeAnalysisRetry(ctx, attempt, 0); waitErr != nil {
					return "", waitErr
				}
				continue
			}
			return "", lastErr
		}

		retryAfter := retryAfterDuration(resp.Header.Get("Retry-After"))
		payload, err := readOpenAICompatibleChatResponse(resp)
		resp.Body.Close()
		if err == nil {
			return payload, nil
		}
		lastErr = err
		if shouldRetryOpenAICompatibleAnalysisRequest(err, resp.StatusCode) && attempt < p.maxRetries {
			if waitErr := waitBeforeAnalysisRetry(ctx, attempt, retryAfter); waitErr != nil {
				return "", waitErr
			}
			continue
		}
		return "", err
	}

	if lastErr == nil {
		lastErr = analysisDependencyUnavailable("%s request failed", openAICompatibleAnalysisRuntime)
	}
	return "", lastErr
}

func readOpenAICompatibleChatResponse(resp *http.Response) (string, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", analysisDependencyUnavailable("read %s response: %v", openAICompatibleAnalysisRuntime, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := openaicompat.ExtractErrorMessage(body)
		if message == "" {
			message = strings.TrimSpace(string(body))
		}
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return "", &index.DependencyUnavailableError{
			Runtime: openAICompatibleAnalysisRuntime,
			Message: fmt.Sprintf("%s endpoint %s returned %s: %s", openAICompatibleAnalysisRuntime, resp.Request.URL, resp.Status, message),
		}
	}

	var payload openAICompatibleChatResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", analysisDependencyUnavailable("decode %s response: %v", openAICompatibleAnalysisRuntime, err)
	}
	if message := openaicompat.ExtractErrorValue(payload.Err); message != "" {
		return "", &index.DependencyUnavailableError{
			Runtime: openAICompatibleAnalysisRuntime,
			Message: fmt.Sprintf("%s endpoint %s returned an error: %s", openAICompatibleAnalysisRuntime, resp.Request.URL, message),
		}
	}
	if len(payload.Choices) == 0 {
		return "", analysisDependencyUnavailable("%s returned no choices", openAICompatibleAnalysisRuntime)
	}

	text := extractOpenAICompatibleMessageText(payload.Choices[0].Message.Content)
	if text == "" {
		return "", analysisDependencyUnavailable("%s returned an empty message", openAICompatibleAnalysisRuntime)
	}
	text = normalizeJSONResponseText(text)
	if text == "" {
		return "", analysisDependencyUnavailable("%s returned no JSON object", openAICompatibleAnalysisRuntime)
	}
	return text, nil
}

func extractOpenAICompatibleMessageText(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var builder strings.Builder
		for _, part := range parts {
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(strings.TrimSpace(part.Text))
		}
		return strings.TrimSpace(builder.String())
	}

	return strings.TrimSpace(string(raw))
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

func analysisDependencyUnavailable(format string, args ...any) *index.DependencyUnavailableError {
	return &index.DependencyUnavailableError{
		Runtime: openAICompatibleAnalysisRuntime,
		Message: fmt.Sprintf(format, args...),
	}
}

func shouldRetryOpenAICompatibleAnalysisRequest(err error, statusCode int) bool {
	if statusCode == http.StatusTooManyRequests || statusCode >= 500 {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "broken pipe")
}

func analysisSpecsFromMap(specs map[string]specDocument, orderedRefs []string) []analysisSpecPrompt {
	result := make([]analysisSpecPrompt, 0, len(orderedRefs))
	for _, ref := range orderedRefs {
		spec, ok := specs[ref]
		if !ok {
			continue
		}
		result = append(result, analysisSpecFromDocument(spec))
	}
	return result
}

func analysisSpecsFromSlice(specs map[string]specDocument) []analysisSpecPrompt {
	refs := make([]string, 0, len(specs))
	for ref := range specs {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return analysisSpecsFromMap(specs, refs)
}

func analysisSpecFromDocument(spec specDocument) analysisSpecPrompt {
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
		Sections:  analysisSectionsFromEmbedded(spec.Sections),
	}
}

func analysisDocFromDocument(doc docDocument) analysisDocPrompt {
	return analysisDocPrompt{
		Ref:       doc.Record.Ref,
		Title:     doc.Record.Title,
		SourceRef: doc.Record.SourceRef,
		Sections:  analysisSectionsFromEmbedded(doc.Sections),
	}
}

func analysisSectionsFromEmbedded(sections []embeddedSection) []analysisSectionPrompt {
	result := make([]analysisSectionPrompt, 0, minInt(len(sections), analysisPromptSectionLimit))
	for _, section := range sections {
		if len(result) == analysisPromptSectionLimit {
			break
		}
		content := strings.TrimSpace(section.Content)
		if content == "" {
			continue
		}
		result = append(result, analysisSectionPrompt{
			Heading: strings.TrimSpace(section.Heading),
			Content: truncateForAnalysisPrompt(content, analysisPromptSectionContentLimit),
		})
	}
	return result
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

func retryAfterDuration(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
		return 0
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return 0
}

func waitBeforeAnalysisRetry(ctx context.Context, attempt int, retryAfter time.Duration) error {
	delay := retryAfter
	if delay <= 0 {
		delay = 200 * time.Millisecond
		for i := 0; i < attempt; i++ {
			delay *= 2
			if delay >= 2*time.Second {
				delay = 2 * time.Second
				break
			}
		}
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
		SourceRef:   base.SourceRef,
		Suggestions: result,
	}
}

func normalizeProvidedEvidence(base, provided DocRemediationEvidence) DocRemediationEvidence {
	result := base
	if value := strings.TrimSpace(provided.SpecSection); value != "" {
		result.SpecSection = value
	}
	if value := strings.TrimSpace(provided.SpecExcerpt); value != "" {
		result.SpecExcerpt = value
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
