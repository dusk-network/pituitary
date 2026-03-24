package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultConfigName  = "pituitary.toml"
	localConfigDirName = ".pituitary"
	configEnvVar       = "PITUITARY_CONFIG"

	configSourceCommandFlag = "command_flag"
	configSourceGlobalFlag  = "global_flag"
	configSourceEnv         = "env"
	configSourceDiscovery   = "discovered_local"
)

type cliGlobalOptions struct {
	ConfigPath string
}

type configPathOption struct {
	Source string
	Path   string
}

type configResolution struct {
	WorkingDir string                      `json:"working_dir"`
	SelectedBy string                      `json:"selected_by,omitempty"`
	Reason     string                      `json:"reason,omitempty"`
	Candidates []configResolutionCandidate `json:"candidates"`
}

type configResolutionCandidate struct {
	Precedence int    `json:"precedence"`
	Source     string `json:"source"`
	Path       string `json:"path,omitempty"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
}

type cliConfigPathContextKey struct{}

func parseGlobalCLIOptions(args []string) (cliGlobalOptions, []string, error) {
	fs := flag.NewFlagSet("pituitary", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var options cliGlobalOptions
	fs.StringVar(&options.ConfigPath, "config", "", "path to workspace config")

	if err := fs.Parse(args); err != nil {
		return cliGlobalOptions{}, nil, err
	}

	options.ConfigPath = strings.TrimSpace(options.ConfigPath)
	return options, fs.Args(), nil
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

func resolveCommandConfigPath(ctx context.Context, localConfigPath string) (string, error) {
	resolvedPath, _, err := resolveCommandConfigPathWithResolution(ctx, localConfigPath)
	return resolvedPath, err
}

func resolveCommandConfigPathWithResolution(ctx context.Context, localConfigPath string) (string, *configResolution, error) {
	return resolveCLIConfigPathWithResolution(
		configPathOption{Source: configSourceCommandFlag, Path: localConfigPath},
		configPathOption{Source: configSourceGlobalFlag, Path: cliConfigPathFromContext(ctx)},
	)
}

func resolveCLIConfigPath(explicitPaths ...string) (string, error) {
	options := make([]configPathOption, 0, len(explicitPaths))
	for i, explicitPath := range explicitPaths {
		options = append(options, configPathOption{
			Source: fmt.Sprintf("explicit_%d", i+1),
			Path:   explicitPath,
		})
	}
	resolvedPath, _, err := resolveCLIConfigPathWithResolution(options...)
	return resolvedPath, err
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

	return selected.Path, resolution, nil
}

func discoverCLIConfigPath(startDir string) (string, bool) {
	dir := filepath.Clean(startDir)
	for {
		for _, candidate := range []string{
			filepath.Join(dir, localConfigDirName, defaultConfigName),
			filepath.Join(dir, defaultConfigName),
		} {
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() {
				return candidate, true
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
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
