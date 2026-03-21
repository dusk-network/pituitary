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
	"github.com/dusk-network/pituitary/internal/model"
)

func runCheckOverlap(args []string, stdout, stderr io.Writer) int {
	return runCheckOverlapContext(context.Background(), args, stdout, stderr)
}

func runCheckOverlapContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check-overlap", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		specRef        string
		specRecordFile string
		format         string
		configPath     string
	)
	fs.StringVar(&specRef, "spec-ref", "", "indexed spec ref")
	fs.StringVar(&specRecordFile, "spec-record-file", "", "path to canonical spec_record JSON")
	fs.StringVar(&format, "format", "text", "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if err := fs.Parse(args); err != nil {
		return writeCLIError(stdout, stderr, format, "check-overlap", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "check-overlap", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	request, err := overlapRequestFromFlags(strings.TrimSpace(specRef), strings.TrimSpace(specRecordFile))
	if err != nil {
		return writeCLIError(stdout, stderr, format, "check-overlap", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	if !isSupportedFormat(format) {
		return writeCLIError(stdout, stderr, format, "check-overlap", request, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unsupported format %q", format),
		}, 2)
	}
	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "check-overlap", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	operation := app.CheckOverlap(ctx, resolvedConfigPath, request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "check-overlap", operation.Request, cliIssue{
			Code:    operation.Issue.Code,
			Message: operation.Issue.Message,
		}, operation.Issue.ExitCode)
	}

	return writeCLISuccess(stdout, stderr, format, "check-overlap", operation.Request, operation.Result, nil)
}

func overlapRequestFromFlags(specRef, specRecordFile string) (analysis.OverlapRequest, error) {
	request := analysis.OverlapRequest{SpecRef: specRef}
	switch {
	case specRef != "" && specRecordFile != "":
		return request, fmt.Errorf("exactly one of --spec-ref or --spec-record-file is allowed")
	case specRef == "" && specRecordFile == "":
		return request, fmt.Errorf("one of --spec-ref or --spec-record-file is required")
	case specRef != "":
		return request, nil
	default:
		record, err := loadSpecRecordFile(specRecordFile)
		if err != nil {
			return request, err
		}
		request.SpecRecord = &record
		return request, nil
	}
}

func loadSpecRecordFile(path string) (model.SpecRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.SpecRecord{}, fmt.Errorf("read spec record file %q: %w", path, err)
	}
	var record model.SpecRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return model.SpecRecord{}, fmt.Errorf("parse spec record file %q: %w", path, err)
	}
	return record, nil
}
