package mcp

import (
	"context"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/index"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

type compareSpecsArgs struct {
	SpecRefs []string `json:"spec_refs" jsonschema_description:"Indexed spec refs to compare"`
}

type analyzeImpactArgs struct {
	SpecRef    string `json:"spec_ref" jsonschema_description:"Indexed spec ref to analyze"`
	ChangeType string `json:"change_type,omitempty" jsonschema_description:"accepted, modified, or deprecated"`
}

func searchSpecsTool(options Options) mcpserver.ServerTool {
	tool := mcpgo.NewTool(
		"search_specs",
		mcpgo.WithDescription("Search indexed spec sections semantically."),
		mcpgo.WithInputSchema[index.SearchSpecRequest](),
		mcpgo.WithOutputSchema[index.SearchSpecResult](),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: mcpgo.NewTypedToolHandler(searchSpecsHandler(options)),
	}
}

func checkOverlapTool(options Options) mcpserver.ServerTool {
	tool := mcpgo.NewTool(
		"check_overlap",
		mcpgo.WithDescription("Detect overlapping indexed or draft specs."),
		mcpgo.WithInputSchema[analysis.OverlapRequest](),
		mcpgo.WithOutputSchema[analysis.OverlapResult](),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: mcpgo.NewTypedToolHandler(checkOverlapHandler(options)),
	}
}

func compareSpecsTool(options Options) mcpserver.ServerTool {
	tool := mcpgo.NewTool(
		"compare_specs",
		mcpgo.WithDescription("Compare indexed specs and report tradeoffs."),
		mcpgo.WithInputSchema[compareSpecsArgs](),
		mcpgo.WithOutputSchema[analysis.CompareResult](),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: mcpgo.NewTypedToolHandler(compareSpecsHandler(options)),
	}
}

func analyzeImpactTool(options Options) mcpserver.ServerTool {
	tool := mcpgo.NewTool(
		"analyze_impact",
		mcpgo.WithDescription("Analyze impacted specs, refs, and docs for an indexed spec."),
		mcpgo.WithInputSchema[analyzeImpactArgs](),
		mcpgo.WithOutputSchema[analysis.AnalyzeImpactResult](),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: mcpgo.NewTypedToolHandler(analyzeImpactHandler(options)),
	}
}

func checkDocDriftTool(options Options) mcpserver.ServerTool {
	tool := mcpgo.NewTool(
		"check_doc_drift",
		mcpgo.WithDescription("Find documentation that contradicts accepted specs."),
		mcpgo.WithInputSchema[analysis.DocDriftRequest](),
		mcpgo.WithOutputSchema[analysis.DocDriftResult](),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: mcpgo.NewTypedToolHandler(checkDocDriftHandler(options)),
	}
}

func reviewSpecTool(options Options) mcpserver.ServerTool {
	tool := mcpgo.NewTool(
		"review_spec",
		mcpgo.WithDescription("Run overlap, comparison, impact, and targeted doc drift as one workflow."),
		mcpgo.WithInputSchema[analysis.ReviewRequest](),
		mcpgo.WithOutputSchema[analysis.ReviewResult](),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: mcpgo.NewTypedToolHandler(reviewSpecHandler(options)),
	}
}

func searchSpecsHandler(options Options) mcpgo.TypedToolHandlerFunc[index.SearchSpecRequest] {
	return func(ctx context.Context, request mcpgo.CallToolRequest, args index.SearchSpecRequest) (*mcpgo.CallToolResult, error) {
		operation := app.SearchSpecs(ctx, options.normalized().ConfigPath, args)
		if operation.Issue != nil {
			return mcpgo.NewToolResultError(operation.Issue.Message), nil
		}
		return mcpgo.NewToolResultStructuredOnly(operation.Result), nil
	}
}

func checkOverlapHandler(options Options) mcpgo.TypedToolHandlerFunc[analysis.OverlapRequest] {
	return func(ctx context.Context, request mcpgo.CallToolRequest, args analysis.OverlapRequest) (*mcpgo.CallToolResult, error) {
		operation := app.CheckOverlap(ctx, options.normalized().ConfigPath, args)
		if operation.Issue != nil {
			return mcpgo.NewToolResultError(operation.Issue.Message), nil
		}
		return mcpgo.NewToolResultStructuredOnly(operation.Result), nil
	}
}

func compareSpecsHandler(options Options) mcpgo.TypedToolHandlerFunc[compareSpecsArgs] {
	return func(ctx context.Context, request mcpgo.CallToolRequest, args compareSpecsArgs) (*mcpgo.CallToolResult, error) {
		operation := app.CompareSpecs(ctx, options.normalized().ConfigPath, analysis.CompareRequest{SpecRefs: args.SpecRefs})
		if operation.Issue != nil {
			return mcpgo.NewToolResultError(operation.Issue.Message), nil
		}
		return mcpgo.NewToolResultStructuredOnly(operation.Result), nil
	}
}

func analyzeImpactHandler(options Options) mcpgo.TypedToolHandlerFunc[analyzeImpactArgs] {
	return func(ctx context.Context, request mcpgo.CallToolRequest, args analyzeImpactArgs) (*mcpgo.CallToolResult, error) {
		operation := app.AnalyzeImpact(ctx, options.normalized().ConfigPath, analysis.AnalyzeImpactRequest{
			SpecRef:    args.SpecRef,
			ChangeType: args.ChangeType,
		})
		if operation.Issue != nil {
			return mcpgo.NewToolResultError(operation.Issue.Message), nil
		}
		return mcpgo.NewToolResultStructuredOnly(operation.Result), nil
	}
}

func checkDocDriftHandler(options Options) mcpgo.TypedToolHandlerFunc[analysis.DocDriftRequest] {
	return func(ctx context.Context, request mcpgo.CallToolRequest, args analysis.DocDriftRequest) (*mcpgo.CallToolResult, error) {
		operation := app.CheckDocDrift(ctx, options.normalized().ConfigPath, args)
		if operation.Issue != nil {
			return mcpgo.NewToolResultError(operation.Issue.Message), nil
		}
		return mcpgo.NewToolResultStructuredOnly(operation.Result), nil
	}
}

func reviewSpecHandler(options Options) mcpgo.TypedToolHandlerFunc[analysis.ReviewRequest] {
	return func(ctx context.Context, request mcpgo.CallToolRequest, args analysis.ReviewRequest) (*mcpgo.CallToolResult, error) {
		operation := app.ReviewSpec(ctx, options.normalized().ConfigPath, args)
		if operation.Issue != nil {
			return mcpgo.NewToolResultError(operation.Issue.Message), nil
		}
		return mcpgo.NewToolResultStructuredOnly(operation.Result), nil
	}
}
