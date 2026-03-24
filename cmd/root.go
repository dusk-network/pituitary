package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
)

type commandSpec struct {
	Run func(context.Context, []string, io.Writer, io.Writer) int
}

var commands = map[string]commandSpec{
	"canonicalize":      {Run: runCanonicalizeContext},
	"discover":          {Run: runDiscoverContext},
	"migrate-config":    {Run: runMigrateConfigContext},
	"index":             {Run: runIndexContext},
	"status":            {Run: runStatusContext},
	"version":           {Run: runVersionContext},
	"preview-sources":   {Run: runPreviewSourcesContext},
	"explain-file":      {Run: runExplainFileContext},
	"search-specs":      {Run: runSearchSpecsContext},
	"check-overlap":     {Run: runCheckOverlapContext},
	"compare-specs":     {Run: runCompareSpecsContext},
	"analyze-impact":    {Run: runAnalyzeImpactContext},
	"check-terminology": {Run: runCheckTerminologyContext},
	"check-compliance":  {Run: runCheckComplianceContext},
	"check-doc-drift":   {Run: runCheckDocDriftContext},
	"review-spec":       {Run: runReviewSpecContext},
	"serve":             {Run: runServeContext},
}

func commandDescription(name string) string {
	switch name {
	case "canonicalize":
		return "promote an inferred contract into a spec bundle"
	case "discover":
		return "scan a repo and propose a local config"
	case "migrate-config":
		return "rewrite a legacy config into the current schema"
	case "index":
		return "rebuild or validate the local Pituitary index"
	case "status":
		return "show current index status"
	case "version":
		return "show Pituitary and Go runtime versions"
	case "preview-sources":
		return "show which files each source will index"
	case "explain-file":
		return "explain how one file is treated by configured sources"
	case "search-specs":
		return "search spec sections semantically"
	case "check-overlap":
		return "find overlapping specs"
	case "compare-specs":
		return "compare design tradeoffs across specs"
	case "analyze-impact":
		return "report affected specs and docs"
	case "check-terminology":
		return "audit terminology consistency after conceptual changes"
	case "check-compliance":
		return "check code paths and diffs against accepted specs"
	case "check-doc-drift":
		return "find docs that drift from specs"
	case "review-spec":
		return "run the common spec-review workflow"
	case "serve":
		return "run the optional MCP server transport"
	case "help":
		return "show available commands"
	default:
		return ""
	}
}

// Run executes the bootstrap CLI transport and returns the desired process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	return RunContext(context.Background(), args, stdout, stderr)
}

// RunContext executes the bootstrap CLI transport with the provided context.
func RunContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stdout)
		return 1
	}

	options, remainingArgs, err := parseGlobalCLIOptions(args)
	if err != nil {
		if err == flag.ErrHelp {
			printHelp(stdout)
			return 0
		}
		fmt.Fprintf(stderr, "pituitary: %s\n\n", err)
		printHelp(stderr)
		return 2
	}
	if len(remainingArgs) == 0 {
		printHelp(stdout)
		return 1
	}

	ctx = withCLIConfigPath(ctx, options.ConfigPath)

	name := remainingArgs[0]
	if name == "help" || name == "--help" || name == "-h" {
		printHelp(stdout)
		return 0
	}

	command, ok := commands[name]
	if !ok {
		fmt.Fprintf(stderr, "unknown command %q\n\n", name)
		printHelp(stderr)
		return 1
	}

	return command.Run(ctx, remainingArgs[1:], stdout, stderr)
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "pituitary bootstrap CLI")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "global options:")
	fmt.Fprintln(w, "  --config PATH     path to workspace config")
	fmt.Fprintln(w)
	printSharedConfigResolution(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "available commands:")

	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		fmt.Fprintf(w, "  %-16s %s\n", name, commandDescription(name))
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "run `pituitary <command> --help` for command-specific usage")
}
