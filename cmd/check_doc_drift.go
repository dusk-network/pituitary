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
	help := newCommandHelp("check-doc-drift", "pituitary [--config PATH] check-doc-drift ([--doc-ref REF | --path PATH]... | [--scope all] | [--diff-file PATH|-]) [--request-file PATH|-] [--format FORMAT] [--timings]")

	var (
		docRefs     docRefList
		docPaths    docRefList
		scope       string
		diffFile    string
		requestFile string
		format      string
		configPath  string
		atDate      string
		timings     bool
	)
	fs.Var(&docRefs, "doc-ref", "target doc ref; repeat to supply doc_refs")
	fs.Var(&docPaths, "path", "workspace-relative or absolute path to an indexed doc; repeat to supply paths")
	fs.StringVar(&scope, "scope", "", "scope selector; only \"all\" is valid")
	fs.StringVar(&diffFile, "diff-file", "", "path to a unified diff file, or - for stdin")
	fs.StringVar(&requestFile, "request-file", "", "path to doc drift request JSON, or - for stdin")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")
	fs.StringVar(&atDate, "at", "", "ISO date for point-in-time governance query (e.g. 2025-03-15)")
	fs.BoolVar(&timings, "timings", false, "include timing metadata in JSON output")

	var minConfidence string
	fs.StringVar(&minConfidence, "min-confidence", "", "minimum confidence tier: extracted, inferred, or ambiguous")

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
	case trimmedRequestFile != "" && (len(docRefs) > 0 || len(docPaths) > 0 || strings.TrimSpace(scope) != "" || strings.TrimSpace(diffFile) != ""):
		return writeCLIError(stdout, stderr, format, "check-doc-drift", nil, cliIssue{
			Code:    "validation_error",
			Message: "use either --request-file or the doc-ref/path/scope/diff-file flags",
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
		if request.DiffText == "" && strings.TrimSpace(request.DiffFile) != "" {
			request.DiffText, err = loadComplianceDiffFile(cfg.Workspace.RootPath, request.DiffFile)
			if err != nil {
				return writeCLIError(stdout, stderr, format, "check-doc-drift", request, cliIssue{
					Code:    "validation_error",
					Message: err.Error(),
				}, 2)
			}
		}
	default:
		cfg, err := config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-doc-drift", nil, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
		resolvedDocRefs := append([]string(nil), docRefs...)
		if len(docPaths) > 0 {
			resolvedPaths, err := resolveIndexedDocRefsWithConfigContext(ctx, cfg, []string(docPaths))
			if err != nil {
				return writeDocPathResolutionError(stdout, stderr, format, "check-doc-drift", nil, err)
			}
			resolvedDocRefs = append(resolvedDocRefs, resolvedPaths...)
		}
		request, err = docDriftRequestFromFlags(cfg.Workspace.RootPath, resolvedDocRefs, strings.TrimSpace(scope), strings.TrimSpace(diffFile))
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-doc-drift", nil, cliIssue{
				Code:    "validation_error",
				Message: err.Error(),
			}, 2)
		}
	}

	if trimmedAt := strings.TrimSpace(atDate); trimmedAt != "" {
		request.AtDate = trimmedAt
	}
	if trimmedConf := strings.TrimSpace(minConfidence); trimmedConf != "" {
		request.MinConfidence = trimmedConf
	}
	ctx, tracker, started := withCommandTimings(ctx, timings && format == commandFormatJSON)

	operation := app.CheckDocDrift(ctx, resolvedConfigPath, request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "check-doc-drift", operation.Request, cliIssueFromAppIssue(operation.Issue), operation.Issue.ExitCode)
	}

	return writeCLISuccessWithTimings(stdout, stderr, format, "check-doc-drift", operation.Request, operation.Result, nil, snapshotCommandTimings(tracker, started))
}

func docDriftRequestFromFlags(workspaceRoot string, docRefs []string, scope, diffFile string) (analysis.DocDriftRequest, error) {
	refs := make([]string, 0, len(docRefs))
	for _, ref := range docRefs {
		ref = strings.TrimSpace(ref)
		if ref != "" {
			refs = append(refs, ref)
		}
	}

	switch {
	case len(refs) > 0 && (scope != "" || diffFile != ""):
		return analysis.DocDriftRequest{}, fmt.Errorf("exactly one of --doc-ref/--path, --scope, or --diff-file is required")
	case scope != "" && diffFile != "":
		return analysis.DocDriftRequest{}, fmt.Errorf("exactly one of --doc-ref/--path, --scope, or --diff-file is required")
	case len(refs) == 0 && scope == "" && diffFile == "":
		return analysis.DocDriftRequest{}, fmt.Errorf("one of --doc-ref/--path, --scope, or --diff-file is required")
	case diffFile != "":
		diffText, err := loadComplianceDiffFile(workspaceRoot, diffFile)
		if err != nil {
			return analysis.DocDriftRequest{}, err
		}
		return analysis.DocDriftRequest{
			DiffFile: diffFile,
			DiffText: diffText,
		}, nil
	case len(refs) == 0:
		return analysis.DocDriftRequest{Scope: scope}, nil
	case len(refs) == 1:
		return analysis.DocDriftRequest{DocRef: refs[0], Scope: scope}, nil
	default:
		return analysis.DocDriftRequest{DocRefs: refs, Scope: scope}, nil
	}
}
