package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/app"
)

func runReviewSpec(args []string, stdout, stderr io.Writer) int {
	return runReviewSpecContext(context.Background(), args, stdout, stderr)
}

func runReviewSpecContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("review-spec", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var (
		specRef        string
		specRecordFile string
		format         string
	)
	fs.StringVar(&specRef, "spec-ref", "", "indexed spec ref")
	fs.StringVar(&specRecordFile, "spec-record-file", "", "path to canonical spec_record JSON")
	fs.StringVar(&format, "format", "text", "output format")

	if err := fs.Parse(args); err != nil {
		return writeCLIError(stdout, stderr, format, "review-spec", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "review-spec", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	request, err := reviewRequestFromFlags(strings.TrimSpace(specRef), strings.TrimSpace(specRecordFile))
	if err != nil {
		return writeCLIError(stdout, stderr, format, "review-spec", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	if !isSupportedFormat(format) {
		return writeCLIError(stdout, stderr, format, "review-spec", request, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unsupported format %q", format),
		}, 2)
	}

	operation := app.ReviewSpec(ctx, "pituitary.toml", request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "review-spec", operation.Request, cliIssue{
			Code:    operation.Issue.Code,
			Message: operation.Issue.Message,
		}, operation.Issue.ExitCode)
	}

	return writeCLISuccess(stdout, stderr, format, "review-spec", operation.Request, operation.Result, nil)
}

func reviewRequestFromFlags(specRef, specRecordFile string) (analysis.ReviewRequest, error) {
	request := analysis.ReviewRequest{SpecRef: specRef}
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
