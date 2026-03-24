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
	fs := flag.NewFlagSet("fix", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("fix", "pituitary [--config PATH] fix (--path PATH | --scope VALUE) [--dry-run] [--yes] [--format FORMAT]")

	var (
		path       string
		scope      string
		dryRun     bool
		yes        bool
		format     string
		configPath string
	)
	fs.StringVar(&path, "path", "", "target doc path to remediate")
	fs.StringVar(&scope, "scope", "", "target scope: accepted spec ref or all")
	fs.BoolVar(&dryRun, "dry-run", false, "plan deterministic edits without writing files")
	fs.BoolVar(&yes, "yes", false, "apply all planned edits without prompting")
	fs.StringVar(&format, "format", "text", "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "fix", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "fix", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	request := fixCLIRequest{
		Path:   strings.TrimSpace(path),
		Scope:  strings.TrimSpace(scope),
		DryRun: dryRun,
		Yes:    yes,
	}
	if err := validateCLIFormat("fix", format); err != nil {
		return writeCLIError(stdout, stderr, format, "fix", request, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	if request.Path == "" && request.Scope == "" {
		return writeCLIError(stdout, stderr, format, "fix", request, cliIssue{
			Code:    "validation_error",
			Message: "exactly one of --path or --scope is required",
		}, 2)
	}
	if request.Path != "" && request.Scope != "" {
		return writeCLIError(stdout, stderr, format, "fix", request, cliIssue{
			Code:    "validation_error",
			Message: "exactly one of --path or --scope is required",
		}, 2)
	}

	if format != commandFormatText && !request.DryRun && !request.Yes {
		return writeCLIError(stdout, stderr, format, "fix", request, cliIssue{
			Code:    "validation_error",
			Message: "non-text fix runs require either --dry-run or --yes",
		}, 2)
	}

	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "fix", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	if request.DryRun || request.Yes || format != commandFormatText {
		response := app.FixDocDrift(ctx, resolvedConfigPath, app.FixRequest{
			Path:  request.Path,
			Scope: request.Scope,
			Apply: request.Yes && !request.DryRun,
		})
		if response.Issue != nil {
			return writeCLIError(stdout, stderr, format, "fix", request, cliIssue{
				Code:    response.Issue.Code,
				Message: response.Issue.Message,
			}, response.Issue.ExitCode)
		}
		return writeCLISuccess(stdout, stderr, format, "fix", request, response.Result, nil)
	}

	if !stdinIsTTY() {
		return writeCLIError(stdout, stderr, format, "fix", request, cliIssue{
			Code:    "validation_error",
			Message: "interactive fix confirmation requires a TTY; rerun with --yes or --dry-run",
		}, 2)
	}

	planResponse := app.FixDocDrift(ctx, resolvedConfigPath, app.FixRequest{
		Path:  request.Path,
		Scope: request.Scope,
		Apply: false,
	})
	if planResponse.Issue != nil {
		return writeCLIError(stdout, stderr, format, "fix", request, cliIssue{
			Code:    planResponse.Issue.Code,
			Message: planResponse.Issue.Message,
		}, planResponse.Issue.ExitCode)
	}
	plan := planResponse.Result
	if plan == nil {
		return 0
	}

	selectedDocRefs := make([]string, 0, len(plan.Files))
	for _, file := range plan.Files {
		renderFixPromptFile(stdout, plan.Selector, file)
		applyFile, err := promptFixConfirmation(stdout, func() {
			fmt.Fprintln(stdout)
			renderFixPromptFile(stdout, plan.Selector, file)
		})
		if err != nil {
			return writeCLIError(stdout, stderr, format, "fix", request, cliIssue{
				Code:    "validation_error",
				Message: err.Error(),
			}, 2)
		}
		if applyFile {
			selectedDocRefs = append(selectedDocRefs, file.DocRef)
		}
	}
	if len(selectedDocRefs) == 0 {
		plan.Guidance = append(plan.Guidance, "No files selected; nothing was changed.")
		return writeCLISuccess(stdout, stderr, format, "fix", request, plan, nil)
	}

	applyResponse := app.FixDocDrift(ctx, resolvedConfigPath, app.FixRequest{
		DocRefs: selectedDocRefs,
		Apply:   true,
	})
	if applyResponse.Issue != nil {
		return writeCLIError(stdout, stderr, format, "fix", request, cliIssue{
			Code:    applyResponse.Issue.Code,
			Message: applyResponse.Issue.Message,
		}, applyResponse.Issue.ExitCode)
	}
	return writeCLISuccess(stdout, stderr, format, "fix", request, applyResponse.Result, nil)
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
