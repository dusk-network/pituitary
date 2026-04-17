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
	var (
		specRef        string
		specPath       string
		specRecordFile string
	)

	return runCommand[analysis.OverlapRequest, analysis.OverlapResult](
		ctx, args, stdout, stderr,
		commandRun[analysis.OverlapRequest, analysis.OverlapResult]{
			Name:  "check-overlap",
			Usage: "pituitary [--config PATH] check-overlap (--path PATH | --spec-ref REF | --spec-record-file PATH|- | --request-file PATH|-) [--format FORMAT]",
			Options: commandRunOptions{
				RequestFile:   true,
				ConfigForFile: true,
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
			LoadRequestFile: autoLoadWorkspaceRequest[analysis.OverlapRequest],
			BuildRequest: func(ctx context.Context, _ *config.Config, resolvedConfigPath string, _ []string) (analysis.OverlapRequest, error) {
				trimmedSpecRef := strings.TrimSpace(specRef)
				trimmedSpecPath := strings.TrimSpace(specPath)
				trimmedRecord := strings.TrimSpace(specRecordFile)
				if nonEmptyCount(trimmedSpecRef, trimmedSpecPath, trimmedRecord) > 1 {
					return analysis.OverlapRequest{}, fmt.Errorf("exactly one of --path, --spec-ref, or --spec-record-file is allowed")
				}
				var (
					cfg           *config.Config
					workspaceRoot string
				)
				if trimmedSpecPath != "" || trimmedRecord != "" {
					loaded, cfgErr := config.Load(resolvedConfigPath)
					if cfgErr != nil {
						return analysis.OverlapRequest{}, configLoadError(cfgErr)
					}
					cfg = loaded
					workspaceRoot = cfg.Workspace.RootPath
				}
				if trimmedSpecPath != "" {
					resolved, err := resolveIndexedSpecRefWithConfigContext(ctx, cfg, trimmedSpecPath)
					if err != nil {
						return analysis.OverlapRequest{}, specPathResolutionError(err)
					}
					trimmedSpecRef = resolved
				}
				return overlapRequestFromFlagsContext(workspaceRoot, trimmedSpecRef, trimmedRecord)
			},
			Execute: func(ctx context.Context, cfgPath string, req analysis.OverlapRequest, _ string) (analysis.OverlapRequest, *analysis.OverlapResult, *app.Issue) {
				op := app.CheckOverlap(ctx, cfgPath, req)
				return op.Request, op.Result, op.Issue
			},
		},
	)
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
		data, err = readBoundedRequestFile(absPath, "spec record")
		if err != nil {
			return model.SpecRecord{}, err
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
