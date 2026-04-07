package mcp

import (
	"context"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/runtimeprobe"
	"github.com/dusk-network/pituitary/internal/source"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// --- Argument types for tools that need MCP-specific input shapes ---

type compareSpecsArgs struct {
	SpecRefs []string `json:"spec_refs" jsonschema_description:"Indexed spec refs to compare"`
}

type analyzeImpactArgs struct {
	SpecRef    string `json:"spec_ref" jsonschema_description:"Indexed spec ref to analyze"`
	ChangeType string `json:"change_type,omitempty" jsonschema_description:"accepted, modified, or deprecated"`
	AtDate     string `json:"at_date,omitempty" jsonschema_description:"ISO date for point-in-time governance query"`
}

type compilePreviewArgs struct {
	Scope string `json:"scope,omitempty" jsonschema_description:"Target scope: accepted spec ref or all"`
}

type fixPreviewArgs struct {
	Path    string   `json:"path,omitempty" jsonschema_description:"Doc path to preview fixes for"`
	Scope   string   `json:"scope,omitempty" jsonschema_description:"Spec ref or all"`
	DocRefs []string `json:"doc_refs,omitempty" jsonschema_description:"Specific doc refs to preview fixes for"`
}

type explainFileArgs struct {
	Path string `json:"path" jsonschema_description:"File path to classify"`
}

// mcpStatusResult is the tiered context response for MCP agents.
// L0 (~50 tokens): workspace identity, counts, index freshness.
// L1 (~200 tokens): active governance findings and protocol.
// L2/L3 are served by domain-specific tools (search_specs, governed_by, etc.).
type mcpStatusResult struct {
	// L0: Workspace identity
	WorkspaceRoot    string                 `json:"workspace_root"`
	IndexExists      bool                   `json:"index_exists"`
	SpecCount        int                    `json:"spec_count"`
	DocCount         int                    `json:"doc_count"`
	ChunkCount       int                    `json:"chunk_count"`
	EmbedderProvider string                 `json:"embedder_provider,omitempty"`
	AnalysisProvider string                 `json:"analysis_provider,omitempty"`
	Freshness        *index.FreshnessStatus `json:"freshness,omitempty"`
	// L1: Active governance signals
	RelationGraph      *index.RelationGraphStatus `json:"relation_graph,omitempty"`
	GovernanceCoverage *index.GovernanceCoverage  `json:"governance_coverage,omitempty"`
	Guidance           []string                   `json:"guidance,omitempty"`
	// Governance protocol: tells the AI when to use each tool
	GovernanceProtocol *governanceProtocol `json:"governance_protocol,omitempty"`
}

// governanceProtocol teaches the AI when to use governance tools.
type governanceProtocol struct {
	Instructions []governanceInstruction `json:"instructions"`
}

// governanceInstruction is one rule in the governance protocol.
type governanceInstruction struct {
	Trigger string `json:"trigger"`
	Action  string `json:"action"`
	Tool    string `json:"tool"`
}

// defaultGovernanceProtocol returns the standard governance protocol that
// teaches the AI when to check governance automatically.
func defaultGovernanceProtocol() *governanceProtocol {
	return &governanceProtocol{
		Instructions: []governanceInstruction{
			{
				Trigger: "BEFORE modifying any file",
				Action:  "Check for governing specs that constrain the file",
				Tool:    "governed_by",
			},
			{
				Trigger: "BEFORE committing code changes",
				Action:  "Verify no spec violations in the diff",
				Tool:    "check_compliance",
			},
			{
				Trigger: "AFTER accepting or modifying a spec",
				Action:  "Trace downstream effects on other specs, code, and docs",
				Tool:    "analyze_impact",
			},
			{
				Trigger: "WHEN writing or updating documentation",
				Action:  "Verify the doc aligns with accepted specs",
				Tool:    "check_doc_drift",
			},
		},
	}
}

// --- Existing tools ---

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
		mcpgo.WithDescription("Analyze impacted specs, refs, and docs for an indexed spec. Per governance protocol: call AFTER accepting or modifying a spec."),
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
		mcpgo.WithDescription("Find documentation that contradicts accepted specs. Per governance protocol: call WHEN writing or updating documentation."),
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

// --- New tools (#235) ---

func checkComplianceTool(options Options) mcpserver.ServerTool {
	tool := mcpgo.NewTool(
		"check_compliance",
		mcpgo.WithDescription("Check whether code or a diff complies with accepted specs. Per governance protocol: call BEFORE committing."),
		mcpgo.WithInputSchema[analysis.ComplianceRequest](),
		mcpgo.WithOutputSchema[analysis.ComplianceResult](),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: mcpgo.NewTypedToolHandler(checkComplianceHandler(options)),
	}
}

func checkTerminologyTool(options Options) mcpserver.ServerTool {
	tool := mcpgo.NewTool(
		"check_terminology",
		mcpgo.WithDescription("Audit docs and specs for displaced or deprecated terminology."),
		mcpgo.WithInputSchema[analysis.TerminologyAuditRequest](),
		mcpgo.WithOutputSchema[analysis.TerminologyAuditResult](),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: mcpgo.NewTypedToolHandler(checkTerminologyHandler(options)),
	}
}

func governedByTool(options Options) mcpserver.ServerTool {
	tool := mcpgo.NewTool(
		"governed_by",
		mcpgo.WithDescription("Return the accepted specs that govern a given file path via applies_to edges. Per governance protocol: call BEFORE modifying any file."),
		mcpgo.WithInputSchema[app.GovernedByRequest](),
		mcpgo.WithOutputSchema[app.GovernedByResult](),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: mcpgo.NewTypedToolHandler(governedByHandler(options)),
	}
}

func compilePreviewTool(options Options) mcpserver.ServerTool {
	tool := mcpgo.NewTool(
		"compile_preview",
		mcpgo.WithDescription("Preview deterministic terminology edits without applying them."),
		mcpgo.WithInputSchema[compilePreviewArgs](),
		mcpgo.WithOutputSchema[app.CompileResult](),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: mcpgo.NewTypedToolHandler(compilePreviewHandler(options)),
	}
}

func fixPreviewTool(options Options) mcpserver.ServerTool {
	tool := mcpgo.NewTool(
		"fix_preview",
		mcpgo.WithDescription("Preview deterministic doc-drift remediations without applying them."),
		mcpgo.WithInputSchema[fixPreviewArgs](),
		mcpgo.WithOutputSchema[app.FixResult](),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: mcpgo.NewTypedToolHandler(fixPreviewHandler(options)),
	}
}

func statusTool(options Options) mcpserver.ServerTool {
	tool := mcpgo.NewTool(
		"status",
		mcpgo.WithDescription("Check index freshness, workspace health, and governance protocol. Call this first to get the governance_protocol that tells you when to use each tool."),
		mcpgo.WithInputSchema[struct{}](),
		mcpgo.WithOutputSchema[mcpStatusResult](),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: mcpgo.NewTypedToolHandler(statusHandler(options)),
	}
}

func explainFileTool(options Options) mcpserver.ServerTool {
	tool := mcpgo.NewTool(
		"explain_file",
		mcpgo.WithDescription("Classify a file path against configured sources."),
		mcpgo.WithInputSchema[explainFileArgs](),
		mcpgo.WithOutputSchema[source.ExplainFileResult](),
	)
	return mcpserver.ServerTool{
		Tool:    tool,
		Handler: mcpgo.NewTypedToolHandler(explainFileHandler(options)),
	}
}

// --- Existing handlers ---

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
			AtDate:     args.AtDate,
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

// --- New handlers (#235) ---

func checkComplianceHandler(options Options) mcpgo.TypedToolHandlerFunc[analysis.ComplianceRequest] {
	return func(ctx context.Context, request mcpgo.CallToolRequest, args analysis.ComplianceRequest) (*mcpgo.CallToolResult, error) {
		operation := app.CheckCompliance(ctx, options.normalized().ConfigPath, args)
		if operation.Issue != nil {
			return mcpgo.NewToolResultError(operation.Issue.Message), nil
		}
		return mcpgo.NewToolResultStructuredOnly(operation.Result), nil
	}
}

func checkTerminologyHandler(options Options) mcpgo.TypedToolHandlerFunc[analysis.TerminologyAuditRequest] {
	return func(ctx context.Context, request mcpgo.CallToolRequest, args analysis.TerminologyAuditRequest) (*mcpgo.CallToolResult, error) {
		operation := app.CheckTerminology(ctx, options.normalized().ConfigPath, args)
		if operation.Issue != nil {
			return mcpgo.NewToolResultError(operation.Issue.Message), nil
		}
		return mcpgo.NewToolResultStructuredOnly(operation.Result), nil
	}
}

func governedByHandler(options Options) mcpgo.TypedToolHandlerFunc[app.GovernedByRequest] {
	return func(ctx context.Context, request mcpgo.CallToolRequest, args app.GovernedByRequest) (*mcpgo.CallToolResult, error) {
		operation := app.GovernedBy(ctx, options.normalized().ConfigPath, args)
		if operation.Issue != nil {
			return mcpgo.NewToolResultError(operation.Issue.Message), nil
		}
		return mcpgo.NewToolResultStructuredOnly(operation.Result), nil
	}
}

func compilePreviewHandler(options Options) mcpgo.TypedToolHandlerFunc[compilePreviewArgs] {
	return func(ctx context.Context, request mcpgo.CallToolRequest, args compilePreviewArgs) (*mcpgo.CallToolResult, error) {
		operation := app.CompileTerminology(ctx, options.normalized().ConfigPath, app.CompileRequest{
			Scope: args.Scope,
			Apply: false,
		})
		if operation.Issue != nil {
			return mcpgo.NewToolResultError(operation.Issue.Message), nil
		}
		return mcpgo.NewToolResultStructuredOnly(operation.Result), nil
	}
}

func fixPreviewHandler(options Options) mcpgo.TypedToolHandlerFunc[fixPreviewArgs] {
	return func(ctx context.Context, request mcpgo.CallToolRequest, args fixPreviewArgs) (*mcpgo.CallToolResult, error) {
		operation := app.FixDocDrift(ctx, options.normalized().ConfigPath, app.FixRequest{
			Path:    args.Path,
			Scope:   args.Scope,
			DocRefs: args.DocRefs,
			Apply:   false,
		})
		if operation.Issue != nil {
			return mcpgo.NewToolResultError(operation.Issue.Message), nil
		}
		return mcpgo.NewToolResultStructuredOnly(operation.Result), nil
	}
}

func statusHandler(options Options) mcpgo.TypedToolHandlerFunc[struct{}] {
	return func(ctx context.Context, request mcpgo.CallToolRequest, _ struct{}) (*mcpgo.CallToolResult, error) {
		operation := app.Status(ctx, options.normalized().ConfigPath, app.StatusRequest{
			CheckRuntime: runtimeprobe.ScopeNone,
		})
		if operation.Issue != nil {
			return mcpgo.NewToolResultError(operation.Issue.Message), nil
		}
		r := operation.Result
		result := &mcpStatusResult{
			// L0: Workspace identity
			WorkspaceRoot:    r.WorkspaceRoot,
			EmbedderProvider: r.EmbedderProvider,
			AnalysisProvider: r.AnalysisProvider,
			Freshness:        r.Freshness,
			// L1: Active governance signals
			RelationGraph:      r.RelationGraph,
			Guidance:           r.Guidance,
			GovernanceProtocol: defaultGovernanceProtocol(),
		}
		if r.Index != nil {
			result.IndexExists = r.Index.Exists
			result.SpecCount = r.Index.SpecCount
			result.DocCount = r.Index.DocCount
			result.ChunkCount = r.Index.ChunkCount
			result.GovernanceCoverage = r.Index.GovernanceCoverage
		}
		return mcpgo.NewToolResultStructuredOnly(result), nil
	}
}

func explainFileHandler(options Options) mcpgo.TypedToolHandlerFunc[explainFileArgs] {
	return func(ctx context.Context, request mcpgo.CallToolRequest, args explainFileArgs) (*mcpgo.CallToolResult, error) {
		operation := app.ExplainFile(ctx, options.normalized().ConfigPath, app.ExplainFileRequest{
			Path: args.Path,
		})
		if operation.Issue != nil {
			return mcpgo.NewToolResultError(operation.Issue.Message), nil
		}
		return mcpgo.NewToolResultStructuredOnly(operation.Result), nil
	}
}
