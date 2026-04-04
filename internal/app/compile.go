package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/config"
)

// CompileRequest is the normalized input for terminology compilation.
type CompileRequest struct {
	Scope string `json:"scope,omitempty"`
	Apply bool   `json:"apply,omitempty"`
}

// CompileFileResult is the per-file result of a compile-terminology operation.
type CompileFileResult struct {
	Ref              string    `json:"ref"`
	SourceRef        string    `json:"source_ref,omitempty"`
	Path             string    `json:"path"`
	Status           string    `json:"status"`
	Reason           string    `json:"reason,omitempty"`
	Warnings         []string  `json:"warnings,omitempty"`
	Edits            []FixEdit `json:"edits,omitempty"`
	originalContent  string
	originalChecksum string
}

// CompileResult is the structured result of a compile-terminology operation.
type CompileResult struct {
	Scope            string              `json:"scope"`
	Applied          bool                `json:"applied"`
	Files            []CompileFileResult `json:"files"`
	PlannedFileCount int                 `json:"planned_file_count"`
	PlannedEditCount int                 `json:"planned_edit_count"`
	AppliedFileCount int                 `json:"applied_file_count,omitempty"`
	AppliedEditCount int                 `json:"applied_edit_count,omitempty"`
	SkippedCount     int                 `json:"skipped_count,omitempty"`
	Guidance         []string            `json:"guidance,omitempty"`
}

func runCompileTerminology(ctx context.Context, cfg *config.Config, request CompileRequest) (*CompileResult, error) {
	scope := strings.TrimSpace(request.Scope)
	if scope == "" {
		scope = "all"
	}

	auditResult, err := analysis.CheckTerminologyContext(ctx, cfg, analysis.TerminologyAuditRequest{
		Scope: scope,
	})
	if err != nil {
		return nil, err
	}

	result := &CompileResult{
		Scope:   scope,
		Applied: request.Apply,
		Files:   make([]CompileFileResult, 0),
	}

	for _, finding := range auditResult.Findings {
		fileResult, err := buildCompileFileResult(cfg, finding, request.Apply)
		if err != nil {
			return nil, err
		}
		result.Files = append(result.Files, fileResult)
	}

	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].Path < result.Files[j].Path
	})

	for _, file := range result.Files {
		switch file.Status {
		case "planned", "applied":
			if len(file.Edits) > 0 {
				result.PlannedFileCount++
				result.PlannedEditCount += len(file.Edits)
			}
			if file.Status == "applied" {
				result.AppliedFileCount++
				result.AppliedEditCount += len(file.Edits)
			}
		case "skipped":
			result.SkippedCount++
		}
	}

	if request.Apply {
		result.Guidance = append(result.Guidance, "The workspace index is now stale; run `pituitary index --rebuild` before the next analysis command.")
	} else if result.PlannedEditCount > 0 {
		result.Guidance = append(result.Guidance, "Re-run with `--yes` to apply these deterministic edits.")
	}
	if len(result.Files) == 0 {
		result.Guidance = append(result.Guidance, "No terminology compile edits were available for the selected scope.")
	}

	return result, nil
}

func buildCompileFileResult(cfg *config.Config, finding analysis.TerminologyFinding, apply bool) (CompileFileResult, error) {
	path, err := resolveSourceFilePath(cfg.Workspace.RootPath, finding.SourceRef)
	if err != nil {
		return CompileFileResult{}, err
	}

	// #nosec G304 -- path is resolved from a workspace source reference before reading.
	bodyBytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CompileFileResult{
				Ref:       finding.Ref,
				SourceRef: finding.SourceRef,
				Path:      filepath.ToSlash(path),
				Status:    "skipped",
				Reason:    "source file no longer exists",
			}, nil
		}
		return CompileFileResult{}, fmt.Errorf("read %s: %w", path, err)
	}
	body := string(bodyBytes)

	edits, warnings := planCompileEdits(body, finding)
	fileResult := CompileFileResult{
		Ref:              finding.Ref,
		SourceRef:        finding.SourceRef,
		Path:             filepath.ToSlash(path),
		Status:           "planned",
		Warnings:         warnings,
		originalContent:  body,
		originalChecksum: contentChecksum(body),
	}

	if len(edits) == 0 {
		fileResult.Status = "skipped"
		fileResult.Reason = "No deterministic terminology edits could be planned for this file."
		return fileResult, nil
	}

	fileResult.Edits = make([]FixEdit, 0, len(edits))
	for _, edit := range edits {
		fileResult.Edits = append(fileResult.Edits, FixEdit{
			Code:      "terminology_compile",
			Summary:   fmt.Sprintf("Replace %q with %q", edit.Replace, edit.With),
			Action:    "replace_term",
			Replace:   edit.Replace,
			With:      edit.With,
			Line:      edit.Line,
			StartByte: edit.StartByte,
			EndByte:   edit.EndByte,
			Before:    edit.Replace,
			After:     edit.With,
		})
	}

	if !apply {
		return fileResult, nil
	}

	patchEdits := make([]plannedEdit, len(edits))
	for i, e := range edits {
		patchEdits[i] = e
	}
	if err := applyEdits(path, body, fileResult.originalChecksum, patchEdits); err != nil {
		fileResult.Status = "skipped"
		fileResult.Reason = err.Error()
		return fileResult, nil
	}
	fileResult.Status = "applied"
	return fileResult, nil
}

func planCompileEdits(body string, finding analysis.TerminologyFinding) ([]plannedEdit, []string) {
	codeRanges := buildCodeRanges(body)

	// editByStart maps a start offset to the best (longest) edit at that position.
	editByStart := make(map[int]plannedEdit)
	warnings := make([]string, 0)

	for _, section := range finding.Sections {
		for _, match := range section.Matches {
			if match.Tolerated {
				continue
			}
			if strings.TrimSpace(match.Replacement) == "" {
				continue
			}

			indices := allMatchIndicesFold(body, match.Term)
			if len(indices) == 0 {
				warnings = append(warnings, fmt.Sprintf("Term %q no longer found in %s.", match.Term, finding.Ref))
				continue
			}

			for _, start := range indices {
				end := start + len(match.Term)

				// v2: skip matches inside code blocks or inline code spans.
				if isInsideCodeRange(start, end, codeRanges) {
					continue
				}

				// v2: skip matches that are part of a file path or compound identifier.
				if isPathContext(body, start, end) {
					continue
				}

				original := body[start:end]
				replacement := preserveCase(original, match.Replacement)
				line := 1 + strings.Count(body[:start], "\n")

				candidate := plannedEdit{
					Replace:   original,
					With:      replacement,
					Line:      line,
					StartByte: start,
					EndByte:   end,
					start:     start,
					end:       end,
				}

				if existing, ok := editByStart[start]; ok {
					// Keep the longer (more specific) match at this offset.
					if candidate.end > existing.end {
						editByStart[start] = candidate
					}
					continue
				}
				editByStart[start] = candidate
			}
		}
	}

	edits := make([]plannedEdit, 0, len(editByStart))
	for _, edit := range editByStart {
		edits = append(edits, edit)
	}

	sort.Slice(edits, func(i, j int) bool {
		return edits[i].start < edits[j].start
	})

	// Resolve overlaps: when a shorter match is fully contained in a longer
	// one that starts at an earlier offset, keep only the longer match.
	resolved := make([]plannedEdit, 0, len(edits))
	for _, edit := range edits {
		if len(resolved) > 0 && edit.start < resolved[len(resolved)-1].end {
			prev := &resolved[len(resolved)-1]
			// If the previous edit fully contains this one, skip the shorter.
			if edit.end <= prev.end {
				continue
			}
			// If this edit extends beyond the previous one, it's a genuine
			// ambiguous overlap — bail out for safety.
			return nil, []string{"Skipping file because planned terminology edits overlap."}
		}
		resolved = append(resolved, edit)
	}

	return resolved, warnings
}

// codeRange represents a byte range that is inside a code block or inline code span.
type codeRange struct {
	start, end int
}

// buildCodeRanges identifies all fenced code blocks (``` ... ```) and inline
// code spans (` ... `) in the markdown body, returning their byte ranges.
func buildCodeRanges(body string) []codeRange {
	var ranges []codeRange

	// Fenced code blocks: lines starting with ``` or ~~~
	i := 0
	for i < len(body) {
		// Find start of fenced block.
		lineStart := i
		if i > 0 && body[i-1] != '\n' {
			// Not at line start; advance to next line.
			nl := strings.IndexByte(body[i:], '\n')
			if nl < 0 {
				break
			}
			i += nl + 1
			continue
		}

		line := body[lineStart:]
		if nl := strings.IndexByte(line, '\n'); nl >= 0 {
			line = line[:nl]
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "```") && !strings.HasPrefix(trimmed, "~~~") {
			nl := strings.IndexByte(body[i:], '\n')
			if nl < 0 {
				break
			}
			i += nl + 1
			continue
		}

		fence := trimmed[:3]
		fenceStart := lineStart

		// Advance past opening fence line.
		nl := strings.IndexByte(body[i:], '\n')
		if nl < 0 {
			ranges = append(ranges, codeRange{fenceStart, len(body)})
			break
		}
		i += nl + 1

		// Find closing fence.
		closed := false
		for i < len(body) {
			closeLine := body[i:]
			if nextNl := strings.IndexByte(closeLine, '\n'); nextNl >= 0 {
				closeLine = closeLine[:nextNl]
			}
			closeEnd := i + len(closeLine)
			if strings.IndexByte(closeLine, '\n') >= 0 {
				closeEnd = i + strings.IndexByte(body[i:], '\n') + 1
			} else {
				closeEnd = len(body)
			}

			if strings.TrimSpace(closeLine) == fence {
				ranges = append(ranges, codeRange{fenceStart, closeEnd})
				nl2 := strings.IndexByte(body[i:], '\n')
				if nl2 < 0 {
					i = len(body)
				} else {
					i += nl2 + 1
				}
				closed = true
				break
			}

			nl2 := strings.IndexByte(body[i:], '\n')
			if nl2 < 0 {
				i = len(body)
				break
			}
			i += nl2 + 1
		}
		if !closed {
			ranges = append(ranges, codeRange{fenceStart, len(body)})
		}
	}

	// Inline code spans: `...` (not inside fenced blocks).
	for idx := 0; idx < len(body); idx++ {
		if body[idx] != '`' {
			continue
		}
		// Skip if inside a fenced block.
		if isInsideCodeRange(idx, idx+1, ranges) {
			continue
		}
		// Find the closing backtick.
		closeIdx := strings.IndexByte(body[idx+1:], '`')
		if closeIdx < 0 {
			break
		}
		closeIdx += idx + 1
		// Only count single-backtick spans (not fenced).
		if closeIdx > idx+1 {
			ranges = append(ranges, codeRange{idx, closeIdx + 1})
			idx = closeIdx
		}
	}

	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].start < ranges[j].start
	})
	return ranges
}

// isInsideCodeRange checks if a byte span [start, end) falls inside any code range.
func isInsideCodeRange(start, end int, ranges []codeRange) bool {
	for _, r := range ranges {
		if start >= r.start && end <= r.end {
			return true
		}
		if r.start > end {
			break
		}
	}
	return false
}

// isPathContext checks if a match at [start, end) in body appears to be part
// of a file path, URL, or compound identifier that should not be rewritten.
func isPathContext(body string, start, end int) bool {
	// Check characters immediately before the match.
	if start > 0 {
		prev := body[start-1]
		// Part of a path: /openclaw, ~/.openclaw, .openclaw
		if prev == '/' || prev == '.' {
			return true
		}
		// Part of a hyphenated compound: openclaw-server, openclaw-bridge
		if prev == '-' {
			return true
		}
	}

	// Check characters immediately after the match.
	if end < len(body) {
		next := body[end]
		// Part of a path: openclaw/foo
		if next == '/' {
			return true
		}
		// Part of a file extension: openclaw.json, openclaw.toml
		// But NOT end-of-sentence punctuation: "...openclaw." or "...openclaw,"
		if next == '.' && end+1 < len(body) {
			afterDot := body[end+1]
			if afterDot != ' ' && afterDot != '\n' && afterDot != '\r' && afterDot != ')' && afterDot != '"' && afterDot != '\'' {
				return true
			}
		}
		// Part of a hyphenated compound: openclaw-server
		if next == '-' {
			return true
		}
		// Part of a compound with underscore: openclaw_auth_token
		if next == '_' {
			return true
		}
	}

	return false
}

// preserveCase adjusts the replacement to match the casing pattern of the original text.
// If the original is ALL CAPS, the replacement is uppercased.
// If the original is Title Case (first letter upper, rest lower), the replacement is title-cased.
// Otherwise the replacement is returned as-is.
func preserveCase(original, replacement string) string {
	if original == "" || replacement == "" {
		return replacement
	}

	if isAllUpper(original) {
		return strings.ToUpper(replacement)
	}

	runes := []rune(original)
	if unicode.IsUpper(runes[0]) && isRestLower(original) {
		return titleCase(replacement)
	}

	return replacement
}

func isAllUpper(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

func isRestLower(s string) bool {
	first := true
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		if first {
			first = false
			continue
		}
		if unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
