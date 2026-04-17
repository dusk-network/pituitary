package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dusk-network/pituitary/internal/app"
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
	return runCommand[explainFileRequest, source.ExplainFileResult](
		ctx, reorderExplainFileArgs(args), stdout, stderr,
		commandRun[explainFileRequest, source.ExplainFileResult]{
			Name:  "explain-file",
			Usage: "pituitary [--config PATH] explain-file PATH [--format FORMAT]",
			Options: commandRunOptions{
				ExactPositional: 1,
			},
			BuildRequest: func(_ context.Context, _ *config.Config, _ string, positional []string) (explainFileRequest, error) {
				return explainFileRequest{Path: positional[0]}, nil
			},
			Execute: func(_ context.Context, cfgPath string, req explainFileRequest, _ string) (explainFileRequest, *source.ExplainFileResult, *app.Issue) {
				cfg, err := config.Load(cfgPath)
				if err != nil {
					return req, nil, &app.Issue{
						Code:     "config_error",
						Message:  "invalid config:\n" + err.Error(),
						ExitCode: 2,
					}
				}
				targetPath, err := resolveExplainPath(cfg, req.Path)
				if err != nil {
					return req, nil, plainIssue(err, "validation_error")
				}
				result, err := source.ExplainFile(cfg, targetPath)
				if err != nil {
					return req, nil, &app.Issue{
						Code:     "source_error",
						Message:  "file explanation failed:\n" + err.Error(),
						ExitCode: 2,
					}
				}
				return req, result, nil
			},
		},
	)
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
