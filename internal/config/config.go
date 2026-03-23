package config

import (
	"bufio"
	"errors"
	"fmt"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
)

const (
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
	Files        []string
	Include      []string
	Exclude      []string
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
	files   []string
	include []string
	exclude []string
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
		return nil, fmt.Errorf("open %s: %w", configPath, err)
	}
	defer file.Close()

	raw, err := parse(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", configPath, err)
	}

	cfg := &Config{
		ConfigPath: configPath,
		ConfigDir:  configBaseDir(configPath),
		Workspace: Workspace{
			Root:      raw.workspace.root,
			IndexPath: raw.workspace.indexPath,
		},
		Runtime: Runtime{
			Embedder: RuntimeProvider{
				Provider:   defaultString(raw.runtimeEmbedder.provider, RuntimeProviderFixture),
				Model:      raw.runtimeEmbedder.model,
				Endpoint:   raw.runtimeEmbedder.endpoint,
				APIKeyEnv:  raw.runtimeEmbedder.apiKeyEnv,
				TimeoutMS:  defaultInt(raw.runtimeEmbedder.timeoutMS, 1000),
				MaxRetries: raw.runtimeEmbedder.maxRetries,
			},
			Analysis: RuntimeProvider{
				Provider:   defaultString(raw.runtimeAnalysis.provider, RuntimeProviderDisabled),
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
			Files:   append([]string(nil), source.files...),
			Include: append([]string(nil), source.include...),
			Exclude: append([]string(nil), source.exclude...),
		})
	}
	if cfg.Runtime.Embedder.Provider == RuntimeProviderFixture && strings.TrimSpace(cfg.Runtime.Embedder.Model) == "" {
		cfg.Runtime.Embedder.Model = "fixture-8d"
	}

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", configPath, err)
	}
	return cfg, nil
}

func configBaseDir(configPath string) string {
	configDir := filepath.Dir(configPath)
	if filepath.Base(configPath) == "pituitary.toml" && filepath.Base(configDir) == ".pituitary" {
		return filepath.Dir(configDir)
	}
	return configDir
}

func parse(file *os.File) (rawConfig, error) {
	var cfg rawConfig
	var section string
	var currentSource *rawSource
	var activeSourceArrayKey string
	var activeSourceArrayLine int

	scanner := bufio.NewScanner(file)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}

		if activeSourceArrayKey != "" {
			if line == "]" {
				activeSourceArrayKey = ""
				activeSourceArrayLine = 0
				continue
			}
			if !strings.HasPrefix(line, "\"") && !strings.HasPrefix(line, ",") {
				return rawConfig{}, fmt.Errorf(
					"line %d: sources.%s: unterminated array; expected ] to close the array opened on line %d",
					lineNo,
					activeSourceArrayKey,
					activeSourceArrayLine,
				)
			}
			if currentSource == nil {
				return rawConfig{}, fmt.Errorf("line %d: source entry missing array header", lineNo)
			}
			values, err := parseQuotedValues(line)
			if err != nil {
				return rawConfig{}, fmt.Errorf("line %d: sources.%s: %w", lineNo, activeSourceArrayKey, err)
			}
			if err := assignSourceArrayField(currentSource, activeSourceArrayKey, values); err != nil {
				return rawConfig{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
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
			if value == "[" {
				if !isSourceArrayField(key) {
					return rawConfig{}, fmt.Errorf("line %d: unsupported sources array field %q", lineNo, key)
				}
				activeSourceArrayKey = key
				activeSourceArrayLine = lineNo
				if err := assignSourceArrayField(currentSource, key, nil); err != nil {
					return rawConfig{}, fmt.Errorf("line %d: %w", lineNo, err)
				}
				continue
			}
			if strings.HasPrefix(value, "[") {
				if !isSourceArrayField(key) {
					return rawConfig{}, fmt.Errorf("line %d: unsupported sources array field %q", lineNo, key)
				}
				values, err := parseQuotedValues(value)
				if err != nil {
					return rawConfig{}, fmt.Errorf("line %d: sources.%s: %w", lineNo, key, err)
				}
				if err := assignSourceArrayField(currentSource, key, values); err != nil {
					return rawConfig{}, fmt.Errorf("line %d: %w", lineNo, err)
				}
				continue
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
	if activeSourceArrayKey != "" {
		return rawConfig{}, fmt.Errorf(
			"line %d: sources.%s: unterminated array; expected ] before end of file",
			activeSourceArrayLine,
			activeSourceArrayKey,
		)
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

func isSourceArrayField(key string) bool {
	switch key {
	case "files", "include", "exclude":
		return true
	default:
		return false
	}
}

func assignSourceArrayField(source *rawSource, key string, values []string) error {
	switch key {
	case "files":
		source.files = append(source.files, values...)
	case "include":
		source.include = append(source.include, values...)
	case "exclude":
		source.exclude = append(source.exclude, values...)
	default:
		return fmt.Errorf("unsupported sources array field %q", key)
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

func parseQuotedValues(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("expected quoted string")
	}
	if strings.HasPrefix(value, "[") {
		if !strings.HasSuffix(value, "]") {
			return nil, fmt.Errorf("unterminated array")
		}
		value = strings.TrimSpace(value[1 : len(value)-1])
	}

	var values []string
	for {
		value = strings.TrimSpace(value)
		switch {
		case value == "":
			return values, nil
		case strings.HasPrefix(value, ","):
			value = value[1:]
			continue
		case strings.HasPrefix(value, "]"):
			value = strings.TrimSpace(value[1:])
			if value != "" {
				return nil, fmt.Errorf("unexpected trailing content %q", value)
			}
			return values, nil
		case !strings.HasPrefix(value, "\""):
			return nil, fmt.Errorf("expected quoted string")
		}

		quoted := nextQuotedString(value)
		parsed, err := strconv.Unquote(quoted)
		if err != nil {
			return nil, err
		}
		values = append(values, parsed)
		value = value[len(quoted):]
	}
}

func nextQuotedString(value string) string {
	escaped := false
	for i := 1; i < len(value); i++ {
		switch {
		case escaped:
			escaped = false
		case value[i] == '\\':
			escaped = true
		case value[i] == '"':
			return value[:i+1]
		}
	}
	return value
}

// Validate resolves derived paths and verifies that a config can be used by the
// current bootstrap runtime.
func Validate(cfg *Config) error {
	return validate(cfg)
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
		} else if source.Kind != SourceKindSpecBundle && source.Kind != SourceKindMarkdownDocs && source.Kind != SourceKindMarkdownContract {
			errs.add("%s.kind: unsupported kind %q", label, source.Kind)
		}

		if source.Path == "" {
			errs.add("%s.path: value is required", label)
			continue
		}
		files := make([]string, 0, len(source.Files))
		seenFiles := make(map[string]struct{}, len(source.Files))
		for i, value := range source.Files {
			normalized, err := normalizeSourceFileSelector(value)
			if err != nil {
				errs.add("%s.files[%d]: %v", label, i, err)
				continue
			}
			if source.Kind == SourceKindSpecBundle && pathpkg.Base(normalized) != "spec.toml" {
				errs.add("%s.files[%d]: %q must point to a spec.toml file for kind %q", label, i, value, source.Kind)
				continue
			}
			if (source.Kind == SourceKindMarkdownDocs || source.Kind == SourceKindMarkdownContract) && pathpkg.Ext(normalized) != ".md" {
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
		source.ResolvedPath = resolvePath(cfg.Workspace.RootPath, source.Path)
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

func defaultInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}
