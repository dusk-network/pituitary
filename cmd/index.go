package cmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

type indexRequest struct {
	Rebuild   bool `json:"rebuild"`
	Update    bool `json:"update,omitempty"`
	DryRun    bool `json:"dry_run,omitempty"`
	Full      bool `json:"full,omitempty"`
	Verbose   bool `json:"verbose,omitempty"`
	ShowDelta bool `json:"show_delta,omitempty"`
}

type indexProgressLine struct {
	Event        string `json:"event"`
	Command      string `json:"command"`
	Phase        string `json:"phase"`
	ArtifactKind string `json:"artifact_kind"`
	ArtifactRef  string `json:"artifact_ref"`
	Current      int    `json:"current"`
	Total        int    `json:"total"`
	ChunkCount   int    `json:"chunk_count,omitempty"`
}

func runIndex(args []string, stdout, stderr io.Writer) int {
	return runIndexContext(context.Background(), args, stdout, stderr)
}

func runIndexContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("index", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("index", "pituitary [--config PATH] index (--rebuild | --update | --dry-run) [--full] [--format FORMAT]")

	var (
		rebuild    bool
		update     bool
		dryRun     bool
		full       bool
		verbose    bool
		showDelta  bool
		format     string
		configPath string
	)
	fs.BoolVar(&rebuild, "rebuild", false, "rebuild the local index")
	fs.BoolVar(&update, "update", false, "incrementally update the index, writing only changed artifacts")
	fs.BoolVar(&dryRun, "dry-run", false, "validate config and sources without writing the index")
	fs.BoolVar(&full, "full", false, "force a full re-embed instead of reusing compatible chunk vectors")
	fs.BoolVar(&verbose, "verbose", false, "include per-source details for index planning and rebuild output")
	fs.BoolVar(&showDelta, "show-delta", false, "show governance graph changes after an incremental update")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
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
		return writeCLIError(stdout, stderr, format, "index", indexRequest{Rebuild: rebuild, Update: update, DryRun: dryRun, Full: full, Verbose: verbose}, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	request := indexRequest{Rebuild: rebuild, Update: update, DryRun: dryRun, Full: full, Verbose: verbose, ShowDelta: showDelta}
	modeCount := 0
	if rebuild {
		modeCount++
	}
	if update {
		modeCount++
	}
	if dryRun {
		modeCount++
	}
	if modeCount != 1 {
		return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
			Code:    "validation_error",
			Message: "exactly one of --rebuild, --update, or --dry-run is required",
		}, 2)
	}
	if full && update {
		return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
			Code:    "validation_error",
			Message: "--full is only valid with --rebuild",
		}, 2)
	}
	if showDelta && !update {
		return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
			Code:    "validation_error",
			Message: "--show-delta is only valid with --update",
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
	records, err := source.LoadFromConfigWithOptions(cfg, source.LoadOptions{Logger: cliLoggerFromContext(ctx)})
	if err != nil {
		return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
			Code:    "source_error",
			Message: "source load failed:\n" + err.Error(),
		}, 2)
	}
	if dryRun {
		result, err := index.PrepareRebuildContextWithOptions(ctx, cfg, records, index.RebuildOptions{Full: full})
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
	if update {
		result, err = runIndexUpdate(ctx, cfg, records, showDelta, format, stderr)
	} else {
		result, err = runIndexRebuild(ctx, cfg, records, full, format, stderr)
	}
	if err != nil {
		if index.IsMissingIndex(err) {
			return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
				Code:    "missing_index",
				Message: err.Error(),
			}, 2)
		}
		if index.IsUpdatePrecondition(err) {
			return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
				Code:    "precondition_error",
				Message: err.Error(),
			}, 2)
		}
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
		action := "rebuild"
		if update {
			action = "update"
		}
		return writeCLIError(stdout, stderr, format, "index", request, cliIssue{
			Code:    "internal_error",
			Message: action + " failed:\n" + err.Error(),
		}, 2)
	}
	if !verbose {
		result.Sources = nil
	}
	if !showDelta {
		result.Delta = nil
	}

	return writeCLISuccess(stdout, stderr, format, "index", request, result, nil)
}

func runIndexUpdate(ctx context.Context, cfg *config.Config, records *source.LoadResult, showDelta bool, format string, stderr io.Writer) (*index.RebuildResult, error) {
	progressReporter := indexProgressReporter(format, stderr)
	var result *index.RebuildResult
	var err error
	if showDelta {
		result, err = index.UpdateWithDeltaContextAndOptions(ctx, cfg, records, index.UpdateOptions{ComputeDelta: true}, progressReporter)
	} else {
		result, err = index.UpdateWithProgressContextAndOptions(ctx, cfg, records, progressReporter)
	}
	// Emit the contextualizer line only after a successful update so
	// stderr never implies that contextualization applied when the
	// update actually failed before publish.
	if err == nil {
		emitRebuildContextualizerConfig(cfg, format, stderr)
	}
	return result, err
}

func runIndexRebuild(ctx context.Context, cfg *config.Config, records *source.LoadResult, full bool, format string, stderr io.Writer) (*index.RebuildResult, error) {
	progressReporter := indexProgressReporter(format, stderr)
	result, err := index.RebuildWithProgressContextAndOptions(ctx, cfg, records, index.RebuildOptions{Full: full}, progressReporter)
	// Emit the contextualizer line only after a successful rebuild so
	// stderr never implies that contextualization applied when the
	// rebuild actually failed before publish.
	if err == nil {
		emitRebuildContextualizerConfig(cfg, format, stderr)
	}
	return result, err
}

// emitRebuildContextualizerConfig announces a non-nil chunk
// contextualizer once per rebuild or update, after the data-path call
// returns without error. Callers must only invoke this on success so
// stderr never implies the contextualizer was applied when rebuild or
// update aborted before publish. The disabled path is silent per #347
// so day-to-day runs stay quiet and only opted-in behavior produces
// an extra line.
//
// Text mode only. JSON mode stderr is a progress-only NDJSON stream
// (every line is a "rebuild_progress"/"index" event; see the strict
// decoder in cmd/index_test.go's decodeIndexProgressEvents). Mixing a
// second event type into that stream would silently break existing
// strict parsers. Machine consumers should read contextualizer state
// from `pituitary status --format json`, which carries it on the
// runtime_config.contextualizer field for both enabled and disabled
// postures.
func emitRebuildContextualizerConfig(cfg *config.Config, format string, stderr io.Writer) {
	if cfg == nil || format != commandFormatText {
		return
	}
	contextualizer := strings.TrimSpace(cfg.Runtime.Chunking.Contextualizer.Format)
	if contextualizer == "" {
		return
	}
	fmt.Fprintf(stderr, "pituitary index: chunking contextualizer active (format=%s)\n", contextualizer)
}

func indexProgressReporter(format string, stderr io.Writer) index.RebuildProgressReporter {
	switch format {
	case commandFormatText:
		return func(event index.RebuildProgressEvent) {
			fmt.Fprintf(stderr, "pituitary index: %s %d/%d %s %s (%d chunk(s))\n", event.Phase, event.Current, event.Total, event.ArtifactKind, event.ArtifactRef, event.ChunkCount)
		}
	case commandFormatJSON:
		encoder := json.NewEncoder(stderr)
		return func(event index.RebuildProgressEvent) {
			_ = encoder.Encode(indexProgressLine{
				Event:        "rebuild_progress",
				Command:      "index",
				Phase:        event.Phase,
				ArtifactKind: event.ArtifactKind,
				ArtifactRef:  event.ArtifactRef,
				Current:      event.Current,
				Total:        event.Total,
				ChunkCount:   event.ChunkCount,
			})
		}
	default:
		return nil
	}
}
