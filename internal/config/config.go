package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	AdapterFilesystem      = "filesystem"
	SourceKindSpecBundle   = "spec_bundle"
	SourceKindMarkdownDocs = "markdown_docs"
)

// Config is the validated workspace configuration resolved from pituitary.toml.
type Config struct {
	ConfigPath string
	ConfigDir  string
	Workspace  Workspace
	Runtime    Runtime
	Sources    []Source
}

// Workspace describes the configured workspace root and index path.
type Workspace struct {
	Root              string
	RootPath          string
	IndexPath         string
	ResolvedIndexPath string
}

// Source describes one configured input source.
type Source struct {
	Name         string
	Adapter      string
	Kind         string
	Path         string
	ResolvedPath string
}

// Runtime captures provider configuration needed by later pipeline stages.
type Runtime struct {
	Embedder RuntimeProvider
	Analysis RuntimeProvider
}

// RuntimeProvider describes one configured runtime dependency.
type RuntimeProvider struct {
	Provider   string
	Model      string
	Endpoint   string
	APIKeyEnv  string
	TimeoutMS  int
	MaxRetries int
}

type rawConfig struct {
	workspace       rawWorkspace
	runtimeEmbedder rawRuntimeProvider
	runtimeAnalysis rawRuntimeProvider
	sources         []rawSource
}

type rawWorkspace struct {
	root      string
	indexPath string
}

type rawSource struct {
	name    string
	adapter string
	kind    string
	paths   string
}

type rawRuntimeProvider struct {
	provider   string
	model      string
	endpoint   string
	apiKeyEnv  string
	timeoutMS  int
	maxRetries int
}

type validationErrors struct {
	items []string
}

func (v *validationErrors) add(format string, args ...any) {
	v.items = append(v.items, fmt.Sprintf(format, args...))
}

func (v *validationErrors) err() error {
	if len(v.items) == 0 {
		return nil
	}
	return errors.New(strings.Join(v.items, "\n"))
}

// Load parses and validates a repo-local pituitary.toml file.
func Load(path string) (*Config, error) {
	configPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	file, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	raw, err := parse(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}

	cfg := &Config{
		ConfigPath: configPath,
		ConfigDir:  filepath.Dir(configPath),
		Workspace: Workspace{
			Root:      raw.workspace.root,
			IndexPath: raw.workspace.indexPath,
		},
		Runtime: Runtime{
			Embedder: RuntimeProvider{
				Provider:   defaultString(raw.runtimeEmbedder.provider, "fixture"),
				Model:      defaultString(raw.runtimeEmbedder.model, "fixture-8d"),
				Endpoint:   raw.runtimeEmbedder.endpoint,
				APIKeyEnv:  raw.runtimeEmbedder.apiKeyEnv,
				TimeoutMS:  defaultInt(raw.runtimeEmbedder.timeoutMS, 1000),
				MaxRetries: raw.runtimeEmbedder.maxRetries,
			},
			Analysis: RuntimeProvider{
				Provider:   defaultString(raw.runtimeAnalysis.provider, "disabled"),
				Model:      raw.runtimeAnalysis.model,
				Endpoint:   raw.runtimeAnalysis.endpoint,
				APIKeyEnv:  raw.runtimeAnalysis.apiKeyEnv,
				TimeoutMS:  defaultInt(raw.runtimeAnalysis.timeoutMS, 1000),
				MaxRetries: raw.runtimeAnalysis.maxRetries,
			},
		},
		Sources: make([]Source, 0, len(raw.sources)),
	}
	for _, source := range raw.sources {
		cfg.Sources = append(cfg.Sources, Source{
			Name:    source.name,
			Adapter: source.adapter,
			Kind:    source.kind,
			Path:    source.paths,
		})
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func parse(file *os.File) (rawConfig, error) {
	var cfg rawConfig
	var section string
	var currentSource *rawSource

	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "[[") && strings.HasSuffix(line, "]]"):
			name := strings.TrimSpace(line[2 : len(line)-2])
			if name != "sources" {
				return rawConfig{}, fmt.Errorf("line %d: unsupported array section %q", lineNo, name)
			}
			cfg.sources = append(cfg.sources, rawSource{})
			currentSource = &cfg.sources[len(cfg.sources)-1]
			section = name
			continue
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			name := strings.TrimSpace(line[1 : len(line)-1])
			switch name {
			case "workspace", "runtime.embedder", "runtime.analysis":
				section = name
				currentSource = nil
			default:
				return rawConfig{}, fmt.Errorf("line %d: unsupported section %q", lineNo, name)
			}
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return rawConfig{}, fmt.Errorf("line %d: expected key = value", lineNo)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch section {
		case "workspace":
			if err := parseWorkspaceField(&cfg.workspace, key, value, lineNo); err != nil {
				return rawConfig{}, err
			}
		case "runtime.embedder":
			if err := parseRuntimeField(&cfg.runtimeEmbedder, key, value, lineNo, section); err != nil {
				return rawConfig{}, err
			}
		case "runtime.analysis":
			if err := parseRuntimeField(&cfg.runtimeAnalysis, key, value, lineNo, section); err != nil {
				return rawConfig{}, err
			}
		case "sources":
			if currentSource == nil {
				return rawConfig{}, fmt.Errorf("line %d: source entry missing array header", lineNo)
			}
			if err := parseSourceField(currentSource, key, value, lineNo); err != nil {
				return rawConfig{}, err
			}
		default:
			return rawConfig{}, fmt.Errorf("line %d: key %q is outside a supported section", lineNo, key)
		}
	}

	if err := scanner.Err(); err != nil {
		return rawConfig{}, fmt.Errorf("read config: %w", err)
	}
	return cfg, nil
}

func parseWorkspaceField(workspace *rawWorkspace, key, value string, lineNo int) error {
	parsed, err := parseQuotedString(value)
	if err != nil {
		return fmt.Errorf("line %d: workspace.%s: %w", lineNo, key, err)
	}

	switch key {
	case "root":
		workspace.root = parsed
	case "index_path":
		workspace.indexPath = parsed
	default:
		return fmt.Errorf("line %d: unsupported workspace field %q", lineNo, key)
	}
	return nil
}

func parseSourceField(source *rawSource, key, value string, lineNo int) error {
	parsed, err := parseQuotedString(value)
	if err != nil {
		return fmt.Errorf("line %d: sources.%s: %w", lineNo, key, err)
	}

	switch key {
	case "name":
		source.name = parsed
	case "adapter":
		source.adapter = parsed
	case "kind":
		source.kind = parsed
	case "path":
		source.paths = parsed
	default:
		return fmt.Errorf("line %d: unsupported sources field %q", lineNo, key)
	}
	return nil
}

func parseRuntimeField(runtime *rawRuntimeProvider, key, value string, lineNo int, section string) error {
	switch key {
	case "provider", "model", "endpoint", "api_key_env":
		parsed, err := parseQuotedString(value)
		if err != nil {
			return fmt.Errorf("line %d: %s.%s: %w", lineNo, section, key, err)
		}
		switch key {
		case "provider":
			runtime.provider = parsed
		case "model":
			runtime.model = parsed
		case "endpoint":
			runtime.endpoint = parsed
		case "api_key_env":
			runtime.apiKeyEnv = parsed
		}
		return nil
	case "timeout_ms", "max_retries":
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("line %d: %s.%s: expected integer", lineNo, section, key)
		}
		if key == "timeout_ms" {
			runtime.timeoutMS = parsed
		} else {
			runtime.maxRetries = parsed
		}
		return nil
	default:
		return fmt.Errorf("line %d: unsupported %s field %q", lineNo, section, key)
	}
}

func parseQuotedString(value string) (string, error) {
	if !strings.HasPrefix(value, "\"") {
		return "", fmt.Errorf("expected quoted string")
	}
	parsed, err := strconv.Unquote(value)
	if err != nil {
		return "", fmt.Errorf("parse quoted string: %w", err)
	}
	return parsed, nil
}

func validate(cfg *Config) error {
	var errs validationErrors

	if cfg.Workspace.Root == "" {
		errs.add("workspace.root: value is required")
	} else {
		cfg.Workspace.RootPath = resolvePath(cfg.ConfigDir, cfg.Workspace.Root)
		info, err := os.Stat(cfg.Workspace.RootPath)
		switch {
		case err == nil && !info.IsDir():
			errs.add("workspace.root: %q is not a directory", cfg.Workspace.Root)
		case err != nil:
			errs.add("workspace.root: %q does not exist", cfg.Workspace.Root)
		}
	}

	if cfg.Workspace.IndexPath == "" {
		errs.add("workspace.index_path: value is required")
	} else if cfg.Workspace.RootPath != "" {
		cfg.Workspace.ResolvedIndexPath = resolvePath(cfg.Workspace.RootPath, cfg.Workspace.IndexPath)
		if info, err := os.Stat(cfg.Workspace.ResolvedIndexPath); err == nil && info.IsDir() {
			errs.add("workspace.index_path: %q resolves to a directory, not a database file", cfg.Workspace.IndexPath)
		}
	}

	if len(cfg.Sources) == 0 {
		errs.add("sources: at least one source is required")
	}

	seenNames := make(map[string]struct{}, len(cfg.Sources))
	for i := range cfg.Sources {
		source := &cfg.Sources[i]
		label := fmt.Sprintf("sources[%d]", i)
		if source.Name == "" {
			errs.add("%s.name: value is required", label)
		} else {
			if _, exists := seenNames[source.Name]; exists {
				errs.add("%s.name: %q is duplicated", label, source.Name)
			}
			seenNames[source.Name] = struct{}{}
			label = fmt.Sprintf("source %q", source.Name)
		}

		if source.Adapter == "" {
			errs.add("%s.adapter: value is required", label)
		} else if source.Adapter != AdapterFilesystem {
			errs.add("%s.adapter: unsupported adapter %q (v1 supports only %q)", label, source.Adapter, AdapterFilesystem)
		}

		if source.Kind == "" {
			errs.add("%s.kind: value is required", label)
		} else if source.Kind != SourceKindSpecBundle && source.Kind != SourceKindMarkdownDocs {
			errs.add("%s.kind: unsupported kind %q", label, source.Kind)
		}

		if source.Path == "" {
			errs.add("%s.path: value is required", label)
			continue
		}
		if cfg.Workspace.RootPath == "" {
			continue
		}
		source.ResolvedPath = resolvePath(cfg.Workspace.RootPath, source.Path)
		info, err := os.Stat(source.ResolvedPath)
		switch {
		case err == nil && !info.IsDir():
			errs.add("%s.path: %q is not a directory", label, source.Path)
		case err != nil:
			errs.add("%s.path: %q does not exist", label, source.Path)
		}
	}

	if err := validateRuntime(cfg.Runtime); err != nil {
		errs.items = append(errs.items, err.Error())
	}

	return errs.err()
}

func validateRuntime(runtime Runtime) error {
	var errs validationErrors

	validateProvider := func(label string, provider RuntimeProvider, allowDisabled bool) {
		if provider.Provider == "" {
			errs.add("%s.provider: value is required", label)
			return
		}
		switch provider.Provider {
		case "fixture":
			if provider.Model == "" {
				errs.add("%s.model: value is required for provider %q", label, provider.Provider)
			}
		case "disabled":
			if !allowDisabled {
				errs.add("%s.provider: %q is not valid for the embedder", label, provider.Provider)
			}
		case "openai_compatible":
			if provider.Model == "" {
				errs.add("%s.model: value is required for provider %q", label, provider.Provider)
			}
		default:
			errs.add("%s.provider: unsupported provider %q", label, provider.Provider)
		}
		if provider.TimeoutMS < 0 {
			errs.add("%s.timeout_ms: must be >= 0", label)
		}
		if provider.MaxRetries < 0 {
			errs.add("%s.max_retries: must be >= 0", label)
		}
	}

	validateProvider("runtime.embedder", runtime.Embedder, false)
	validateProvider("runtime.analysis", runtime.Analysis, true)
	return errs.err()
}

func resolvePath(base, value string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(base, value))
}

func stripComment(line string) string {
	var builder strings.Builder
	inString := false
	escaped := false

	for _, r := range line {
		switch {
		case escaped:
			builder.WriteRune(r)
			escaped = false
		case r == '\\':
			builder.WriteRune(r)
			escaped = true
		case r == '"':
			builder.WriteRune(r)
			inString = !inString
		case r == '#' && !inString:
			return builder.String()
		default:
			builder.WriteRune(r)
		}
	}

	return builder.String()
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
