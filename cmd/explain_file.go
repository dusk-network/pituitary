package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

	var (
		format     string
		configPath string
	)
	fs.StringVar(&format, "format", "text", "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if err := fs.Parse(args); err != nil {
		return writeCLIError(stdout, stderr, format, "explain-file", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	if !isSupportedFormat(format) {
		return writeCLIError(stdout, stderr, format, "explain-file", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unsupported format %q", format),
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

	targetPath, err := resolveExplainPath(request.Path)
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

func resolveExplainPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path must not be empty")
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	return filepath.Join(cwd, path), nil
}
