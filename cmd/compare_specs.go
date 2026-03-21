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

type compareSpecRefs []string

func (l *compareSpecRefs) String() string {
	return strings.Join(*l, ",")
}

func (l *compareSpecRefs) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func runCompareSpecs(args []string, stdout, stderr io.Writer) int {
	return runCompareSpecsContext(context.Background(), args, stdout, stderr)
}

func runCompareSpecsContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("compare-specs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("compare-specs", "pituitary [--config PATH] compare-specs --spec-ref REF --spec-ref REF [--format FORMAT]")

	var (
		specRefs   compareSpecRefs
		format     string
		configPath string
	)
	fs.Var(&specRefs, "spec-ref", "indexed spec ref; pass exactly two to compare")
	fs.StringVar(&format, "format", "text", "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "compare-specs", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "compare-specs", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	request := analysis.CompareRequest{SpecRefs: []string(specRefs)}
	if !isSupportedFormat(format) {
		return writeCLIError(stdout, stderr, format, "compare-specs", request, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unsupported format %q", format),
		}, 2)
	}
	if len(request.SpecRefs) != 2 {
		return writeCLIError(stdout, stderr, format, "compare-specs", request, cliIssue{
			Code:    "validation_error",
			Message: "exactly two --spec-ref flags are required",
		}, 2)
	}
	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "compare-specs", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	operation := app.CompareSpecs(ctx, resolvedConfigPath, request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "compare-specs", operation.Request, cliIssue{
			Code:    operation.Issue.Code,
			Message: operation.Issue.Message,
		}, operation.Issue.ExitCode)
	}

	return writeCLISuccess(stdout, stderr, format, "compare-specs", operation.Request, operation.Result, nil)
}
