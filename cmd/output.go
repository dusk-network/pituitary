package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/source"
)

type cliIssue struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	Path    string         `json:"path,omitempty"`
}

type cliEnvelope struct {
	Request  any        `json:"request"`
	Result   any        `json:"result"`
	Warnings []cliIssue `json:"warnings"`
	Errors   []cliIssue `json:"errors"`
}

func isSupportedFormat(format string) bool {
	return format == commandFormatText || format == commandFormatJSON || format == commandFormatTable || format == commandFormatMarkdown || format == commandFormatHTML
}

func validateCLIFormat(command, format string) error {
	if !isSupportedFormat(format) {
		return fmt.Errorf("unsupported format %q", format)
	}
	if !commandSupportsFormat(command, format) {
		switch format {
		case commandFormatTable:
			return fmt.Errorf("format %q is only supported for search-specs", format)
		case commandFormatMarkdown:
			return fmt.Errorf("format %q is only supported for review-spec", format)
		case commandFormatHTML:
			return fmt.Errorf("format %q is only supported for review-spec", format)
		default:
			return fmt.Errorf("format %q is not supported for %s", format, command)
		}
	}
	return nil
}

func writeCLISuccess(stdout, stderr io.Writer, format, command string, request, result any, warnings []cliIssue) int {
	if len(warnings) == 0 {
		warnings = cliWarningsForResult(result)
	}
	if format == commandFormatJSON {
		return writeCLIJSON(stdout, cliEnvelope{
			Request:  request,
			Result:   result,
			Warnings: warnings,
			Errors:   []cliIssue{},
		})
	}
	if format == commandFormatTable {
		writeCLIWarnings(stderr, command, warnings)
		if err := renderCommandTable(stdout, command, result); err != nil {
			fmt.Fprintf(stderr, "pituitary %s: %s\n", command, err)
			return 2
		}
		return 0
	}
	if format == commandFormatMarkdown {
		writeCLIWarnings(stderr, command, warnings)
		if err := renderCommandMarkdown(stdout, command, result); err != nil {
			fmt.Fprintf(stderr, "pituitary %s: %s\n", command, err)
			return 2
		}
		return 0
	}
	if format == commandFormatHTML {
		writeCLIWarnings(stderr, command, warnings)
		if err := renderCommandHTML(stdout, command, result); err != nil {
			fmt.Fprintf(stderr, "pituitary %s: %s\n", command, err)
			return 2
		}
		return 0
	}
	writeCLIWarnings(stderr, command, warnings)
	if err := renderCommandResult(stdout, command, result); err != nil {
		fmt.Fprintf(stderr, "pituitary %s: %s\n", command, err)
		return 2
	}
	return 0
}

func writeCLIError(stdout, stderr io.Writer, format, command string, request any, issue cliIssue, exitCode int) int {
	if format == "json" {
		if writeCLIJSON(stdout, cliEnvelope{
			Request:  request,
			Result:   nil,
			Warnings: []cliIssue{},
			Errors:   []cliIssue{issue},
		}) != 0 {
			return 2
		}
		return exitCode
	}
	p := presentationForWriter(stderr)
	fmt.Fprintf(stderr, "%s %s\n", p.red("pituitary "+command+":"), issue.Message)
	return exitCode
}

func writeCLIJSON(w io.Writer, payload cliEnvelope) int {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		return 2
	}
	return 0
}

func cliIssueFromAppIssue(issue *app.Issue) cliIssue {
	if issue == nil {
		return cliIssue{}
	}
	return cliIssue{
		Code:    issue.Code,
		Message: issue.Message,
		Details: issue.Details,
	}
}

func cliWarningsForResult(result any) []cliIssue {
	switch typed := result.(type) {
	case *source.DiscoverResult:
		return discoverWarningsToCLIIssues(typed.Warnings)
	case *source.NewSpecBundleResult:
		return newSpecWarningsToCLIIssues(typed.Warnings)
	case *initResult:
		if typed.Discover == nil {
			return nil
		}
		return discoverWarningsToCLIIssues(typed.Discover.Warnings)
	case *analysis.AnalyzeImpactResult:
		return warningsToCLIIssues(typed.Warnings)
	case *analysis.TerminologyAuditResult:
		return warningsToCLIIssues(typed.Warnings)
	case *analysis.DocDriftResult:
		return warningsToCLIIssues(typed.Warnings)
	case *analysis.ReviewResult:
		return warningsToCLIIssues(typed.Warnings)
	default:
		return nil
	}
}

func newSpecWarningsToCLIIssues(warnings []source.NewSpecBundleWarning) []cliIssue {
	if len(warnings) == 0 {
		return nil
	}
	issues := make([]cliIssue, 0, len(warnings))
	for _, warning := range warnings {
		issues = append(issues, cliIssue{
			Code:    warning.Code,
			Message: warning.Message,
		})
	}
	return issues
}

func discoverWarningsToCLIIssues(warnings []source.DiscoverWarning) []cliIssue {
	if len(warnings) == 0 {
		return nil
	}
	issues := make([]cliIssue, 0, len(warnings))
	for _, warning := range warnings {
		issues = append(issues, cliIssue{
			Code:    warning.Code,
			Message: warning.Message,
			Path:    warning.SkippedPath,
		})
	}
	return issues
}

func warningsToCLIIssues(warnings []analysis.Warning) []cliIssue {
	if len(warnings) == 0 {
		return nil
	}
	issues := make([]cliIssue, 0, len(warnings))
	for _, warning := range warnings {
		issues = append(issues, cliIssue{
			Code:    warning.Code,
			Message: warning.Message,
		})
	}
	return issues
}

func writeCLIWarnings(stderr io.Writer, command string, warnings []cliIssue) {
	p := presentationForWriter(stderr)
	for _, warning := range warnings {
		fmt.Fprintf(stderr, "%s %s\n", p.yellow("pituitary "+command+": warning:"), warning.Message)
	}
}
