package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"runtime"
	"strings"
)

// Version is the Pituitary build version. It defaults to "dev" and can be
// overridden at build time with -ldflags.
var Version = "dev"

// Commit is optional build metadata that can be set at build time with -ldflags.
var Commit string

// BuildDate is optional build metadata that can be set at build time with -ldflags.
var BuildDate string

type versionRequest struct{}

type versionResult struct {
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
}

func runVersion(args []string, stdout, stderr io.Writer) int {
	return runVersionContext(context.Background(), args, stdout, stderr)
}

func runVersionContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	_ = ctx

	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newStandaloneCommandHelp("version", "pituitary version [--format FORMAT]")

	var format string
	fs.StringVar(&format, "format", "text", "output format")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "version", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "version", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}
	if !isSupportedFormat(format) {
		return writeCLIError(stdout, stderr, format, "version", versionRequest{}, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unsupported format %q", format),
		}, 2)
	}

	return writeCLISuccess(stdout, stderr, format, "version", versionRequest{}, &versionResult{
		Version:   Version,
		GoVersion: runtime.Version(),
		Commit:    Commit,
		BuildDate: BuildDate,
	}, nil)
}
