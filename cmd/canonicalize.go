package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/source"
)

type canonicalizeRequest struct {
	Path      string `json:"path"`
	BundleDir string `json:"bundle_dir,omitempty"`
	Write     bool   `json:"write,omitempty"`
}

func runCanonicalize(args []string, stdout, stderr io.Writer) int {
	return runCanonicalizeContext(context.Background(), args, stdout, stderr)
}

func runCanonicalizeContext(_ context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("canonicalize", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newStandaloneCommandHelp("canonicalize", "pituitary canonicalize --path PATH [--bundle-dir PATH] [--write] [--format FORMAT]")

	var (
		path      string
		bundleDir string
		write     bool
		format    string
	)
	fs.StringVar(&path, "path", "", "workspace-relative or absolute path to a markdown contract")
	fs.StringVar(&bundleDir, "bundle-dir", "", "bundle directory to preview or write")
	fs.BoolVar(&write, "write", false, "write the generated bundle")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "canonicalize", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "canonicalize", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}
	if err := validateCLIFormat("canonicalize", format); err != nil {
		return writeCLIError(stdout, stderr, format, "canonicalize", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	if strings.TrimSpace(path) == "" {
		return writeCLIError(stdout, stderr, format, "canonicalize", nil, cliIssue{
			Code:    "validation_error",
			Message: "--path is required",
		}, 2)
	}

	request := canonicalizeRequest{
		Path:      path,
		BundleDir: strings.TrimSpace(bundleDir),
		Write:     write,
	}
	result, err := source.CanonicalizeMarkdownContract(source.CanonicalizeOptions{
		Path:      request.Path,
		BundleDir: request.BundleDir,
		Write:     request.Write,
	})
	if err != nil {
		return writeCLIError(stdout, stderr, format, "canonicalize", request, cliIssue{
			Code:    "canonicalize_error",
			Message: err.Error(),
		}, 2)
	}

	return writeCLISuccess(stdout, stderr, format, "canonicalize", request, result, nil)
}
