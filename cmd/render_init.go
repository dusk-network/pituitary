package cmd

import (
	"fmt"
	"io"
)

func renderInitResult(w io.Writer, result *initResult) {
	p := presentationForWriter(w)
	fmt.Fprintln(w, p.headerLine("init", ""))
	fmt.Fprintln(w)

	sourceCount := 0
	if result.Discover != nil {
		sourceCount = len(result.Discover.Sources)
	}
	artifactCount := 0
	chunkCount := 0
	if result.Index != nil {
		artifactCount = result.Index.ArtifactCount
		chunkCount = result.Index.ChunkCount
	}
	specCount := 0
	docCount := 0
	freshness := "unknown"
	embedderProvider := ""
	if result.Status != nil {
		specCount = result.Status.SpecCount
		docCount = result.Status.DocCount
		if result.Status.Freshness != nil && result.Status.Freshness.State != "" {
			freshness = result.Status.Freshness.State
		}
		embedderProvider = result.Status.EmbedderProvider
	}
	fmt.Fprintf(
		w,
		"  %s sources  %s artifacts  %s chunks  %s  %s\n",
		p.bold(fmt.Sprintf("%d", sourceCount)),
		p.bold(fmt.Sprintf("%d", artifactCount)),
		p.bold(fmt.Sprintf("%d", chunkCount)),
		renderFreshnessLabel(p, freshness),
		p.dim(statusEmbedderSummary(embedderProvider)),
	)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s %s\n", p.dim("workspace:"), result.WorkspaceRoot)
	fmt.Fprintf(w, "  %s %s\n", p.dim("config:"), result.ConfigPath)
	fmt.Fprintf(w, "  %s %s\n", p.dim("action:"), result.ConfigAction)
	fmt.Fprintf(w, "  %s %d specs · %d docs\n", p.dim("index:"), specCount, docCount)
	if result.Status != nil {
		for _, guidance := range result.Status.Guidance {
			fmt.Fprintf(w, "  %s %s\n", p.arrow(), guidance)
		}
	}
	if result.ConfigAction == "preview" {
		fmt.Fprintf(w, "  %s %s\n", p.arrow(), "run `pituitary init` without --dry-run to write the config and build the index")
	} else if specCount > 0 && docCount > 0 {
		fmt.Fprintf(w, "\n  %s run `pituitary check-doc-drift --scope all` to see findings\n", p.arrow())
	}
}

func renderMigrateConfigResult(w io.Writer, result *migrateConfigResult) {
	fmt.Fprintf(w, "config path: %s\n", result.ConfigPath)
	fmt.Fprintf(w, "detected schema: %s\n", result.DetectedSchema)
	fmt.Fprintf(w, "target schema_version: %d\n", result.TargetSchemaVersion)
	if result.WroteConfig {
		fmt.Fprintln(w, "config write: wrote migrated config")
	} else {
		fmt.Fprintln(w, "config write: skipped")
	}
	for _, note := range result.Notes {
		fmt.Fprintf(w, "note: %s\n", note)
	}
	fmt.Fprintln(w, "migrated config:")
	fmt.Fprint(w, result.Config)
}

func renderVersionResult(w io.Writer, result *versionResult) {
	fmt.Fprintf(w, "version: %s\n", result.Version)
	fmt.Fprintf(w, "go version: %s\n", result.GoVersion)
	if result.Commit != "" {
		fmt.Fprintf(w, "commit: %s\n", result.Commit)
	}
	if result.BuildDate != "" {
		fmt.Fprintf(w, "build date: %s\n", result.BuildDate)
	}
}
