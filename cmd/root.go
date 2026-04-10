package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/dusk-network/pituitary/internal/diag"
)

const (
	commandFormatText     = "text"
	commandFormatJSON     = "json"
	commandFormatTable    = "table"
	commandFormatMarkdown = "markdown"
	commandFormatHTML     = "html"
)

type commandSpec struct {
	Description string
	Formats     map[string]struct{}
	Run         func(context.Context, []string, io.Writer, io.Writer) int
}

func commandRegistry() map[string]commandSpec {
	return map[string]commandSpec{
		"canonicalize":         {Description: "promote an inferred contract into a spec bundle", Formats: commandFormats(), Run: runCanonicalizeContext},
		"discover":             {Description: "scan a repo and propose a local config", Formats: commandFormats(), Run: runDiscoverContext},
		"init":                 {Description: "discover, write, index, and summarize a workspace", Formats: commandFormats(), Run: runInitContext},
		"new":                  {Description: "scaffold a draft spec bundle from a template", Formats: commandFormats(), Run: runNewContext},
		"migrate-config":       {Description: "rewrite a legacy config into the current schema", Formats: commandFormats(), Run: runMigrateConfigContext},
		"index":                {Description: "rebuild or validate the local Pituitary index", Formats: commandFormats(), Run: runIndexContext},
		"status":               {Description: "show current index status", Formats: commandFormats(), Run: runStatusContext},
		"version":              {Description: "show Pituitary and Go runtime versions", Formats: commandFormats(), Run: runVersionContext},
		"preview-sources":      {Description: "preview indexed files per source and selector diagnostics", Formats: commandFormats(), Run: runPreviewSourcesContext},
		"explain-file":         {Description: "debug why one file is in or out of scope for configured sources", Formats: commandFormats(), Run: runExplainFileContext},
		"search-specs":         {Description: "search spec sections semantically", Formats: commandFormats(commandFormatTable), Run: runSearchSpecsContext},
		"check-overlap":        {Description: "find overlapping specs", Formats: commandFormats(), Run: runCheckOverlapContext},
		"compare-specs":        {Description: "compare design tradeoffs across specs", Formats: commandFormats(), Run: runCompareSpecsContext},
		"analyze-impact":       {Description: "report affected specs and docs", Formats: commandFormats(), Run: runAnalyzeImpactContext},
		"check-terminology":    {Description: "audit terminology consistency after conceptual changes", Formats: commandFormats(), Run: runCheckTerminologyContext},
		"check-compliance":     {Description: "check code paths and diffs against accepted specs", Formats: commandFormats(), Run: runCheckComplianceContext},
		"check-doc-drift":      {Description: "find docs that drift from specs", Formats: commandFormats(), Run: runCheckDocDriftContext},
		"check-spec-freshness": {Description: "detect specs that may be stale or superseded by decisions", Formats: commandFormats(), Run: runCheckSpecFreshnessContext},
		"compile":              {Description: "apply terminology edits to align docs with governed terms", Formats: commandFormats(), Run: runCompileContext},
		"fix":                  {Description: "apply deterministic doc-drift remediations", Formats: commandFormats(), Run: runFixContext},
		"review-spec":          {Description: "run the common spec-review workflow", Formats: commandFormats(commandFormatMarkdown, commandFormatHTML), Run: runReviewSpecContext},
		"schema":               {Description: "describe machine-readable command contracts", Formats: commandFormats(), Run: runSchemaContext},
		"serve":                {Description: "run the optional MCP server transport", Formats: commandFormats(), Run: runServeContext},
	}
}

func commandFormats(extra ...string) map[string]struct{} {
	formats := map[string]struct{}{
		commandFormatText: {},
		commandFormatJSON: {},
	}
	for _, format := range extra {
		formats[format] = struct{}{}
	}
	return formats
}

func commandDescription(name string) string {
	if name == "help" {
		return "show available commands"
	}
	command, ok := commandRegistry()[name]
	if !ok {
		return ""
	}
	return command.Description
}

func commandSupportsFormat(name, format string) bool {
	command, ok := commandRegistry()[name]
	if !ok {
		return false
	}
	_, ok = command.Formats[format]
	return ok
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
	stdout = wrapCLIWriter(stdout, options.ColorMode)
	stderr = wrapCLIWriter(stderr, options.ColorMode)
	logLevel, _ := diag.ParseLevel(options.LogLevel)
	ctx = withCLILogger(ctx, diag.NewLogger(stderr, logLevel))
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

	command, ok := commandRegistry()[name]
	if !ok {
		fmt.Fprintf(stderr, "unknown command %q\n\n", name)
		printHelp(stderr)
		return 1
	}

	return command.Run(ctx, remainingArgs[1:], stdout, stderr)
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "pituitary - consistency governance for specs, docs, and code")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "global options:")
	fmt.Fprintln(w, "  --config PATH     path to workspace config")
	fmt.Fprintln(w, "  --color MODE      terminal color: auto, always, or never")
	fmt.Fprintln(w, "  --log-level LEVEL diagnostics: off, error, warn, info, or debug")
	fmt.Fprintln(w)
	printSharedConfigResolution(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "available commands:")

	registry := commandRegistry()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		fmt.Fprintf(w, "  %-16s %s\n", name, commandDescription(name))
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "debugging:")
	fmt.Fprintln(w, "  when a file looks unexpectedly included or excluded, run `pituitary explain-file PATH` first")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "run `pituitary <command> --help` for command-specific usage")
}
