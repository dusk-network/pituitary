package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/source"
)

type previewSourcesRequest struct {
	Verbose bool `json:"verbose,omitempty"`
}

func runPreviewSources(args []string, stdout, stderr io.Writer) int {
	return runPreviewSourcesContext(context.Background(), args, stdout, stderr)
}

func runPreviewSourcesContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("preview-sources", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("preview-sources", "pituitary [--config PATH] preview-sources [--verbose] [--format FORMAT]")

	var (
		format     string
		configPath string
		verbose    bool
	)
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")
	fs.BoolVar(&verbose, "verbose", false, "show selector-match diagnostics for previewed files")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "preview-sources", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "preview-sources", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}
	if err := validateCLIFormat("preview-sources", format); err != nil {
		return writeCLIError(stdout, stderr, format, "preview-sources", previewSourcesRequest{}, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	request := previewSourcesRequest{Verbose: verbose}

	resolvedConfigPath, resolution, err := resolveCommandConfigPathWithResolution(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "preview-sources", request, configErrorIssue(err.Error()), 2)
	}

	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "preview-sources", request, configLoadIssue("invalid config:\n"+err.Error(), resolution), 2)
	}
	emitMultirepoShadowWarning(resolution)

	result, err := source.PreviewFromConfigWithOptions(cfg, source.PreviewOptions{
		Logger:  cliLoggerFromContext(ctx),
		Verbose: verbose,
	})
	if err != nil {
		return writeCLIError(stdout, stderr, format, "preview-sources", request, cliIssue{
			Code:    "source_error",
			Message: "source preview failed:\n" + err.Error(),
		}, 2)
	}

	return writeCLISuccess(stdout, stderr, format, "preview-sources", request, result, nil)
}
