package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/source"
)

type discoverRequest struct {
	Path  string `json:"path"`
	Write bool   `json:"write,omitempty"`
}

func runDiscover(args []string, stdout, stderr io.Writer) int {
	return runDiscoverContext(context.Background(), args, stdout, stderr)
}

func runDiscoverContext(_ context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newStandaloneCommandHelp("discover", "pituitary discover [--path PATH] [--write] [--format FORMAT]")

	var (
		path   string
		write  bool
		format string
	)
	fs.StringVar(&path, "path", ".", "workspace path to scan")
	fs.BoolVar(&write, "write", false, "write .pituitary/pituitary.toml")
	fs.StringVar(&format, "format", "text", "output format")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "discover", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "discover", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}
	if err := validateCLIFormat("discover", format); err != nil {
		return writeCLIError(stdout, stderr, format, "discover", discoverRequest{Path: path, Write: write}, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	request := discoverRequest{
		Path:  path,
		Write: write,
	}
	result, err := source.DiscoverWorkspace(source.DiscoverOptions{
		RootPath: path,
		Write:    write,
	})
	if err != nil {
		return writeCLIError(stdout, stderr, format, "discover", request, cliIssue{
			Code:    "discovery_error",
			Message: err.Error(),
		}, 2)
	}

	return writeCLISuccess(stdout, stderr, format, "discover", request, result, nil)
}
