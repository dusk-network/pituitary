package cmd

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
	"github.com/invopop/jsonschema"
)

type schemaRequest struct {
	Command string `json:"command,omitempty"`
}

type schemaCatalogResult struct {
	Commands []schemaCommandSummary `json:"commands"`
}

type schemaCommandSummary struct {
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	MutatesState        bool     `json:"mutates_state"`
	ConfigScoped        bool     `json:"config_scoped"`
	SupportsRequestFile bool     `json:"supports_request_file,omitempty"`
	SupportedFormats    []string `json:"supported_formats"`
}

type schemaCommandResult struct {
	Name                string             `json:"name"`
	Description         string             `json:"description"`
	MutatesState        bool               `json:"mutates_state"`
	ConfigScoped        bool               `json:"config_scoped"`
	SupportsRequestFile bool               `json:"supports_request_file,omitempty"`
	SupportedFormats    []string           `json:"supported_formats"`
	InputSchema         *jsonschema.Schema `json:"input_schema,omitempty"`
	OutputSchema        *jsonschema.Schema `json:"output_schema,omitempty"`
}

type schemaCommandSpec struct {
	Summary    schemaCommandSummary
	InputType  any
	OutputType any
}

type serveRequest struct {
	Transport string `json:"transport,omitempty"`
}

func runSchema(args []string, stdout, stderr io.Writer) int {
	return runSchemaContext(context.Background(), args, stdout, stderr)
}

func runSchemaContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	_ = ctx
	args = reorderSchemaArgs(args)

	fs := flag.NewFlagSet("schema", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newStandaloneCommandHelp("schema", "pituitary schema [COMMAND] [--format FORMAT]")

	var format string
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "schema", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() > 1 {
		return writeCLIError(stdout, stderr, format, "schema", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args()[1:], " ")),
		}, 2)
	}
	if err := validateCLIFormat("schema", format); err != nil {
		return writeCLIError(stdout, stderr, format, "schema", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	request := schemaRequest{}
	if fs.NArg() == 1 {
		request.Command = strings.TrimSpace(fs.Arg(0))
	}

	registry := commandSchemaRegistry()
	if request.Command == "" {
		return writeCLISuccess(stdout, stderr, format, "schema", request, buildSchemaCatalogResult(registry), nil)
	}

	spec, ok := registry[request.Command]
	if !ok {
		return writeCLIError(stdout, stderr, format, "schema", request, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unknown command %q", request.Command),
		}, 2)
	}

	result := &schemaCommandResult{
		Name:                spec.Summary.Name,
		Description:         spec.Summary.Description,
		MutatesState:        spec.Summary.MutatesState,
		ConfigScoped:        spec.Summary.ConfigScoped,
		SupportsRequestFile: spec.Summary.SupportsRequestFile,
		SupportedFormats:    append([]string(nil), spec.Summary.SupportedFormats...),
		InputSchema:         reflectSchema(spec.InputType),
		OutputSchema:        reflectSchema(spec.OutputType),
	}
	return writeCLISuccess(stdout, stderr, format, "schema", request, result, nil)
}

func reorderSchemaArgs(args []string) []string {
	flagArgs := make([]string, 0, len(args))
	positionalArgs := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionalArgs = append(positionalArgs, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionalArgs = append(positionalArgs, arg)
			continue
		}

		flagArgs = append(flagArgs, arg)
		if schemaFlagTakesValue(arg) && !strings.Contains(arg, "=") && i+1 < len(args) {
			flagArgs = append(flagArgs, args[i+1])
			i++
		}
	}

	return append(flagArgs, positionalArgs...)
}

func schemaFlagTakesValue(arg string) bool {
	switch arg {
	case "-format", "--format":
		return true
	default:
		return false
	}
}

func buildSchemaCatalogResult(registry map[string]schemaCommandSpec) *schemaCatalogResult {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)

	result := &schemaCatalogResult{Commands: make([]schemaCommandSummary, 0, len(names))}
	for _, name := range names {
		result.Commands = append(result.Commands, registry[name].Summary)
	}
	return result
}

func commandSchemaRegistry() map[string]schemaCommandSpec {
	return map[string]schemaCommandSpec{
		"analyze-impact": {
			Summary: schemaCommandSummary{
				Name:                "analyze-impact",
				Description:         commandDescription("analyze-impact"),
				ConfigScoped:        true,
				SupportsRequestFile: true,
				SupportedFormats:    sortedCommandFormats("analyze-impact"),
			},
			InputType:  analysis.AnalyzeImpactRequest{},
			OutputType: analysis.AnalyzeImpactResult{},
		},
		"canonicalize": {
			Summary: schemaCommandSummary{
				Name:             "canonicalize",
				Description:      commandDescription("canonicalize"),
				MutatesState:     true,
				SupportedFormats: sortedCommandFormats("canonicalize"),
			},
			InputType:  canonicalizeRequest{},
			OutputType: source.CanonicalizeResult{},
		},
		"check-compliance": {
			Summary: schemaCommandSummary{
				Name:                "check-compliance",
				Description:         commandDescription("check-compliance"),
				ConfigScoped:        true,
				SupportsRequestFile: true,
				SupportedFormats:    sortedCommandFormats("check-compliance"),
			},
			InputType:  analysis.ComplianceRequest{},
			OutputType: analysis.ComplianceResult{},
		},
		"check-doc-drift": {
			Summary: schemaCommandSummary{
				Name:                "check-doc-drift",
				Description:         commandDescription("check-doc-drift"),
				ConfigScoped:        true,
				SupportsRequestFile: true,
				SupportedFormats:    sortedCommandFormats("check-doc-drift"),
			},
			InputType:  analysis.DocDriftRequest{},
			OutputType: analysis.DocDriftResult{},
		},
		"check-spec-freshness": {
			Summary: schemaCommandSummary{
				Name:                "check-spec-freshness",
				Description:         commandDescription("check-spec-freshness"),
				ConfigScoped:        true,
				SupportsRequestFile: true,
				SupportedFormats:    sortedCommandFormats("check-spec-freshness"),
			},
			InputType:  analysis.FreshnessRequest{},
			OutputType: analysis.FreshnessResult{},
		},
		"check-overlap": {
			Summary: schemaCommandSummary{
				Name:                "check-overlap",
				Description:         commandDescription("check-overlap"),
				ConfigScoped:        true,
				SupportsRequestFile: true,
				SupportedFormats:    sortedCommandFormats("check-overlap"),
			},
			InputType:  analysis.OverlapRequest{},
			OutputType: analysis.OverlapResult{},
		},
		"check-terminology": {
			Summary: schemaCommandSummary{
				Name:                "check-terminology",
				Description:         commandDescription("check-terminology"),
				ConfigScoped:        true,
				SupportsRequestFile: true,
				SupportedFormats:    sortedCommandFormats("check-terminology"),
			},
			InputType:  analysis.TerminologyAuditRequest{},
			OutputType: analysis.TerminologyAuditResult{},
		},
		"compare-specs": {
			Summary: schemaCommandSummary{
				Name:                "compare-specs",
				Description:         commandDescription("compare-specs"),
				ConfigScoped:        true,
				SupportsRequestFile: true,
				SupportedFormats:    sortedCommandFormats("compare-specs"),
			},
			InputType:  analysis.CompareRequest{},
			OutputType: analysis.CompareResult{},
		},
		"discover": {
			Summary: schemaCommandSummary{
				Name:             "discover",
				Description:      commandDescription("discover"),
				MutatesState:     true,
				SupportedFormats: sortedCommandFormats("discover"),
			},
			InputType:  discoverRequest{},
			OutputType: source.DiscoverResult{},
		},
		"new": {
			Summary: schemaCommandSummary{
				Name:             "new",
				Description:      commandDescription("new"),
				MutatesState:     true,
				ConfigScoped:     true,
				SupportedFormats: sortedCommandFormats("new"),
			},
			InputType:  newRequest{},
			OutputType: source.NewSpecBundleResult{},
		},
		"explain-file": {
			Summary: schemaCommandSummary{
				Name:             "explain-file",
				Description:      commandDescription("explain-file"),
				ConfigScoped:     true,
				SupportedFormats: sortedCommandFormats("explain-file"),
			},
			InputType:  explainFileRequest{},
			OutputType: source.ExplainFileResult{},
		},
		"fix": {
			Summary: schemaCommandSummary{
				Name:             "fix",
				Description:      commandDescription("fix"),
				MutatesState:     true,
				ConfigScoped:     true,
				SupportedFormats: sortedCommandFormats("fix"),
			},
			InputType:  fixCLIRequest{},
			OutputType: app.FixResult{},
		},
		"index": {
			Summary: schemaCommandSummary{
				Name:             "index",
				Description:      commandDescription("index"),
				MutatesState:     true,
				ConfigScoped:     true,
				SupportedFormats: sortedCommandFormats("index"),
			},
			InputType:  indexRequest{},
			OutputType: index.RebuildResult{},
		},
		"init": {
			Summary: schemaCommandSummary{
				Name:             "init",
				Description:      commandDescription("init"),
				MutatesState:     true,
				SupportedFormats: sortedCommandFormats("init"),
			},
			InputType:  initRequest{},
			OutputType: initResult{},
		},
		"migrate-config": {
			Summary: schemaCommandSummary{
				Name:             "migrate-config",
				Description:      commandDescription("migrate-config"),
				MutatesState:     true,
				SupportedFormats: sortedCommandFormats("migrate-config"),
			},
			InputType:  migrateConfigRequest{},
			OutputType: migrateConfigResult{},
		},
		"preview-sources": {
			Summary: schemaCommandSummary{
				Name:             "preview-sources",
				Description:      commandDescription("preview-sources"),
				ConfigScoped:     true,
				SupportedFormats: sortedCommandFormats("preview-sources"),
			},
			InputType:  struct{}{},
			OutputType: source.PreviewResult{},
		},
		"review-spec": {
			Summary: schemaCommandSummary{
				Name:                "review-spec",
				Description:         commandDescription("review-spec"),
				ConfigScoped:        true,
				SupportsRequestFile: true,
				SupportedFormats:    sortedCommandFormats("review-spec"),
			},
			InputType:  analysis.ReviewRequest{},
			OutputType: analysis.ReviewResult{},
		},
		"schema": {
			Summary: schemaCommandSummary{
				Name:             "schema",
				Description:      commandDescription("schema"),
				SupportedFormats: sortedCommandFormats("schema"),
			},
			InputType:  schemaRequest{},
			OutputType: nil,
		},
		"search-specs": {
			Summary: schemaCommandSummary{
				Name:                "search-specs",
				Description:         commandDescription("search-specs"),
				ConfigScoped:        true,
				SupportsRequestFile: true,
				SupportedFormats:    sortedCommandFormats("search-specs"),
			},
			InputType:  index.SearchSpecRequest{},
			OutputType: index.SearchSpecResult{},
		},
		"serve": {
			Summary: schemaCommandSummary{
				Name:             "serve",
				Description:      commandDescription("serve"),
				ConfigScoped:     true,
				SupportedFormats: nil,
			},
			InputType:  serveRequest{},
			OutputType: nil,
		},
		"status": {
			Summary: schemaCommandSummary{
				Name:             "status",
				Description:      commandDescription("status"),
				ConfigScoped:     true,
				SupportedFormats: sortedCommandFormats("status"),
			},
			InputType:  statusRequest{},
			OutputType: statusResult{},
		},
		"version": {
			Summary: schemaCommandSummary{
				Name:             "version",
				Description:      commandDescription("version"),
				SupportedFormats: sortedCommandFormats("version"),
			},
			InputType:  versionRequest{},
			OutputType: versionResult{},
		},
	}
}

func sortedCommandFormats(name string) []string {
	command, ok := commandRegistry()[name]
	if !ok {
		return nil
	}
	formats := make([]string, 0, len(command.Formats))
	for format := range command.Formats {
		formats = append(formats, format)
	}
	sort.Strings(formats)
	return formats
}

func reflectSchema(value any) *jsonschema.Schema {
	if value == nil {
		return nil
	}
	return (&jsonschema.Reflector{
		Anonymous:      true,
		DoNotReference: true,
	}).Reflect(value)
}

func renderSchemaCatalogResult(w io.Writer, result *schemaCatalogResult) {
	fmt.Fprintln(w, "pituitary schema: describe machine-readable command contracts")
	for _, command := range result.Commands {
		fmt.Fprintf(
			w,
			"command: %s | mutates: %t | config: %t | request-file: %t | formats: %s\n",
			command.Name,
			command.MutatesState,
			command.ConfigScoped,
			command.SupportsRequestFile,
			strings.Join(command.SupportedFormats, ", "),
		)
		fmt.Fprintf(w, "description: %s\n", command.Description)
	}
}

func renderSchemaCommandResult(w io.Writer, result *schemaCommandResult) {
	fmt.Fprintln(w, "pituitary schema: describe machine-readable command contracts")
	fmt.Fprintf(w, "command: %s\n", result.Name)
	fmt.Fprintf(w, "description: %s\n", result.Description)
	fmt.Fprintf(w, "mutates state: %t\n", result.MutatesState)
	fmt.Fprintf(w, "config scoped: %t\n", result.ConfigScoped)
	fmt.Fprintf(w, "supports request-file: %t\n", result.SupportsRequestFile)
	fmt.Fprintf(w, "supported formats: %s\n", strings.Join(result.SupportedFormats, ", "))
	renderSchemaJSONBlock(w, "input schema", result.InputSchema)
	renderSchemaJSONBlock(w, "output schema", result.OutputSchema)
}

func renderSchemaJSONBlock(w io.Writer, heading string, schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		fmt.Fprintf(w, "%s: <marshal error: %v>\n", heading, err)
		return
	}
	fmt.Fprintf(w, "%s:\n%s\n", heading, string(data))
}
