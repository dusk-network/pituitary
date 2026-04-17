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

type stringList []string

func (l *stringList) String() string {
	return strings.Join(*l, ",")
}

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func runCheckTerminology(args []string, stdout, stderr io.Writer) int {
	return runCheckTerminologyContext(context.Background(), args, stdout, stderr)
}

func runCheckTerminologyContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var (
		terms          stringList
		canonicalTerms stringList
		specRef        string
		specPath       string
		scope          string
	)

	return runCommand[analysis.TerminologyAuditRequest, analysis.TerminologyAuditResult](
		ctx, args, stdout, stderr,
		commandRun[analysis.TerminologyAuditRequest, analysis.TerminologyAuditResult]{
			Name:  "check-terminology",
			Usage: "pituitary [--config PATH] check-terminology ([--term TERM]... [--canonical-term TERM]... [--spec-ref REF | --path PATH] [--scope SCOPE] | --request-file PATH|-) [--format FORMAT] [--timings]",
			Options: commandRunOptions{
				RequestFile:    true,
				Timings:        true,
				ConfigForFile:  true,
				ConfigForFlags: true,
			},
			BindFlags: func(fs *flag.FlagSet) {
				fs.Var(&terms, "term", "displaced or governed term to audit; repeat to narrow a configured policy set or supply ad hoc terms")
				fs.Var(&canonicalTerms, "canonical-term", "replacement or canonical term; repeat to supply multiple terms")
				fs.StringVar(&specRef, "spec-ref", "", "indexed spec ref used to anchor the audit")
				fs.StringVar(&specPath, "path", "", "workspace-relative or absolute path to an indexed spec used to anchor the audit")
				fs.StringVar(&scope, "scope", "all", "artifact scope: all, docs, or specs")
			},
			InlineFlagsSet: func(fs *flag.FlagSet) bool {
				return countNonEmptyStrings([]string(terms)) > 0 ||
					countNonEmptyStrings([]string(canonicalTerms)) > 0 ||
					strings.TrimSpace(specRef) != "" ||
					strings.TrimSpace(specPath) != "" ||
					flagWasSet(fs, "scope")
			},
			LoadRequestFile: autoLoadWorkspaceRequest[analysis.TerminologyAuditRequest](),
			BuildRequest: func(ctx context.Context, cfg *config.Config, _ string, _ []string) (analysis.TerminologyAuditRequest, error) {
				req := analysis.TerminologyAuditRequest{
					Terms:          []string(terms),
					CanonicalTerms: []string(canonicalTerms),
					SpecRef:        strings.TrimSpace(specRef),
					Scope:          strings.TrimSpace(scope),
				}
				trimmedPath := strings.TrimSpace(specPath)
				if req.SpecRef != "" && trimmedPath != "" {
					return req, fmt.Errorf("exactly one of --spec-ref or --path is allowed")
				}
				if trimmedPath != "" {
					resolved, err := resolveIndexedSpecRefWithConfigContext(ctx, cfg, trimmedPath)
					if err != nil {
						return req, specPathResolutionError(err)
					}
					req.SpecRef = resolved
				}
				return req, nil
			},
			Execute: func(ctx context.Context, cfgPath string, req analysis.TerminologyAuditRequest, _ string) (analysis.TerminologyAuditRequest, *analysis.TerminologyAuditResult, *app.Issue) {
				op := app.CheckTerminology(ctx, cfgPath, req)
				return op.Request, op.Result, op.Issue
			},
		},
	)
}

func countNonEmptyStrings(values []string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}
