package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

type initRequest struct {
	Path       string `json:"path"`
	ConfigPath string `json:"config_path,omitempty"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

type initResult struct {
	WorkspaceRoot string                 `json:"workspace_root"`
	ConfigPath    string                 `json:"config_path"`
	ConfigAction  string                 `json:"config_action"`
	Discover      *source.DiscoverResult `json:"discover,omitempty"`
	Index         *index.RebuildResult   `json:"index,omitempty"`
	Status        *statusResult          `json:"status,omitempty"`
}

func runInit(args []string, stdout, stderr io.Writer) int {
	return runInitContext(context.Background(), args, stdout, stderr)
}

func runInitContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newStandaloneCommandHelp("init", "pituitary init [--path PATH] [--config-path PATH] [--dry-run] [--format FORMAT]")

	var (
		path       string
		configPath string
		dryRun     bool
		format     string
	)
	fs.StringVar(&path, "path", ".", "workspace path to initialize")
	fs.StringVar(&configPath, "config-path", "", "where init should place the generated config")
	fs.BoolVar(&dryRun, "dry-run", false, "preview the generated config and discovered sources without writing or indexing")
	fs.StringVar(&format, "format", "text", "output format")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "init", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "init", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}
	if err := validateCLIFormat("init", format); err != nil {
		return writeCLIError(stdout, stderr, format, "init", initRequest{Path: path, ConfigPath: strings.TrimSpace(configPath), DryRun: dryRun}, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	request := initRequest{
		Path:       path,
		ConfigPath: strings.TrimSpace(configPath),
		DryRun:     dryRun,
	}
	discovered, err := source.DiscoverWorkspace(source.DiscoverOptions{
		RootPath:   path,
		ConfigPath: request.ConfigPath,
		Write:      false,
	})
	if err != nil {
		return writeCLIError(stdout, stderr, format, "init", request, cliIssue{
			Code:    "discovery_error",
			Message: err.Error(),
		}, 2)
	}

	if dryRun {
		return writeCLISuccess(stdout, stderr, format, "init", request, &initResult{
			WorkspaceRoot: discovered.WorkspaceRoot,
			ConfigPath:    discovered.ConfigPath,
			ConfigAction:  "preview",
			Discover:      discovered,
		}, nil)
	}

	if err := ensureInitConfigAbsent(discovered.ConfigPath); err != nil {
		return writeCLIError(stdout, stderr, format, "init", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	// Validate sources before writing config. This prevents the user from
	// getting trapped: config written but sources fail to load, and re-running
	// init refuses because the config already exists.
	cfg, err := config.LoadFromText(discovered.Config, discovered.ConfigPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "init", request, cliIssue{
			Code:    "config_error",
			Message: "generated config is invalid:\n" + err.Error(),
		}, 2)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "init", request, cliIssue{
			Code:    "source_error",
			Message: "source load failed (config was not written):\n" + err.Error(),
		}, 2)
	}

	if err := source.WriteDiscoveredConfig(discovered.ConfigPath, discovered.Config); err != nil {
		return writeCLIError(stdout, stderr, format, "init", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	var rebuild *index.RebuildResult
	if format == commandFormatText {
		rebuild, err = index.RebuildWithProgressContextAndOptions(ctx, cfg, records, index.RebuildOptions{}, func(event index.RebuildProgressEvent) {
			fmt.Fprintf(stderr, "pituitary init: %s %d/%d %s %s (%d chunk(s))\n", event.Phase, event.Current, event.Total, event.ArtifactKind, event.ArtifactRef, event.ChunkCount)
		})
	} else {
		rebuild, err = index.RebuildContextWithOptions(ctx, cfg, records, index.RebuildOptions{})
	}
	if err != nil {
		switch {
		case index.IsGraphValidationError(err):
			return writeCLIError(stdout, stderr, format, "init", request, cliIssue{
				Code:    "validation_error",
				Message: "relation graph invalid:\n" + err.Error(),
			}, 2)
		case index.IsDependencyUnavailable(err):
			return writeCLIError(stdout, stderr, format, "init", request, cliIssue{
				Code:    "dependency_unavailable",
				Message: "dependency unavailable:\n" + err.Error(),
			}, 3)
		default:
			return writeCLIError(stdout, stderr, format, "init", request, cliIssue{
				Code:    "internal_error",
				Message: "rebuild failed:\n" + err.Error(),
			}, 2)
		}
	}

	statusResponse := app.Status(ctx, discovered.ConfigPath, app.StatusRequest{})
	if statusResponse.Issue != nil {
		return writeCLIError(stdout, stderr, format, "init", request, cliIssue{
			Code:    statusResponse.Issue.Code,
			Message: statusResponse.Issue.Message,
		}, statusResponse.Issue.ExitCode)
	}

	return writeCLISuccess(stdout, stderr, format, "init", request, &initResult{
		WorkspaceRoot: discovered.WorkspaceRoot,
		ConfigPath:    discovered.ConfigPath,
		ConfigAction:  "wrote",
		Discover:      discovered,
		Index:         rebuild,
		Status:        newStatusResult(statusResponse.Result, nil),
	}, nil)
}

func ensureInitConfigAbsent(path string) error {
	info, err := os.Stat(path)
	switch {
	case err == nil && info.IsDir():
		return fmt.Errorf("config path %s is a directory; choose a file path with --config-path", path)
	case err == nil:
		return fmt.Errorf("config already exists at %s; use `pituitary status` or `pituitary index --rebuild`, or choose a different --config-path", path)
	case os.IsNotExist(err):
		return nil
	default:
		return fmt.Errorf("stat config path: %w", err)
	}
}
