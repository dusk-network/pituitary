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

	var (
		docRefs    docRefList
		scope      string
		format     string
		configPath string
	)
	fs.Var(&docRefs, "doc-ref", "target doc ref; repeat to supply doc_refs")
	fs.StringVar(&scope, "scope", "", "scope selector; only \"all\" is valid")
	fs.StringVar(&format, "format", "text", "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if err := fs.Parse(args); err != nil {
		return writeCLIError(stdout, stderr, format, "check-doc-drift", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "check-doc-drift", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	request := docDriftRequestFromFlags([]string(docRefs), strings.TrimSpace(scope))
	if !isSupportedFormat(format) {
		return writeCLIError(stdout, stderr, format, "check-doc-drift", request, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unsupported format %q", format),
		}, 2)
	}
	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "check-doc-drift", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	operation := app.CheckDocDrift(ctx, resolvedConfigPath, request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "check-doc-drift", operation.Request, cliIssue{
			Code:    operation.Issue.Code,
			Message: operation.Issue.Message,
		}, operation.Issue.ExitCode)
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
