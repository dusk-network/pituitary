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
	DryRun  bool `json:"dry_run,omitempty"`
	Verbose bool `json:"verbose,omitempty"`
}

func runIndex(args []string, stdout, stderr io.Writer) int {
	return runIndexContext(context.Background(), args, stdout, stderr)
}

func runIndexContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("index", "pituitary [--config PATH] index (--rebuild | --dry-run) [--format FORMAT]")

	var (
		rebuild    bool
		dryRun     bool
		verbose    bool
		format     string
		configPath string
	)
	fs.BoolVar(&rebuild, "rebuild", false, "rebuild the local index")
	fs.BoolVar(&dryRun, "dry-run", false, "validate config and sources without writing the index")
	fs.BoolVar(&verbose, "verbose", false, "include per-source details for index planning and rebuild output")
	fs.StringVar(&format, "format", "text", "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "index", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "index", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}
	if err := validateCLIFormat("index", format); err != nil {
		return writeCLIError(stdout, stderr, format, "index", indexRequest{Rebuild: rebuild, DryRun: dryRun, Verbose: verbose}, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	request := indexRequest{Rebuild: rebuild, DryRun: dryRun, Verbose: verbose}
	if rebuild && dryRun {
		return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
			Code:    "validation_error",
			Message: "exactly one of --rebuild or --dry-run is allowed",
		}, 2)
	}
	if !rebuild && !dryRun {
		return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
			Code:    "validation_error",
			Message: "one of --rebuild or --dry-run is required",
		}, 2)
	}

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
	if dryRun {
		result, err := index.PrepareRebuildContext(ctx, cfg, records)
		if err != nil {
			if index.IsGraphValidationError(err) {
				return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
					Code:    "validation_error",
					Message: "relation graph invalid:\n" + err.Error(),
				}, 2)
			}
			if index.IsDependencyUnavailable(err) {
				return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
					Code:    "dependency_unavailable",
					Message: "dependency unavailable:\n" + err.Error(),
				}, 3)
			}
			return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
				Code:    "internal_error",
				Message: "dry run failed:\n" + err.Error(),
			}, 2)
		}
		if !verbose {
			result.Sources = nil
		}
		return writeCLISuccess(stdout, stderr, format, "index", request, result, nil)
	}

	var result *index.RebuildResult
	if format == "text" {
		result, err = index.RebuildWithProgressContext(ctx, cfg, records, func(event index.RebuildProgressEvent) {
			fmt.Fprintf(stderr, "pituitary index: %s %d/%d %s %s (%d chunk(s))\n", event.Phase, event.Current, event.Total, event.ArtifactKind, event.ArtifactRef, event.ChunkCount)
		})
	} else {
		result, err = index.RebuildContext(ctx, cfg, records)
	}
	if err != nil {
		if index.IsGraphValidationError(err) {
			return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
				Code:    "validation_error",
				Message: "relation graph invalid:\n" + err.Error(),
			}, 2)
		}
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
	if !verbose {
		result.Sources = nil
	}

	return writeCLISuccess(stdout, stderr, format, "index", request, result, nil)
}
