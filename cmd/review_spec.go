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

func runReviewSpec(args []string, stdout, stderr io.Writer) int {
	return runReviewSpecContext(context.Background(), args, stdout, stderr)
}

func runReviewSpecContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("review-spec", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("review-spec", "pituitary [--config PATH] review-spec (--path PATH | --spec-ref REF | --spec-record-file PATH|- | --request-file PATH|-) [--format FORMAT]")

	var (
		specRef        string
		specPath       string
		specRecordFile string
		requestFile    string
		format         string
		configPath     string
	)
	fs.StringVar(&specRef, "spec-ref", "", "indexed spec ref")
	fs.StringVar(&specPath, "path", "", "workspace-relative or absolute path to an indexed spec")
	fs.StringVar(&specRecordFile, "spec-record-file", "", "path to canonical spec_record JSON, or - for stdin")
	fs.StringVar(&requestFile, "request-file", "", "path to review request JSON, or - for stdin")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "review-spec", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "review-spec", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	if err := validateCLIFormat("review-spec", format); err != nil {
		return writeCLIError(stdout, stderr, format, "review-spec", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "review-spec", nil, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}
	trimmedSpecRef := strings.TrimSpace(specRef)
	trimmedSpecPath := strings.TrimSpace(specPath)
	trimmedSpecRecordFile := strings.TrimSpace(specRecordFile)
	trimmedRequestFile := strings.TrimSpace(requestFile)
	if nonEmptyCount(trimmedSpecRef, trimmedSpecPath, trimmedSpecRecordFile, trimmedRequestFile) > 1 {
		return writeCLIError(stdout, stderr, format, "review-spec", nil, cliIssue{
			Code:    "validation_error",
			Message: "exactly one of --path, --spec-ref, --spec-record-file, or --request-file is allowed",
		}, 2)
	}
	var cfg *config.Config
	if trimmedSpecPath != "" || trimmedSpecRecordFile != "" || trimmedRequestFile != "" {
		cfg, err = config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "review-spec", nil, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
	}
	if trimmedRequestFile != "" {
		request, err := loadWorkspaceScopedJSONFile[analysis.ReviewRequest](cfg.Workspace.RootPath, trimmedRequestFile, "request file")
		if err != nil {
			return writeCLIError(stdout, stderr, format, "review-spec", nil, cliIssue{
				Code:    "validation_error",
				Message: err.Error(),
			}, 2)
		}
		operation := app.ReviewSpec(ctx, resolvedConfigPath, request)
		if operation.Issue != nil {
			return writeCLIError(stdout, stderr, format, "review-spec", operation.Request, cliIssue{
				Code:    operation.Issue.Code,
				Message: operation.Issue.Message,
			}, operation.Issue.ExitCode)
		}
		return writeCLISuccess(stdout, stderr, format, "review-spec", operation.Request, operation.Result, nil)
	}
	if trimmedSpecPath != "" {
		trimmedSpecRef, err = resolveIndexedSpecRefWithConfigContext(ctx, cfg, trimmedSpecPath)
		if err != nil {
			return writeSpecPathResolutionError(stdout, stderr, format, "review-spec", nil, err)
		}
	}
	workspaceRoot := ""
	if cfg != nil {
		workspaceRoot = cfg.Workspace.RootPath
	}
	request, err := reviewRequestFromFlags(workspaceRoot, trimmedSpecRef, trimmedSpecRecordFile)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "review-spec", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	operation := app.ReviewSpec(ctx, resolvedConfigPath, request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "review-spec", operation.Request, cliIssue{
			Code:    operation.Issue.Code,
			Message: operation.Issue.Message,
		}, operation.Issue.ExitCode)
	}

	return writeCLISuccess(stdout, stderr, format, "review-spec", operation.Request, operation.Result, nil)
}

func reviewRequestFromFlags(workspaceRoot, specRef, specRecordFile string) (analysis.ReviewRequest, error) {
	request := analysis.ReviewRequest{SpecRef: specRef}
	switch {
	case specRef != "" && specRecordFile != "":
		return request, fmt.Errorf("exactly one of --spec-ref or --spec-record-file is allowed")
	case specRef == "" && specRecordFile == "":
		return request, fmt.Errorf("one of --spec-ref or --spec-record-file is required")
	case specRef != "":
		return request, nil
	default:
		record, err := loadSpecRecordFile(workspaceRoot, specRecordFile)
		if err != nil {
			return request, err
		}
		request.SpecRecord = &record
		return request, nil
	}
}
