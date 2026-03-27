package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
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
	fs := flag.NewFlagSet("check-compliance", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("check-compliance", "pituitary [--config PATH] check-compliance (--path PATH... | --diff-file PATH|- | --request-file PATH|-) [--format FORMAT]")

	var (
		paths       compliancePathList
		diffFile    string
		requestFile string
		format      string
		configPath  string
	)
	fs.Var(&paths, "path", "workspace-relative or absolute file path; repeat to check multiple files")
	fs.StringVar(&diffFile, "diff-file", "", "path to a unified diff file, or - for stdin")
	fs.StringVar(&requestFile, "request-file", "", "path to compliance request JSON, or - for stdin")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "check-compliance", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "check-compliance", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	if err := validateCLIFormat("check-compliance", format); err != nil {
		return writeCLIError(stdout, stderr, format, "check-compliance", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "check-compliance", nil, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}
	trimmedRequestFile := strings.TrimSpace(requestFile)

	var request analysis.ComplianceRequest
	switch {
	case trimmedRequestFile != "" && (len(paths) > 0 || strings.TrimSpace(diffFile) != ""):
		return writeCLIError(stdout, stderr, format, "check-compliance", nil, cliIssue{
			Code:    "validation_error",
			Message: "use either --request-file or the path/diff-file flags",
		}, 2)
	case trimmedRequestFile != "":
		cfg, err := config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-compliance", nil, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
		request, err = loadWorkspaceScopedJSONFile[analysis.ComplianceRequest](cfg.Workspace.RootPath, trimmedRequestFile, "request file")
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-compliance", nil, cliIssue{
				Code:    "validation_error",
				Message: err.Error(),
			}, 2)
		}
		if request.DiffText == "" && strings.TrimSpace(request.DiffFile) != "" {
			request.DiffText, err = loadComplianceDiffFile(cfg.Workspace.RootPath, request.DiffFile)
			if err != nil {
				return writeCLIError(stdout, stderr, format, "check-compliance", request, cliIssue{
					Code:    "validation_error",
					Message: err.Error(),
				}, 2)
			}
		}
	default:
		cfg, err := config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-compliance", nil, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
		request, err = complianceRequestFromFlags(cfg.Workspace.RootPath, []string(paths), strings.TrimSpace(diffFile))
		if err != nil {
			return writeCLIError(stdout, stderr, format, "check-compliance", nil, cliIssue{
				Code:    "validation_error",
				Message: err.Error(),
			}, 2)
		}
	}

	operation := app.CheckCompliance(ctx, resolvedConfigPath, request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "check-compliance", operation.Request, cliIssue{
			Code:    operation.Issue.Code,
			Message: operation.Issue.Message,
		}, operation.Issue.ExitCode)
	}

	return writeCLISuccess(stdout, stderr, format, "check-compliance", operation.Request, operation.Result, nil)
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
		data, err = io.ReadAll(cliStdin)
		if err != nil {
			return "", fmt.Errorf("read diff from stdin: %w", err)
		}
	} else {
		absPath, err := resolveWorkspaceScopedCLIPath(workspaceRoot, path, "diff file")
		if err != nil {
			return "", err
		}
		// #nosec G304 -- absPath is validated to remain under the configured workspace root.
		data, err = os.ReadFile(absPath)
		if err != nil {
			return "", fmt.Errorf("read diff file %q: %w", path, err)
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
