package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/dusk-network/pituitary/extensions/astinfer"
	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

type benchmarkCase struct {
	ID           string                `json:"id"`
	Operation    string                `json:"operation"`
	Description  string                `json:"description,omitempty"`
	Request      json.RawMessage       `json:"request"`
	Expectations benchmarkExpectations `json:"expectations"`
}

type benchmarkExpectations struct {
	MustIncludePaths          []string          `json:"must_include_paths,omitempty"`
	MustIncludeSpecRefs       []string          `json:"must_include_spec_refs,omitempty"`
	MustIncludeDocRefs        []string          `json:"must_include_doc_refs,omitempty"`
	MustExcludeDocRefs        []string          `json:"must_exclude_doc_refs,omitempty"`
	MustIncludeAffectedRefs   []string          `json:"must_include_affected_refs,omitempty"`
	MustIncludeFindingCodes   []string          `json:"must_include_finding_codes,omitempty"`
	AssessmentStatuses        map[string]string `json:"assessment_statuses,omitempty"`
	CompatibilityLevel        string            `json:"compatibility_level,omitempty"`
	RecommendationContains    []string          `json:"recommendation_contains,omitempty"`
	MinimumCompliantFindings  int               `json:"minimum_compliant_findings,omitempty"`
	MaximumConflictFindings   *int              `json:"maximum_conflict_findings,omitempty"`
	RequireStructuredEvidence bool              `json:"require_structured_evidence,omitempty"`
}

type benchmarkCheck struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Details string `json:"details,omitempty"`
}

type benchmarkCaseResult struct {
	ID                  string           `json:"id"`
	Operation           string           `json:"operation"`
	ExecutedOperation   string           `json:"executed_operation"`
	OperationNote       string           `json:"operation_note,omitempty"`
	Description         string           `json:"description,omitempty"`
	LatencyMS           float64          `json:"latency_ms"`
	JSONValid           bool             `json:"json_valid"`
	OutputBytes         int              `json:"output_bytes"`
	AnalysisRuntimeUsed bool             `json:"analysis_runtime_used"`
	ChatCompletionCalls int              `json:"chat_completion_calls"`
	PromptSizeBytes     *int             `json:"prompt_size_bytes"`
	ResponseSizeBytes   *int             `json:"response_size_bytes"`
	Passed              bool             `json:"passed"`
	Error               string           `json:"error,omitempty"`
	Checks              []benchmarkCheck `json:"checks,omitempty"`
}

type benchmarkSummary struct {
	TotalCases                   int     `json:"total_cases"`
	PassedCases                  int     `json:"passed_cases"`
	FailedCases                  int     `json:"failed_cases"`
	MeanLatencyMS                float64 `json:"mean_latency_ms"`
	CasesUsingAnalysisRuntime    int     `json:"cases_using_analysis_runtime"`
	CasesWithPromptMeasurement   int     `json:"cases_with_prompt_measurement"`
	CasesWithResponseMeasurement int     `json:"cases_with_response_measurement"`
}

type benchmarkReport struct {
	GeneratedAt      string                `json:"generated_at"`
	ConfigPath       string                `json:"config_path"`
	CasesDir         string                `json:"cases_dir"`
	AnalysisProvider string                `json:"analysis_provider"`
	Summary          benchmarkSummary      `json:"summary"`
	Cases            []benchmarkCaseResult `json:"cases"`
}

type analysisCapture struct {
	Calls         int
	PromptBytes   int
	ResponseBytes int
}

type analysisProxy struct {
	client  *http.Client
	server  *httptest.Server
	target  string
	mu      sync.Mutex
	capture analysisCapture
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		configPath string
		casesDir   string
		format     string
	)
	fs.StringVar(&configPath, "config", "pituitary.toml", "path to workspace config")
	fs.StringVar(&casesDir, "cases-dir", "testdata/bench", "path to benchmark cases")
	fs.StringVar(&format, "format", "text", "output format: text or json")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "bench: unexpected positional arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}
	if format != "text" && format != "json" {
		fmt.Fprintf(stderr, "bench: unsupported format %q\n", format)
		return 2
	}

	report, err := runBenchmarks(ctx, configPath, casesDir)
	if err != nil {
		fmt.Fprintf(stderr, "bench: %v\n", err)
		return 1
	}

	if format == "json" {
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "bench: encode json report: %v\n", err)
			return 1
		}
		_, _ = stdout.Write(encoded)
		_, _ = stdout.Write([]byte("\n"))
		return 0
	}

	renderTextReport(stdout, report)
	return 0
}

func runBenchmarks(ctx context.Context, configPath, casesDir string) (*benchmarkReport, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	cases, err := loadBenchmarkCases(casesDir)
	if err != nil {
		return nil, err
	}

	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("load sources: %w", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		return nil, fmt.Errorf("rebuild index: %w", err)
	}

	cfg = cloneConfig(cfg)
	var proxy *analysisProxy
	if cfg.Runtime.Analysis.Provider == config.RuntimeProviderOpenAI && strings.TrimSpace(cfg.Runtime.Analysis.Endpoint) != "" {
		proxy, err = newAnalysisProxy(cfg.Runtime.Analysis.Endpoint)
		if err != nil {
			return nil, err
		}
		defer proxy.Close()
		cfg.Runtime.Analysis.Endpoint = proxy.server.URL
	}

	report := &benchmarkReport{
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		ConfigPath:       cfg.ConfigPath,
		CasesDir:         filepath.Clean(casesDir),
		AnalysisProvider: cfg.Runtime.Analysis.Provider,
		Cases:            make([]benchmarkCaseResult, 0, len(cases)),
	}

	var totalLatency float64
	for _, benchCase := range cases {
		result := runBenchmarkCase(ctx, cfg, proxy, benchCase)
		totalLatency += result.LatencyMS
		report.Cases = append(report.Cases, result)
	}

	report.Summary = summarizeBenchmarkResults(report.Cases, totalLatency)
	return report, nil
}

func loadBenchmarkCases(casesDir string) ([]benchmarkCase, error) {
	entries, err := os.ReadDir(casesDir)
	if err != nil {
		return nil, fmt.Errorf("read cases dir %s: %w", casesDir, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("no benchmark case files found in %s", casesDir)
	}

	cases := make([]benchmarkCase, 0, len(names))
	for _, name := range names {
		path := filepath.Join(casesDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read benchmark case %s: %w", path, err)
		}
		var benchCase benchmarkCase
		if err := json.Unmarshal(data, &benchCase); err != nil {
			return nil, fmt.Errorf("decode benchmark case %s: %w", path, err)
		}
		if strings.TrimSpace(benchCase.ID) == "" {
			return nil, fmt.Errorf("benchmark case %s: id is required", path)
		}
		if len(benchCase.Request) == 0 {
			return nil, fmt.Errorf("benchmark case %s: request is required", path)
		}
		cases = append(cases, benchCase)
	}

	return cases, nil
}

func runBenchmarkCase(ctx context.Context, cfg *config.Config, proxy *analysisProxy, benchCase benchmarkCase) benchmarkCaseResult {
	executedOperation, note, err := resolveOperation(benchCase.Operation)
	result := benchmarkCaseResult{
		ID:                benchCase.ID,
		Operation:         benchCase.Operation,
		ExecutedOperation: executedOperation,
		OperationNote:     note,
		Description:       benchCase.Description,
	}
	if err != nil {
		result.Error = err.Error()
		return result
	}

	if proxy != nil {
		proxy.Reset()
	}

	start := time.Now()
	output, runErr := executeBenchmarkCase(ctx, cfg, benchCase.Operation, executedOperation, benchCase.Request)
	result.LatencyMS = float64(time.Since(start).Microseconds()) / 1000.0
	if runErr != nil {
		result.Error = runErr.Error()
		result.Passed = false
		if proxy != nil {
			applyCapture(&result, proxy.Snapshot())
		}
		return result
	}

	encoded, marshalErr := json.Marshal(output)
	result.JSONValid = marshalErr == nil
	if marshalErr == nil {
		result.OutputBytes = len(encoded)
	} else {
		result.Error = fmt.Sprintf("encode result JSON: %v", marshalErr)
	}

	if proxy != nil {
		applyCapture(&result, proxy.Snapshot())
	}

	result.Checks = evaluateBenchmarkCase(benchCase, output)
	result.Passed = benchmarkChecksPassed(result.Checks) && result.Error == ""
	return result
}

func resolveOperation(operation string) (string, string, error) {
	switch strings.TrimSpace(operation) {
	case "compare-specs":
		return "compare-specs", "", nil
	case "check-doc-drift":
		return "check-doc-drift", "", nil
	case "check-compliance":
		return "check-compliance", "", nil
	case "analyze-impact":
		return "analyze-impact", "", nil
	case "analyze-impact-severity":
		return "analyze-impact", "Current ship does not split analyze-impact severity into a separate runtime call, so this case exercises the shipped analyze-impact baseline.", nil
	default:
		return "", "", fmt.Errorf("unsupported operation %q", operation)
	}
}

func executeBenchmarkCase(ctx context.Context, cfg *config.Config, declaredOperation, executedOperation string, rawRequest json.RawMessage) (any, error) {
	switch executedOperation {
	case "compare-specs":
		var request analysis.CompareRequest
		if err := json.Unmarshal(rawRequest, &request); err != nil {
			return nil, fmt.Errorf("decode %s request: %w", declaredOperation, err)
		}
		return analysis.CompareSpecsContext(ctx, cfg, request)
	case "check-doc-drift":
		var request analysis.DocDriftRequest
		if err := json.Unmarshal(rawRequest, &request); err != nil {
			return nil, fmt.Errorf("decode %s request: %w", declaredOperation, err)
		}
		return analysis.CheckDocDriftContext(ctx, cfg, request)
	case "check-compliance":
		var request analysis.ComplianceRequest
		if err := json.Unmarshal(rawRequest, &request); err != nil {
			return nil, fmt.Errorf("decode %s request: %w", declaredOperation, err)
		}
		return analysis.CheckComplianceContext(ctx, cfg, request)
	case "analyze-impact":
		var request analysis.AnalyzeImpactRequest
		if err := json.Unmarshal(rawRequest, &request); err != nil {
			return nil, fmt.Errorf("decode %s request: %w", declaredOperation, err)
		}
		return analysis.AnalyzeImpactContext(ctx, cfg, request)
	default:
		return nil, fmt.Errorf("unsupported executed operation %q", executedOperation)
	}
}

func evaluateBenchmarkCase(benchCase benchmarkCase, output any) []benchmarkCheck {
	switch result := output.(type) {
	case *analysis.CompareResult:
		return evaluateCompareCase(benchCase.Expectations, result)
	case *analysis.DocDriftResult:
		return evaluateDocDriftCase(benchCase.Expectations, result)
	case *analysis.ComplianceResult:
		return evaluateComplianceCase(benchCase.Expectations, result)
	case *analysis.AnalyzeImpactResult:
		return evaluateImpactCase(benchCase.Expectations, result)
	default:
		return []benchmarkCheck{{
			Name:    "unsupported_result_type",
			Passed:  false,
			Details: fmt.Sprintf("unsupported result type %T", output),
		}}
	}
}

func evaluateCompareCase(expect benchmarkExpectations, result *analysis.CompareResult) []benchmarkCheck {
	checks := make([]benchmarkCheck, 0, 4)
	checks = append(checks, containsAllCheck("spec_refs", result.SpecRefs, expect.MustIncludeSpecRefs))
	if expect.CompatibilityLevel != "" {
		checks = append(checks, benchmarkCheck{
			Name:    "compatibility_level",
			Passed:  result.Comparison.Compatibility.Level == expect.CompatibilityLevel,
			Details: fmt.Sprintf("got %q want %q", result.Comparison.Compatibility.Level, expect.CompatibilityLevel),
		})
	}
	if len(expect.RecommendationContains) > 0 {
		checks = append(checks, containsSubstringsCheck("recommendation", result.Comparison.Recommendation, expect.RecommendationContains))
	}
	return checks
}

func evaluateDocDriftCase(expect benchmarkExpectations, result *analysis.DocDriftResult) []benchmarkCheck {
	checks := make([]benchmarkCheck, 0, 6)
	driftDocRefs := make([]string, 0, len(result.DriftItems))
	specRefs := make([]string, 0)
	for _, item := range result.DriftItems {
		driftDocRefs = append(driftDocRefs, item.DocRef)
		specRefs = append(specRefs, item.SpecRefs...)
	}
	checks = append(checks, containsAllCheck("drift_doc_refs", driftDocRefs, expect.MustIncludeDocRefs))
	checks = append(checks, excludesAllCheck("excluded_doc_refs", driftDocRefs, expect.MustExcludeDocRefs))
	checks = append(checks, containsAllCheck("spec_refs", uniqueSorted(specRefs), expect.MustIncludeSpecRefs))
	if len(expect.AssessmentStatuses) > 0 {
		statuses := make(map[string]string, len(result.Assessments))
		for _, assessment := range result.Assessments {
			statuses[assessment.DocRef] = assessment.Status
		}
		for docRef, want := range expect.AssessmentStatuses {
			got := statuses[docRef]
			checks = append(checks, benchmarkCheck{
				Name:    "assessment_status:" + docRef,
				Passed:  got == want,
				Details: fmt.Sprintf("got %q want %q", got, want),
			})
		}
	}
	if expect.RequireStructuredEvidence {
		structured := false
		for _, item := range result.DriftItems {
			for _, finding := range item.Findings {
				if finding.Evidence != nil && finding.Evidence.SpecSection != "" && finding.Evidence.DocSection != "" {
					structured = true
					break
				}
			}
		}
		checks = append(checks, benchmarkCheck{
			Name:    "structured_evidence",
			Passed:  structured,
			Details: "at least one drift finding should include spec/doc sections",
		})
	}
	return checks
}

func evaluateComplianceCase(expect benchmarkExpectations, result *analysis.ComplianceResult) []benchmarkCheck {
	checks := make([]benchmarkCheck, 0, 6)
	relevantSpecRefs := make([]string, 0, len(result.RelevantSpecs))
	for _, spec := range result.RelevantSpecs {
		relevantSpecRefs = append(relevantSpecRefs, spec.SpecRef)
	}
	checks = append(checks, containsAllCheck("paths", result.Paths, expect.MustIncludePaths))
	checks = append(checks, containsAllCheck("relevant_spec_refs", uniqueSorted(relevantSpecRefs), expect.MustIncludeSpecRefs))
	checks = append(checks, benchmarkCheck{
		Name:    "minimum_compliant_findings",
		Passed:  len(result.Compliant) >= expect.MinimumCompliantFindings,
		Details: fmt.Sprintf("got %d want >= %d", len(result.Compliant), expect.MinimumCompliantFindings),
	})
	if expect.MaximumConflictFindings != nil {
		checks = append(checks, benchmarkCheck{
			Name:    "maximum_conflict_findings",
			Passed:  len(result.Conflicts) <= *expect.MaximumConflictFindings,
			Details: fmt.Sprintf("got %d want <= %d", len(result.Conflicts), *expect.MaximumConflictFindings),
		})
	}
	if len(expect.MustIncludeFindingCodes) > 0 {
		codes := make([]string, 0, len(result.Compliant)+len(result.Conflicts)+len(result.Unspecified))
		for _, finding := range result.Compliant {
			codes = append(codes, finding.Code)
		}
		for _, finding := range result.Conflicts {
			codes = append(codes, finding.Code)
		}
		for _, finding := range result.Unspecified {
			codes = append(codes, finding.Code)
		}
		checks = append(checks, containsAllCheck("finding_codes", uniqueSorted(codes), expect.MustIncludeFindingCodes))
	}
	if expect.RequireStructuredEvidence {
		structured := false
		for _, finding := range append(append([]analysis.ComplianceFinding{}, result.Compliant...), result.Conflicts...) {
			if finding.SectionHeading != "" {
				structured = true
				break
			}
		}
		checks = append(checks, benchmarkCheck{
			Name:    "structured_evidence",
			Passed:  structured,
			Details: "at least one compliance finding should include a section heading",
		})
	}
	return checks
}

func evaluateImpactCase(expect benchmarkExpectations, result *analysis.AnalyzeImpactResult) []benchmarkCheck {
	checks := make([]benchmarkCheck, 0, 6)
	affectedSpecRefs := make([]string, 0, len(result.AffectedSpecs))
	for _, spec := range result.AffectedSpecs {
		affectedSpecRefs = append(affectedSpecRefs, spec.Ref)
	}
	affectedDocRefs := make([]string, 0, len(result.AffectedDocs))
	for _, doc := range result.AffectedDocs {
		affectedDocRefs = append(affectedDocRefs, doc.Ref)
	}
	affectedRefs := make([]string, 0, len(result.AffectedRefs))
	for _, ref := range result.AffectedRefs {
		affectedRefs = append(affectedRefs, ref.Ref)
	}

	checks = append(checks, containsAllCheck("affected_spec_refs", uniqueSorted(affectedSpecRefs), expect.MustIncludeSpecRefs))
	checks = append(checks, containsAllCheck("affected_doc_refs", uniqueSorted(affectedDocRefs), expect.MustIncludeDocRefs))
	checks = append(checks, containsAllCheck("affected_refs", uniqueSorted(affectedRefs), expect.MustIncludeAffectedRefs))
	if expect.RequireStructuredEvidence {
		structured := false
		for _, doc := range result.AffectedDocs {
			if doc.Evidence != nil && doc.Evidence.SpecSection != "" && doc.Evidence.DocSection != "" && len(doc.SuggestedTargets) > 0 {
				structured = true
				break
			}
		}
		checks = append(checks, benchmarkCheck{
			Name:    "structured_evidence",
			Passed:  structured,
			Details: "at least one impacted doc should include linked evidence and suggested targets",
		})
	}
	return checks
}

func containsAllCheck(name string, got []string, want []string) benchmarkCheck {
	if len(want) == 0 {
		return benchmarkCheck{Name: name, Passed: true, Details: "no expectation"}
	}
	missing := make([]string, 0)
	have := make(map[string]struct{}, len(got))
	for _, value := range got {
		have[value] = struct{}{}
	}
	for _, value := range want {
		if _, ok := have[value]; !ok {
			missing = append(missing, value)
		}
	}
	return benchmarkCheck{
		Name:    name,
		Passed:  len(missing) == 0,
		Details: fmt.Sprintf("missing=%v got=%v", missing, got),
	}
}

func excludesAllCheck(name string, got []string, excluded []string) benchmarkCheck {
	if len(excluded) == 0 {
		return benchmarkCheck{Name: name, Passed: true, Details: "no expectation"}
	}
	found := make([]string, 0)
	have := make(map[string]struct{}, len(got))
	for _, value := range got {
		have[value] = struct{}{}
	}
	for _, value := range excluded {
		if _, ok := have[value]; ok {
			found = append(found, value)
		}
	}
	return benchmarkCheck{
		Name:    name,
		Passed:  len(found) == 0,
		Details: fmt.Sprintf("found=%v got=%v", found, got),
	}
}

func containsSubstringsCheck(name string, got string, want []string) benchmarkCheck {
	missing := make([]string, 0)
	for _, value := range want {
		if !strings.Contains(got, value) {
			missing = append(missing, value)
		}
	}
	return benchmarkCheck{
		Name:    name,
		Passed:  len(missing) == 0,
		Details: fmt.Sprintf("missing=%v got=%q", missing, got),
	}
}

func benchmarkChecksPassed(checks []benchmarkCheck) bool {
	if len(checks) == 0 {
		return false
	}
	for _, check := range checks {
		if !check.Passed {
			return false
		}
	}
	return true
}

func summarizeBenchmarkResults(results []benchmarkCaseResult, totalLatency float64) benchmarkSummary {
	summary := benchmarkSummary{TotalCases: len(results)}
	if len(results) > 0 {
		summary.MeanLatencyMS = totalLatency / float64(len(results))
	}
	for _, result := range results {
		if result.Passed {
			summary.PassedCases++
		} else {
			summary.FailedCases++
		}
		if result.AnalysisRuntimeUsed {
			summary.CasesUsingAnalysisRuntime++
		}
		if result.PromptSizeBytes != nil {
			summary.CasesWithPromptMeasurement++
		}
		if result.ResponseSizeBytes != nil {
			summary.CasesWithResponseMeasurement++
		}
	}
	return summary
}

func renderTextReport(w io.Writer, report *benchmarkReport) {
	fmt.Fprintf(w, "Pituitary minimal benchmark harness\n\n")
	fmt.Fprintf(w, "config: %s\n", report.ConfigPath)
	fmt.Fprintf(w, "cases: %s\n", report.CasesDir)
	fmt.Fprintf(w, "analysis runtime: %s\n", defaultString(report.AnalysisProvider, "disabled"))
	fmt.Fprintf(w, "summary: %d/%d passed | mean latency %.2fms | runtime-used %d case(s)\n\n",
		report.Summary.PassedCases,
		report.Summary.TotalCases,
		report.Summary.MeanLatencyMS,
		report.Summary.CasesUsingAnalysisRuntime,
	)
	for _, result := range report.Cases {
		prompt := "n/a"
		if result.PromptSizeBytes != nil {
			prompt = fmt.Sprintf("%dB", *result.PromptSizeBytes)
		}
		response := "n/a"
		if result.ResponseSizeBytes != nil {
			response = fmt.Sprintf("%dB", *result.ResponseSizeBytes)
		}
		status := "FAIL"
		if result.Passed {
			status = "PASS"
		}
		fmt.Fprintf(w, "%s | %s | latency %.2fms | prompt %s | response %s | checks %d\n",
			status, result.ID, result.LatencyMS, prompt, response, len(result.Checks))
		if result.OperationNote != "" {
			fmt.Fprintf(w, "  note: %s\n", result.OperationNote)
		}
		if result.Error != "" {
			fmt.Fprintf(w, "  error: %s\n", result.Error)
			continue
		}
		for _, check := range result.Checks {
			checkStatus := "ok"
			if !check.Passed {
				checkStatus = "fail"
			}
			fmt.Fprintf(w, "  - %s: %s\n", checkStatus, check.Name)
		}
	}
}

func newAnalysisProxy(target string) (*analysisProxy, error) {
	target = strings.TrimRight(strings.TrimSpace(target), "/")
	if target == "" {
		return nil, errors.New("analysis proxy target is required")
	}
	proxy := &analysisProxy{client: &http.Client{}, target: target}
	proxy.server = httptest.NewServer(http.HandlerFunc(proxy.handle))
	return proxy, nil
}

func (p *analysisProxy) handle(w http.ResponseWriter, r *http.Request) {
	requestBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("read proxy request: %v", err), http.StatusBadGateway)
		return
	}
	_ = r.Body.Close()

	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, p.target+r.URL.Path, bytes.NewReader(requestBody))
	if err != nil {
		http.Error(w, fmt.Sprintf("build upstream request: %v", err), http.StatusBadGateway)
		return
	}
	upstreamReq.Header = r.Header.Clone()
	upstreamReq.URL.RawQuery = r.URL.RawQuery

	resp, err := p.client.Do(upstreamReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy upstream request: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("read upstream response: %v", err), http.StatusBadGateway)
		return
	}

	p.mu.Lock()
	p.capture.Calls++
	p.capture.PromptBytes += len(requestBody)
	p.capture.ResponseBytes += len(responseBody)
	p.mu.Unlock()

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(responseBody)
}

func (p *analysisProxy) Reset() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.capture = analysisCapture{}
}

func (p *analysisProxy) Snapshot() analysisCapture {
	if p == nil {
		return analysisCapture{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.capture
}

func (p *analysisProxy) Close() {
	if p == nil || p.server == nil {
		return
	}
	p.server.Close()
}

func applyCapture(result *benchmarkCaseResult, capture analysisCapture) {
	result.ChatCompletionCalls = capture.Calls
	result.AnalysisRuntimeUsed = capture.Calls > 0
	if capture.Calls == 0 {
		return
	}
	result.PromptSizeBytes = intPtr(capture.PromptBytes)
	result.ResponseSizeBytes = intPtr(capture.ResponseBytes)
}

func cloneConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	cloned.Runtime = cfg.Runtime
	cloned.Runtime.Profiles = make(map[string]config.RuntimeProvider, len(cfg.Runtime.Profiles))
	for name, profile := range cfg.Runtime.Profiles {
		cloned.Runtime.Profiles[name] = profile
	}
	cloned.Sources = append([]config.Source(nil), cfg.Sources...)
	return &cloned
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
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
	}
	sort.Strings(result)
	return result
}

func intPtr(value int) *int {
	return &value
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
