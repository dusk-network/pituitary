package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/config"
)

type stringList []string

func (l *stringList) String() string {
	return strings.Join(*l, ",")
}

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func runCheckTerminology(args []string, stdout, stderr io.Writer) int {
	return runCheckTerminologyContext(context.Background(), args, stdout, stderr)
}

func runCheckTerminologyContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check-terminology", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("check-terminology", "pituitary [--config PATH] check-terminology (--term TERM... [--canonical-term TERM]... [--spec-ref REF | --path PATH] [--scope SCOPE] | --request-file PATH|-) [--format FORMAT]")

	var (
		terms          stringList
		canonicalTerms stringList
		specRef        string
		specPath       string
		scope          string
		requestFile    string
		format         string
		configPath     string
	)
	fs.Var(&terms, "term", "displaced term to audit; repeat to supply multiple terms")
	fs.Var(&canonicalTerms, "canonical-term", "replacement or canonical term; repeat to supply multiple terms")
	fs.StringVar(&specRef, "spec-ref", "", "indexed spec ref used to anchor the audit")
	fs.StringVar(&specPath, "path", "", "workspace-relative or absolute path to an indexed spec used to anchor the audit")
	fs.StringVar(&scope, "scope", "all", "artifact scope: all, docs, or specs")
	fs.StringVar(&requestFile, "request-file", "", "path to terminology audit request JSON, or - for stdin")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "check-terminology", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "check-terminology", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	request := analysis.TerminologyAuditRequest{
		Terms:          []string(terms),
		CanonicalTerms: []string(canonicalTerms),
		SpecRef:        strings.TrimSpace(specRef),
		Scope:          strings.TrimSpace(scope),
	}
	if err := validateCLIFormat("check-terminology", format); err != nil {
		return writeCLIError(stdout, stderr, format, "check-terminology", request, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "check-terminology", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	trimmedRequestFile := strings.TrimSpace(requestFile)
	if trimmedRequestFile != "" && (countNonEmptyStrings(request.Terms) > 0 || countNonEmptyStrings(request.CanonicalTerms) > 0 || request.SpecRef != "" || strings.TrimSpace(specPath) != "" || flagWasSet(fs, "scope")) {
		return writeCLIError(stdout, stderr, format, "check-terminology", request, cliIssue{
			Code:    "validation_error",
			Message: "use either --request-file or the terminology flags",
		}, 2)
	}
	if trimmedRequestFile != "" {
		cfg, err := config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-terminology", request, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
		request, err = loadWorkspaceScopedJSONFile[analysis.TerminologyAuditRequest](cfg.Workspace.RootPath, trimmedRequestFile, "request file")
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-terminology", request, cliIssue{
				Code:    "validation_error",
				Message: err.Error(),
			}, 2)
		}
	}
	if trimmedRequestFile == "" && countNonEmptyStrings(request.Terms) == 0 {
		return writeCLIError(stdout, stderr, format, "check-terminology", request, cliIssue{
			Code:    "validation_error",
			Message: "at least one term is required",
		}, 2)
	}

	trimmedPath := strings.TrimSpace(specPath)
	if trimmedRequestFile == "" && request.SpecRef != "" && trimmedPath != "" {
		return writeCLIError(stdout, stderr, format, "check-terminology", request, cliIssue{
			Code:    "validation_error",
			Message: "exactly one of --spec-ref or --path is allowed",
		}, 2)
	}
	if trimmedRequestFile == "" && trimmedPath != "" {
		cfg, err := config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-terminology", request, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
		request.SpecRef, err = resolveIndexedSpecRefWithConfigContext(ctx, cfg, trimmedPath)
		if err != nil {
			return writeSpecPathResolutionError(stdout, stderr, format, "check-terminology", request, err)
		}
	}

	operation := app.CheckTerminology(ctx, resolvedConfigPath, request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "check-terminology", operation.Request, cliIssue{
			Code:    operation.Issue.Code,
			Message: operation.Issue.Message,
		}, operation.Issue.ExitCode)
	}

	return writeCLISuccess(stdout, stderr, format, "check-terminology", operation.Request, operation.Result, nil)
}

func countNonEmptyStrings(values []string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}
