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

func runCheckSpecFreshness(args []string, stdout, stderr io.Writer) int {
	return runCheckSpecFreshnessContext(context.Background(), args, stdout, stderr)
}

func runCheckSpecFreshnessContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check-spec-freshness", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("check-spec-freshness", "pituitary [--config PATH] check-spec-freshness [--spec-ref REF | --path PATH | --scope all | --request-file PATH|-] [--format FORMAT]")

	var (
		specRef     string
		specPath    string
		requestFile string
		scope       string
		format      string
		configPath  string
	)
	fs.StringVar(&specRef, "spec-ref", "", "indexed spec ref")
	fs.StringVar(&specPath, "path", "", "workspace-relative or absolute path to an indexed spec")
	fs.StringVar(&requestFile, "request-file", "", "path to request JSON, or - for stdin")
	fs.StringVar(&scope, "scope", "all", "scope: all (default)")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "check-spec-freshness", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "check-spec-freshness", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	request := analysis.FreshnessRequest{
		Scope: strings.TrimSpace(scope),
	}
	if err := validateCLIFormat("check-spec-freshness", format); err != nil {
		return writeCLIError(stdout, stderr, format, "check-spec-freshness", request, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	trimmedSpecRef := strings.TrimSpace(specRef)
	trimmedSpecPath := strings.TrimSpace(specPath)
	trimmedRequestFile := strings.TrimSpace(requestFile)
	if nonEmptyCount(trimmedSpecRef, trimmedSpecPath, trimmedRequestFile) > 1 {
		return writeCLIError(stdout, stderr, format, "check-spec-freshness", request, cliIssue{
			Code:    "validation_error",
			Message: "at most one of --path, --spec-ref, or --request-file is allowed",
		}, 2)
	}

	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "check-spec-freshness", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	if trimmedSpecPath != "" {
		cfg, err := config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-spec-freshness", request, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
		trimmedSpecRef, err = resolveIndexedSpecRefWithConfigContext(ctx, cfg, trimmedSpecPath)
		if err != nil {
			return writeSpecPathResolutionError(stdout, stderr, format, "check-spec-freshness", request, err)
		}
	}

	switch {
	case trimmedRequestFile != "":
		cfg, err := config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-spec-freshness", request, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
		request, err = loadWorkspaceScopedJSONFile[analysis.FreshnessRequest](cfg.Workspace.RootPath, trimmedRequestFile, "request file")
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-spec-freshness", request, cliIssue{
				Code:    "validation_error",
				Message: err.Error(),
			}, 2)
		}
	case trimmedSpecRef != "":
		request.SpecRef = trimmedSpecRef
		request.Scope = ""
	default:
		// scope=all is the default
	}

	operation := app.CheckSpecFreshness(ctx, resolvedConfigPath, request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "check-spec-freshness", operation.Request, cliIssueFromAppIssue(operation.Issue), operation.Issue.ExitCode)
	}

	return writeCLISuccess(stdout, stderr, format, "check-spec-freshness", operation.Request, operation.Result, nil)
}
