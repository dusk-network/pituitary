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
	RuntimeConfig     *statusRuntimeConfig       `json:"runtime_config,omitempty"`
	ConfigResolution  *configResolution          `json:"config_resolution,omitempty"`
	IndexPath         string                     `json:"index_path"`
	IndexExists       bool                       `json:"index_exists"`
	Freshness         *index.FreshnessStatus     `json:"freshness,omitempty"`
	SpecCount         int                        `json:"spec_count"`
	DocCount          int                        `json:"doc_count"`
	ChunkCount        int                        `json:"chunk_count"`
	Repos             []index.RepoCoverage       `json:"repo_coverage,omitempty"`
	ArtifactLocations *statusArtifactLocation    `json:"artifact_locations,omitempty"`
	RelationGraph     *index.RelationGraphStatus `json:"relation_graph,omitempty"`
	Families          *index.FamilyResult        `json:"families,omitempty"`
	Runtime           *runtimeprobe.Result       `json:"runtime,omitempty"`
	Guidance          []string                   `json:"guidance,omitempty"`
}

type statusRuntimeConfig struct {
	Embedder statusRuntimeProvider `json:"embedder"`
	Analysis statusRuntimeProvider `json:"analysis"`
}

type statusRuntimeProvider struct {
	Profile    string `json:"profile,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	TimeoutMS  int    `json:"timeout_ms,omitempty"`
	MaxRetries int    `json:"max_retries,omitempty"`
}

type statusArtifactLocation struct {
	IndexDir               string   `json:"index_dir"`
	DiscoverConfigPath     string   `json:"discover_config_path"`
	CanonicalizeBundleRoot string   `json:"canonicalize_bundle_root"`
	IgnorePatterns         []string `json:"ignore_patterns,omitempty"`
	RelocationHints        []string `json:"relocation_hints,omitempty"`
	MultirepoParent        string   `json:"multirepo_parent,omitempty"`
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
		showFamilies bool
	)
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")
	fs.StringVar(&configPath, "config", "", "path to workspace config")
	fs.StringVar(&checkRuntime, "check-runtime", string(runtimeprobe.ScopeNone), "runtime probe scope: none, embedder, analysis, or all")
	fs.BoolVar(&showFamilies, "show-families", false, "discover and display spec families via dependency graph clustering")

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
		return writeCLIError(stdout, stderr, format, "status", request, cliIssueFromAppIssue(response.Issue), response.Issue.ExitCode)
	}
	result := response.Result

	statusOut := newStatusResult(result, resolution)
	if showFamilies && statusOut != nil && statusOut.IndexExists {
		familyResult, err := index.DiscoverFamiliesContext(ctx, result.Index.IndexPath)
		if err == nil {
			statusOut.Families = familyResult
		}
	}

	return writeCLISuccess(stdout, stderr, format, "status", request, statusOut, nil)
}

func newStatusResult(result *app.StatusResult, resolution *configResolution) *statusResult {
	if result == nil || result.Index == nil {
		return nil
	}
	guidance := append([]string(nil), result.Guidance...)
	guidance = append(guidance, statusResolutionGuidance(result.ConfigPath, resolution)...)
	return &statusResult{
		WorkspaceRoot:     result.WorkspaceRoot,
		ConfigPath:        result.ConfigPath,
		EmbedderProvider:  result.EmbedderProvider,
		AnalysisProvider:  result.AnalysisProvider,
		RuntimeConfig:     newStatusRuntimeConfig(result.RuntimeConfig),
		ConfigResolution:  resolution,
		IndexPath:         result.Index.IndexPath,
		IndexExists:       result.Index.Exists,
		Freshness:         result.Freshness,
		SpecCount:         result.Index.SpecCount,
		DocCount:          result.Index.DocCount,
		ChunkCount:        result.Index.ChunkCount,
		Repos:             append([]index.RepoCoverage(nil), result.Index.Repos...),
		ArtifactLocations: buildStatusArtifactLocations(result.WorkspaceRoot, result.ConfigPath, result.Index.IndexPath, resolution),
		RelationGraph:     result.RelationGraph,
		Runtime:           result.Runtime,
		Guidance:          guidance,
	}
}

func newStatusRuntimeConfig(runtimeConfig *app.RuntimeConfigStatus) *statusRuntimeConfig {
	if runtimeConfig == nil {
		return nil
	}
	return &statusRuntimeConfig{
		Embedder: statusRuntimeProvider{
			Profile:    runtimeConfig.Embedder.Profile,
			Provider:   runtimeConfig.Embedder.Provider,
			Model:      runtimeConfig.Embedder.Model,
			Endpoint:   runtimeConfig.Embedder.Endpoint,
			TimeoutMS:  runtimeConfig.Embedder.TimeoutMS,
			MaxRetries: runtimeConfig.Embedder.MaxRetries,
		},
		Analysis: statusRuntimeProvider{
			Profile:    runtimeConfig.Analysis.Profile,
			Provider:   runtimeConfig.Analysis.Provider,
			Model:      runtimeConfig.Analysis.Model,
			Endpoint:   runtimeConfig.Analysis.Endpoint,
			TimeoutMS:  runtimeConfig.Analysis.TimeoutMS,
			MaxRetries: runtimeConfig.Analysis.MaxRetries,
		},
	}
}

func buildStatusArtifactLocations(workspaceRoot, configPath, indexPath string, resolution *configResolution) *statusArtifactLocation {
	if strings.TrimSpace(workspaceRoot) == "" || strings.TrimSpace(indexPath) == "" {
		return nil
	}

	indexDir := filepath.Dir(indexPath)
	artifactBaseDir := statusArtifactBaseDir(workspaceRoot, configPath)
	discoverConfigPath := filepath.Join(artifactBaseDir, defaultConfigName)
	canonicalizeBundleRoot := filepath.Join(artifactBaseDir, "canonicalized")

	ignoreSet := map[string]struct{}{}
	if cliPathWithinRoot(workspaceRoot, artifactBaseDir) {
		artifactPattern := relativeStatusPath(workspaceRoot, artifactBaseDir)
		if artifactPattern != "" && artifactPattern != "." {
			ignoreSet[strings.TrimSuffix(filepath.ToSlash(artifactPattern), "/")+"/"] = struct{}{}
		}
	}
	if cliPathWithinRoot(workspaceRoot, indexPath) {
		indexPattern := relativeStatusPath(workspaceRoot, indexPath)
		if indexPattern != "" && indexPattern != "." {
			ignoreSet[indexPattern] = struct{}{}
		}
	}
	ignorePatterns := make([]string, 0, len(ignoreSet))
	for pattern := range ignoreSet {
		ignorePatterns = append(ignorePatterns, pattern)
	}
	sort.Strings(ignorePatterns)

	loc := &statusArtifactLocation{
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
	if resolution != nil && resolution.ShadowedMultirepoConfig != "" {
		loc.MultirepoParent = filepath.ToSlash(resolution.ShadowedMultirepoConfig)
	}
	return loc
}

func statusArtifactBaseDir(workspaceRoot, configPath string) string {
	defaultBase := filepath.Join(workspaceRoot, localConfigDirName)
	if strings.TrimSpace(configPath) == "" {
		return defaultBase
	}
	configDir := filepath.Dir(configPath)
	if filepath.Base(configPath) == defaultConfigName && filepath.Base(configDir) == localConfigDirName {
		return configDir
	}
	return defaultBase
}

func statusResolutionGuidance(selectedConfigPath string, resolution *configResolution) []string {
	if resolution == nil || resolution.ShadowedMultirepoConfig == "" {
		return nil
	}
	return []string{
		fmt.Sprintf(
			"selected config %s shadows parent multirepo config %s; use `pituitary --config %s ...` when you intend to operate on the shared workspace",
			filepath.ToSlash(selectedConfigPath),
			filepath.ToSlash(resolution.ShadowedMultirepoConfig),
			filepath.ToSlash(resolution.ShadowedMultirepoConfig),
		),
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
