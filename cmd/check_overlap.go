package cmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
)

var cliStdin io.Reader = os.Stdin

func runCheckOverlap(args []string, stdout, stderr io.Writer) int {
	return runCheckOverlapContext(context.Background(), args, stdout, stderr)
}

func runCheckOverlapContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check-overlap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("check-overlap", "pituitary [--config PATH] check-overlap (--path PATH | --spec-ref REF | --spec-record-file PATH|- | --request-file PATH|-) [--format FORMAT]")

	var (
		specRef        string
		specPath       string
		specRecordFile string
		requestFile    string
		format         string
		configPath     string
	)
	fs.StringVar(&specRef, "spec-ref", "", "indexed spec ref")
	fs.StringVar(&specPath, "path", "", "workspace-relative or absolute path to an indexed spec")
	fs.StringVar(&specRecordFile, "spec-record-file", "", "path to canonical spec_record JSON, or - for stdin")
	fs.StringVar(&requestFile, "request-file", "", "path to overlap request JSON, or - for stdin")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "check-overlap", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "check-overlap", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	if err := validateCLIFormat("check-overlap", format); err != nil {
		return writeCLIError(stdout, stderr, format, "check-overlap", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "check-overlap", nil, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}
	trimmedSpecRef := strings.TrimSpace(specRef)
	trimmedSpecPath := strings.TrimSpace(specPath)
	trimmedSpecRecordFile := strings.TrimSpace(specRecordFile)
	trimmedRequestFile := strings.TrimSpace(requestFile)
	if nonEmptyCount(trimmedSpecRef, trimmedSpecPath, trimmedSpecRecordFile, trimmedRequestFile) > 1 {
		return writeCLIError(stdout, stderr, format, "check-overlap", nil, cliIssue{
			Code:    "validation_error",
			Message: "exactly one of --path, --spec-ref, --spec-record-file, or --request-file is allowed",
		}, 2)
	}
	var cfg *config.Config
	if trimmedSpecPath != "" || trimmedSpecRecordFile != "" || trimmedRequestFile != "" {
		cfg, err = config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-overlap", nil, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
	}
	if trimmedRequestFile != "" {
		request, err := loadWorkspaceScopedJSONFile[analysis.OverlapRequest](cfg.Workspace.RootPath, trimmedRequestFile, "request file")
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-overlap", nil, cliIssue{
				Code:    "validation_error",
				Message: err.Error(),
			}, 2)
		}
		operation := app.CheckOverlap(ctx, resolvedConfigPath, request)
		if operation.Issue != nil {
			return writeCLIError(stdout, stderr, format, "check-overlap", operation.Request, cliIssueFromAppIssue(operation.Issue), operation.Issue.ExitCode)
		}
		return writeCLISuccess(stdout, stderr, format, "check-overlap", operation.Request, operation.Result, nil)
	}
	if trimmedSpecPath != "" {
		trimmedSpecRef, err = resolveIndexedSpecRefWithConfigContext(ctx, cfg, trimmedSpecPath)
		if err != nil {
			return writeSpecPathResolutionError(stdout, stderr, format, "check-overlap", nil, err)
		}
	}
	workspaceRoot := ""
	if cfg != nil {
		workspaceRoot = cfg.Workspace.RootPath
	}
	request, err := overlapRequestFromFlagsContext(workspaceRoot, trimmedSpecRef, trimmedSpecRecordFile)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "check-overlap", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	operation := app.CheckOverlap(ctx, resolvedConfigPath, request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "check-overlap", operation.Request, cliIssueFromAppIssue(operation.Issue), operation.Issue.ExitCode)
	}

	return writeCLISuccess(stdout, stderr, format, "check-overlap", operation.Request, operation.Result, nil)
}

func overlapRequestFromFlagsContext(workspaceRoot, specRef, specRecordFile string) (analysis.OverlapRequest, error) {
	request := analysis.OverlapRequest{SpecRef: specRef}
	switch {
	case specRef != "" && specRecordFile != "":
		return request, fmt.Errorf("exactly one of --spec-ref or --spec-record-file is allowed")
	case specRef == "" && specRecordFile == "":
		return request, fmt.Errorf("one of --spec-ref or --spec-record-file is required")
	case specRef != "":
		return request, nil
	default:
		record, err := loadSpecRecordFile(workspaceRoot, specRecordFile)
		if err != nil {
			return request, err
		}
		request.SpecRecord = &record
		return request, nil
	}
}

func loadSpecRecordFile(workspaceRoot, path string) (model.SpecRecord, error) {
	var (
		data []byte
		err  error
	)
	if path == "-" {
		data, err = readBoundedStdin("spec record")
		if err != nil {
			return model.SpecRecord{}, err
		}
	} else {
		absPath, err := resolveWorkspaceScopedCLIPath(workspaceRoot, path, "spec record file")
		if err != nil {
			return model.SpecRecord{}, err
		}
		// #nosec G304 -- absPath is validated to remain under the configured workspace root.
		data, err = os.ReadFile(absPath)
		if err != nil {
			return model.SpecRecord{}, fmt.Errorf("read spec record file %q: %w", path, err)
		}
	}
	var record model.SpecRecord
	if err := json.Unmarshal(data, &record); err != nil {
		if path == "-" {
			return model.SpecRecord{}, fmt.Errorf("parse spec record from stdin: %w", err)
		}
		return model.SpecRecord{}, fmt.Errorf("parse spec record file %q: %w", path, err)
	}
	return record, nil
}
