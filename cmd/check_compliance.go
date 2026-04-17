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

type compliancePathList []string

func (l *compliancePathList) String() string {
	return strings.Join(*l, ",")
}

func (l *compliancePathList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func runCheckCompliance(args []string, stdout, stderr io.Writer) int {
	return runCheckComplianceContext(context.Background(), args, stdout, stderr)
}

func runCheckComplianceContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var (
		paths         compliancePathList
		diffFile      string
		atDate        string
		minConfidence string
	)

	return runCommand[analysis.ComplianceRequest, analysis.ComplianceResult](
		ctx, args, stdout, stderr,
		commandRun[analysis.ComplianceRequest, analysis.ComplianceResult]{
			Name:  "check-compliance",
			Usage: "pituitary [--config PATH] check-compliance (--path PATH... | --diff-file PATH|- | --request-file PATH|-) [--format FORMAT] [--timings]",
			Options: commandRunOptions{
				RequestFile:    true,
				Timings:        true,
				ConfigForFile:  true,
				ConfigForFlags: true,
			},
			BindFlags: func(fs *flag.FlagSet) {
				fs.Var(&paths, "path", "workspace-relative or absolute file path; repeat to check multiple files")
				fs.StringVar(&diffFile, "diff-file", "", "path to a unified diff file, or - for stdin")
				fs.StringVar(&atDate, "at", "", "ISO date for point-in-time governance query (e.g. 2025-03-15)")
				fs.StringVar(&minConfidence, "min-confidence", "", "minimum confidence tier: extracted, inferred, or ambiguous")
			},
			InlineFlagsSet: func(_ *flag.FlagSet) bool {
				return len(paths) > 0 || strings.TrimSpace(diffFile) != ""
			},
			LoadRequestFile: func(_ context.Context, cfg *config.Config, trimmedPath string) (*analysis.ComplianceRequest, error) {
				req, err := loadWorkspaceScopedJSONFile[analysis.ComplianceRequest](cfg.Workspace.RootPath, trimmedPath, "request file")
				if err != nil {
					return nil, err
				}
				if req.DiffText == "" && strings.TrimSpace(req.DiffFile) != "" {
					diffText, diffErr := loadComplianceDiffFile(cfg.Workspace.RootPath, req.DiffFile)
					if diffErr != nil {
						return &req, diffErr
					}
					req.DiffText = diffText
				}
				return &req, nil
			},
			BuildRequest: func(_ context.Context, cfg *config.Config, _ string) (analysis.ComplianceRequest, error) {
				return complianceRequestFromFlags(cfg.Workspace.RootPath, []string(paths), strings.TrimSpace(diffFile))
			},
			Normalize: func(_ context.Context, req analysis.ComplianceRequest) (analysis.ComplianceRequest, error) {
				if trimmedAt := strings.TrimSpace(atDate); trimmedAt != "" {
					req.AtDate = trimmedAt
				}
				if trimmedConf := strings.TrimSpace(minConfidence); trimmedConf != "" {
					req.MinConfidence = trimmedConf
				}
				return req, nil
			},
			Execute: func(ctx context.Context, cfgPath string, req analysis.ComplianceRequest) (analysis.ComplianceRequest, *analysis.ComplianceResult, *app.Issue) {
				op := app.CheckCompliance(ctx, cfgPath, req)
				return op.Request, op.Result, op.Issue
			},
		},
	)
}

func complianceRequestFromFlags(workspaceRoot string, paths []string, diffFile string) (analysis.ComplianceRequest, error) {
	trimmedPaths := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			trimmedPaths = append(trimmedPaths, path)
		}
	}
	diffFile = strings.TrimSpace(diffFile)

	switch {
	case len(trimmedPaths) > 0 && diffFile != "":
		return analysis.ComplianceRequest{}, fmt.Errorf("exactly one of --path or --diff-file is allowed")
	case len(trimmedPaths) == 0 && diffFile == "":
		return analysis.ComplianceRequest{}, fmt.Errorf("one of --path or --diff-file is required")
	case len(trimmedPaths) > 0:
		return analysis.ComplianceRequest{Paths: trimmedPaths}, nil
	default:
		diffText, err := loadComplianceDiffFile(workspaceRoot, diffFile)
		if err != nil {
			return analysis.ComplianceRequest{}, err
		}
		return analysis.ComplianceRequest{
			DiffFile: diffFile,
			DiffText: diffText,
		}, nil
	}
}

func loadComplianceDiffFile(workspaceRoot, path string) (string, error) {
	var (
		data []byte
		err  error
	)
	if path == "-" {
		data, err = readBoundedStdin("diff")
		if err != nil {
			return "", err
		}
	} else {
		absPath, err := resolveWorkspaceScopedCLIPath(workspaceRoot, path, "diff file")
		if err != nil {
			return "", err
		}
		data, err = readBoundedRequestFile(absPath, "diff")
		if err != nil {
			return "", err
		}
	}
	if strings.TrimSpace(string(data)) == "" {
		if path == "-" {
			return "", fmt.Errorf("diff from stdin is empty")
		}
		return "", fmt.Errorf("diff file %q is empty", path)
	}
	return string(data), nil
}
