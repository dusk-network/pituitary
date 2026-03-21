package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

type indexRequest struct {
	Rebuild bool `json:"rebuild"`
}

func runIndex(args []string, stdout, stderr io.Writer) int {
	return runIndexContext(context.Background(), args, stdout, stderr)
}

func runIndexContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		rebuild    bool
		format     string
		configPath string
	)
	fs.BoolVar(&rebuild, "rebuild", false, "rebuild the local index")
	fs.StringVar(&format, "format", "text", "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if err := fs.Parse(args); err != nil {
		return writeCLIError(stdout, stderr, format, "index", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "index", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}
	if !isSupportedFormat(format) {
		return writeCLIError(stdout, stderr, format, "index", indexRequest{Rebuild: rebuild}, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unsupported format %q", format),
		}, 2)
	}
	if !rebuild {
		return writeCLIError(stdout, stderr, format, "index", indexRequest{Rebuild: false}, cliIssue{
			Code:    "validation_error",
			Message: "--rebuild is required",
		}, 2)
	}

	request := indexRequest{Rebuild: rebuild}

	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
			Code:    "config_error",
			Message: "invalid config:\n" + err.Error(),
		}, 2)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
			Code:    "source_error",
			Message: "source load failed:\n" + err.Error(),
		}, 2)
	}
	result, err := index.RebuildContext(ctx, cfg, records)
	if err != nil {
		if index.IsDependencyUnavailable(err) {
			return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
				Code:    "dependency_unavailable",
				Message: "dependency unavailable:\n" + err.Error(),
			}, 3)
		}
		return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
			Code:    "internal_error",
			Message: "rebuild failed:\n" + err.Error(),
		}, 2)
	}

	return writeCLISuccess(stdout, stderr, format, "index", request, result, nil)
}
