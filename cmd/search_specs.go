package cmd

import (
	"context"
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
	fs := flag.NewFlagSet("search-specs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("search-specs", "pituitary [--config PATH] search-specs (--query TEXT | --request-file PATH|-) [--domain VALUE] [--status VALUE]... [--limit N] [--format FORMAT]")

	var (
		query       string
		requestFile string
		format      string
		domain      string
		configPath  string
		statuses    searchSpecsFlagList
		limit       int
		familyID    int
	)
	fs.StringVar(&query, "query", "", "semantic query")
	fs.StringVar(&requestFile, "request-file", "", "path to search request JSON, or - for stdin")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format (text, json, table)")
	fs.StringVar(&configPath, "config", "", "path to workspace config")
	fs.StringVar(&domain, "domain", "", "filter by domain")
	fs.Var(&statuses, "status", "filter by status; repeat to set multiple statuses")
	fs.IntVar(&limit, "limit", 10, "maximum matches to return")
	fs.IntVar(&familyID, "family", -1, "filter results to specs in the given family ID")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "search-specs", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "search-specs", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	if err := validateCLIFormat("search-specs", format); err != nil {
		return writeCLIError(stdout, stderr, format, "search-specs", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "search-specs", nil, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	trimmedRequestFile := strings.TrimSpace(requestFile)
	var request index.SearchSpecRequest
	switch {
	case trimmedRequestFile != "" && (strings.TrimSpace(query) != "" || strings.TrimSpace(domain) != "" || len(statuses) > 0 || flagWasSet(fs, "limit")):
		return writeCLIError(stdout, stderr, format, "search-specs", nil, cliIssue{
			Code:    "validation_error",
			Message: "use either --request-file or fine-grained search flags",
		}, 2)
	case trimmedRequestFile != "":
		cfg, err := config.Load(resolvedConfigPath)
		if err != nil {
			return writeCLIError(stdout, stderr, format, "search-specs", nil, cliIssue{
				Code:    "config_error",
				Message: err.Error(),
			}, 2)
		}
		request, err = loadWorkspaceScopedJSONFile[index.SearchSpecRequest](cfg.Workspace.RootPath, trimmedRequestFile, "request file")
		if err != nil {
			return writeCLIError(stdout, stderr, format, "search-specs", nil, cliIssue{
				Code:    "validation_error",
				Message: err.Error(),
			}, 2)
		}
	default:
		request = index.SearchSpecRequest{
			Query: strings.TrimSpace(query),
			Filters: index.SearchSpecFilters{
				Domain:   strings.TrimSpace(domain),
				Statuses: []string(statuses),
			},
			Limit: &limit,
		}
	}
	queryArgs, err := request.ToQuery()
	if err != nil {
		return writeCLIError(stdout, stderr, format, "search-specs", request, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	request.Filters.Statuses = queryArgs.Statuses
	request.Query = queryArgs.Query
	request.Filters.Domain = queryArgs.Domain
	requestLimit := queryArgs.Limit
	request.Limit = &requestLimit
	if request.Query == "" {
		return writeCLIError(stdout, stderr, format, "search-specs", request, cliIssue{
			Code:    "validation_error",
			Message: "query is required",
		}, 2)
	}

	operation := app.SearchSpecs(ctx, resolvedConfigPath, request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "search-specs", operation.Request, cliIssueFromAppIssue(operation.Issue), operation.Issue.ExitCode)
	}

	// Post-filter by family if --family was specified.
	if familyID >= 0 && operation.Result != nil {
		cfg, cfgErr := config.Load(resolvedConfigPath)
		if cfgErr != nil {
			return writeCLIError(stdout, stderr, format, "search-specs", operation.Request, cliIssue{
				Code:    "config_error",
				Message: fmt.Sprintf("failed to load config for --family filtering: %v", cfgErr),
			}, 2)
		}
		familyResult, famErr := index.DiscoverFamiliesContext(ctx, cfg.Workspace.ResolvedIndexPath)
		if famErr != nil {
			return writeCLIError(stdout, stderr, format, "search-specs", operation.Request, cliIssue{
				Code:    "internal_error",
				Message: fmt.Sprintf("failed to compute families for --family filtering: %v", famErr),
			}, 2)
		}
		familyMembers := make(map[string]bool)
		for _, a := range familyResult.Assignments {
			if a.FamilyID == familyID {
				familyMembers[a.Ref] = true
			}
		}
		var filtered []index.SearchSpecMatch
		for _, m := range operation.Result.Matches {
			if familyMembers[m.Ref] {
				filtered = append(filtered, m)
			}
		}
		operation.Result.Matches = filtered
	}

	return writeCLISuccess(stdout, stderr, format, "search-specs", operation.Request, operation.Result, nil)
}
