//go:build precision_bench

package index

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
	stindex "github.com/dusk-network/stroma/v3/index"
)

const (
	armBDefaultTopRecords      = 5
	armBDefaultSectionBytes    = 1800
	armBDefaultAnswerTokens    = 320
	armBDefaultGradeTokens     = 220
	armBDefaultMinMidBodyCases = 20
)

type armBCase struct {
	ID                  string       `json:"id"`
	Query               string       `json:"query"`
	ReferenceAnswer     string       `json:"reference_answer"`
	RelevantDocRefs     []string     `json:"relevant_doc_refs"`
	RelevantSourceSpans []sourceSpan `json:"relevant_source_spans,omitempty"`
	Tags                []string     `json:"tags,omitempty"`
}

type armBReport struct {
	Label                string     `json:"label,omitempty"`
	GeneratedAt          string     `json:"generated_at"`
	ConfigPath           string     `json:"config_path"`
	CasesPath            string     `json:"cases_path"`
	CaseCount            int        `json:"case_count"`
	MidBodyCaseCount     int        `json:"mid_body_case_count"`
	CorpusDocCount       int        `json:"corpus_doc_count"`
	AnalysisModel        string     `json:"analysis_model"`
	TopRecords           int        `json:"top_records"`
	MaxSectionBytes      int        `json:"max_section_bytes"`
	ActionabilityCeiling string     `json:"actionability_ceiling"`
	Arms                 []armBArm  `json:"arms"`
	Delta                *armBDelta `json:"delta,omitempty"`
	SnapshotSizeBytes    int64      `json:"snapshot_size_bytes,omitempty"`
}

type armBArm struct {
	Name          string           `json:"name"`
	IncludeParent bool             `json:"include_parent"`
	Summary       armBArmSummary   `json:"summary"`
	Cases         []armBCaseResult `json:"cases"`
}

type armBArmSummary struct {
	MeanScore              float64        `json:"mean_score"`
	MedianScore            float64        `json:"median_score"`
	Distribution           map[string]int `json:"distribution"`
	MeanRetrievalLatencyMS float64        `json:"mean_retrieval_latency_ms"`
	P95RetrievalLatencyMS  float64        `json:"p95_retrieval_latency_ms"`
	StromaSearches         int            `json:"stroma_searches"`
	ContextExpansionCalls  int            `json:"context_expansion_calls"`
	ModelCalls             int            `json:"model_calls"`
	GeneratorModelCalls    int            `json:"generator_model_calls,omitempty"`
	GraderModelCalls       int            `json:"grader_model_calls,omitempty"`
	PromptTokens           int            `json:"prompt_tokens,omitempty"`
	CompletionTokens       int            `json:"completion_tokens,omitempty"`
	ContextBytes           int            `json:"context_bytes"`
	ContextTokenEstimate   int            `json:"context_token_estimate"`
	ErroredCases           int            `json:"errored_cases"`
	CostNote               string         `json:"cost_note"`
}

type armBCaseResult struct {
	ID                    string                      `json:"id"`
	Query                 string                      `json:"query"`
	Score                 int                         `json:"score"`
	GradeReason           string                      `json:"grade_reason,omitempty"`
	Answer                string                      `json:"answer,omitempty"`
	Error                 string                      `json:"error,omitempty"`
	RetrievalLatencyMS    float64                     `json:"retrieval_latency_ms"`
	GenerationLatencyMS   float64                     `json:"generation_latency_ms,omitempty"`
	GradeLatencyMS        float64                     `json:"grade_latency_ms,omitempty"`
	StromaSearches        int                         `json:"stroma_searches"`
	ContextExpansionCalls int                         `json:"context_expansion_calls"`
	ModelCalls            int                         `json:"model_calls"`
	GeneratorModelCalls   int                         `json:"generator_model_calls,omitempty"`
	GraderModelCalls      int                         `json:"grader_model_calls,omitempty"`
	PromptTokens          int                         `json:"prompt_tokens,omitempty"`
	CompletionTokens      int                         `json:"completion_tokens,omitempty"`
	ContextBytes          int                         `json:"context_bytes"`
	ContextTokenEstimate  int                         `json:"context_token_estimate"`
	Sections              []armBContextSectionSummary `json:"sections,omitempty"`
}

type armBContextSectionSummary struct {
	ChunkID   int64  `json:"chunk_id"`
	Ref       string `json:"ref"`
	Role      string `json:"role"`
	Heading   string `json:"heading"`
	Bytes     int    `json:"bytes"`
	Truncated bool   `json:"truncated,omitempty"`
}

type armBGrade struct {
	Score  int    `json:"score"`
	Reason string `json:"reason"`
}

type armBDelta struct {
	BaselineMeanScore         float64 `json:"baseline_mean_score"`
	ParentMeanScore           float64 `json:"parent_mean_score"`
	MeanScoreDelta            float64 `json:"mean_score_delta"`
	BaselineMeanContextTokens int     `json:"baseline_mean_context_token_estimate"`
	ParentMeanContextTokens   int     `json:"parent_mean_context_token_estimate"`
}

// TestRetrievalArmBBench is the #361 opt-in Arm B harness. It runs only when
// PITUITARY_ARMB_CONFIG and PITUITARY_ARMB_CASES are set because it needs a
// live embedder plus an OpenAI-compatible analysis runtime for answer
// generation and grading.
func TestRetrievalArmBBench(t *testing.T) {
	configPath := strings.TrimSpace(os.Getenv("PITUITARY_ARMB_CONFIG"))
	casesPath := strings.TrimSpace(os.Getenv("PITUITARY_ARMB_CASES"))
	if configPath == "" || casesPath == "" {
		t.Skip("set PITUITARY_ARMB_CONFIG and PITUITARY_ARMB_CASES to run")
	}

	report, err := runArmBBenchmark(context.Background(), configPath, casesPath, strings.TrimSpace(os.Getenv("PITUITARY_ARMB_LABEL")))
	if err != nil {
		t.Fatalf("run Arm B benchmark: %v", err)
	}
	if os.Getenv("PITUITARY_ARMB_STRICT") == "1" {
		if err := validateArmBReportNoErrors(report); err != nil {
			t.Fatalf("Arm B strict validation: %v", err)
		}
	}
	if reportPath := strings.TrimSpace(os.Getenv("PITUITARY_ARMB_REPORT")); reportPath != "" {
		if err := writeArmBReport(reportPath, report); err != nil {
			t.Fatalf("write Arm B report: %v", err)
		}
		t.Logf("wrote Arm B report: %s", reportPath)
	}
}

func runArmBBenchmark(ctx context.Context, configPath, casesPath, label string) (*armBReport, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	cases, err := loadArmBCases(casesPath)
	if err != nil {
		return nil, err
	}
	if err := validateArmBCaseFloor(cases); err != nil {
		return nil, err
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("load sources: %w", err)
	}
	if os.Getenv("PITUITARY_ARMB_SKIP_REBUILD") != "1" {
		if _, err := RebuildContext(ctx, cfg, records); err != nil {
			return nil, fmt.Errorf("rebuild: %w", err)
		}
	}

	db, err := OpenReadOnlyContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return nil, fmt.Errorf("open index db: %w", err)
	}
	defer db.Close()
	snapshot, err := OpenStromaSnapshotContext(ctx, db, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return nil, fmt.Errorf("open stroma snapshot: %w", err)
	}
	defer snapshot.Close()

	embedder, err := newEmbedder(cfg.Runtime.Embedder)
	if err != nil {
		return nil, fmt.Errorf("build embedder: %w", err)
	}
	chatClient, err := newArmBChatClient(cfg.Runtime.Analysis)
	if err != nil {
		return nil, err
	}

	report := &armBReport{
		Label:                label,
		GeneratedAt:          time.Now().UTC().Format(time.RFC3339),
		ConfigPath:           configPath,
		CasesPath:            casesPath,
		CaseCount:            len(cases),
		MidBodyCaseCount:     countArmBMidBodyCases(cases),
		CorpusDocCount:       len(records.Docs),
		AnalysisModel:        strings.TrimSpace(cfg.Runtime.Analysis.Model),
		TopRecords:           armBDefaultTopRecords,
		MaxSectionBytes:      armBDefaultSectionBytes,
		ActionabilityCeiling: "review-spec --outline-context: production-positive only when quality improves without default credentials and p95 retrieval latency stays within an interactive budget",
	}
	report.Arms = []armBArm{
		runArmBArm(ctx, cfg, snapshot, embedder, chatClient, cases, false),
		runArmBArm(ctx, cfg, snapshot, embedder, chatClient, cases, true),
	}
	report.Delta = buildArmBDelta(report.Arms)
	return report, nil
}

func loadArmBCases(path string) ([]armBCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []armBCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, fmt.Errorf("decode Arm B cases: %w", err)
	}
	for i := range cases {
		cases[i].ID = strings.TrimSpace(cases[i].ID)
		cases[i].Query = strings.TrimSpace(cases[i].Query)
		cases[i].ReferenceAnswer = strings.TrimSpace(cases[i].ReferenceAnswer)
		switch {
		case cases[i].ID == "":
			return nil, fmt.Errorf("case %d: id is required", i)
		case cases[i].Query == "":
			return nil, fmt.Errorf("case %s: query is required", cases[i].ID)
		case cases[i].ReferenceAnswer == "":
			return nil, fmt.Errorf("case %s: reference_answer is required", cases[i].ID)
		case len(cases[i].RelevantDocRefs) == 0:
			return nil, fmt.Errorf("case %s: relevant_doc_refs is required", cases[i].ID)
		case len(cases[i].RelevantSourceSpans) == 0:
			return nil, fmt.Errorf("case %s: relevant_source_spans is required", cases[i].ID)
		}
	}
	return cases, nil
}

func validateArmBCaseFloor(cases []armBCase) error {
	if countArmBMidBodyCases(cases) < armBDefaultMinMidBodyCases {
		return fmt.Errorf("Arm B requires at least %d mid_body cases, got %d", armBDefaultMinMidBodyCases, countArmBMidBodyCases(cases))
	}
	return nil
}

func runArmBArm(ctx context.Context, cfg *config.Config, snapshot *stindex.Snapshot, embedder Embedder, chatClient *armBChatClient, cases []armBCase, includeParent bool) armBArm {
	arm := armBArm{
		Name:          "leaf_only",
		IncludeParent: includeParent,
		Cases:         make([]armBCaseResult, 0, len(cases)),
	}
	if includeParent {
		arm.Name = "leaf_plus_parent"
	}
	for _, benchCase := range cases {
		arm.Cases = append(arm.Cases, runArmBCase(ctx, cfg, snapshot, embedder, chatClient, benchCase, includeParent))
	}
	arm.Summary = summarizeArmBCases(arm.Cases)
	return arm
}

func runArmBCase(ctx context.Context, cfg *config.Config, snapshot *stindex.Snapshot, embedder Embedder, benchClient *armBChatClient, benchCase armBCase, includeParent bool) armBCaseResult {
	result := armBCaseResult{
		ID:             benchCase.ID,
		Query:          benchCase.Query,
		StromaSearches: 1,
	}

	retrievalStart := time.Now()
	outline, err := RetrieveOutlineContextWithSnapshotContext(ctx, cfg, snapshot, embedder, OutlineContextQuery{
		Query:           benchCase.Query,
		Kinds:           []string{model.ArtifactKindDoc},
		Limit:           armBDefaultTopRecords,
		IncludeParent:   includeParent,
		NeighborWindow:  0,
		MaxSectionBytes: armBDefaultSectionBytes,
	})
	result.RetrievalLatencyMS = elapsedMS(retrievalStart)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	contextText, sections := armBPromptContext(outline)
	result.Sections = sections
	result.ContextBytes = len(contextText)
	result.ContextTokenEstimate = estimateArmBTokens(contextText)
	result.ContextExpansionCalls = countArmBSelections(outline)

	answerStart := time.Now()
	answer, usage, err := benchClient.complete(ctx, armBAnswerMessages(benchCase.Query, contextText), armBDefaultAnswerTokens)
	result.GenerationLatencyMS = elapsedMS(answerStart)
	result.ModelCalls++
	result.GeneratorModelCalls++
	result.PromptTokens += usage.PromptTokens
	result.CompletionTokens += usage.CompletionTokens
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Answer = answer

	gradeStart := time.Now()
	grade, gradeUsage, err := gradeArmBAnswer(ctx, benchClient, benchCase, answer)
	result.GradeLatencyMS = elapsedMS(gradeStart)
	result.ModelCalls++
	result.GraderModelCalls++
	result.PromptTokens += gradeUsage.PromptTokens
	result.CompletionTokens += gradeUsage.CompletionTokens
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Score = grade.Score
	result.GradeReason = grade.Reason
	return result
}

func armBPromptContext(outline *OutlineContextResult) (string, []armBContextSectionSummary) {
	if outline == nil {
		return "", nil
	}
	var builder strings.Builder
	summaries := make([]armBContextSectionSummary, 0)
	seen := make(map[int64]struct{})
	for _, record := range outline.Records {
		for _, selection := range record.Selections {
			for _, section := range selection.Expanded {
				if _, ok := seen[section.ChunkID]; ok {
					continue
				}
				seen[section.ChunkID] = struct{}{}
				fmt.Fprintf(
					&builder, "[chunk:%d ref:%s role:%s heading:%s]\n%s\n\n",
					section.ChunkID,
					section.Ref,
					section.Role,
					section.Heading,
					strings.TrimSpace(section.Content),
				)
				summaries = append(summaries, armBContextSectionSummary{
					ChunkID:   section.ChunkID,
					Ref:       section.Ref,
					Role:      section.Role,
					Heading:   section.Heading,
					Bytes:     len(section.Content),
					Truncated: section.ContentTruncated,
				})
			}
		}
	}
	return strings.TrimSpace(builder.String()), summaries
}

func armBAnswerMessages(query, contextText string) []armBChatMessage {
	return []armBChatMessage{
		{
			Role:    "system",
			Content: "Answer the user's question using only the provided context. Be concise. If the context is insufficient, say what is missing instead of inventing details.",
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("Question:\n%s\n\nContext:\n%s", query, contextText),
		},
	}
}

func gradeArmBAnswer(ctx context.Context, client *armBChatClient, benchCase armBCase, answer string) (armBGrade, armBChatUsage, error) {
	messages := []armBChatMessage{
		{
			Role:    "system",
			Content: "Grade the candidate answer against the reference answer for correctness and completeness. Return only JSON: {\"score\":0-5,\"reason\":\"short reason\"}. Score 5 means fully correct and complete; 0 means wrong or unsupported.",
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("Question:\n%s\n\nReference answer:\n%s\n\nCandidate answer:\n%s", benchCase.Query, benchCase.ReferenceAnswer, answer),
		},
	}
	text, usage, err := client.complete(ctx, messages, armBDefaultGradeTokens)
	if err != nil {
		return armBGrade{}, usage, err
	}
	var grade armBGrade
	if err := json.Unmarshal(extractArmBJSONObject(text), &grade); err != nil {
		return armBGrade{}, usage, fmt.Errorf("decode grader JSON %q: %w", text, err)
	}
	if grade.Score < 0 {
		grade.Score = 0
	}
	if grade.Score > 5 {
		grade.Score = 5
	}
	grade.Reason = strings.TrimSpace(grade.Reason)
	return grade, usage, nil
}

func extractArmBJSONObject(text string) []byte {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return []byte(text)
	}
	return []byte(text[start : end+1])
}

func countArmBSelections(outline *OutlineContextResult) int {
	if outline == nil {
		return 0
	}
	total := 0
	for _, record := range outline.Records {
		total += len(record.Selections)
	}
	return total
}

func summarizeArmBCases(results []armBCaseResult) armBArmSummary {
	summary := armBArmSummary{
		Distribution: map[string]int{"0": 0, "1": 0, "2": 0, "3": 0, "4": 0, "5": 0},
		CostNote:     "model pricing is not configured in the harness; use prompt/completion token counts as the neutral cost envelope",
	}
	if len(results) == 0 {
		return summary
	}
	scores := make([]int, 0, len(results))
	latencies := make([]float64, 0, len(results))
	for _, result := range results {
		score := result.Score
		if result.Error != "" {
			summary.ErroredCases++
			score = 0
		}
		summary.MeanScore += float64(score)
		summary.Distribution[strconv.Itoa(score)]++
		scores = append(scores, score)
		latencies = append(latencies, result.RetrievalLatencyMS)
		summary.MeanRetrievalLatencyMS += result.RetrievalLatencyMS
		summary.StromaSearches += result.StromaSearches
		summary.ContextExpansionCalls += result.ContextExpansionCalls
		summary.ModelCalls += result.ModelCalls
		summary.GeneratorModelCalls += result.GeneratorModelCalls
		summary.GraderModelCalls += result.GraderModelCalls
		summary.PromptTokens += result.PromptTokens
		summary.CompletionTokens += result.CompletionTokens
		summary.ContextBytes += result.ContextBytes
		summary.ContextTokenEstimate += result.ContextTokenEstimate
	}
	denominator := float64(len(results))
	summary.MeanScore /= denominator
	summary.MedianScore = medianArmBScore(scores)
	summary.MeanRetrievalLatencyMS /= denominator
	summary.P95RetrievalLatencyMS = percentileArmBFloat(latencies, 0.95)
	return summary
}

func buildArmBDelta(arms []armBArm) *armBDelta {
	if len(arms) < 2 {
		return nil
	}
	baseline := arms[0].Summary
	parent := arms[1].Summary
	return &armBDelta{
		BaselineMeanScore:         baseline.MeanScore,
		ParentMeanScore:           parent.MeanScore,
		MeanScoreDelta:            parent.MeanScore - baseline.MeanScore,
		BaselineMeanContextTokens: meanArmBInt(baseline.ContextTokenEstimate, len(arms[0].Cases)),
		ParentMeanContextTokens:   meanArmBInt(parent.ContextTokenEstimate, len(arms[1].Cases)),
	}
}

func validateArmBReportNoErrors(report *armBReport) error {
	if report == nil {
		return nil
	}
	var bad []string
	for _, arm := range report.Arms {
		for _, result := range arm.Cases {
			if result.Error != "" {
				bad = append(bad, fmt.Sprintf("%s/%s: %s", arm.Name, result.ID, result.Error))
			}
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("%d errored case(s): %s", len(bad), strings.Join(bad, "; "))
	}
	return nil
}

func countArmBMidBodyCases(cases []armBCase) int {
	count := 0
	for _, benchCase := range cases {
		for _, tag := range benchCase.Tags {
			if tag == "mid_body" {
				count++
				break
			}
		}
	}
	return count
}

func medianArmBScore(scores []int) float64 {
	if len(scores) == 0 {
		return 0
	}
	sort.Ints(scores)
	mid := len(scores) / 2
	if len(scores)%2 == 1 {
		return float64(scores[mid])
	}
	return float64(scores[mid-1]+scores[mid]) / 2
}

func percentileArmBFloat(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	index := int(float64(len(values)-1) * percentile)
	return values[index]
}

func estimateArmBTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	estimate := len(text) / 4
	if estimate == 0 {
		return 1
	}
	return estimate
}

func meanArmBInt(total, count int) int {
	if count <= 0 {
		return 0
	}
	return total / count
}

func elapsedMS(start time.Time) float64 {
	return float64(time.Since(start).Microseconds()) / 1000.0
}

func writeArmBReport(path string, report *armBReport) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
