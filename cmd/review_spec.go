package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/config"
)

func runReviewSpec(args []string, stdout, stderr io.Writer) int {
	return runReviewSpecContext(context.Background(), args, stdout, stderr)
}

func runReviewSpecContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var (
		specRef        string
		specPath       string
		specRecordFile string
	)

	return runCommand[analysis.ReviewRequest, analysis.ReviewResult](
		ctx, args, stdout, stderr,
		commandRun[analysis.ReviewRequest, analysis.ReviewResult]{
			Name:  "review-spec",
			Usage: "pituitary [--config PATH] review-spec (--path PATH | --spec-ref REF | --spec-record-file PATH|- | --request-file PATH|-) [--format FORMAT]",
			Options: commandRunOptions{
				RequestFile:    true,
				ConfigForFile:  true,
				ConfigForFlags: true,
			},
			BindFlags: func(fs *flag.FlagSet) {
				fs.StringVar(&specRef, "spec-ref", "", "indexed spec ref")
				fs.StringVar(&specPath, "path", "", "workspace-relative or absolute path to an indexed spec")
				fs.StringVar(&specRecordFile, "spec-record-file", "", "path to canonical spec_record JSON, or - for stdin")
			},
			InlineFlagsSet: func(_ *flag.FlagSet) bool {
				return strings.TrimSpace(specRef) != "" ||
					strings.TrimSpace(specPath) != "" ||
					strings.TrimSpace(specRecordFile) != ""
			},
			LoadRequestFile: func(_ context.Context, cfg *config.Config, trimmedPath string) (*analysis.ReviewRequest, error) {
				req, err := loadWorkspaceScopedJSONFile[analysis.ReviewRequest](cfg.Workspace.RootPath, trimmedPath, "request file")
				if err != nil {
					return nil, err
				}
				return &req, nil
			},
			BuildRequest: func(ctx context.Context, cfg *config.Config, _ string) (analysis.ReviewRequest, error) {
				trimmedSpecRef := strings.TrimSpace(specRef)
				trimmedSpecPath := strings.TrimSpace(specPath)
				trimmedRecord := strings.TrimSpace(specRecordFile)
				if nonEmptyCount(trimmedSpecRef, trimmedSpecPath, trimmedRecord) > 1 {
					return analysis.ReviewRequest{}, fmt.Errorf("exactly one of --path, --spec-ref, --spec-record-file, or --request-file is allowed")
				}
				if trimmedSpecPath != "" {
					resolved, err := resolveIndexedSpecRefWithConfigContext(ctx, cfg, trimmedSpecPath)
					if err != nil {
						return analysis.ReviewRequest{}, specPathResolutionError(err)
					}
					trimmedSpecRef = resolved
				}
				return reviewRequestFromFlags(cfg.Workspace.RootPath, trimmedSpecRef, trimmedRecord)
			},
			Execute: func(ctx context.Context, cfgPath string, req analysis.ReviewRequest) (analysis.ReviewRequest, *analysis.ReviewResult, *app.Issue) {
				op := app.ReviewSpec(ctx, cfgPath, req)
				return op.Request, op.Result, op.Issue
			},
		},
	)
}

func reviewRequestFromFlags(workspaceRoot, specRef, specRecordFile string) (analysis.ReviewRequest, error) {
	request := analysis.ReviewRequest{SpecRef: specRef}
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
