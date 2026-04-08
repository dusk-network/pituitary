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
	"github.com/dusk-network/pituitary/internal/index"
)

func runAnalyzeImpact(args []string, stdout, stderr io.Writer) int {
	return runAnalyzeImpactContext(context.Background(), args, stdout, stderr)
}

func runAnalyzeImpactContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("analyze-impact", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("analyze-impact", "pituitary [--config PATH] analyze-impact (--path PATH | --spec-ref REF | --request-file PATH|-) [--change-type TYPE] [--summary] [--format FORMAT]")

	var (
		specRef     string
		specPath    string
		requestFile string
		changeType  string
		summary     bool
		format      string
		configPath  string
		atDate      string
	)
	fs.StringVar(&specRef, "spec-ref", "", "indexed spec ref")
	fs.StringVar(&specPath, "path", "", "workspace-relative or absolute path to an indexed spec")
	fs.StringVar(&requestFile, "request-file", "", "path to impact request JSON, or - for stdin")
	fs.StringVar(&changeType, "change-type", "accepted", "change type: accepted, modified, or deprecated")
	fs.BoolVar(&summary, "summary", false, "emit a concise ranked summary in text output and include summary metadata in JSON")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")
	fs.StringVar(&atDate, "at", "", "ISO date for point-in-time governance query (e.g. 2025-03-15)")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "analyze-impact", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "analyze-impact", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	request := analysis.AnalyzeImpactRequest{
		ChangeType: strings.TrimSpace(changeType),
		Summary:    summary,
	}
	if err := validateCLIFormat("analyze-impact", format); err != nil {
		return writeCLIError(stdout, stderr, format, "analyze-impact", request, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	trimmedSpecRef := strings.TrimSpace(specRef)
	trimmedSpecPath := strings.TrimSpace(specPath)
	trimmedRequestFile := strings.TrimSpace(requestFile)
	if nonEmptyCount(trimmedSpecRef, trimmedSpecPath, trimmedRequestFile) > 1 {
		return writeCLIError(stdout, stderr, format, "analyze-impact", request, cliIssue{
			Code:    "validation_error",
			Message: "exactly one of --path, --spec-ref, or --request-file is allowed",
		}, 2)
	}
	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "analyze-impact", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}
	if trimmedSpecPath != "" {
		cfg, err := config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "analyze-impact", request, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
		trimmedSpecRef, err = resolveIndexedSpecRefWithConfigContext(ctx, cfg, trimmedSpecPath)
		if err != nil {
			return writeSpecPathResolutionError(stdout, stderr, format, "analyze-impact", request, err)
		}
	}
	switch {
	case trimmedRequestFile != "":
		cfg, err := config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "analyze-impact", request, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
		request, err = loadWorkspaceScopedJSONFile[analysis.AnalyzeImpactRequest](cfg.Workspace.RootPath, trimmedRequestFile, "request file")
		if err != nil {
			return writeCLIError(stdout, stderr, format, "analyze-impact", request, cliIssue{
				Code:    "validation_error",
				Message: err.Error(),
			}, 2)
		}
	default:
		request.SpecRef = trimmedSpecRef
		if request.SpecRef == "" {
			return writeCLIError(stdout, stderr, format, "analyze-impact", request, cliIssue{
				Code:    "validation_error",
				Message: "one of --path, --spec-ref, or --request-file is required",
			}, 2)
		}
	}

	if trimmedAt := strings.TrimSpace(atDate); trimmedAt != "" {
		request.AtDate = trimmedAt
	}
	operation := app.AnalyzeImpact(ctx, resolvedConfigPath, request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "analyze-impact", operation.Request, cliIssueFromAppIssue(operation.Issue), operation.Issue.ExitCode)
	}

	// Annotate cross-family impact.
	annotateCrossFamilyImpact(ctx, resolvedConfigPath, request.SpecRef, operation.Result)

	return writeCLISuccess(stdout, stderr, format, "analyze-impact", operation.Request, operation.Result, nil)
}

func annotateCrossFamilyImpact(ctx context.Context, configPath string, sourceRef string, result *analysis.AnalyzeImpactResult) {
	if result == nil || len(result.AffectedSpecs) == 0 || sourceRef == "" {
		return
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return
	}
	familyResult, err := index.DiscoverFamiliesContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil || len(familyResult.Assignments) == 0 {
		return
	}

	assignmentMap := make(map[string]int)
	for _, a := range familyResult.Assignments {
		assignmentMap[a.Ref] = a.FamilyID
	}

	sourceFamily, ok := assignmentMap[sourceRef]
	if !ok {
		return
	}

	for i := range result.AffectedSpecs {
		if targetFamily, exists := assignmentMap[result.AffectedSpecs[i].Ref]; exists && targetFamily != sourceFamily {
			result.AffectedSpecs[i].CrossFamily = true
		}
	}
}
