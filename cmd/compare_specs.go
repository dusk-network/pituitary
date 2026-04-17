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
	var (
		specRefs  compareSpecRefs
		specPaths compareSpecRefs
	)

	return runCommand[analysis.CompareRequest, analysis.CompareResult](
		ctx, args, stdout, stderr,
		commandRun[analysis.CompareRequest, analysis.CompareResult]{
			Name:  "compare-specs",
			Usage: "pituitary [--config PATH] compare-specs (--spec-ref REF --spec-ref REF | --path PATH --path PATH | --request-file PATH|-) [--format FORMAT]",
			Options: commandRunOptions{
				RequestFile:    true,
				ConfigForFile:  true,
				ConfigForFlags: true,
			},
			BindFlags: func(fs *flag.FlagSet) {
				fs.Var(&specRefs, "spec-ref", "indexed spec ref; pass exactly two to compare")
				fs.Var(&specPaths, "path", "workspace-relative or absolute path to an indexed spec; pass exactly two to compare")
			},
			InlineFlagsSet: func(_ *flag.FlagSet) bool {
				return len(specRefs) > 0 || len(specPaths) > 0
			},
			LoadRequestFile: func(_ context.Context, cfg *config.Config, trimmedPath string) (*analysis.CompareRequest, error) {
				req, err := loadWorkspaceScopedJSONFile[analysis.CompareRequest](cfg.Workspace.RootPath, trimmedPath, "request file")
				if err != nil {
					return nil, err
				}
				return &req, nil
			},
			BuildRequest: func(ctx context.Context, cfg *config.Config, _ string, _ []string) (analysis.CompareRequest, error) {
				req := analysis.CompareRequest{}
				switch {
				case len(specRefs) > 0 && len(specPaths) > 0:
					return req, fmt.Errorf("use either two --spec-ref flags or two --path flags")
				case len(specRefs) > 0:
					req.SpecRefs = []string(specRefs)
				case len(specPaths) > 0:
					resolved, err := resolveIndexedSpecRefsWithConfigContext(ctx, cfg, []string(specPaths))
					if err != nil {
						return req, specPathResolutionError(err)
					}
					req.SpecRefs = resolved
				}
				return req, nil
			},
			Normalize: func(_ context.Context, req analysis.CompareRequest, _ string) (analysis.CompareRequest, error) {
				switch {
				case req.SpecRecord == nil && len(req.SpecRefs) != 2:
					return req, fmt.Errorf("exactly two --spec-ref flags or two --path flags are required")
				case req.SpecRecord != nil && len(req.SpecRefs) != 1:
					return req, fmt.Errorf("request files with spec_record require exactly one indexed spec_ref")
				}
				return req, nil
			},
			Execute: func(ctx context.Context, cfgPath string, req analysis.CompareRequest, _ string) (analysis.CompareRequest, *analysis.CompareResult, *app.Issue) {
				op := app.CompareSpecs(ctx, cfgPath, req)
				return op.Request, op.Result, op.Issue
			},
		},
	)
}
