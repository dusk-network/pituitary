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
	var (
		docRefs       docRefList
		docPaths      docRefList
		scope         string
		diffFile      string
		atDate        string
		minConfidence string
	)

	return runCommand[analysis.DocDriftRequest, analysis.DocDriftResult](
		ctx, args, stdout, stderr,
		commandRun[analysis.DocDriftRequest, analysis.DocDriftResult]{
			Name:  "check-doc-drift",
			Usage: "pituitary [--config PATH] check-doc-drift ([--doc-ref REF | --path PATH]... | [--scope all] | [--diff-file PATH|-]) [--request-file PATH|-] [--format FORMAT] [--timings]",
			Options: commandRunOptions{
				RequestFile:    true,
				Timings:        true,
				ConfigForFile:  true,
				ConfigForFlags: true,
			},
			BindFlags: func(fs *flag.FlagSet) {
				fs.Var(&docRefs, "doc-ref", "target doc ref; repeat to supply doc_refs")
				fs.Var(&docPaths, "path", "workspace-relative or absolute path to an indexed doc; repeat to supply paths")
				fs.StringVar(&scope, "scope", "", "scope selector; only \"all\" is valid")
				fs.StringVar(&diffFile, "diff-file", "", "path to a unified diff file, or - for stdin")
				fs.StringVar(&atDate, "at", "", "ISO date for point-in-time governance query (e.g. 2025-03-15)")
				fs.StringVar(&minConfidence, "min-confidence", "", "minimum confidence tier: extracted, inferred, or ambiguous")
			},
			InlineFlagsSet: func(_ *flag.FlagSet) bool {
				return len(docRefs) > 0 || len(docPaths) > 0 || strings.TrimSpace(scope) != "" || strings.TrimSpace(diffFile) != ""
			},
			LoadRequestFile: func(_ context.Context, cfg *config.Config, trimmedPath string) (*analysis.DocDriftRequest, error) {
				req, err := loadWorkspaceScopedJSONFile[analysis.DocDriftRequest](cfg.Workspace.RootPath, trimmedPath, "request file")
				if err != nil {
					return nil, err
				}
				if req.DiffText == "" && strings.TrimSpace(req.DiffFile) != "" {
					diffText, diffErr := loadComplianceDiffFile(cfg.Workspace.RootPath, req.DiffFile)
					if diffErr != nil {
						return &req, diffErr
					}
					req.DiffText = diffText
				}
				return &req, nil
			},
			BuildRequest: func(ctx context.Context, cfg *config.Config, _ string, _ []string) (analysis.DocDriftRequest, error) {
				resolvedDocRefs := append([]string(nil), docRefs...)
				if len(docPaths) > 0 {
					resolvedPaths, err := resolveIndexedDocRefsWithConfigContext(ctx, cfg, []string(docPaths))
					if err != nil {
						return analysis.DocDriftRequest{}, docPathResolutionError(err)
					}
					resolvedDocRefs = append(resolvedDocRefs, resolvedPaths...)
				}
				return docDriftRequestFromFlags(cfg.Workspace.RootPath, resolvedDocRefs, strings.TrimSpace(scope), strings.TrimSpace(diffFile))
			},
			Normalize: func(_ context.Context, req analysis.DocDriftRequest, _ string) (analysis.DocDriftRequest, error) {
				if trimmedAt := strings.TrimSpace(atDate); trimmedAt != "" {
					req.AtDate = trimmedAt
				}
				if trimmedConf := strings.TrimSpace(minConfidence); trimmedConf != "" {
					req.MinConfidence = trimmedConf
				}
				return req, nil
			},
			Execute: func(ctx context.Context, cfgPath string, req analysis.DocDriftRequest, _ string) (analysis.DocDriftRequest, *analysis.DocDriftResult, *app.Issue) {
				op := app.CheckDocDrift(ctx, cfgPath, req)
				return op.Request, op.Result, op.Issue
			},
		},
	)
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
