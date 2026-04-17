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
	var (
		specRef    string
		specPath   string
		changeType string
		summary    bool
		atDate     string
	)

	return runCommand[analysis.AnalyzeImpactRequest, analysis.AnalyzeImpactResult](
		ctx, args, stdout, stderr,
		commandRun[analysis.AnalyzeImpactRequest, analysis.AnalyzeImpactResult]{
			Name:  "analyze-impact",
			Usage: "pituitary [--config PATH] analyze-impact (--path PATH | --spec-ref REF | --request-file PATH|-) [--change-type TYPE] [--summary] [--format FORMAT]",
			Options: commandRunOptions{
				RequestFile:    true,
				ConfigForFile:  true,
				ConfigForFlags: true,
			},
			BindFlags: func(fs *flag.FlagSet) {
				fs.StringVar(&specRef, "spec-ref", "", "indexed spec ref")
				fs.StringVar(&specPath, "path", "", "workspace-relative or absolute path to an indexed spec")
				fs.StringVar(&changeType, "change-type", "accepted", "change type: accepted, modified, or deprecated")
				fs.BoolVar(&summary, "summary", false, "emit a concise ranked summary in text output and include summary metadata in JSON")
				fs.StringVar(&atDate, "at", "", "ISO date for point-in-time governance query (e.g. 2025-03-15)")
			},
			InlineFlagsSet: func(_ *flag.FlagSet) bool {
				return strings.TrimSpace(specRef) != "" || strings.TrimSpace(specPath) != ""
			},
			LoadRequestFile: autoLoadWorkspaceRequest[analysis.AnalyzeImpactRequest](),
			BuildRequest: func(ctx context.Context, cfg *config.Config, _ string, _ []string) (analysis.AnalyzeImpactRequest, error) {
				trimmedSpecRef := strings.TrimSpace(specRef)
				trimmedSpecPath := strings.TrimSpace(specPath)
				req := analysis.AnalyzeImpactRequest{
					ChangeType: strings.TrimSpace(changeType),
					Summary:    summary,
					SpecRef:    trimmedSpecRef,
				}
				if trimmedSpecRef != "" && trimmedSpecPath != "" {
					return req, fmt.Errorf("exactly one of --path or --spec-ref is allowed")
				}
				if trimmedSpecPath != "" {
					resolved, err := resolveIndexedSpecRefWithConfigContext(ctx, cfg, trimmedSpecPath)
					if err != nil {
						return req, specPathResolutionError(err)
					}
					req.SpecRef = resolved
				}
				if req.SpecRef == "" {
					return req, fmt.Errorf("one of --path, --spec-ref, or --request-file is required")
				}
				return req, nil
			},
			Normalize: func(_ context.Context, req analysis.AnalyzeImpactRequest, _ string) (analysis.AnalyzeImpactRequest, error) {
				if trimmedAt := strings.TrimSpace(atDate); trimmedAt != "" {
					req.AtDate = trimmedAt
				}
				return req, nil
			},
			Execute: func(ctx context.Context, cfgPath string, req analysis.AnalyzeImpactRequest, _ string) (analysis.AnalyzeImpactRequest, *analysis.AnalyzeImpactResult, *app.Issue) {
				op := app.AnalyzeImpact(ctx, cfgPath, req)
				return op.Request, op.Result, op.Issue
			},
			PostProcess: func(ctx context.Context, cfgPath string, req analysis.AnalyzeImpactRequest, res *analysis.AnalyzeImpactResult) (*analysis.AnalyzeImpactResult, *cliIssue, int) {
				annotateCrossFamilyImpact(ctx, cfgPath, req.SpecRef, res)
				return res, nil, 0
			},
		},
	)
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
