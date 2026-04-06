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

	targetPath, err := resolveExplainPath(cfg, request.Path)
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

func resolveExplainPath(cfg *config.Config, path string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("config is required to resolve file path")
	}
	trimmed, err := validateCLIPathValue(path, "file path")
	if err != nil {
		return "", err
	}

	rootPath := filepath.Clean(cfg.Workspace.RootPath)
	if !filepath.IsAbs(rootPath) {
		rootPath, err = filepath.Abs(rootPath)
		if err != nil {
			return "", fmt.Errorf("resolve workspace root %q: %w", cfg.Workspace.RootPath, err)
		}
	}

	absPath := trimmed
	if !filepath.IsAbs(absPath) {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return "", fmt.Errorf("resolve working directory: %w", cwdErr)
		}
		absPath = filepath.Join(cwd, absPath)
	}
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve file path %q: %w", path, err)
	}
	absPath = filepath.Clean(absPath)
	if !explainPathWithinConfiguredRoots(cfg, absPath) {
		return "", fmt.Errorf("file path %q resolves outside workspace root %q and configured repo roots", path, filepath.ToSlash(rootPath))
	}

	info, err := os.Stat(absPath)
	switch {
	case err != nil:
		return "", fmt.Errorf("stat file path %q: %w", path, err)
	case info.IsDir():
		return "", fmt.Errorf("file path %q is a directory", path)
	default:
		return absPath, nil
	}
}

func explainPathWithinConfiguredRoots(cfg *config.Config, absPath string) bool {
	if cliPathWithinRoot(cfg.Workspace.RootPath, absPath) {
		return true
	}
	for _, repo := range cfg.Workspace.Repos {
		if cliPathWithinRoot(repo.RootPath, absPath) {
			return true
		}
	}
	return false
}
