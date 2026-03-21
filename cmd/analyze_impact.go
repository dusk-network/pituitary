package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/app"
)

func runAnalyzeImpact(args []string, stdout, stderr io.Writer) int {
	return runAnalyzeImpactContext(context.Background(), args, stdout, stderr)
}

func runAnalyzeImpactContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("analyze-impact", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		specRef    string
		changeType string
		format     string
		configPath string
	)
	fs.StringVar(&specRef, "spec-ref", "", "indexed spec ref")
	fs.StringVar(&changeType, "change-type", "accepted", "change type: accepted, modified, or deprecated")
	fs.StringVar(&format, "format", "text", "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if err := fs.Parse(args); err != nil {
		return writeCLIError(stdout, stderr, format, "analyze-impact", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "analyze-impact", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	request := analysis.AnalyzeImpactRequest{
		SpecRef:    strings.TrimSpace(specRef),
		ChangeType: strings.TrimSpace(changeType),
	}
	if !isSupportedFormat(format) {
		return writeCLIError(stdout, stderr, format, "analyze-impact", request, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unsupported format %q", format),
		}, 2)
	}
	if request.SpecRef == "" {
		return writeCLIError(stdout, stderr, format, "analyze-impact", request, cliIssue{
			Code:    "validation_error",
			Message: "--spec-ref is required",
		}, 2)
	}
	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "analyze-impact", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	operation := app.AnalyzeImpact(ctx, resolvedConfigPath, request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "analyze-impact", operation.Request, cliIssue{
			Code:    operation.Issue.Code,
			Message: operation.Issue.Message,
		}, operation.Issue.ExitCode)
	}

	return writeCLISuccess(stdout, stderr, format, "analyze-impact", operation.Request, operation.Result, nil)
}
