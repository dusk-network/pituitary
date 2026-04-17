package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/config"
)

type compileCLIRequest struct {
	Scope  string `json:"scope,omitempty"`
	DryRun bool   `json:"dry_run,omitempty"`
	Yes    bool   `json:"yes,omitempty"`
}

func runCompile(args []string, stdout, stderr io.Writer) int {
	return runCompileContext(context.Background(), args, stdout, stderr)
}

func runCompileContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var (
		scope  string
		dryRun bool
		yes    bool
	)

	return runCommand[compileCLIRequest, app.CompileResult](
		ctx, args, stdout, stderr,
		commandRun[compileCLIRequest, app.CompileResult]{
			Name:  "compile",
			Usage: "pituitary [--config PATH] compile --scope SCOPE [--dry-run] [--yes] [--format FORMAT] [--timings]",
			Options: commandRunOptions{
				Timings: true,
			},
			BindFlags: func(fs *flag.FlagSet) {
				fs.StringVar(&scope, "scope", "", "target scope: accepted spec ref or all")
				fs.BoolVar(&dryRun, "dry-run", false, "plan deterministic edits without writing files")
				fs.BoolVar(&yes, "yes", false, "apply all planned edits without prompting")
			},
			BuildRequest: func(_ context.Context, _ *config.Config, _ string, _ []string) (compileCLIRequest, error) {
				req := compileCLIRequest{
					Scope:  strings.TrimSpace(scope),
					DryRun: dryRun,
					Yes:    yes,
				}
				if req.Scope == "" {
					return req, fmt.Errorf("--scope is required")
				}
				return req, nil
			},
			Normalize: func(_ context.Context, req compileCLIRequest, format string) (compileCLIRequest, error) {
				if format != commandFormatText && !req.DryRun && !req.Yes {
					return req, fmt.Errorf("non-text compile runs require either --dry-run or --yes")
				}
				return req, nil
			},
			Execute: func(ctx context.Context, cfgPath string, req compileCLIRequest, format string) (compileCLIRequest, *app.CompileResult, *app.Issue) {
				apply := req.Yes && !req.DryRun
				// In text mode without --dry-run or --yes, default to dry-run behavior with a hint.
				if format == commandFormatText && !req.DryRun && !req.Yes {
					apply = false
				}
				response := app.CompileTerminology(ctx, cfgPath, app.CompileRequest{
					Scope: req.Scope,
					Apply: apply,
				})
				result := response.Result
				if response.Issue == nil && format == commandFormatText && !req.DryRun && !req.Yes && result != nil {
					result.Guidance = append(result.Guidance, "Showing planned edits (dry-run). Re-run with --yes to apply.")
				}
				return req, result, response.Issue
			},
		},
	)
}
