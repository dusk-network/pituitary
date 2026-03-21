package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/dusk-network/pituitary/internal/app"
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

	var (
		query      string
		format     string
		domain     string
		configPath string
		statuses   searchSpecsFlagList
		limit      int
	)
	fs.StringVar(&query, "query", "", "semantic query")
	fs.StringVar(&format, "format", "text", "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")
	fs.StringVar(&domain, "domain", "", "filter by domain")
	fs.Var(&statuses, "status", "filter by status; repeat to set multiple statuses")
	fs.IntVar(&limit, "limit", 10, "maximum matches to return")

	if err := fs.Parse(args); err != nil {
		return writeCLIError(stdout, stderr, format, "search-specs", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "search-specs", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}

	request := index.SearchSpecRequest{
		Query: strings.TrimSpace(query),
		Filters: index.SearchSpecFilters{
			Domain:   strings.TrimSpace(domain),
			Statuses: []string(statuses),
		},
		Limit: &limit,
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
			Message: "--query is required",
		}, 2)
	}
	if !isSupportedFormat(format) {
		return writeCLIError(stdout, stderr, format, "search-specs", request, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unsupported format %q", format),
		}, 2)
	}
	resolvedConfigPath, err := resolveCommandConfigPath(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "search-specs", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	operation := app.SearchSpecs(ctx, resolvedConfigPath, request)
	if operation.Issue != nil {
		return writeCLIError(stdout, stderr, format, "search-specs", operation.Request, cliIssue{
			Code:    operation.Issue.Code,
			Message: operation.Issue.Message,
		}, operation.Issue.ExitCode)
	}

	return writeCLISuccess(stdout, stderr, format, "search-specs", operation.Request, operation.Result, nil)
}
