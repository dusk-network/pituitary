package cmd

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
)

type searchSpecsFlagList []string

func (l *searchSpecsFlagList) String() string {
	return strings.Join(*l, ",")
}

func (l *searchSpecsFlagList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func runSearchSpecs(args []string, stdout, stderr io.Writer) int {
	return runSearchSpecsContext(context.Background(), args, stdout, stderr)
}

func runSearchSpecsContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var (
		query    string
		domain   string
		statuses searchSpecsFlagList
		limit    int
		familyID int
	)

	return runCommand[index.SearchSpecRequest, index.SearchSpecResult](
		ctx, args, stdout, stderr,
		commandRun[index.SearchSpecRequest, index.SearchSpecResult]{
			Name:  "search-specs",
			Usage: "pituitary [--config PATH] search-specs (--query TEXT | --request-file PATH|-) [--domain VALUE] [--status VALUE]... [--limit N] [--format FORMAT]",
			Options: commandRunOptions{
				RequestFile:   true,
				ConfigForFile: true,
			},
			BindFlags: func(fs *flag.FlagSet) {
				fs.StringVar(&query, "query", "", "semantic query")
				fs.StringVar(&domain, "domain", "", "filter by domain")
				fs.Var(&statuses, "status", "filter by status; repeat to set multiple statuses")
				fs.IntVar(&limit, "limit", 10, "maximum matches to return")
				fs.IntVar(&familyID, "family", -1, "filter results to specs in the given family ID")
			},
			InlineFlagsSet: func(fs *flag.FlagSet) bool {
				return strings.TrimSpace(query) != "" || strings.TrimSpace(domain) != "" || len(statuses) > 0 || flagWasSet(fs, "limit")
			},
			LoadRequestFile: autoLoadWorkspaceRequest[index.SearchSpecRequest],
			BuildRequest: func(_ context.Context, _ *config.Config, _ string, _ []string) (index.SearchSpecRequest, error) {
				return index.SearchSpecRequest{
					Query: strings.TrimSpace(query),
					Filters: index.SearchSpecFilters{
						Domain:   strings.TrimSpace(domain),
						Statuses: []string(statuses),
					},
					Limit: &limit,
				}, nil
			},
			Normalize: func(_ context.Context, req index.SearchSpecRequest, _ string) (index.SearchSpecRequest, error) {
				queryArgs, err := req.ToQuery()
				if err != nil {
					return req, err
				}
				req.Filters.Statuses = queryArgs.Statuses
				req.Query = queryArgs.Query
				req.Filters.Domain = queryArgs.Domain
				requestLimit := queryArgs.Limit
				req.Limit = &requestLimit
				if req.Query == "" {
					return req, errors.New("query is required")
				}
				return req, nil
			},
			Execute: func(ctx context.Context, cfgPath string, req index.SearchSpecRequest, _ string) (index.SearchSpecRequest, *index.SearchSpecResult, *app.Issue) {
				op := app.SearchSpecs(ctx, cfgPath, req)
				return op.Request, op.Result, op.Issue
			},
			PostProcess: func(ctx context.Context, cfgPath string, _ index.SearchSpecRequest, res *index.SearchSpecResult) (*index.SearchSpecResult, *cliIssue, int) {
				if familyID < 0 || res == nil {
					return res, nil, 0
				}
				cfg, cfgErr := config.Load(cfgPath)
				if cfgErr != nil {
					return res, &cliIssue{
						Code:    "config_error",
						Message: fmt.Sprintf("failed to load config for --family filtering: %v", cfgErr),
					}, 2
				}
				familyResult, famErr := index.DiscoverFamiliesContext(ctx, cfg.Workspace.ResolvedIndexPath)
				if famErr != nil {
					return res, &cliIssue{
						Code:    "internal_error",
						Message: fmt.Sprintf("failed to compute families for --family filtering: %v", famErr),
					}, 2
				}
				familyMembers := make(map[string]bool)
				for _, a := range familyResult.Assignments {
					if a.FamilyID == familyID {
						familyMembers[a.Ref] = true
					}
				}
				var filtered []index.SearchSpecMatch
				for _, m := range res.Matches {
					if familyMembers[m.Ref] {
						filtered = append(filtered, m)
					}
				}
				res.Matches = filtered
				return res, nil, 0
			},
		},
	)
}
