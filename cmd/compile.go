package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/app"
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
	fs := flag.NewFlagSet("compile", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("compile", "pituitary [--config PATH] compile --scope SCOPE [--dry-run] [--yes] [--format FORMAT]")

	var (
		scope      string
		dryRun     bool
		yes        bool
		format     string
		configPath string
	)
	fs.StringVar(&scope, "scope", "", "target scope: accepted spec ref or all")
	fs.BoolVar(&dryRun, "dry-run", false, "plan deterministic edits without writing files")
	fs.BoolVar(&yes, "yes", false, "apply all planned edits without prompting")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "compile", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "compile", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	request := compileCLIRequest{
		Scope:  strings.TrimSpace(scope),
		DryRun: dryRun,
		Yes:    yes,
	}
	if err := validateCLIFormat("compile", format); err != nil {
		return writeCLIError(stdout, stderr, format, "compile", request, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	if request.Scope == "" {
		return writeCLIError(stdout, stderr, format, "compile", request, cliIssue{
			Code:    "validation_error",
			Message: "--scope is required",
		}, 2)
	}

	if format != commandFormatText && !request.DryRun && !request.Yes {
		return writeCLIError(stdout, stderr, format, "compile", request, cliIssue{
			Code:    "validation_error",
			Message: "non-text compile runs require either --dry-run or --yes",
		}, 2)
	}

	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "compile", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	apply := request.Yes && !request.DryRun

	// In text mode without --dry-run or --yes, default to dry-run behavior with a hint.
	if format == commandFormatText && !request.DryRun && !request.Yes {
		apply = false
	}

	response := app.CompileTerminology(ctx, resolvedConfigPath, app.CompileRequest{
		Scope: request.Scope,
		Apply: apply,
	})
	if response.Issue != nil {
		return writeCLIError(stdout, stderr, format, "compile", request, cliIssue{
			Code:    response.Issue.Code,
			Message: response.Issue.Message,
		}, response.Issue.ExitCode)
	}

	if format == commandFormatText && !request.DryRun && !request.Yes && response.Result != nil {
		response.Result.Guidance = append(response.Result.Guidance, "Showing planned edits (dry-run). Re-run with --yes to apply.")
	}

	return writeCLISuccess(stdout, stderr, format, "compile", request, response.Result, nil)
}
