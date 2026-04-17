package cmd

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/config"
)

type fixCLIRequest struct {
	Path   string `json:"path,omitempty"`
	Scope  string `json:"scope,omitempty"`
	DryRun bool   `json:"dry_run,omitempty"`
	Yes    bool   `json:"yes,omitempty"`
}

func runFix(args []string, stdout, stderr io.Writer) int {
	return runFixContext(context.Background(), args, stdout, stderr)
}

func runFixContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var (
		path   string
		scope  string
		dryRun bool
		yes    bool
	)

	return runCommand[fixCLIRequest, app.FixResult](
		ctx, args, stdout, stderr,
		commandRun[fixCLIRequest, app.FixResult]{
			Name:  "fix",
			Usage: "pituitary [--config PATH] fix (--path PATH | --scope VALUE) [--dry-run] [--yes] [--format FORMAT]",
			BindFlags: func(fs *flag.FlagSet) {
				fs.StringVar(&path, "path", "", "target doc path to remediate")
				fs.StringVar(&scope, "scope", "", "target scope: accepted spec ref or all")
				fs.BoolVar(&dryRun, "dry-run", false, "plan deterministic edits without writing files")
				fs.BoolVar(&yes, "yes", false, "apply all planned edits without prompting")
			},
			BuildRequest: func(_ context.Context, _ *config.Config, _ string, _ []string) (fixCLIRequest, error) {
				req := fixCLIRequest{
					Path:   strings.TrimSpace(path),
					Scope:  strings.TrimSpace(scope),
					DryRun: dryRun,
					Yes:    yes,
				}
				if req.Path == "" && req.Scope == "" {
					return req, fmt.Errorf("exactly one of --path or --scope is required")
				}
				if req.Path != "" && req.Scope != "" {
					return req, fmt.Errorf("exactly one of --path or --scope is required")
				}
				return req, nil
			},
			Normalize: func(_ context.Context, req fixCLIRequest, format string) (fixCLIRequest, error) {
				if format != commandFormatText && !req.DryRun && !req.Yes {
					return req, fmt.Errorf("non-text fix runs require either --dry-run or --yes")
				}
				return req, nil
			},
			Execute: func(ctx context.Context, cfgPath string, req fixCLIRequest, format string) (fixCLIRequest, *app.FixResult, *app.Issue) {
				// Non-interactive path: a single FixDocDrift call carries the run.
				if req.DryRun || req.Yes || format != commandFormatText {
					response := app.FixDocDrift(ctx, cfgPath, app.FixRequest{
						Path:  req.Path,
						Scope: req.Scope,
						Apply: req.Yes && !req.DryRun,
					})
					return req, response.Result, response.Issue
				}

				// Interactive text path: plan → prompt per file → apply selected.
				if !stdinIsTTY() {
					return req, nil, &app.Issue{
						Code:     "validation_error",
						Message:  "interactive fix confirmation requires a TTY; rerun with --yes or --dry-run",
						ExitCode: 2,
					}
				}

				planResponse := app.FixDocDrift(ctx, cfgPath, app.FixRequest{
					Path:  req.Path,
					Scope: req.Scope,
					Apply: false,
				})
				if planResponse.Issue != nil {
					return req, nil, planResponse.Issue
				}
				plan := planResponse.Result
				if plan == nil {
					// Return an empty result to avoid crashing the typed-nil
					// renderer branch; the "no deterministic doc-drift edits
					// available" line is the strict edge-case improvement
					// over the original silent zero exit.
					return req, &app.FixResult{}, nil
				}

				selectedDocRefs := make([]string, 0, len(plan.Files))
				for _, file := range plan.Files {
					renderFixPromptFile(stdout, plan.Selector, file)
					applyFile, err := promptFixConfirmation(stdout, func() {
						fmt.Fprintln(stdout)
						renderFixPromptFile(stdout, plan.Selector, file)
					})
					if err != nil {
						return req, nil, plainIssue(err, "validation_error")
					}
					if applyFile {
						selectedDocRefs = append(selectedDocRefs, file.DocRef)
					}
				}
				if len(selectedDocRefs) == 0 {
					plan.Guidance = append(plan.Guidance, "No files selected; nothing was changed.")
					return req, plan, nil
				}

				applyResponse := app.FixDocDrift(ctx, cfgPath, app.FixRequest{
					DocRefs: selectedDocRefs,
					Apply:   true,
				})
				return req, applyResponse.Result, applyResponse.Issue
			},
		},
	)
}

func stdinIsTTY() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func promptFixConfirmation(stdout io.Writer, renderDiff func()) (bool, error) {
	p := presentationForWriter(stdout)
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(stdout, "  %s %s ", p.yellow("apply these edits?"), p.bold("[y/n/diff]"))
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return false, fmt.Errorf("read confirmation: %w", err)
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		switch answer {
		case "y", "yes":
			fmt.Fprintln(stdout)
			return true, nil
		case "n", "no", "":
			fmt.Fprintln(stdout)
			return false, nil
		case "d", "diff":
			fmt.Fprintln(stdout)
			if renderDiff != nil {
				renderDiff()
			}
			continue
		default:
			fmt.Fprintln(stdout)
		}
	}
}
