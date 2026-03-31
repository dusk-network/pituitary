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

type docRefList []string

func (l *docRefList) String() string {
	return strings.Join(*l, ",")
}

func (l *docRefList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func runCheckDocDrift(args []string, stdout, stderr io.Writer) int {
	return runCheckDocDriftContext(context.Background(), args, stdout, stderr)
}

func runCheckDocDriftContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check-doc-drift", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("check-doc-drift", "pituitary [--config PATH] check-doc-drift [--doc-ref REF]... [--scope all] [--request-file PATH|-] [--format FORMAT]")

	var (
		docRefs     docRefList
		scope       string
		requestFile string
		format      string
		configPath  string
	)
	fs.Var(&docRefs, "doc-ref", "target doc ref; repeat to supply doc_refs")
	fs.StringVar(&scope, "scope", "", "scope selector; only \"all\" is valid")
	fs.StringVar(&requestFile, "request-file", "", "path to doc drift request JSON, or - for stdin")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "check-doc-drift", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "check-doc-drift", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	if err := validateCLIFormat("check-doc-drift", format); err != nil {
		return writeCLIError(stdout, stderr, format, "check-doc-drift", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "check-doc-drift", nil, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}
	trimmedRequestFile := strings.TrimSpace(requestFile)

	var request analysis.DocDriftRequest
	switch {
	case trimmedRequestFile != "" && (len(docRefs) > 0 || strings.TrimSpace(scope) != ""):
		return writeCLIError(stdout, stderr, format, "check-doc-drift", nil, cliIssue{
			Code:    "validation_error",
			Message: "use either --request-file or the doc-ref/scope flags",
		}, 2)
	case trimmedRequestFile != "":
		cfg, err := config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-doc-drift", nil, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
		request, err = loadWorkspaceScopedJSONFile[analysis.DocDriftRequest](cfg.Workspace.RootPath, trimmedRequestFile, "request file")
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-doc-drift", nil, cliIssue{
				Code:    "validation_error",
				Message: err.Error(),
			}, 2)
		}
	default:
		request = docDriftRequestFromFlags([]string(docRefs), strings.TrimSpace(scope))
	}

	operation := app.CheckDocDrift(ctx, resolvedConfigPath, request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "check-doc-drift", operation.Request, cliIssueFromAppIssue(operation.Issue), operation.Issue.ExitCode)
	}

	return writeCLISuccess(stdout, stderr, format, "check-doc-drift", operation.Request, operation.Result, nil)
}

func docDriftRequestFromFlags(docRefs []string, scope string) analysis.DocDriftRequest {
	refs := make([]string, 0, len(docRefs))
	for _, ref := range docRefs {
		ref = strings.TrimSpace(ref)
		if ref != "" {
			refs = append(refs, ref)
		}
	}

	switch len(refs) {
	case 0:
		return analysis.DocDriftRequest{Scope: scope}
	case 1:
		return analysis.DocDriftRequest{DocRef: refs[0], Scope: scope}
	default:
		return analysis.DocDriftRequest{DocRefs: refs, Scope: scope}
	}
}
