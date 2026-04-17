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
	var (
		specRef  string
		specPath string
		scope    string
	)

	return runCommand[analysis.FreshnessRequest, analysis.FreshnessResult](
		ctx, args, stdout, stderr,
		commandRun[analysis.FreshnessRequest, analysis.FreshnessResult]{
			Name:  "check-spec-freshness",
			Usage: "pituitary [--config PATH] check-spec-freshness [--spec-ref REF | --path PATH | --scope all | --request-file PATH|-] [--format FORMAT]",
			Options: commandRunOptions{
				RequestFile:    true,
				ConfigForFile:  true,
				ConfigForFlags: true,
			},
			BindFlags: func(fs *flag.FlagSet) {
				fs.StringVar(&specRef, "spec-ref", "", "indexed spec ref")
				fs.StringVar(&specPath, "path", "", "workspace-relative or absolute path to an indexed spec")
				fs.StringVar(&scope, "scope", "all", "scope: all (default)")
			},
			InlineFlagsSet: func(_ *flag.FlagSet) bool {
				return strings.TrimSpace(specRef) != "" || strings.TrimSpace(specPath) != ""
			},
			LoadRequestFile: autoLoadWorkspaceRequest[analysis.FreshnessRequest](),
			BuildRequest: func(ctx context.Context, cfg *config.Config, _ string, _ []string) (analysis.FreshnessRequest, error) {
				req := analysis.FreshnessRequest{Scope: strings.TrimSpace(scope)}
				resolvedSpecRef := strings.TrimSpace(specRef)
				trimmedPath := strings.TrimSpace(specPath)
				if resolvedSpecRef != "" && trimmedPath != "" {
					return req, fmt.Errorf("at most one of --path or --spec-ref may be specified")
				}
				if trimmedPath != "" {
					resolved, err := resolveIndexedSpecRefWithConfigContext(ctx, cfg, trimmedPath)
					if err != nil {
						return req, specPathResolutionError(err)
					}
					resolvedSpecRef = resolved
				}
				if resolvedSpecRef != "" {
					req.SpecRef = resolvedSpecRef
					req.Scope = ""
				}
				return req, nil
			},
			Execute: func(ctx context.Context, cfgPath string, req analysis.FreshnessRequest, _ string) (analysis.FreshnessRequest, *analysis.FreshnessResult, *app.Issue) {
				op := app.CheckSpecFreshness(ctx, cfgPath, req)
				return op.Request, op.Result, op.Issue
			},
		},
	)
}
