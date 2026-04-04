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
	"unicode"

	"github.com/dusk-network/pituitary/sdk"
)

const (
	CurrentSchemaVersion       = 3
	AdapterFilesystem          = "filesystem"
	SourceKindSpecBundle       = "spec_bundle"
	SourceKindMarkdownDocs     = "markdown_docs"
	SourceKindMarkdownContract = "markdown_contract"
	SourceRoleCanonical        = "canonical"
	SourceRoleCurrentState     = "current_state"
	SourceRoleRuntimeAuth      = "runtime_authoritative"
	SourceRolePlanning         = "planning"
	SourceRoleHistorical       = "historical"
	SourceRoleGenerated        = "generated"
	SourceRoleMirror           = "mirror"
	SourceRoleDecisionLog      = "decision_log"
	RuntimeProviderFixture     = "fixture"
	RuntimeProviderOpenAI      = "openai_compatible"
	RuntimeProviderDisabled    = "disabled"
	TerminologySeverityIgnore  = "ignore"
	TerminologySeverityWarning = "warning"
	TerminologySeverityError   = "error"
)

// Config is the validated workspace configuration resolved from pituitary.toml.
type Config struct {
	SchemaVersion int
	ConfigPath    string
	ConfigDir     string
	Workspace     Workspace
	Runtime       Runtime
	Terminology   Terminology
	Sources       []Source
}

// Workspace describes the configured workspace root and index path.
type Workspace struct {
	Root              string
	RepoID            string
	RootPath          string
	IndexPath         string
	ResolvedIndexPath string
	Repos             []WorkspaceRepo
}

// WorkspaceRepo describes one additional repo root that participates in the
// logical multi-repo workspace.
type WorkspaceRepo struct {
	ID       string
	Root     string
	RootPath string
}

// Source describes one configured input source.
type Source struct {
	Name         string
	Adapter      string
	Kind         string
	Role         string
	Repo         string
	Path         string
	Files        []string
	Include      []string
	Exclude      []string
	Options      map[string]any
	ResolvedRepo string
	PrimaryRepo  string
	RepoRootPath string
	ResolvedPath string
}

// Runtime captures provider configuration needed by later pipeline stages.
type Runtime struct {
	Profiles map[string]RuntimeProvider
	Embedder RuntimeProvider
	Analysis RuntimeProvider
}

// RuntimeProvider describes one configured runtime dependency.
type RuntimeProvider struct {
	Profile    string
	Provider   string
	Model      string
	Endpoint   string
	APIKeyEnv  string
	TimeoutMS  int
	MaxRetries int

	providerSet   bool
	modelSet      bool
	endpointSet   bool
	apiKeyEnvSet  bool
	timeoutMSSet  bool
	maxRetriesSet bool
}

type Terminology struct {
	Policies []TerminologyPolicy
}

type TerminologyPolicy struct {
	Preferred         string
	HistoricalAliases []string
	DeprecatedTerms   []string
	ForbiddenCurrent  []string
	DocsSeverity      string
	SpecsSeverity     string
}

type rawConfig struct {
	SchemaVersion int            `toml:"schema_version"`
	Workspace     rawWorkspace   `toml:"workspace"`
	Runtime       rawRuntime     `toml:"runtime"`
	Terminology   rawTerminology `toml:"terminology"`
	Sources       []rawSource    `toml:"sources"`
}

type rawWorkspace struct {
	Root      string             `toml:"root"`
	RepoID    string             `toml:"repo_id"`
	IndexPath string             `toml:"index_path"`
	Repos     []rawWorkspaceRepo `toml:"repos"`
}

type rawWorkspaceRepo struct {
	ID   string `toml:"id"`
	Root string `toml:"root"`
}

type rawRuntime struct {
	Profiles map[string]rawRuntimeProvider `toml:"profiles"`
	Embedder rawRuntimeProvider            `toml:"embedder"`
	Analysis rawRuntimeProvider            `toml:"analysis"`
}

type rawSource struct {
	Name    string         `toml:"name"`
	Adapter string         `toml:"adapter"`
	Kind    string         `toml:"kind"`
	Role    string         `toml:"role"`
	Repo    string         `toml:"repo"`
	Path    string         `toml:"path"`
	Files   []string       `toml:"files"`
	Include []string       `toml:"include"`
	Exclude []string       `toml:"exclude"`
	Options map[string]any `toml:"options"`
}

type rawTerminology struct {
	Policies []rawTerminologyPolicy `toml:"policies"`
}

type rawTerminologyPolicy struct {
	Preferred         string   `toml:"preferred"`
	HistoricalAliases []string `toml:"historical_aliases"`
	DeprecatedTerms   []string `toml:"deprecated_terms"`
	ForbiddenCurrent  []string `toml:"forbidden_current"`
	DocsSeverity      string   `toml:"docs_severity"`
	SpecsSeverity     string   `toml:"specs_severity"`
}

type rawRuntimeProvider struct {
	Profile    string `toml:"profile"`
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
			RepoID:    strings.TrimSpace(raw.Workspace.RepoID),
			IndexPath: raw.Workspace.IndexPath,
			Repos:     make([]WorkspaceRepo, 0, len(raw.Workspace.Repos)),
		},
		Runtime: Runtime{
			Profiles: make(map[string]RuntimeProvider, len(raw.Runtime.Profiles)),
			Embedder: buildRuntimeProvider(raw.Runtime.Embedder, RuntimeProviderFixture),
			Analysis: buildRuntimeProvider(raw.Runtime.Analysis, RuntimeProviderDisabled),
		},
		Terminology: Terminology{
			Policies: make([]TerminologyPolicy, 0, len(raw.Terminology.Policies)),
		},
		Sources: make([]Source, 0, len(raw.Sources)),
	}
	for name, profile := range raw.Runtime.Profiles {
		cfg.Runtime.Profiles[strings.TrimSpace(name)] = buildRuntimeProvider(profile, "")
	}
	for _, policy := range raw.Terminology.Policies {
		cfg.Terminology.Policies = append(cfg.Terminology.Policies, TerminologyPolicy{
			Preferred:         strings.TrimSpace(policy.Preferred),
			HistoricalAliases: uniqueStringList(policy.HistoricalAliases),
			DeprecatedTerms:   uniqueStringList(policy.DeprecatedTerms),
			ForbiddenCurrent:  uniqueStringList(policy.ForbiddenCurrent),
			DocsSeverity:      NormalizeTerminologySeverity(policy.DocsSeverity),
			SpecsSeverity:     NormalizeTerminologySeverity(policy.SpecsSeverity),
		})
	}
	for _, repo := range raw.Workspace.Repos {
		cfg.Workspace.Repos = append(cfg.Workspace.Repos, WorkspaceRepo{
			ID:   strings.TrimSpace(repo.ID),
			Root: repo.Root,
		})
	}
	for _, source := range raw.Sources {
		cfg.Sources = append(cfg.Sources, Source{
			Name:    source.Name,
			Adapter: source.Adapter,
			Kind:    source.Kind,
			Role:    NormalizeSourceRole(source.Role),
			Repo:    strings.TrimSpace(source.Repo),
			Path:    source.Path,
			Files:   append([]string(nil), source.Files...),
			Include: append([]string(nil), source.Include...),
			Exclude: append([]string(nil), source.Exclude...),
			Options: CloneSourceOptions(source.Options),
		})
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

func uniqueStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

// NormalizeTerminologySeverity maps blank severities to the default warning
// level while preserving unsupported values for later validation.
func NormalizeTerminologySeverity(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return TerminologySeverityWarning
	}
	return value
}

// IsValidTerminologySeverity reports whether the configured terminology
// governance severity is supported by the CLI and JSON contracts.
func IsValidTerminologySeverity(value string) bool {
	switch NormalizeTerminologySeverity(value) {
	case TerminologySeverityIgnore, TerminologySeverityWarning, TerminologySeverityError:
		return true
	default:
		return false
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

	if cfg.Workspace.RepoID != "" && !isValidRepoID(cfg.Workspace.RepoID) {
		errs.add("workspace.repo_id: unsupported repo id %q", cfg.Workspace.RepoID)
	}
	primaryRepoID := effectiveWorkspaceRepoID(cfg.Workspace)
	seenRepoIDs := map[string]struct{}{}
	if primaryRepoID != "" {
		seenRepoIDs[primaryRepoID] = struct{}{}
	}
	for i := range cfg.Workspace.Repos {
		repo := &cfg.Workspace.Repos[i]
		label := fmt.Sprintf("workspace.repos[%d]", i)
		if repo.ID == "" {
			errs.add("%s.id: value is required", label)
		} else {
			if !isValidRepoID(repo.ID) {
				errs.add("%s.id: unsupported repo id %q", label, repo.ID)
			} else if _, exists := seenRepoIDs[repo.ID]; exists {
				errs.add("%s.id: %q is duplicated", label, repo.ID)
			} else {
				seenRepoIDs[repo.ID] = struct{}{}
			}
		}
		if repo.Root == "" {
			errs.add("%s.root: value is required", label)
			continue
		}
		repo.RootPath = resolvePath(cfg.ConfigDir, repo.Root)
		info, err := os.Stat(repo.RootPath)
		switch {
		case err == nil && !info.IsDir():
			errs.add("%s.root: %q is not a directory", label, repo.Root)
		case err != nil:
			errs.add("%s.root: %q does not exist", label, repo.Root)
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
		if source.Role != "" && !IsValidSourceRole(source.Role) {
			errs.add("%s.role: unsupported role %q", label, source.Role)
		}
		if source.Repo != "" && !isValidRepoID(source.Repo) {
			errs.add("%s.repo: unsupported repo id %q", label, source.Repo)
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
		repoID := primaryRepoID
		repoRootPath := cfg.Workspace.RootPath
		if source.Repo != "" {
			repoID = source.Repo
			resolved, ok := lookupWorkspaceRepoRoot(cfg.Workspace, repoID)
			if !ok {
				errs.add("%s.repo: unknown repo %q", label, source.Repo)
				continue
			}
			repoRootPath = resolved
		}
		source.ResolvedRepo = repoID
		source.PrimaryRepo = primaryRepoID
		source.RepoRootPath = repoRootPath
		if strings.TrimSpace(source.Path) != "" {
			source.ResolvedPath = resolvePath(repoRootPath, source.Path)
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

	if err := validateRuntime(&cfg.Runtime); err != nil {
		errs.items = append(errs.items, err.Error())
	}
	if err := validateTerminology(&cfg.Terminology); err != nil {
		errs.items = append(errs.items, err.Error())
	}

	return errs.err()
}

func validateTerminology(terminology *Terminology) error {
	if terminology == nil {
		return nil
	}

	var errs validationErrors
	seenPreferred := make(map[string]string, len(terminology.Policies))
	seenGoverned := make(map[string]string)
	for i := range terminology.Policies {
		policy := &terminology.Policies[i]
		label := fmt.Sprintf("terminology.policies[%d]", i)
		policy.Preferred = strings.TrimSpace(policy.Preferred)
		policy.HistoricalAliases = uniqueStringList(policy.HistoricalAliases)
		policy.DeprecatedTerms = uniqueStringList(policy.DeprecatedTerms)
		policy.ForbiddenCurrent = uniqueStringList(policy.ForbiddenCurrent)
		policy.DocsSeverity = NormalizeTerminologySeverity(policy.DocsSeverity)
		policy.SpecsSeverity = NormalizeTerminologySeverity(policy.SpecsSeverity)

		if policy.Preferred == "" {
			errs.add("%s.preferred: value is required", label)
		}
		if len(policy.HistoricalAliases) == 0 && len(policy.DeprecatedTerms) == 0 && len(policy.ForbiddenCurrent) == 0 {
			errs.add("%s: at least one historical_aliases, deprecated_terms, or forbidden_current term is required", label)
		}
		if !IsValidTerminologySeverity(policy.DocsSeverity) {
			errs.add(
				"%s.docs_severity: unsupported severity %q (supported: %q, %q, %q)",
				label,
				policy.DocsSeverity,
				TerminologySeverityIgnore,
				TerminologySeverityWarning,
				TerminologySeverityError,
			)
		}
		if !IsValidTerminologySeverity(policy.SpecsSeverity) {
			errs.add(
				"%s.specs_severity: unsupported severity %q (supported: %q, %q, %q)",
				label,
				policy.SpecsSeverity,
				TerminologySeverityIgnore,
				TerminologySeverityWarning,
				TerminologySeverityError,
			)
		}

		preferredKey := strings.ToLower(policy.Preferred)
		if policy.Preferred != "" {
			if owner, exists := seenGoverned[preferredKey]; exists {
				errs.add("%s.preferred: %q conflicts with governed term already declared by %s", label, policy.Preferred, owner)
			}
			if owner, exists := seenPreferred[preferredKey]; exists {
				errs.add("%s.preferred: %q duplicates %s", label, policy.Preferred, owner)
			} else {
				seenPreferred[preferredKey] = label + ".preferred"
			}
		}

		registerGoverned := func(field string, values []string) {
			for j, term := range values {
				term = strings.TrimSpace(term)
				if term == "" {
					continue
				}
				key := strings.ToLower(term)
				if key == preferredKey && policy.Preferred != "" {
					errs.add("%s.%s[%d]: %q duplicates the preferred term", label, field, j, term)
					continue
				}
				if owner, exists := seenPreferred[key]; exists {
					errs.add("%s.%s[%d]: %q conflicts with preferred term declared by %s", label, field, j, term, owner)
					continue
				}
				if owner, exists := seenGoverned[key]; exists {
					errs.add("%s.%s[%d]: %q is already governed by %s", label, field, j, term, owner)
					continue
				}
				seenGoverned[key] = fmt.Sprintf("%s.%s[%d]", label, field, j)
			}
		}

		registerGoverned("historical_aliases", policy.HistoricalAliases)
		registerGoverned("deprecated_terms", policy.DeprecatedTerms)
		registerGoverned("forbidden_current", policy.ForbiddenCurrent)
	}

	return errs.err()
}

func effectiveWorkspaceRepoID(workspace Workspace) string {
	if explicit := strings.TrimSpace(workspace.RepoID); explicit != "" {
		return explicit
	}
	candidate := strings.TrimSpace(filepath.Base(workspace.RootPath))
	if isValidRepoID(candidate) {
		return candidate
	}
	return "workspace"
}

// PrimaryRepoID returns the effective repo identity for the configured primary
// workspace root.
func PrimaryRepoID(cfg *Config) string {
	if cfg == nil {
		return ""
	}
	return effectiveWorkspaceRepoID(cfg.Workspace)
}

func lookupWorkspaceRepoRoot(workspace Workspace, repoID string) (string, bool) {
	repoID = strings.TrimSpace(repoID)
	if repoID == "" {
		return "", false
	}
	if repoID == effectiveWorkspaceRepoID(workspace) {
		return workspace.RootPath, workspace.RootPath != ""
	}
	for _, repo := range workspace.Repos {
		if strings.TrimSpace(repo.ID) == repoID {
			return repo.RootPath, repo.RootPath != ""
		}
	}
	return "", false
}

func isValidRepoID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
		case r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
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

func NormalizeSourceRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

func IsValidSourceRole(role string) bool {
	switch NormalizeSourceRole(role) {
	case SourceRoleCanonical,
		SourceRoleCurrentState,
		SourceRoleRuntimeAuth,
		SourceRolePlanning,
		SourceRoleHistorical,
		SourceRoleGenerated,
		SourceRoleMirror,
		SourceRoleDecisionLog:
		return true
	default:
		return false
	}
}

func validateRuntime(runtime *Runtime) error {
	var errs validationErrors
	if runtime == nil {
		return nil
	}

	for name, profile := range runtime.Profiles {
		label := fmt.Sprintf("runtime.profiles.%s", name)
		if !isValidRuntimeProfileName(name) {
			errs.add("%s: unsupported profile name %q", label, name)
		}
		if strings.TrimSpace(profile.Profile) != "" {
			errs.add("%s.profile: nested profile references are not supported", label)
		}
		validateRuntimeProfileFields(&errs, label, profile)
	}

	runtime.Embedder = resolveRuntimeProvider(runtime.Embedder, runtime.Profiles, RuntimeProviderFixture)
	if profile := strings.TrimSpace(runtime.Embedder.Profile); profile != "" {
		if _, ok := runtime.Profiles[profile]; !ok {
			errs.add("runtime.embedder.profile: unknown profile %q", profile)
		}
	}

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

	runtime.Analysis = resolveRuntimeProvider(runtime.Analysis, runtime.Profiles, RuntimeProviderDisabled)
	if profile := strings.TrimSpace(runtime.Analysis.Profile); profile != "" {
		if _, ok := runtime.Profiles[profile]; !ok {
			errs.add("runtime.analysis.profile: unknown profile %q", profile)
		}
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

func buildRuntimeProvider(raw rawRuntimeProvider, defaultProvider string) RuntimeProvider {
	return RuntimeProvider{
		Profile:       strings.TrimSpace(raw.Profile),
		Provider:      defaultString(strings.TrimSpace(raw.Provider), defaultProvider),
		Model:         strings.TrimSpace(raw.Model),
		Endpoint:      strings.TrimSpace(raw.Endpoint),
		APIKeyEnv:     strings.TrimSpace(raw.APIKeyEnv),
		TimeoutMS:     defaultOptionalInt(raw.TimeoutMS, 1000),
		MaxRetries:    defaultOptionalInt(raw.MaxRetries, 0),
		providerSet:   strings.TrimSpace(raw.Provider) != "",
		modelSet:      strings.TrimSpace(raw.Model) != "",
		endpointSet:   strings.TrimSpace(raw.Endpoint) != "",
		apiKeyEnvSet:  strings.TrimSpace(raw.APIKeyEnv) != "",
		timeoutMSSet:  raw.TimeoutMS != nil,
		maxRetriesSet: raw.MaxRetries != nil,
	}
}

func resolveRuntimeProvider(provider RuntimeProvider, profiles map[string]RuntimeProvider, defaultProvider string) RuntimeProvider {
	profile := RuntimeProvider{}
	hasProfile := false
	if name := strings.TrimSpace(provider.Profile); name != "" {
		profile, hasProfile = profiles[name]
	}

	resolved := provider
	providerResolved := provider.providerSet
	modelResolved := provider.modelSet
	endpointResolved := provider.endpointSet
	apiKeyEnvResolved := provider.apiKeyEnvSet
	timeoutResolved := provider.timeoutMSSet
	maxRetriesResolved := provider.maxRetriesSet

	if !providerResolved && hasProfile && profile.providerSet {
		resolved.Provider = profile.Provider
		providerResolved = true
	}
	if !modelResolved && hasProfile && profile.modelSet {
		resolved.Model = profile.Model
		modelResolved = true
	}
	if !endpointResolved && hasProfile && profile.endpointSet {
		resolved.Endpoint = profile.Endpoint
		endpointResolved = true
	}
	if !apiKeyEnvResolved && hasProfile && profile.apiKeyEnvSet {
		resolved.APIKeyEnv = profile.APIKeyEnv
		apiKeyEnvResolved = true
	}
	if !timeoutResolved && hasProfile && profile.timeoutMSSet {
		resolved.TimeoutMS = profile.TimeoutMS
		timeoutResolved = true
	}
	if !maxRetriesResolved && hasProfile && profile.maxRetriesSet {
		resolved.MaxRetries = profile.MaxRetries
		maxRetriesResolved = true
	}
	if !providerResolved {
		resolved.Provider = defaultProvider
	}
	if !timeoutResolved {
		resolved.TimeoutMS = 1000
	}
	if !maxRetriesResolved {
		resolved.MaxRetries = 0
	}
	if strings.TrimSpace(resolved.Provider) == RuntimeProviderFixture && !modelResolved {
		resolved.Model = "fixture-8d"
	}
	return resolved
}

func validateRuntimeProfileFields(errs *validationErrors, label string, profile RuntimeProvider) {
	if provider := strings.TrimSpace(profile.Provider); provider != "" {
		switch provider {
		case RuntimeProviderFixture, RuntimeProviderOpenAI, RuntimeProviderDisabled:
		default:
			errs.add(
				"%s.provider: unsupported provider %q (supported providers: %q, %q, %q)",
				label,
				provider,
				RuntimeProviderFixture,
				RuntimeProviderOpenAI,
				RuntimeProviderDisabled,
			)
		}
	}
	if endpoint := strings.TrimSpace(profile.Endpoint); endpoint != "" {
		parsed, err := url.Parse(endpoint)
		switch {
		case err != nil:
			errs.add("%s.endpoint: invalid URL %q: %v", label, profile.Endpoint, err)
		case !parsed.IsAbs() || parsed.Host == "":
			errs.add("%s.endpoint: %q must be an absolute URL", label, profile.Endpoint)
		case parsed.Scheme != "http" && parsed.Scheme != "https":
			errs.add("%s.endpoint: %q must use http or https", label, profile.Endpoint)
		}
	}
	if profile.TimeoutMS < 0 {
		errs.add("%s.timeout_ms: must be >= 0", label)
	}
	if profile.MaxRetries < 0 {
		errs.add("%s.max_retries: must be >= 0", label)
	}
}

func isValidRuntimeProfileName(value string) bool {
	return isValidRepoID(value)
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
