package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

type newRequest struct {
	Title     string `json:"title"`
	Domain    string `json:"domain,omitempty"`
	ID        string `json:"id,omitempty"`
	BundleDir string `json:"bundle_dir,omitempty"`
}

type newCommandDefaults struct {
	WorkspaceRoot string
	SpecRoot      string
	ConfigPath    string
	SourceName    string
	SourceHasFile bool
	Config        *config.Config
	Warnings      []source.NewSpecBundleWarning
}

func runNew(args []string, stdout, stderr io.Writer) int {
	return runNewContext(context.Background(), args, stdout, stderr)
}

func runNewContext(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	help := newCommandHelp("new", "pituitary [--config PATH] new --title TITLE [--domain DOMAIN] [--id ID] [--bundle-dir PATH] [--format FORMAT]")

	var (
		title     string
		domain    string
		id        string
		bundleDir string
		format    string
	)
	fs.StringVar(&title, "title", "", "spec title")
	fs.StringVar(&domain, "domain", "", "spec domain (defaults to unknown)")
	fs.StringVar(&id, "id", "", "explicit spec id")
	fs.StringVar(&bundleDir, "bundle-dir", "", "bundle directory to write")
	fs.StringVar(&format, "format", defaultCommandFormatForWriter(stdout, commandFormatText), "output format")

	if handled, err := parseCommandFlags(fs, args, stdout, help); err != nil {
		return writeCLIError(stdout, stderr, format, "new", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	} else if handled {
		return 0
	}
	if fs.NArg() != 0 {
		return writeCLIError(stdout, stderr, format, "new", nil, cliIssue{
			Code:    "validation_error",
			Message: fmt.Sprintf("unexpected positional arguments: %s", strings.Join(fs.Args(), " ")),
		}, 2)
	}
	if err := validateCLIFormat("new", format); err != nil {
		return writeCLIError(stdout, stderr, format, "new", nil, cliIssue{
			Code:    "validation_error",
			Message: err.Error(),
		}, 2)
	}

	request := newRequest{
		Title:     strings.TrimSpace(title),
		Domain:    strings.TrimSpace(domain),
		ID:        strings.TrimSpace(id),
		BundleDir: strings.TrimSpace(bundleDir),
	}
	if request.Title == "" {
		return writeCLIError(stdout, stderr, format, "new", request, cliIssue{
			Code:    "validation_error",
			Message: "--title is required",
		}, 2)
	}

	defaults, err := resolveNewCommandDefaults(ctx)
	if err != nil {
		return writeCLIError(stdout, stderr, format, "new", request, cliIssue{
			Code:    "config_error",
			Message: err.Error(),
		}, 2)
	}

	result, err := source.NewSpecBundle(source.NewSpecBundleOptions{
		WorkspaceRoot: defaults.WorkspaceRoot,
		SpecRoot:      defaults.SpecRoot,
		BundleDir:     request.BundleDir,
		ID:            request.ID,
		Title:         request.Title,
		Domain:        request.Domain,
	})
	if err != nil {
		return writeCLIError(stdout, stderr, format, "new", request, cliIssue{
			Code:    "new_error",
			Message: err.Error(),
		}, 2)
	}
	if defaults.ConfigPath != "" {
		result.ConfigPath = filepath.ToSlash(defaults.ConfigPath)
	}
	// Run semantic overlap pre-check if index is available.
	if defaults.Config != nil {
		overlapWarnings := checkNewSpecOverlap(ctx, defaults.Config, request.Title)
		result.Warnings = append(result.Warnings, overlapWarnings...)
	}
	result.Warnings = append(result.Warnings, defaults.Warnings...)
	appendNewConfigWarnings(result, defaults)

	return writeCLISuccess(stdout, stderr, format, "new", request, result, nil)
}

func resolveNewCommandDefaults(ctx context.Context) (newCommandDefaults, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return newCommandDefaults{}, fmt.Errorf("resolve working directory: %w", err)
	}
	cwd, err = filepath.Abs(cwd)
	if err != nil {
		return newCommandDefaults{}, fmt.Errorf("resolve workspace root: %w", err)
	}

	defaults := newCommandDefaults{
		WorkspaceRoot: cwd,
		SpecRoot:      filepath.Join(cwd, "specs"),
	}

	resolvedConfigPath, resolution, err := resolveCommandConfigPathWithResolution(ctx, "")
	if err != nil {
		return defaults, nil
	}

	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		if resolution != nil && resolution.SelectedBy != configSourceDiscovery {
			return newCommandDefaults{}, err
		}
		defaults.WorkspaceRoot = workspaceRootForConfigPath(resolvedConfigPath)
		defaults.SpecRoot = filepath.Join(defaults.WorkspaceRoot, "specs")
		defaults.Warnings = append(defaults.Warnings, source.NewSpecBundleWarning{
			Code:    "config_fallback",
			Message: fmt.Sprintf("ignoring discovered config %s: %v; using default spec root %s", filepath.ToSlash(resolvedConfigPath), err, filepath.ToSlash(defaults.SpecRoot)),
		})
		return defaults, nil
	}

	defaults.WorkspaceRoot = cfg.Workspace.RootPath
	defaults.Config = cfg
	defaults.ConfigPath = resolvedConfigPath
	selectedSource := firstFilesystemSpecSource(cfg)
	if selectedSource == nil {
		defaults.SpecRoot = filepath.Join(cfg.Workspace.RootPath, "specs")
		defaults.Warnings = append(defaults.Warnings, source.NewSpecBundleWarning{
			Code:    "spec_source_fallback",
			Message: fmt.Sprintf("config %s has no filesystem spec_bundle source; using default spec root %s", filepath.ToSlash(resolvedConfigPath), filepath.ToSlash(defaults.SpecRoot)),
		})
		return defaults, nil
	}

	defaults.SpecRoot = selectedSource.ResolvedPath
	defaults.SourceName = selectedSource.Name
	defaults.SourceHasFile = len(selectedSource.Files) > 0
	return defaults, nil
}

func firstFilesystemSpecSource(cfg *config.Config) *config.Source {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Sources {
		source := &cfg.Sources[i]
		if source.Adapter == config.AdapterFilesystem && source.Kind == config.SourceKindSpecBundle {
			return source
		}
	}
	return nil
}

func workspaceRootForConfigPath(configPath string) string {
	configPath = filepath.Clean(configPath)
	configDir := filepath.Dir(configPath)
	if filepath.Base(configPath) == defaultConfigName && filepath.Base(configDir) == localConfigDirName {
		return filepath.Dir(configDir)
	}
	return configDir
}

func appendNewConfigWarnings(result *source.NewSpecBundleResult, defaults newCommandDefaults) {
	if result == nil || defaults.ConfigPath == "" || defaults.SourceName == "" {
		return
	}

	bundlePath := filepath.Join(defaults.WorkspaceRoot, filepath.FromSlash(result.BundleDir))
	specPath := filepath.Join(bundlePath, "spec.toml")
	if !pathWithinRootNew(defaults.SpecRoot, bundlePath) {
		result.Warnings = append(result.Warnings, source.NewSpecBundleWarning{
			Code:    "outside_config_scope",
			Message: fmt.Sprintf("bundle %s is outside configured spec source %q (%s); it will not be indexed until config changes", filepath.ToSlash(result.BundleDir), defaults.SourceName, filepath.ToSlash(defaults.SpecRoot)),
		})
		return
	}
	if !defaults.SourceHasFile {
		return
	}

	relSpecPath, err := filepath.Rel(defaults.SpecRoot, specPath)
	if err != nil {
		return
	}
	result.Warnings = append(result.Warnings, source.NewSpecBundleWarning{
		Code:    "config_files_selector",
		Message: fmt.Sprintf("config source %q uses explicit files selectors; add %q if you want this bundle indexed", defaults.SourceName, filepath.ToSlash(relSpecPath)),
	})
}

func pathWithinRootNew(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func checkNewSpecOverlap(ctx context.Context, cfg *config.Config, title string) []source.NewSpecBundleWarning {
	searchResult, err := index.SearchSpecsContext(ctx, cfg, index.SearchSpecQuery{
		Query:    title,
		Kind:     "spec",
		Statuses: []string{"draft", "review", "accepted"},
		Limit:    3,
	})
	if err != nil || searchResult == nil {
		return nil
	}

	const overlapThreshold = 0.50
	var warnings []source.NewSpecBundleWarning
	for _, match := range searchResult.Matches {
		if match.Score >= overlapThreshold {
			warnings = append(warnings, source.NewSpecBundleWarning{
				Code:    "similar_spec_exists",
				Message: fmt.Sprintf("existing spec %s %q has %.0f%% semantic similarity; review before proceeding", match.Ref, match.Title, match.Score*100),
			})
		}
	}
	return warnings
}
