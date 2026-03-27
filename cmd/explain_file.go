package cmd

import (
	"context"
	"flag"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/source"
)

type explainFileRequest struct {
	Path string `json:"path"`
}

func runExplainFile(args []string, stdout, stderr io.Writer) int {
	return runExplainFileContext(context.Background(), args, stdout, stderr)
}

func runExplainFileContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	args = reorderExplainFileArgs(args)

	fs := flag.NewFlagSet("explain-file", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("explain-file", "pituitary [--config PATH] explain-file PATH [--format FORMAT]")

	var (
		format     string
		configPath string
	)
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "explain-file", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if err := validateCLIFormat("explain-file", format); err != nil {
		return writeCLIError(stdout, stderr, format, "explain-file", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	if fs.NArg() != 1 {
		return writeCLIError(stdout, stderr, format, "explain-file", nil, cliIssue{
			Code:    "validation_error",
			Message: "exactly one file path is required",
		}, 2)
	}

	request := explainFileRequest{Path: fs.Arg(0)}

	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "explain-file", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "explain-file", request, cliIssue{
			Code:    "config_error",
			Message: "invalid config:\n" + err.Error(),
		}, 2)
	}

	targetPath, err := resolveExplainPath(cfg.Workspace.RootPath, request.Path)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "explain-file", request, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	result, err := source.ExplainFile(cfg, targetPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "explain-file", request, cliIssue{
			Code:    "source_error",
			Message: "file explanation failed:\n" + err.Error(),
		}, 2)
	}

	return writeCLISuccess(stdout, stderr, format, "explain-file", request, result, nil)
}

func reorderExplainFileArgs(args []string) []string {
	flagArgs := make([]string, 0, len(args))
	positionalArgs := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionalArgs = append(positionalArgs, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionalArgs = append(positionalArgs, arg)
			continue
		}

		flagArgs = append(flagArgs, arg)
		if explainFileFlagTakesValue(arg) && !strings.Contains(arg, "=") && i+1 < len(args) {
			flagArgs = append(flagArgs, args[i+1])
			i++
		}
	}

	return append(flagArgs, positionalArgs...)
}

func explainFileFlagTakesValue(arg string) bool {
	switch arg {
	case "-format", "--format", "-config", "--config":
		return true
	default:
		return false
	}
}

func resolveExplainPath(workspaceRoot, path string) (string, error) {
	return resolveWorkspaceScopedCLIPath(workspaceRoot, path, "file path")
}
