package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
)

var commands = map[string]string{
	"canonicalize":      "promote an inferred contract into a spec bundle",
	"discover":          "scan a repo and propose a local config",
	"migrate-config":    "rewrite a legacy config into the current schema",
	"index":             "rebuild or validate the local Pituitary index",
	"status":            "show current index status",
	"version":           "show Pituitary and Go runtime versions",
	"preview-sources":   "show which files each source will index",
	"explain-file":      "explain how one file is treated by configured sources",
	"search-specs":      "search spec sections semantically",
	"check-overlap":     "find overlapping specs",
	"compare-specs":     "compare design tradeoffs across specs",
	"analyze-impact":    "report affected specs and docs",
	"check-terminology": "audit terminology consistency after conceptual changes",
	"check-compliance":  "check code paths and diffs against accepted specs",
	"check-doc-drift":   "find docs that drift from specs",
	"review-spec":       "run the common spec-review workflow",
	"serve":             "run the optional MCP server transport",
	"help":              "show available commands",
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

	if name == "index" {
		return runIndexContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "discover" {
		return runDiscoverContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "canonicalize" {
		return runCanonicalizeContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "migrate-config" {
		return runMigrateConfigContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "status" {
		return runStatusContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "version" {
		return runVersionContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "preview-sources" {
		return runPreviewSourcesContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "explain-file" {
		return runExplainFileContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "search-specs" {
		return runSearchSpecsContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "check-overlap" {
		return runCheckOverlapContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "compare-specs" {
		return runCompareSpecsContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "analyze-impact" {
		return runAnalyzeImpactContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "check-terminology" {
		return runCheckTerminologyContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "check-compliance" {
		return runCheckComplianceContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "check-doc-drift" {
		return runCheckDocDriftContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "review-spec" {
		return runReviewSpecContext(ctx, remainingArgs[1:], stdout, stderr)
	}
	if name == "serve" {
		return runServeWithConfig(options.ConfigPath, remainingArgs[1:], stdout, stderr)
	}

	description, ok := commands[name]
	if !ok {
		fmt.Fprintf(stderr, "unknown command %q\n\n", name)
		printHelp(stderr)
		return 1
	}

	fmt.Fprintf(stdout, "pituitary %s: %s\n", name, description)
	fmt.Fprintln(stdout, "status: bootstrap only, not implemented yet")
	return 0
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

	names := make([]string, 0, len(commands)-1)
	for name := range commands {
		if name == "help" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		fmt.Fprintf(w, "  %-16s %s\n", name, commands[name])
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "run `pituitary <command> --help` for command-specific usage")
}
