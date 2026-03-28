package config

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dusk-network/pituitary/sdk"
)

const (
	CurrentSchemaVersion       = 3
	AdapterFilesystem          = "filesystem"
	SourceKindSpecBundle       = "spec_bundle"
	SourceKindMarkdownDocs     = "markdown_docs"
	SourceKindMarkdownContract = "markdown_contract"
	RuntimeProviderFixture     = "fixture"
	RuntimeProviderOpenAI      = "openai_compatible"
	RuntimeProviderDisabled    = "disabled"
)

// Config is the validated workspace configuration resolved from pituitary.toml.
type Config struct {
	SchemaVersion int
	ConfigPath    string
	ConfigDir     string
	Workspace     Workspace
	Runtime       Runtime
	Sources       []Source
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
	Files        []string
	Include      []string
	Exclude      []string
	Options      map[string]any
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
	SchemaVersion int          `toml:"schema_version"`
	Workspace     rawWorkspace `toml:"workspace"`
	Runtime       rawRuntime   `toml:"runtime"`
	Sources       []rawSource  `toml:"sources"`
}

type rawWorkspace struct {
	Root      string `toml:"root"`
	IndexPath string `toml:"index_path"`
}

type rawRuntime struct {
	Embedder rawRuntimeProvider `toml:"embedder"`
	Analysis rawRuntimeProvider `toml:"analysis"`
}

type rawSource struct {
	Name    string         `toml:"name"`
	Adapter string         `toml:"adapter"`
	Kind    string         `toml:"kind"`
	Path    string         `toml:"path"`
	Files   []string       `toml:"files"`
	Include []string       `toml:"include"`
	Exclude []string       `toml:"exclude"`
	Options map[string]any `toml:"options"`
}

type rawRuntimeProvider struct {
	Provider   string `toml:"provider"`
	Model      string `toml:"model"`
	Endpoint   string `toml:"endpoint"`
	APIKeyEnv  string `toml:"api_key_env"`
	TimeoutMS  *int   `toml:"timeout_ms"`
	MaxRetries *int   `toml:"max_retries"`
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

	// #nosec G304 -- configPath is the explicit config file selected by the caller.
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", configPath, err)
	}

	return loadFromData(data, configPath)
}

// LoadFromText parses config from text content as if it were read from the
// given path. This allows validating a generated config before writing it
// to disk.
func LoadFromText(content string, path string) (*Config, error) {
	configPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	return loadFromData([]byte(content), configPath)
}

func loadFromData(data []byte, configPath string) (*Config, error) {
	if legacy, ok, err := detectLegacyProjectConfig(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("%s: %w", configPath, err)
	} else if ok {
		return nil, fmt.Errorf("%s: %s", configPath, legacyConfigLoadMessage(configPath, legacy))
	}

	raw, err := parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", configPath, err)
	}

	cfg, err := buildFromRaw(configPath, raw, true)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", configPath, err)
	}
	return cfg, nil
}

func buildFromRaw(configPath string, raw rawConfig, enforceSchemaVersion bool) (*Config, error) {
	cfg := &Config{
		SchemaVersion: raw.SchemaVersion,
		ConfigPath:    configPath,
		ConfigDir:     configBaseDir(configPath),
		Workspace: Workspace{
			Root:      raw.Workspace.Root,
			IndexPath: raw.Workspace.IndexPath,
		},
		Runtime: Runtime{
			Embedder: RuntimeProvider{
				Provider:   defaultString(raw.Runtime.Embedder.Provider, RuntimeProviderFixture),
				Model:      raw.Runtime.Embedder.Model,
				Endpoint:   raw.Runtime.Embedder.Endpoint,
				APIKeyEnv:  raw.Runtime.Embedder.APIKeyEnv,
				TimeoutMS:  defaultOptionalInt(raw.Runtime.Embedder.TimeoutMS, 1000),
				MaxRetries: defaultOptionalInt(raw.Runtime.Embedder.MaxRetries, 0),
			},
			Analysis: RuntimeProvider{
				Provider:   defaultString(raw.Runtime.Analysis.Provider, RuntimeProviderDisabled),
				Model:      raw.Runtime.Analysis.Model,
				Endpoint:   raw.Runtime.Analysis.Endpoint,
				APIKeyEnv:  raw.Runtime.Analysis.APIKeyEnv,
				TimeoutMS:  defaultOptionalInt(raw.Runtime.Analysis.TimeoutMS, 1000),
				MaxRetries: defaultOptionalInt(raw.Runtime.Analysis.MaxRetries, 0),
			},
		},
		Sources: make([]Source, 0, len(raw.Sources)),
	}
	for _, source := range raw.Sources {
		cfg.Sources = append(cfg.Sources, Source{
			Name:    source.Name,
			Adapter: source.Adapter,
			Kind:    source.Kind,
			Path:    source.Path,
			Files:   append([]string(nil), source.Files...),
			Include: append([]string(nil), source.Include...),
			Exclude: append([]string(nil), source.Exclude...),
			Options: CloneSourceOptions(source.Options),
		})
	}
	if cfg.Runtime.Embedder.Provider == RuntimeProviderFixture && strings.TrimSpace(cfg.Runtime.Embedder.Model) == "" {
		cfg.Runtime.Embedder.Model = "fixture-8d"
	}
	if cfg.SchemaVersion == 0 {
		cfg.SchemaVersion = CurrentSchemaVersion
	} else if enforceSchemaVersion && cfg.SchemaVersion != CurrentSchemaVersion {
		return nil, fmt.Errorf(
			"unsupported schema_version %d (supported: %d); run `pituitary migrate-config --path %s --write` if this is an older config",
			cfg.SchemaVersion,
			CurrentSchemaVersion,
			filepath.ToSlash(configPath),
		)
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}
	cfg.SchemaVersion = CurrentSchemaVersion
	return cfg, nil
}

func configBaseDir(configPath string) string {
	configDir := filepath.Dir(configPath)
	if filepath.Base(configPath) == "pituitary.toml" && filepath.Base(configDir) == ".pituitary" {
		return filepath.Dir(configDir)
	}
	return configDir
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

func CloneSourceOptions(options map[string]any) map[string]any {
	if len(options) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(options))
	for key, value := range options {
		cloned[key] = CloneOptionValue(value)
	}
	return cloned
}

func CloneOptionValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return CloneSourceOptions(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i := range typed {
			cloned[i] = CloneOptionValue(typed[i])
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return typed
	}
}

func RegisteredAdapterNames() []string {
	names := map[string]struct{}{
		AdapterFilesystem: {},
	}
	for _, name := range sdk.RegisteredAdapterNames() {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		names[name] = struct{}{}
	}

	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// Validate resolves derived paths and verifies that a config can be used by the
// current bootstrap runtime.
func Validate(cfg *Config) error {
	return validate(cfg)
}

func validate(cfg *Config) error {
	var errs validationErrors
	registeredAdapters := RegisteredAdapterNames()
	registeredAdapterSet := make(map[string]struct{}, len(registeredAdapters))
	for _, name := range registeredAdapters {
		registeredAdapterSet[name] = struct{}{}
	}

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
		} else if _, exists := registeredAdapterSet[source.Adapter]; !exists {
			errs.add(
				"%s.adapter: unknown adapter %q (registered adapters: %s)",
				label,
				source.Adapter,
				strings.Join(registeredAdapters, ", "),
			)
		}

		if source.Kind == "" {
			errs.add("%s.kind: value is required", label)
		}

		filesystemSource := source.Adapter == AdapterFilesystem
		if strings.TrimSpace(source.Path) == "" && filesystemSource {
			errs.add("%s.path: value is required", label)
		}
		files := make([]string, 0, len(source.Files))
		seenFiles := make(map[string]struct{}, len(source.Files))
		for i, value := range source.Files {
			normalized, err := normalizeSourceFileSelector(value)
			if err != nil {
				errs.add("%s.files[%d]: %v", label, i, err)
				continue
			}
			if filesystemSource && source.Kind == SourceKindSpecBundle && pathpkg.Base(normalized) != "spec.toml" {
				errs.add("%s.files[%d]: %q must point to a spec.toml file for kind %q", label, i, value, source.Kind)
				continue
			}
			if filesystemSource && (source.Kind == SourceKindMarkdownDocs || source.Kind == SourceKindMarkdownContract) && pathpkg.Ext(normalized) != ".md" {
				errs.add("%s.files[%d]: %q must point to a markdown file for kind %q", label, i, value, source.Kind)
				continue
			}
			if _, exists := seenFiles[normalized]; exists {
				continue
			}
			seenFiles[normalized] = struct{}{}
			files = append(files, normalized)
		}
		source.Files = files
		if filesystemSource && len(source.Files) > 0 && strings.TrimSpace(source.Path) == "" {
			errs.add("%s.files: path is required when files are set", label)
		}
		for _, pattern := range source.Include {
			if strings.TrimSpace(pattern) == "" {
				errs.add("%s.include: patterns must not be empty", label)
				continue
			}
			if _, err := pathpkg.Match(pattern, "placeholder"); err != nil {
				errs.add("%s.include: invalid pattern %q: %v", label, pattern, err)
			}
		}
		for _, pattern := range source.Exclude {
			if strings.TrimSpace(pattern) == "" {
				errs.add("%s.exclude: patterns must not be empty", label)
				continue
			}
			if _, err := pathpkg.Match(pattern, "placeholder"); err != nil {
				errs.add("%s.exclude: invalid pattern %q: %v", label, pattern, err)
			}
		}
		if cfg.Workspace.RootPath == "" {
			continue
		}
		if strings.TrimSpace(source.Path) != "" {
			source.ResolvedPath = resolvePath(cfg.Workspace.RootPath, source.Path)
		}
		if filesystemSource && source.ResolvedPath != "" {
			info, err := os.Stat(source.ResolvedPath)
			switch {
			case err == nil && !info.IsDir():
				errs.add("%s.path: %q is not a directory", label, source.Path)
			case err != nil:
				errs.add("%s.path: %q does not exist", label, source.Path)
			}
			for i, relFile := range source.Files {
				resolvedFile := resolvePath(source.ResolvedPath, filepath.FromSlash(relFile))
				info, err := os.Stat(resolvedFile)
				switch {
				case err == nil && info.IsDir():
					errs.add("%s.files[%d]: %q is a directory", label, i, relFile)
				case err != nil:
					errs.add("%s.files[%d]: %q does not exist", label, i, relFile)
				}
			}
		}
	}

	if err := validateRuntime(cfg.Runtime); err != nil {
		errs.items = append(errs.items, err.Error())
	}

	return errs.err()
}

func normalizeSourceFileSelector(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("value must not be empty")
	}
	if filepath.IsAbs(value) {
		return "", fmt.Errorf("%q must be relative to the source root", value)
	}

	normalized := pathpkg.Clean(filepath.ToSlash(value))
	if normalized == "." {
		return "", fmt.Errorf("%q must point to a file under the source root", value)
	}
	if normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", fmt.Errorf("%q escapes the source root", value)
	}
	return normalized, nil
}

func validateRuntime(runtime Runtime) error {
	var errs validationErrors

	if runtime.Embedder.Provider == "" {
		errs.add("runtime.embedder.provider: value is required")
	} else {
		switch runtime.Embedder.Provider {
		case RuntimeProviderFixture:
			if runtime.Embedder.Model == "" {
				errs.add("runtime.embedder.model: value is required for provider %q", runtime.Embedder.Provider)
			}
		case RuntimeProviderOpenAI:
			if runtime.Embedder.Model == "" {
				errs.add("runtime.embedder.model: value is required for provider %q", runtime.Embedder.Provider)
			}
			if endpoint := strings.TrimSpace(runtime.Embedder.Endpoint); endpoint == "" {
				errs.add("runtime.embedder.endpoint: value is required for provider %q", runtime.Embedder.Provider)
			} else {
				parsed, err := url.Parse(endpoint)
				switch {
				case err != nil:
					errs.add("runtime.embedder.endpoint: invalid URL %q: %v", runtime.Embedder.Endpoint, err)
				case !parsed.IsAbs() || parsed.Host == "":
					errs.add("runtime.embedder.endpoint: %q must be an absolute URL", runtime.Embedder.Endpoint)
				case parsed.Scheme != "http" && parsed.Scheme != "https":
					errs.add("runtime.embedder.endpoint: %q must use http or https", runtime.Embedder.Endpoint)
				}
			}
		default:
			errs.add(
				"runtime.embedder.provider: unsupported provider %q (supported providers: %q, %q)",
				runtime.Embedder.Provider,
				RuntimeProviderFixture,
				RuntimeProviderOpenAI,
			)
		}
	}
	if runtime.Embedder.TimeoutMS < 0 {
		errs.add("runtime.embedder.timeout_ms: must be >= 0")
	}
	if runtime.Embedder.MaxRetries < 0 {
		errs.add("runtime.embedder.max_retries: must be >= 0")
	}

	if runtime.Analysis.Provider == "" {
		errs.add("runtime.analysis.provider: value is required")
	} else {
		switch runtime.Analysis.Provider {
		case RuntimeProviderDisabled:
		case RuntimeProviderOpenAI:
			if runtime.Analysis.Model == "" {
				errs.add("runtime.analysis.model: value is required for provider %q", runtime.Analysis.Provider)
			}
			if endpoint := strings.TrimSpace(runtime.Analysis.Endpoint); endpoint == "" {
				errs.add("runtime.analysis.endpoint: value is required for provider %q", runtime.Analysis.Provider)
			} else {
				parsed, err := url.Parse(endpoint)
				switch {
				case err != nil:
					errs.add("runtime.analysis.endpoint: invalid URL %q: %v", runtime.Analysis.Endpoint, err)
				case !parsed.IsAbs() || parsed.Host == "":
					errs.add("runtime.analysis.endpoint: %q must be an absolute URL", runtime.Analysis.Endpoint)
				case parsed.Scheme != "http" && parsed.Scheme != "https":
					errs.add("runtime.analysis.endpoint: %q must use http or https", runtime.Analysis.Endpoint)
				}
			}
		default:
			errs.add(
				"runtime.analysis.provider: unsupported provider %q (supported providers: %q, %q)",
				runtime.Analysis.Provider,
				RuntimeProviderDisabled,
				RuntimeProviderOpenAI,
			)
		}
	}
	if runtime.Analysis.TimeoutMS < 0 {
		errs.add("runtime.analysis.timeout_ms: must be >= 0")
	}
	if runtime.Analysis.MaxRetries < 0 {
		errs.add("runtime.analysis.max_retries: must be >= 0")
	}
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

func defaultOptionalInt(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}
