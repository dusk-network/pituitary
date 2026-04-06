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
	"github.com/dusk-network/pituitary/internal/diag"
)

const (
	defaultConfigName  = "pituitary.toml"
	localConfigDirName = ".pituitary"
	configEnvVar       = "PITUITARY_CONFIG"
	logLevelEnvVar     = "PITUITARY_LOG_LEVEL"
	formatEnvVar       = "PITUITARY_FORMAT"

	colorModeAuto   = "auto"
	colorModeAlways = "always"
	colorModeNever  = "never"

	configSourceCommandFlag = "command_flag"
	configSourceGlobalFlag  = "global_flag"
	configSourceEnv         = "env"
	configSourceDiscovery   = "discovered_local"
)

type cliGlobalOptions struct {
	ConfigPath string
	ColorMode  string
	LogLevel   string
}

type configPathOption struct {
	Source string
	Path   string
}

type configResolution struct {
	WorkingDir              string                      `json:"working_dir"`
	SelectedBy              string                      `json:"selected_by,omitempty"`
	Reason                  string                      `json:"reason,omitempty"`
	Candidates              []configResolutionCandidate `json:"candidates"`
	ShadowedMultirepoConfig string                      `json:"shadowed_multirepo_config,omitempty"`
}

type configResolutionCandidate struct {
	Precedence int    `json:"precedence"`
	Source     string `json:"source"`
	Path       string `json:"path,omitempty"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
}

type cliConfigPathContextKey struct{}
type cliLoggerContextKey struct{}

func parseGlobalCLIOptions(args []string) (cliGlobalOptions, []string, error) {
	fs := flag.NewFlagSet("pituitary", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var options cliGlobalOptions
	fs.StringVar(&options.ConfigPath, "config", "", "path to workspace config")
	fs.StringVar(&options.ColorMode, "color", colorModeAuto, "terminal color: auto, always, or never")
	fs.StringVar(&options.LogLevel, "log-level", "", "diagnostic log level: off, error, warn, info, or debug")

	if err := fs.Parse(args); err != nil {
		return cliGlobalOptions{}, nil, err
	}

	options.ConfigPath = strings.TrimSpace(options.ConfigPath)
	options.ColorMode = strings.TrimSpace(strings.ToLower(options.ColorMode))
	if options.ColorMode == "" {
		options.ColorMode = colorModeAuto
	}
	switch options.ColorMode {
	case colorModeAuto, colorModeAlways, colorModeNever:
	default:
		return cliGlobalOptions{}, nil, fmt.Errorf("invalid --color value %q; expected auto, always, or never", options.ColorMode)
	}
	options.LogLevel = strings.TrimSpace(strings.ToLower(options.LogLevel))
	if options.LogLevel == "" {
		options.LogLevel = strings.TrimSpace(strings.ToLower(os.Getenv(logLevelEnvVar)))
	}
	if options.LogLevel == "" {
		options.LogLevel = "off"
	}
	if _, err := diag.ParseLevel(options.LogLevel); err != nil {
		return cliGlobalOptions{}, nil, err
	}
	return options, fs.Args(), nil
}

func defaultCommandFormat(fallback string) string {
	format := strings.TrimSpace(strings.ToLower(os.Getenv(formatEnvVar)))
	if format == "" {
		return fallback
	}
	return format
}

func defaultCommandFormatForWriter(w io.Writer, fallback string) string {
	if format := defaultCommandFormat(""); format != "" {
		return format
	}

	target := w
	if wrapped, ok := w.(cliPresentationWriter); ok {
		target = wrapped.cliUnderlyingWriter()
	}
	file, ok := target.(*os.File)
	if !ok {
		return fallback
	}
	info, err := file.Stat()
	if err != nil {
		return fallback
	}
	if (info.Mode() & os.ModeCharDevice) == 0 {
		return commandFormatJSON
	}
	return fallback
}

func withCLIConfigPath(ctx context.Context, configPath string) context.Context {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return ctx
	}
	return context.WithValue(ctx, cliConfigPathContextKey{}, configPath)
}

func cliConfigPathFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(cliConfigPathContextKey{}).(string)
	return strings.TrimSpace(value)
}

func withCLILogger(ctx context.Context, logger *diag.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, cliLoggerContextKey{}, logger)
}

func cliLoggerFromContext(ctx context.Context) *diag.Logger {
	if ctx == nil {
		return nil
	}
	logger, _ := ctx.Value(cliLoggerContextKey{}).(*diag.Logger)
	return logger
}

func resolveCommandConfigPath(ctx context.Context, localConfigPath string) (string, error) {
	resolvedPath, resolution, err := resolveCommandConfigPathWithResolution(ctx, localConfigPath)
	if err == nil && resolution != nil {
		emitMultirepoShadowWarning(resolution)
	}
	return resolvedPath, err
}

func emitMultirepoShadowWarning(resolution *configResolution) {
	if resolution == nil || resolution.ShadowedMultirepoConfig == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "warning: selected config shadows parent multirepo config %s\n", filepath.ToSlash(resolution.ShadowedMultirepoConfig))
}

func resolveCommandConfigPathWithResolution(ctx context.Context, localConfigPath string) (string, *configResolution, error) {
	return resolveCLIConfigPathWithResolution(
		configPathOption{Source: configSourceCommandFlag, Path: localConfigPath},
		configPathOption{Source: configSourceGlobalFlag, Path: cliConfigPathFromContext(ctx)},
	)
}

func resolveCLIConfigPathWithResolution(explicitOptions ...configPathOption) (string, *configResolution, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil, fmt.Errorf("resolve working directory: %w", err)
	}
	return resolveCLIConfigPathFromWorkingDir(cwd, explicitOptions...)
}

func resolveCLIConfigPathFromWorkingDir(cwd string, explicitOptions ...configPathOption) (string, *configResolution, error) {
	resolution := &configResolution{
		WorkingDir: filepath.ToSlash(cwd),
		Candidates: make([]configResolutionCandidate, 0, len(explicitOptions)+4),
	}

	for _, option := range explicitOptions {
		resolution.Candidates = append(resolution.Candidates, buildConfigResolutionCandidate(len(resolution.Candidates)+1, option.Source, option.Path))
	}
	resolution.Candidates = append(resolution.Candidates, buildConfigResolutionCandidate(len(resolution.Candidates)+1, configSourceEnv, os.Getenv(configEnvVar)))
	resolution.Candidates = append(resolution.Candidates, discoverCLIConfigCandidates(cwd, len(resolution.Candidates)+1)...)

	selectedIndex := -1
	for i := range resolution.Candidates {
		candidate := &resolution.Candidates[i]
		if candidate.Path == "" || candidate.Status == "missing" {
			continue
		}
		selectedIndex = i
		resolution.SelectedBy = candidate.Source
		candidate.Status = "selected"
		break
	}

	if selectedIndex == -1 {
		return "", resolution, fmt.Errorf(
			"no config found; set --config, set %s, or add %s or %s in %s or a parent directory",
			configEnvVar,
			filepath.ToSlash(filepath.Join(localConfigDirName, defaultConfigName)),
			defaultConfigName,
			filepath.ToSlash(cwd),
		)
	}

	selected := resolution.Candidates[selectedIndex]
	resolution.Reason = configResolutionReason(selected, resolution.Candidates)

	for i := range resolution.Candidates {
		if i == selectedIndex {
			continue
		}
		candidate := &resolution.Candidates[i]
		switch candidate.Status {
		case "missing", "not_set":
			continue
		default:
			candidate.Status = "shadowed"
			if candidate.Detail == "" {
				candidate.Detail = fmt.Sprintf("ignored because %s won first", configSourceLabel(selected.Source))
			}
		}
	}

	// Detect when the selected config shadows a parent multirepo config.
	if selected.Source == configSourceDiscovery {
		selectedSearchDir := discoverySearchDir(selected.Path)
		for i := range resolution.Candidates {
			candidate := &resolution.Candidates[i]
			if candidate.Source != configSourceDiscovery || candidate.Status != "shadowed" || strings.TrimSpace(candidate.Path) == "" {
				continue
			}
			if discoverySearchDir(candidate.Path) == selectedSearchDir {
				continue
			}
			isMultirepo, err := config.DeclaresMultirepoRepos(candidate.Path)
			if err != nil || !isMultirepo {
				continue
			}
			resolution.ShadowedMultirepoConfig = candidate.Path
			break
		}
	}

	return selected.Path, resolution, nil
}

func buildConfigResolutionCandidate(precedence int, source, rawPath string) configResolutionCandidate {
	trimmedPath := strings.TrimSpace(rawPath)
	candidate := configResolutionCandidate{
		Precedence: precedence,
		Source:     source,
		Path:       trimmedPath,
	}
	if trimmedPath == "" {
		candidate.Status = "not_set"
		candidate.Detail = configUnsetDetail(source)
		return candidate
	}
	candidate.Status = "available"
	candidate.Detail = configAvailableDetail(source)
	return candidate
}

func configUnsetDetail(source string) string {
	switch source {
	case configSourceCommandFlag:
		return "command-local --config was not provided"
	case configSourceGlobalFlag:
		return "global --config was not provided"
	case configSourceEnv:
		return fmt.Sprintf("%s is not set", configEnvVar)
	default:
		return "not set"
	}
}

func configAvailableDetail(source string) string {
	switch source {
	case configSourceCommandFlag:
		return "command-local --config is set"
	case configSourceGlobalFlag:
		return "global --config is set"
	case configSourceEnv:
		return fmt.Sprintf("%s is set", configEnvVar)
	default:
		return "available by precedence"
	}
}

func discoverCLIConfigCandidates(startDir string, startPrecedence int) []configResolutionCandidate {
	dir := filepath.Clean(startDir)
	candidates := make([]configResolutionCandidate, 0, 4)
	precedence := startPrecedence
	foundAny := false

	for {
		discoveryPaths := []string{
			filepath.Join(dir, localConfigDirName, defaultConfigName),
			filepath.Join(dir, defaultConfigName),
		}
		for _, candidatePath := range discoveryPaths {
			info, err := os.Stat(candidatePath)
			if err == nil && !info.IsDir() {
				foundAny = true
				candidates = append(candidates, configResolutionCandidate{
					Precedence: precedence,
					Source:     configSourceDiscovery,
					Path:       candidatePath,
					Status:     "available",
					Detail:     fmt.Sprintf("found during working-directory search in %s", filepath.ToSlash(dir)),
				})
				precedence++
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if foundAny {
		return candidates
	}

	return []configResolutionCandidate{
		{
			Precedence: startPrecedence,
			Source:     configSourceDiscovery,
			Path:       filepath.Join(startDir, localConfigDirName, defaultConfigName),
			Status:     "missing",
			Detail:     fmt.Sprintf("not present in %s; search continued to parent directories", filepath.ToSlash(startDir)),
		},
		{
			Precedence: startPrecedence + 1,
			Source:     configSourceDiscovery,
			Path:       filepath.Join(startDir, defaultConfigName),
			Status:     "missing",
			Detail:     fmt.Sprintf("not present in %s; search continued to parent directories", filepath.ToSlash(startDir)),
		},
	}
}

func configResolutionReason(selected configResolutionCandidate, all []configResolutionCandidate) string {
	switch selected.Source {
	case configSourceCommandFlag:
		return "command-local --config won by precedence"
	case configSourceGlobalFlag:
		return "global --config won by precedence"
	case configSourceEnv:
		return fmt.Sprintf("%s won before working-directory discovery", configEnvVar)
	case configSourceDiscovery:
		for _, candidate := range all {
			if candidate.Source != configSourceDiscovery || candidate.Path == selected.Path {
				continue
			}
			if discoverySearchDir(candidate.Path) == discoverySearchDir(selected.Path) {
				return fmt.Sprintf("working-directory search found %s before %s in %s", filepath.ToSlash(selected.Path), filepath.ToSlash(candidate.Path), filepath.ToSlash(discoverySearchDir(selected.Path)))
			}
		}
		return fmt.Sprintf("working-directory search found %s", filepath.ToSlash(selected.Path))
	default:
		return "selected by precedence"
	}
}

func discoverySearchDir(path string) string {
	dir := filepath.Dir(path)
	if filepath.Base(dir) == localConfigDirName {
		return filepath.Dir(dir)
	}
	return dir
}

func configSourceLabel(source string) string {
	switch source {
	case configSourceCommandFlag:
		return "command-local --config"
	case configSourceGlobalFlag:
		return "global --config"
	case configSourceEnv:
		return configEnvVar
	case configSourceDiscovery:
		return "working-directory search"
	default:
		return source
	}
}
