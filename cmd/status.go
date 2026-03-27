package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/runtimeprobe"
)

type statusRequest struct {
	CheckRuntime string `json:"check_runtime,omitempty"`
}

type statusResult struct {
	WorkspaceRoot     string                     `json:"workspace_root"`
	ConfigPath        string                     `json:"config_path"`
	EmbedderProvider  string                     `json:"embedder_provider,omitempty"`
	AnalysisProvider  string                     `json:"analysis_provider,omitempty"`
	ConfigResolution  *configResolution          `json:"config_resolution,omitempty"`
	IndexPath         string                     `json:"index_path"`
	IndexExists       bool                       `json:"index_exists"`
	Freshness         *index.FreshnessStatus     `json:"freshness,omitempty"`
	SpecCount         int                        `json:"spec_count"`
	DocCount          int                        `json:"doc_count"`
	ChunkCount        int                        `json:"chunk_count"`
	ArtifactLocations *statusArtifactLocation    `json:"artifact_locations,omitempty"`
	RelationGraph     *index.RelationGraphStatus `json:"relation_graph,omitempty"`
	Runtime           *runtimeprobe.Result       `json:"runtime,omitempty"`
	Guidance          []string                   `json:"guidance,omitempty"`
}

type statusArtifactLocation struct {
	IndexDir               string   `json:"index_dir"`
	DiscoverConfigPath     string   `json:"discover_config_path"`
	CanonicalizeBundleRoot string   `json:"canonicalize_bundle_root"`
	IgnorePatterns         []string `json:"ignore_patterns,omitempty"`
	RelocationHints        []string `json:"relocation_hints,omitempty"`
}

func runStatus(args []string, stdout, stderr io.Writer) int {
	return runStatusContext(context.Background(), args, stdout, stderr)
}

func runStatusContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("status", "pituitary [--config PATH] status [--format FORMAT] [--check-runtime SCOPE]")

	var (
		format       string
		configPath   string
		checkRuntime string
	)
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")
	fs.StringVar(&checkRuntime, "check-runtime", string(runtimeprobe.ScopeNone), "runtime probe scope: none, embedder, analysis, or all")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "status", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "status", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}
	if err := validateCLIFormat("status", format); err != nil {
		return writeCLIError(stdout, stderr, format, "status", statusRequest{}, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	scope, err := runtimeprobe.ParseScope(checkRuntime)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "status", statusRequest{}, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	request := statusRequest{}
	if scope != runtimeprobe.ScopeNone {
		request.CheckRuntime = string(scope)
	}

	resolvedConfigPath, resolution, err := resolveCommandConfigPathWithResolution(ctx, configPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "status", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	response := app.Status(ctx, resolvedConfigPath, app.StatusRequest{CheckRuntime: scope})
	if response.Issue != nil {
		return writeCLIError(stdout, stderr, format, "status", request, cliIssue{
			Code:    response.Issue.Code,
			Message: response.Issue.Message,
		}, response.Issue.ExitCode)
	}
	result := response.Result

	return writeCLISuccess(stdout, stderr, format, "status", request, newStatusResult(result, resolution), nil)
}

func newStatusResult(result *app.StatusResult, resolution *configResolution) *statusResult {
	if result == nil || result.Index == nil {
		return nil
	}
	return &statusResult{
		WorkspaceRoot:     result.WorkspaceRoot,
		ConfigPath:        result.ConfigPath,
		EmbedderProvider:  result.EmbedderProvider,
		AnalysisProvider:  result.AnalysisProvider,
		ConfigResolution:  resolution,
		IndexPath:         result.Index.IndexPath,
		IndexExists:       result.Index.Exists,
		Freshness:         result.Freshness,
		SpecCount:         result.Index.SpecCount,
		DocCount:          result.Index.DocCount,
		ChunkCount:        result.Index.ChunkCount,
		ArtifactLocations: buildStatusArtifactLocations(result.WorkspaceRoot, result.Index.IndexPath),
		RelationGraph:     result.RelationGraph,
		Runtime:           result.Runtime,
		Guidance:          append([]string(nil), result.Guidance...),
	}
}

func buildStatusArtifactLocations(workspaceRoot, indexPath string) *statusArtifactLocation {
	if strings.TrimSpace(workspaceRoot) == "" || strings.TrimSpace(indexPath) == "" {
		return nil
	}

	indexDir := filepath.Dir(indexPath)
	discoverConfigPath := filepath.Join(workspaceRoot, localConfigDirName, defaultConfigName)
	canonicalizeBundleRoot := filepath.Join(workspaceRoot, localConfigDirName, "canonicalized")

	ignoreSet := map[string]struct{}{
		filepath.ToSlash(localConfigDirName) + "/": {},
	}
	indexPattern := relativeStatusPath(workspaceRoot, indexPath)
	if indexPattern != "" && indexPattern != "." && !strings.HasPrefix(indexPattern, filepath.ToSlash(localConfigDirName)+"/") {
		ignoreSet[indexPattern] = struct{}{}
	}
	ignorePatterns := make([]string, 0, len(ignoreSet))
	for pattern := range ignoreSet {
		ignorePatterns = append(ignorePatterns, pattern)
	}
	sort.Strings(ignorePatterns)

	return &statusArtifactLocation{
		IndexDir:               indexDir,
		DiscoverConfigPath:     discoverConfigPath,
		CanonicalizeBundleRoot: canonicalizeBundleRoot,
		IgnorePatterns:         ignorePatterns,
		RelocationHints: []string{
			"set [workspace].index_path to move the SQLite index",
			"use `pituitary discover --config-path PATH --write` to place generated config elsewhere",
			"use `pituitary canonicalize --bundle-dir PATH` to place generated bundles elsewhere",
		},
	}
}

func relativeStatusPath(root, path string) string {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(path) == "" {
		return ""
	}
	relativePath, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(relativePath)
}
