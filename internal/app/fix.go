package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

type FixRequest struct {
	Path    string   `json:"path,omitempty"`
	Scope   string   `json:"scope,omitempty"`
	DocRefs []string `json:"doc_refs,omitempty"`
	Apply   bool     `json:"apply,omitempty"`
}

type FixEdit struct {
	Code      string `json:"code"`
	Summary   string `json:"summary"`
	Action    string `json:"action"`
	Replace   string `json:"replace,omitempty"`
	With      string `json:"with,omitempty"`
	Line      int    `json:"line,omitempty"`
	StartByte int    `json:"start_byte,omitempty"`
	EndByte   int    `json:"end_byte,omitempty"`
	Before    string `json:"before,omitempty"`
	After     string `json:"after,omitempty"`
}

type FixFileResult struct {
	DocRef             string    `json:"doc_ref"`
	SourceRef          string    `json:"source_ref,omitempty"`
	Path               string    `json:"path"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason,omitempty"`
	Warnings           []string  `json:"warnings,omitempty"`
	Edits              []FixEdit `json:"edits,omitempty"`
	originalContent    string
	originalContentSum string
}

type FixResult struct {
	Selector         string          `json:"selector"`
	Applied          bool            `json:"applied"`
	Files            []FixFileResult `json:"files"`
	PlannedFileCount int             `json:"planned_file_count"`
	PlannedEditCount int             `json:"planned_edit_count"`
	AppliedFileCount int             `json:"applied_file_count,omitempty"`
	AppliedEditCount int             `json:"applied_edit_count,omitempty"`
	Guidance         []string        `json:"guidance,omitempty"`
}

type plannedFixEdit struct {
	FixEdit
	start int
	end   int
}

type fixNotFoundError struct {
	message string
}

var fixPathCaseInsensitive = runtime.GOOS == "windows"

func (e *fixNotFoundError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func FixDocDrift(ctx context.Context, configPath string, request FixRequest) Response[FixRequest, FixResult] {
	return executeWithFreshConfig(ctx, configPath, request, operationExecutionPolicy{
		NotFound: func(err error) bool {
			return analysis.IsNotFound(err) || isFixNotFound(err)
		},
	}, func(cfg *config.Config) (*FixResult, error) {
		return runFixDocDrift(ctx, cfg, request)
	})
}

func runFixDocDrift(ctx context.Context, cfg *config.Config, request FixRequest) (*FixResult, error) {
	selector, targetDocRefs, err := resolveFixTargets(ctx, cfg, request)
	if err != nil {
		return nil, err
	}

	driftRequest, ok := docDriftRequestForTargets(targetDocRefs, selector)
	if !ok {
		return &FixResult{
			Selector: selector,
			Applied:  request.Apply,
			Guidance: []string{"No docs matched the selected scope."},
		}, nil
	}

	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	docsByRef := docRecordsByRef(records.Docs)

	drift, err := analysis.CheckDocDriftContext(ctx, cfg, driftRequest)
	if err != nil {
		return nil, err
	}

	result := &FixResult{
		Selector: selector,
		Applied:  request.Apply,
		Files:    make([]FixFileResult, 0),
	}

	driftItems := make(map[string]analysis.DriftItem, len(drift.DriftItems))
	for _, item := range drift.DriftItems {
		driftItems[item.DocRef] = item
	}

	for _, item := range drift.Remediation.Items {
		record, ok := docsByRef[item.DocRef]
		if !ok {
			continue
		}
		fileResult, err := buildFixFileResult(cfg, record, item, driftItems[item.DocRef], request.Apply)
		if err != nil {
			return nil, err
		}
		result.Files = append(result.Files, fileResult)
	}

	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].Path < result.Files[j].Path
	})

	for _, file := range result.Files {
		if len(file.Edits) > 0 {
			result.PlannedFileCount++
			result.PlannedEditCount += len(file.Edits)
		}
		if file.Status == "applied" {
			result.AppliedFileCount++
			result.AppliedEditCount += len(file.Edits)
		}
	}

	if request.Apply {
		result.Guidance = append(result.Guidance, "The workspace index is now stale; run `pituitary index --rebuild` before the next analysis command.")
	} else if result.PlannedEditCount > 0 {
		result.Guidance = append(result.Guidance, "Re-run with `--yes` to apply these deterministic edits.")
	}
	if len(result.Files) == 0 {
		result.Guidance = append(result.Guidance, "No deterministic doc-drift remediations were available for the selected scope.")
	}

	return result, nil
}

func resolveFixTargets(ctx context.Context, cfg *config.Config, request FixRequest) (string, []string, error) {
	hasPath := strings.TrimSpace(request.Path) != ""
	hasScope := strings.TrimSpace(request.Scope) != ""
	hasDocRefs := len(request.DocRefs) > 0

	count := 0
	if hasPath {
		count++
	}
	if hasScope {
		count++
	}
	if hasDocRefs {
		count++
	}
	if count != 1 {
		return "", nil, fmt.Errorf("exactly one of path, scope, or doc_refs is required")
	}

	if hasDocRefs {
		return "doc_refs", uniqueNonEmptyStrings(request.DocRefs), nil
	}

	if hasPath {
		records, err := source.LoadFromConfig(cfg)
		if err != nil {
			return "", nil, err
		}
		docRef, err := docRefForFixPath(cfg.Workspace.RootPath, request.Path, records.Docs)
		if err != nil {
			return "", nil, err
		}
		return request.Path, []string{docRef}, nil
	}

	scope := strings.TrimSpace(request.Scope)
	if scope == "all" {
		return "all", nil, nil
	}

	impact, err := analysis.AnalyzeImpactContext(ctx, cfg, analysis.AnalyzeImpactRequest{SpecRef: scope})
	if err != nil {
		return "", nil, err
	}
	docRefs := make([]string, 0, len(impact.AffectedDocs))
	for _, item := range impact.AffectedDocs {
		docRefs = append(docRefs, item.Ref)
	}
	return scope, uniqueNonEmptyStrings(docRefs), nil
}

func docDriftRequestForTargets(docRefs []string, selector string) (analysis.DocDriftRequest, bool) {
	if selector == "all" {
		return analysis.DocDriftRequest{Scope: "all"}, true
	}
	switch len(docRefs) {
	case 0:
		return analysis.DocDriftRequest{}, false
	case 1:
		return analysis.DocDriftRequest{DocRef: docRefs[0]}, true
	default:
		return analysis.DocDriftRequest{DocRefs: docRefs}, true
	}
}

func docRefForFixPath(workspaceRoot, path string, docs []model.DocRecord) (string, error) {
	normalized := normalizeFixPath(workspaceRoot, path)
	for _, doc := range docs {
		sourcePath := normalizeFixPath(workspaceRoot, strings.TrimPrefix(doc.SourceRef, "file://"))
		if normalized == sourcePath {
			return doc.Ref, nil
		}
	}
	return "", &fixNotFoundError{message: fmt.Sprintf("doc path %q is not indexed by the current sources", path)}
}

func normalizeFixPath(workspaceRoot, path string) string {
	trimmed := strings.TrimSpace(strings.TrimPrefix(path, "file://"))
	if trimmed == "" {
		return ""
	}
	if filepath.IsAbs(trimmed) {
		if rel, err := filepath.Rel(workspaceRoot, trimmed); err == nil {
			trimmed = rel
		}
	}
	normalized := filepath.ToSlash(filepath.Clean(trimmed))
	if fixPathCaseInsensitive {
		return strings.ToLower(normalized)
	}
	return normalized
}

func docRecordsByRef(records []model.DocRecord) map[string]model.DocRecord {
	result := make(map[string]model.DocRecord, len(records))
	for _, record := range records {
		result[record.Ref] = record
	}
	return result
}

func buildFixFileResult(cfg *config.Config, record model.DocRecord, remediation analysis.DocRemediationItem, driftItem analysis.DriftItem, apply bool) (FixFileResult, error) {
	path, err := resolveSourceFilePath(cfg.Workspace.RootPath, remediation.SourceRef)
	if err != nil {
		return FixFileResult{}, err
	}
	// #nosec G304 -- path is resolved from a workspace source reference before reading.
	bodyBytes, err := os.ReadFile(path)
	if err != nil {
		return FixFileResult{}, fmt.Errorf("read %s: %w", path, err)
	}
	body := string(bodyBytes)

	edits, warnings := planFixEdits(body, remediation, driftItem)
	fileResult := FixFileResult{
		DocRef:             remediation.DocRef,
		SourceRef:          remediation.SourceRef,
		Path:               filepath.ToSlash(path),
		Status:             "planned",
		Warnings:           warnings,
		originalContent:    body,
		originalContentSum: contentChecksum(body),
	}

	if len(edits) == 0 {
		fileResult.Status = "skipped"
		fileResult.Reason = "No deterministic replace-claim edits could be planned for this file."
		return fileResult, nil
	}

	fileResult.Edits = make([]FixEdit, 0, len(edits))
	for _, edit := range edits {
		fileResult.Edits = append(fileResult.Edits, edit.FixEdit)
	}

	if !apply {
		return fileResult, nil
	}

	if err := applyFixEdits(path, body, fileResult.originalContentSum, edits); err != nil {
		fileResult.Status = "skipped"
		fileResult.Reason = err.Error()
		return fileResult, nil
	}
	fileResult.Status = "applied"
	return fileResult, nil
}

func planFixEdits(body string, remediation analysis.DocRemediationItem, driftItem analysis.DriftItem) ([]plannedFixEdit, []string) {
	findingsByKey := make(map[string]analysis.DriftFinding, len(driftItem.Findings))
	for _, finding := range driftItem.Findings {
		findingsByKey[fixSuggestionKey(finding.SpecRef, finding.Code)] = finding
	}

	edits := make([]plannedFixEdit, 0, len(remediation.Suggestions))
	warnings := make([]string, 0)
	for _, suggestion := range remediation.Suggestions {
		finding, ok := findingsByKey[fixSuggestionKey(suggestion.SpecRef, suggestion.Code)]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("Skipping %s: matching drift finding is unavailable.", suggestion.Code))
			continue
		}
		edit, warning := planFixEdit(body, suggestion, finding)
		if warning != "" {
			warnings = append(warnings, warning)
			continue
		}
		edits = append(edits, *edit)
	}

	sort.Slice(edits, func(i, j int) bool {
		return edits[i].start < edits[j].start
	})
	for i := 1; i < len(edits); i++ {
		if edits[i].start < edits[i-1].end {
			return nil, []string{"Skipping file because planned edits overlap."}
		}
	}
	return edits, warnings
}

func planFixEdit(body string, suggestion analysis.DocRemediationSuggestion, finding analysis.DriftFinding) (*plannedFixEdit, string) {
	if suggestion.SuggestedEdit.Action != "replace_claim" || suggestion.SuggestedEdit.Replace == "" || suggestion.SuggestedEdit.With == "" {
		return nil, fmt.Sprintf("Skipping %s: remediation requires a manual section rewrite.", suggestion.Code)
	}

	start, end, replace, with, ok := locateFixSpan(body, suggestion, finding)
	if !ok {
		return nil, fmt.Sprintf("Skipping %s: replacement text is ambiguous or no longer present in the file.", suggestion.Code)
	}

	line := 1 + strings.Count(body[:start], "\n")
	return &plannedFixEdit{
		FixEdit: FixEdit{
			Code:      suggestion.Code,
			Summary:   suggestion.Summary,
			Action:    suggestion.SuggestedEdit.Action,
			Replace:   replace,
			With:      with,
			Line:      line,
			StartByte: start,
			EndByte:   end,
			Before:    replace,
			After:     with,
		},
		start: start,
		end:   end,
	}, ""
}

func locateFixSpan(body string, suggestion analysis.DocRemediationSuggestion, finding analysis.DriftFinding) (int, int, string, string, bool) {
	if start, end, matched, ok := uniqueMatch(body, suggestion.SuggestedEdit.Replace); ok {
		return start, end, matched, suggestion.SuggestedEdit.With, true
	}

	excerpt := strings.TrimSpace(suggestion.Evidence.DocExcerpt)
	if excerpt == "" {
		return 0, 0, "", "", false
	}
	excerptStart, _, excerptMatched, ok := uniqueMatch(body, excerpt)
	if !ok {
		return 0, 0, "", "", false
	}

	replace, with, ok := fallbackFixReplacement(excerptMatched, suggestion, finding)
	if !ok {
		return 0, 0, "", "", false
	}
	innerStart, _, matched, ok := uniqueMatch(excerptMatched, replace)
	if !ok {
		return 0, 0, "", "", false
	}
	return excerptStart + innerStart, excerptStart + innerStart + len(matched), matched, with, true
}

func fallbackFixReplacement(excerpt string, suggestion analysis.DocRemediationSuggestion, finding analysis.DriftFinding) (string, string, bool) {
	switch finding.Code {
	case "default_limit_mismatch":
		if matched, ok := matchObservedVariant(excerpt, finding.Observed); ok {
			return matched, strings.TrimSpace(finding.Expected), true
		}
	case "window_mismatch":
		if matched, ok := matchObservedVariant(excerpt, finding.Observed); ok {
			return matched, strings.TrimSpace(finding.Expected), true
		}
	case "subject_mismatch":
		switch finding.Observed {
		case "api_key":
			if matched, ok := matchObservedVariant(excerpt, "per API key"); ok {
				return matched, "per tenant", true
			}
			if matched, ok := matchObservedVariant(excerpt, "API key"); ok {
				return matched, "tenant", true
			}
			if matched, ok := matchObservedVariant(excerpt, "API-key"); ok {
				return matched, "tenant", true
			}
		}
	case "override_support_mismatch":
		if _, _, matched, ok := uniqueMatch(excerpt, suggestion.SuggestedEdit.Replace); ok {
			return matched, suggestion.SuggestedEdit.With, true
		}
	}
	return "", "", false
}

func matchObservedVariant(text, observed string) (string, bool) {
	switch observed {
	case "100", "200":
		return uniqueObservedToken(text, observed)
	case "fixed-window":
		if matched, ok := uniqueObservedToken(text, "fixed-window"); ok {
			return matched, true
		}
		return uniqueObservedToken(text, "fixed window")
	case "sliding-window":
		if matched, ok := uniqueObservedToken(text, "sliding-window"); ok {
			return matched, true
		}
		return uniqueObservedToken(text, "sliding window")
	case "api_key":
		if matched, ok := uniqueObservedToken(text, "API key"); ok {
			return matched, true
		}
		if matched, ok := uniqueObservedToken(text, "API-key"); ok {
			return matched, true
		}
		return uniqueObservedToken(text, "per API key")
	case "tenant":
		if matched, ok := uniqueObservedToken(text, "tenant"); ok {
			return matched, true
		}
		return uniqueObservedToken(text, "per tenant")
	default:
		return uniqueObservedToken(text, observed)
	}
}

func uniqueObservedToken(text, token string) (string, bool) {
	_, _, matched, ok := uniqueMatch(text, token)
	return matched, ok
}

func applyFixEdits(path, expectedContent, expectedChecksum string, edits []plannedFixEdit) error {
	patchEdits := make([]plannedEdit, len(edits))
	for i, e := range edits {
		patchEdits[i] = plannedEdit{
			Replace:   e.Replace,
			With:      e.With,
			Line:      e.Line,
			StartByte: e.StartByte,
			EndByte:   e.EndByte,
			start:     e.start,
			end:       e.end,
		}
	}
	return applyEdits(path, expectedContent, expectedChecksum, patchEdits)
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]struct{}{}
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
	return result
}

func fixSuggestionKey(specRef, code string) string {
	return specRef + "\x00" + code
}

func isFixNotFound(err error) bool {
	var target *fixNotFoundError
	return errors.As(err, &target)
}
