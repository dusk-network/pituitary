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
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/runtimeprobe"
)

type statusRequest struct {
	CheckRuntime string `json:"check_runtime,omitempty"`
}

type statusResult struct {
	WorkspaceRoot     string                  `json:"workspace_root"`
	ConfigPath        string                  `json:"config_path"`
	ConfigResolution  *configResolution       `json:"config_resolution,omitempty"`
	IndexPath         string                  `json:"index_path"`
	IndexExists       bool                    `json:"index_exists"`
	SpecCount         int                     `json:"spec_count"`
	DocCount          int                     `json:"doc_count"`
	ChunkCount        int                     `json:"chunk_count"`
	ArtifactLocations *statusArtifactLocation `json:"artifact_locations,omitempty"`
	Runtime           *runtimeprobe.Result    `json:"runtime,omitempty"`
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
	fs.StringVar(&format, "format", "text", "output format")
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

	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "status", request, cliIssue{
			Code:    "config_error",
			Message: "invalid config:\n" + err.Error(),
		}, 2)
	}

	status, err := index.ReadStatusContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "status", request, cliIssue{
			Code:    "index_error",
			Message: "inspect index failed:\n" + err.Error(),
		}, 2)
	}

	var runtimeResult *runtimeprobe.Result
	if scope != runtimeprobe.ScopeNone {
		runtimeResult, err = runtimeprobe.Run(ctx, cfg, scope)
		if err != nil {
			code := "internal_error"
			exitCode := 2
			message := err.Error()
			if index.IsDependencyUnavailable(err) {
				code = app.CodeDependencyUnavailable
				exitCode = 3
				message = app.FormatDependencyUnavailableMessage(cfg, err)
			}
			return writeCLIError(stdout, stderr, format, "status", request, cliIssue{
				Code:    code,
				Message: message,
			}, exitCode)
		}
	}

	return writeCLISuccess(stdout, stderr, format, "status", request, &statusResult{
		WorkspaceRoot:     cfg.Workspace.RootPath,
		ConfigPath:        cfg.ConfigPath,
		ConfigResolution:  resolution,
		IndexPath:         status.IndexPath,
		IndexExists:       status.Exists,
		SpecCount:         status.SpecCount,
		DocCount:          status.DocCount,
		ChunkCount:        status.ChunkCount,
		ArtifactLocations: buildStatusArtifactLocations(cfg),
		Runtime:           runtimeResult,
	}, nil)
}

func buildStatusArtifactLocations(cfg *config.Config) *statusArtifactLocation {
	if cfg == nil {
		return nil
	}

	workspaceRoot := cfg.Workspace.RootPath
	indexDir := filepath.Dir(cfg.Workspace.ResolvedIndexPath)
	discoverConfigPath := filepath.Join(workspaceRoot, localConfigDirName, defaultConfigName)
	canonicalizeBundleRoot := filepath.Join(workspaceRoot, localConfigDirName, "canonicalized")

	ignoreSet := map[string]struct{}{
		filepath.ToSlash(localConfigDirName) + "/": {},
	}
	indexPattern := relativeStatusPath(workspaceRoot, cfg.Workspace.ResolvedIndexPath)
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
