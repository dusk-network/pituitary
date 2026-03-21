package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
)

type statusRequest struct{}

type statusResult struct {
	ConfigPath  string `json:"config_path"`
	IndexPath   string `json:"index_path"`
	IndexExists bool   `json:"index_exists"`
	SpecCount   int    `json:"spec_count"`
	DocCount    int    `json:"doc_count"`
	ChunkCount  int    `json:"chunk_count"`
}

func runStatus(args []string, stdout, stderr io.Writer) int {
	return runStatusContext(context.Background(), args, stdout, stderr)
}

func runStatusContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("status", "pituitary [--config PATH] status [--format FORMAT]")

	var (
		format     string
		configPath string
	)
	fs.StringVar(&format, "format", "text", "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "status", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "status", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}
	if err := validateCLIFormat("status", format); err != nil {
		return writeCLIError(stdout, stderr, format, "status", statusRequest{}, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	request := statusRequest{}

	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "status", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "status", request, cliIssue{
			Code:    "config_error",
			Message: "invalid config:\n" + err.Error(),
		}, 2)
	}

	status, err := index.ReadStatusContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "status", request, cliIssue{
			Code:    "index_error",
			Message: "inspect index failed:\n" + err.Error(),
		}, 2)
	}

	return writeCLISuccess(stdout, stderr, format, "status", request, &statusResult{
		ConfigPath:  cfg.ConfigPath,
		IndexPath:   status.IndexPath,
		IndexExists: status.Exists,
		SpecCount:   status.SpecCount,
		DocCount:    status.DocCount,
		ChunkCount:  status.ChunkCount,
	}, nil)
}
