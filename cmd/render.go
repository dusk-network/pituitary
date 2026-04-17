package cmd

import (
	"fmt"
	"io"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

func renderCommandResult(w io.Writer, command string, result any) error {
	description := commandDescription(command)
	if description == "" {
		return fmt.Errorf("unknown command %q", command)
	}

	if !usesSemanticTextRendering(command) {
		fmt.Fprintf(w, "pituitary %s: %s\n", command, description)
	}

	switch typed := result.(type) {
	case *source.CanonicalizeResult:
		renderCanonicalizeResult(w, typed)
	case *source.NewSpecBundleResult:
		renderNewSpecBundleResult(w, typed)
	case *source.DiscoverResult:
		renderDiscoverResult(w, typed)
	case *initResult:
		renderInitResult(w, typed)
	case *migrateConfigResult:
		renderMigrateConfigResult(w, typed)
	case *index.RebuildResult:
		renderIndexResult(w, typed)
	case *statusResult:
		renderStatusResult(w, typed)
	case *versionResult:
		renderVersionResult(w, typed)
	case *source.PreviewResult:
		renderPreviewSourcesResult(w, typed)
	case *source.ExplainFileResult:
		renderExplainFileResult(w, typed)
	case *index.SearchSpecResult:
		renderSearchSpecsResult(w, typed)
	case *analysis.OverlapResult:
		renderOverlapResult(w, typed)
	case *analysis.CompareResult:
		renderCompareResult(w, typed)
	case *analysis.AnalyzeImpactResult:
		renderAnalyzeImpactResult(w, typed)
	case *analysis.TerminologyAuditResult:
		renderTerminologyAuditResult(w, typed)
	case *analysis.ComplianceResult:
		renderComplianceResult(w, typed)
	case *analysis.DocDriftResult:
		renderDocDriftResult(w, typed)
	case *analysis.FreshnessResult:
		renderFreshnessResult(w, typed)
	case *app.FixResult:
		renderFixResult(w, typed)
	case *app.CompileResult:
		renderCompileResult(w, typed)
	case *analysis.ReviewResult:
		renderReviewResult(w, typed)
	case *schemaCatalogResult:
		renderSchemaCatalogResult(w, typed)
	case *schemaCommandResult:
		renderSchemaCommandResult(w, typed)
	default:
		return fmt.Errorf("unsupported result type %T", result)
	}

	return nil
}

func usesSemanticTextRendering(command string) bool {
	switch command {
	case "check-doc-drift", "check-overlap", "review-spec", "check-compliance", "check-spec-freshness", "status", "init", "fix", "compile":
		return true
	default:
		return false
	}
}

func renderCommandTable(w io.Writer, command string, result any) error {
	description := commandDescription(command)
	if description == "" {
		return fmt.Errorf("unknown command %q", command)
	}

	fmt.Fprintf(w, "pituitary %s: %s\n", command, description)

	switch typed := result.(type) {
	case *index.SearchSpecResult:
		renderSearchSpecsTable(w, typed)
		return nil
	default:
		return fmt.Errorf("format %q is only supported for search-specs", "table")
	}
}

func renderCommandMarkdown(w io.Writer, command string, result any) error {
	switch typed := result.(type) {
	case *analysis.ReviewResult:
		if command != "review-spec" {
			return fmt.Errorf("format %q is only supported for review-spec", "markdown")
		}
		renderReviewMarkdown(w, typed)
		return nil
	default:
		return fmt.Errorf("format %q is only supported for review-spec", "markdown")
	}
}

func renderCommandHTML(w io.Writer, command string, result any) error {
	switch typed := result.(type) {
	case *analysis.ReviewResult:
		if command != "review-spec" {
			return fmt.Errorf("format %q is only supported for review-spec", "html")
		}
		renderReviewHTML(w, typed)
		return nil
	default:
		return fmt.Errorf("format %q is only supported for review-spec", "html")
	}
}
