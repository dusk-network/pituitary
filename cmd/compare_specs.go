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
	help := newCommandHelp("compare-specs", "pituitary [--config PATH] compare-specs (--spec-ref REF --spec-ref REF | --path PATH --path PATH | --request-file PATH|-) [--format FORMAT]")

	var (
		specRefs    compareSpecRefs
		specPaths   compareSpecRefs
		requestFile string
		format      string
		configPath  string
	)
	fs.Var(&specRefs, "spec-ref", "indexed spec ref; pass exactly two to compare")
	fs.Var(&specPaths, "path", "workspace-relative or absolute path to an indexed spec; pass exactly two to compare")
	fs.StringVar(&requestFile, "request-file", "", "path to compare request JSON, or - for stdin")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
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

	request := analysis.CompareRequest{}
	if err := validateCLIFormat("compare-specs", format); err != nil {
		return writeCLIError(stdout, stderr, format, "compare-specs", request, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "compare-specs", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}
	trimmedRequestFile := strings.TrimSpace(requestFile)
	switch {
	case trimmedRequestFile != "" && (len(specRefs) > 0 || len(specPaths) > 0):
		return writeCLIError(stdout, stderr, format, "compare-specs", request, cliIssue{
			Code:    "validation_error",
			Message: "use either --request-file or the path/spec-ref flags",
		}, 2)
	case trimmedRequestFile != "":
		cfg, err := config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "compare-specs", request, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
		request, err = loadWorkspaceScopedJSONFile[analysis.CompareRequest](cfg.Workspace.RootPath, trimmedRequestFile, "request file")
		if err != nil {
			return writeCLIError(stdout, stderr, format, "compare-specs", request, cliIssue{
				Code:    "validation_error",
				Message: err.Error(),
			}, 2)
		}
	case len(specRefs) > 0 && len(specPaths) > 0:
		return writeCLIError(stdout, stderr, format, "compare-specs", request, cliIssue{
			Code:    "validation_error",
			Message: "use either two --spec-ref flags or two --path flags",
		}, 2)
	case len(specRefs) == 2:
		request.SpecRefs = []string(specRefs)
	case len(specPaths) == 2:
		cfg, err := config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "compare-specs", request, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
		request.SpecRefs, err = resolveIndexedSpecRefsWithConfigContext(ctx, cfg, []string(specPaths))
		if err != nil {
			return writeSpecPathResolutionError(stdout, stderr, format, "compare-specs", request, err)
		}
	default:
		request.SpecRefs = []string(specRefs)
	}
	switch {
	case request.SpecRecord == nil && len(request.SpecRefs) != 2:
		return writeCLIError(stdout, stderr, format, "compare-specs", request, cliIssue{
			Code:    "validation_error",
			Message: "exactly two --spec-ref flags or two --path flags are required",
		}, 2)
	case request.SpecRecord != nil && len(request.SpecRefs) != 1:
		return writeCLIError(stdout, stderr, format, "compare-specs", request, cliIssue{
			Code:    "validation_error",
			Message: "request files with spec_record require exactly one indexed spec_ref",
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
