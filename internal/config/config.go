package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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
	AdapterJSON                = "json"
	SourceKindSpecBundle       = "spec_bundle"
	SourceKindMarkdownDocs     = "markdown_docs"
	SourceKindMarkdownContract = "markdown_contract"
	SourceKindJSONSpec         = "json_spec"
	SourceKindJSONDoc          = "json_doc"
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

	ChunkPolicyMarkdown  = "markdown"
	ChunkPolicyLateChunk = "late_chunk"

	// Chunk contextualizer formats. Duplicated here (rather than
	// imported from internal/chunk) to keep the config package
	// dependency-free of chunk, matching the pattern used for
	// ChunkPolicy* above. Kept in lock-step with chunk.PrefixFormat*.
	ChunkContextualizerFormatTitleAncestry = "title_ancestry"
	ChunkContextualizerFormatRefTitle      = "ref_title"

	// Search fusion strategy names. Duplicated here (rather than
	// imported from internal/fusion) to keep the config package
	// dependency-free of stroma, matching the ChunkPolicy* pattern
	// above. Kept in lock-step with fusion.Strategy*.
	SearchFusionStrategyDefault = "default_rrf"
	SearchFusionStrategyRRF     = "rrf"

	// Search reranker policies. Empty / SearchRerankerHistorical
	// preserve the pre-#342 historicalSectionReranker byte-for-byte.
	// SearchRerankerArmAwareHistorical opts in to a governance-aware
	// reranker that reads HitProvenance.Arms to prefer FTS-sourced hits
	// when query terminology matches the hit's section heading.
	SearchRerankerHistorical         = "historical"
	SearchRerankerArmAwareHistorical = "arm_aware_historical"
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
	InferAppliesTo    bool
	InferAppliesToSet bool
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
	Chunking ChunkingConfig
	Search   SearchConfig
}

// SearchConfig groups search-time overrides for the retrieval pipeline.
// A zero value means "no overrides" — snapshot.Search keeps stroma's
// DefaultFusion() governing hybrid retrieval exactly as it did pre-#342
// and the historicalSectionReranker stays the sole reranker.
type SearchConfig struct {
	Fusion FusionConfig

	// Reranker selects the governance-aware reranker. Empty or
	// SearchRerankerHistorical preserves the pre-#342 behavior.
	// SearchRerankerArmAwareHistorical opts in to reading
	// HitProvenance.Arms.
	Reranker string
}

// IsZero reports whether no search override is configured.
func (c SearchConfig) IsZero() bool {
	return c.Fusion.IsZero() && strings.TrimSpace(c.Reranker) == ""
}

// FusionConfig tunes the hybrid-search fusion strategy. A zero value
// (or Strategy == SearchFusionStrategyDefault with K == 0) resolves to
// nil so stroma's DefaultFusion() governs the call site byte-for-byte.
type FusionConfig struct {
	// Strategy selects the fusion implementation. Empty or
	// SearchFusionStrategyDefault preserves stroma's DefaultFusion().
	// SearchFusionStrategyRRF selects an explicit RRFFusion parameterised
	// by K.
	Strategy string

	// K is the RRF constant. Only applies to SearchFusionStrategyRRF and
	// must be > 0 there. Zero under SearchFusionStrategyDefault is valid
	// and means "no override".
	K int
}

// IsZero reports whether no fusion override is configured.
func (c FusionConfig) IsZero() bool {
	return c == FusionConfig{}
}

// ChunkingConfig groups per-kind chunking overrides for the rebuild
// pipeline. A zero value means "no overrides" — the rebuild keeps the
// pre-#338 default stroma MarkdownPolicy for every record.
type ChunkingConfig struct {
	Spec           ChunkingKindConfig
	Doc            ChunkingKindConfig
	Contextualizer ChunkingContextualizerConfig
}

// IsZero reports whether no kind or contextualizer override is set.
func (c ChunkingConfig) IsZero() bool {
	return c.Spec.IsZero() && c.Doc.IsZero() && c.Contextualizer.IsZero()
}

// ChunkingContextualizerConfig selects a per-chunk context prefix
// builder. A zero value means "disabled" — the rebuild produces the
// same snapshot it did before #343 (no prefix column writes, no
// reuse-cache invalidation). Opt-in only.
type ChunkingContextualizerConfig struct {
	// Format selects the prefix layout. Empty means disabled.
	// Validated against chunk.PrefixFormat at rebuild resolve time.
	Format string
}

// IsZero reports whether the contextualizer is disabled.
func (c ChunkingContextualizerConfig) IsZero() bool {
	return strings.TrimSpace(c.Format) == ""
}

// ChunkingKindConfig is a per-kind chunking override. A zero value means
// the kind defers to the router default.
type ChunkingKindConfig struct {
	// Policy selects the chunking strategy. Valid values are
	// ChunkPolicyMarkdown (default when non-empty tuning knobs are set
	// without an explicit policy) and ChunkPolicyLateChunk.
	Policy string

	// MaxTokens caps per-section tokens for Markdown and per-parent
	// tokens for late-chunking. Zero disables the cap.
	MaxTokens int

	// OverlapTokens is the approximate token overlap between adjacent
	// sub-sections for ChunkPolicyMarkdown. Ignored by
	// ChunkPolicyLateChunk (which uses ChildOverlapTokens).
	OverlapTokens int

	// MaxSections caps total sections per record. Zero applies
	// stroma's DefaultMaxChunkSections (the same bounded DoS guard
	// that protects the pre-#338 rebuild path). Negative explicitly
	// disables the cap for callers that have upstream validation.
	MaxSections int

	// ChildMaxTokens is required for ChunkPolicyLateChunk and must be
	// > 0; late-chunking with no child budget would silently disable
	// retrieval.
	ChildMaxTokens int

	// ChildOverlapTokens is the approximate overlap between leaf
	// chunks for ChunkPolicyLateChunk. Zero disables overlap.
	ChildOverlapTokens int
}

// IsZero reports whether the kind has no overrides configured.
func (c ChunkingKindConfig) IsZero() bool {
	return c == ChunkingKindConfig{}
}

// RuntimeProvider describes one configured runtime dependency.
type RuntimeProvider struct {
	Profile           string
	Provider          string
	Model             string
	Endpoint          string
	APIKeyEnv         string
	TimeoutMS         int
	MaxRetries        int
	MaxResponseTokens int

	providerSet          bool
	modelSet             bool
	endpointSet          bool
	apiKeyEnvSet         bool
	timeoutMSSet         bool
	maxRetriesSet        bool
	maxResponseTokensSet bool
}

type Terminology struct {
	ExcludePaths           []string
	Policies               []TerminologyPolicy
	IncludeSemanticMatches bool
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
	Root           string             `toml:"root"`
	RepoID         string             `toml:"repo_id"`
	IndexPath      string             `toml:"index_path"`
	InferAppliesTo *bool              `toml:"infer_applies_to"`
	Repos          []rawWorkspaceRepo `toml:"repos"`
}

type rawWorkspaceRepo struct {
	ID   string `toml:"id"`
	Root string `toml:"root"`
}

type rawRuntime struct {
	Profiles map[string]rawRuntimeProvider `toml:"profiles"`
	Embedder rawRuntimeProvider            `toml:"embedder"`
	Analysis rawRuntimeProvider            `toml:"analysis"`
	Chunking rawChunking                   `toml:"chunking"`
	Search   rawSearch                     `toml:"search"`
}

type rawSearch struct {
	Fusion   rawSearchFusion `toml:"fusion"`
	Reranker string          `toml:"reranker"`
}

type rawSearchFusion struct {
	Strategy string `toml:"strategy"`
	K        int    `toml:"k"`
}

type rawChunking struct {
	Spec           rawChunkingKind           `toml:"spec"`
	Doc            rawChunkingKind           `toml:"doc"`
	Contextualizer rawChunkingContextualizer `toml:"contextualizer"`
}

type rawChunkingKind struct {
	Policy             string `toml:"policy"`
	MaxTokens          int    `toml:"max_tokens"`
	OverlapTokens      int    `toml:"overlap_tokens"`
	MaxSections        int    `toml:"max_sections"`
	ChildMaxTokens     int    `toml:"child_max_tokens"`
	ChildOverlapTokens int    `toml:"child_overlap_tokens"`
}

type rawChunkingContextualizer struct {
	Format string `toml:"format"`
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
	ExcludePaths           []string               `toml:"exclude_paths"`
	IncludeSemanticMatches bool                   `toml:"include_semantic_matches"`
	Policies               []rawTerminologyPolicy `toml:"policies"`
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
	Profile           string `toml:"profile"`
	Provider          string `toml:"provider"`
	Model             string `toml:"model"`
	Endpoint          string `toml:"endpoint"`
	APIKeyEnv         string `toml:"api_key_env"`
	TimeoutMS         *int   `toml:"timeout_ms"`
	MaxRetries        *int   `toml:"max_retries"`
	MaxResponseTokens *int   `toml:"max_response_tokens"`
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

// maxConfigBytes caps pituitary.toml at 1 MiB. A realistic config is well under
// ten kilobytes; the limit exists only to keep a crafted or corrupted file from
// exhausting memory during Load.
const maxConfigBytes = 1 * 1024 * 1024

// Load parses and validates a repo-local pituitary.toml file.
func Load(path string) (*Config, error) {
	configPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	data, err := readBoundedConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", configPath, err)
	}

	return loadFromData(data, configPath)
}

func readBoundedConfig(configPath string) ([]byte, error) {
	// #nosec G304 -- configPath is the explicit config file selected by the caller; LimitReader enforces the allocation bound.
	f, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxConfigBytes {
		return nil, fmt.Errorf("config file exceeds %d-byte size limit", maxConfigBytes)
	}
	return data, nil
}

// DeclaresMultirepoRepos reports whether the config file at path declares
// one or more workspace.repos entries. It parses only the TOML structure
// and does not validate filesystem paths, making it safe for discovery-time
// checks where referenced repo roots may not exist on disk.
func DeclaresMultirepoRepos(path string) (bool, error) {
	configPath, err := filepath.Abs(path)
	if err != nil {
		return false, fmt.Errorf("resolve config path: %w", err)
	}

	data, err := readBoundedConfig(configPath)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", configPath, err)
	}

	raw, err := parse(bytes.NewReader(data))
	if err != nil {
		return false, fmt.Errorf("%s: %w", configPath, err)
	}

	return len(raw.Workspace.Repos) > 0, nil
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
			Root:              raw.Workspace.Root,
			RepoID:            strings.TrimSpace(raw.Workspace.RepoID),
			IndexPath:         raw.Workspace.IndexPath,
			InferAppliesTo:    rawBoolValue(raw.Workspace.InferAppliesTo),
			InferAppliesToSet: raw.Workspace.InferAppliesTo != nil,
			Repos:             make([]WorkspaceRepo, 0, len(raw.Workspace.Repos)),
		},
		Runtime: Runtime{
			Profiles: make(map[string]RuntimeProvider, len(raw.Runtime.Profiles)),
			Embedder: buildRuntimeProvider(raw.Runtime.Embedder, RuntimeProviderFixture),
			Analysis: buildRuntimeProvider(raw.Runtime.Analysis, RuntimeProviderDisabled),
			Chunking: buildChunkingConfig(raw.Runtime.Chunking),
			Search:   buildSearchConfig(raw.Runtime.Search),
		},
		Terminology: Terminology{
			ExcludePaths:           uniqueStringList(raw.Terminology.ExcludePaths),
			IncludeSemanticMatches: raw.Terminology.IncludeSemanticMatches,
			Policies:               make([]TerminologyPolicy, 0, len(raw.Terminology.Policies)),
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

func rawBoolValue(value *bool) bool {
	return value != nil && *value
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

	primaryRepoID := validateWorkspace(cfg, &errs)
	validateSources(cfg, &errs, primaryRepoID, registeredAdapters, registeredAdapterSet)

	if err := validateRuntime(&cfg.Runtime); err != nil {
		errs.items = append(errs.items, err.Error())
	}
	if err := validateTerminology(&cfg.Terminology); err != nil {
		errs.items = append(errs.items, err.Error())
	}

	return errs.err()
}

func validateWorkspace(cfg *Config, errs *validationErrors) string {
	validateWorkspaceRoot(cfg, errs)
	if cfg.Workspace.RepoID != "" && !isValidRepoID(cfg.Workspace.RepoID) {
		errs.add("workspace.repo_id: unsupported repo id %q", cfg.Workspace.RepoID)
	}
	primaryRepoID := effectiveWorkspaceRepoID(cfg.Workspace)
	validateWorkspaceRepos(cfg, errs, primaryRepoID)
	validateWorkspaceIndexPath(cfg, errs)
	return primaryRepoID
}

func validateWorkspaceRoot(cfg *Config, errs *validationErrors) {
	if cfg.Workspace.Root == "" {
		errs.add("workspace.root: value is required")
		return
	}

	cfg.Workspace.RootPath = resolvePath(cfg.ConfigDir, cfg.Workspace.Root)
	info, err := os.Stat(cfg.Workspace.RootPath)
	switch {
	case err == nil && !info.IsDir():
		errs.add(
			"workspace.root: %q is not a directory (%s)",
			cfg.Workspace.Root,
			pathResolutionDetail("workspace.root", cfg.Workspace.Root, cfg.ConfigDir, cfg.Workspace.RootPath),
		)
	case err != nil:
		errs.add(
			"workspace.root: %q does not exist (%s)",
			cfg.Workspace.Root,
			pathResolutionDetail("workspace.root", cfg.Workspace.Root, cfg.ConfigDir, cfg.Workspace.RootPath),
		)
	}
}

func validateWorkspaceRepos(cfg *Config, errs *validationErrors, primaryRepoID string) {
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
			errs.add(
				"%s.root: %q is not a directory (%s)",
				label,
				repo.Root,
				pathResolutionDetail(label+".root", repo.Root, cfg.ConfigDir, repo.RootPath),
			)
		case err != nil:
			errs.add(
				"%s.root: %q does not exist (%s)",
				label,
				repo.Root,
				pathResolutionDetail(label+".root", repo.Root, cfg.ConfigDir, repo.RootPath),
			)
		}
	}
}

func validateWorkspaceIndexPath(cfg *Config, errs *validationErrors) {
	if cfg.Workspace.IndexPath == "" {
		errs.add("workspace.index_path: value is required")
		return
	}
	if cfg.Workspace.RootPath == "" {
		return
	}

	cfg.Workspace.ResolvedIndexPath = resolvePath(cfg.Workspace.RootPath, cfg.Workspace.IndexPath)
	if info, err := os.Stat(cfg.Workspace.ResolvedIndexPath); err == nil && info.IsDir() {
		errs.add("workspace.index_path: %q resolves to a directory, not a database file", cfg.Workspace.IndexPath)
	}
}

func validateSources(cfg *Config, errs *validationErrors, primaryRepoID string, registeredAdapters []string, registeredAdapterSet map[string]struct{}) {
	if len(cfg.Sources) == 0 {
		errs.add("sources: at least one source is required")
	}

	seenNames := make(map[string]struct{}, len(cfg.Sources))
	for i := range cfg.Sources {
		validateSource(
			cfg,
			&cfg.Sources[i],
			i,
			errs,
			primaryRepoID,
			registeredAdapters,
			registeredAdapterSet,
			seenNames,
		)
	}
}

func validateSource(cfg *Config, source *Source, index int, errs *validationErrors, primaryRepoID string, registeredAdapters []string, registeredAdapterSet map[string]struct{}, seenNames map[string]struct{}) {
	label := fmt.Sprintf("sources[%d]", index)
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
	jsonSource := source.Adapter == AdapterJSON
	if strings.TrimSpace(source.Path) == "" && (filesystemSource || jsonSource) {
		errs.add("%s.path: value is required", label)
	}
	if jsonSource {
		switch source.Kind {
		case SourceKindJSONSpec, SourceKindJSONDoc:
		default:
			errs.add("%s.kind: unsupported kind %q for adapter %q", label, source.Kind, source.Adapter)
		}
	}

	validateSourceFiles(errs, label, source, filesystemSource, jsonSource)
	if filesystemSource && len(source.Files) > 0 && strings.TrimSpace(source.Path) == "" {
		errs.add("%s.files: path is required when files are set", label)
	}
	validateSourcePatterns(errs, label, "include", source.Include)
	validateSourcePatterns(errs, label, "exclude", source.Exclude)

	if cfg.Workspace.RootPath == "" {
		return
	}

	resolveAndValidateSourcePaths(cfg, errs, label, source, primaryRepoID, filesystemSource)
}

func validateSourceFiles(errs *validationErrors, label string, source *Source, filesystemSource bool, jsonSource bool) {
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
		if jsonSource && pathpkg.Ext(normalized) != ".json" {
			errs.add("%s.files[%d]: %q must point to a JSON file for adapter %q", label, i, value, source.Adapter)
			continue
		}
		if _, exists := seenFiles[normalized]; exists {
			continue
		}
		seenFiles[normalized] = struct{}{}
		files = append(files, normalized)
	}
	source.Files = files
}

func validateSourcePatterns(errs *validationErrors, label string, field string, patterns []string) {
	for _, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" {
			errs.add("%s.%s: patterns must not be empty", label, field)
			continue
		}
		if _, err := pathpkg.Match(pattern, "placeholder"); err != nil {
			errs.add("%s.%s: invalid pattern %q: %v", label, field, pattern, err)
		}
	}
}

func resolveAndValidateSourcePaths(cfg *Config, errs *validationErrors, label string, source *Source, primaryRepoID string, filesystemSource bool) {
	repoID := primaryRepoID
	repoRootPath := cfg.Workspace.RootPath
	if source.Repo != "" {
		repoID = source.Repo
		resolved, ok := lookupWorkspaceRepoRoot(cfg.Workspace, repoID)
		if !ok {
			errs.add("%s.repo: unknown repo %q", label, source.Repo)
			return
		}
		repoRootPath = resolved
	}

	source.ResolvedRepo = repoID
	source.PrimaryRepo = primaryRepoID
	source.RepoRootPath = repoRootPath
	if strings.TrimSpace(source.Path) != "" {
		source.ResolvedPath = resolvePath(repoRootPath, source.Path)
	}
	if !filesystemSource || source.ResolvedPath == "" {
		return
	}

	validateResolvedFilesystemSource(cfg, errs, label, source, repoRootPath)
}

func validateResolvedFilesystemSource(cfg *Config, errs *validationErrors, label string, source *Source, repoRootPath string) {
	info, err := os.Stat(source.ResolvedPath)
	switch {
	case err == nil && !info.IsDir():
		errs.add(
			"%s.path: %q is not a directory (%s)",
			label,
			source.Path,
			sourcePathResolutionDetail(cfg, source, repoRootPath),
		)
	case err != nil:
		errs.add(
			"%s.path: %q does not exist (%s)",
			label,
			source.Path,
			sourcePathResolutionDetail(cfg, source, repoRootPath),
		)
	}

	for i, relFile := range source.Files {
		resolvedFile := resolvePath(source.ResolvedPath, filepath.FromSlash(relFile))
		info, err := os.Stat(resolvedFile)
		switch {
		case err == nil && info.IsDir():
			errs.add(
				"%s.files[%d]: %q is a directory (%s)",
				label,
				i,
				relFile,
				pathResolutionDetail(label+".files", relFile, source.ResolvedPath, resolvedFile),
			)
		case err != nil:
			errs.add(
				"%s.files[%d]: %q does not exist (%s)",
				label,
				i,
				relFile,
				pathResolutionDetail(label+".files", relFile, source.ResolvedPath, resolvedFile),
			)
		}
	}
}

func validateTerminology(terminology *Terminology) error {
	if terminology == nil {
		return nil
	}

	var errs validationErrors
	seenPreferred := make(map[string]string, len(terminology.Policies))
	seenGoverned := make(map[string]string)
	terminology.ExcludePaths = uniqueStringList(terminology.ExcludePaths)
	for i, pattern := range terminology.ExcludePaths {
		if strings.TrimSpace(pattern) == "" {
			errs.add("terminology.exclude_paths[%d]: patterns must not be empty", i)
			continue
		}
		if _, err := pathpkg.Match(pattern, "placeholder"); err != nil {
			errs.add("terminology.exclude_paths[%d]: invalid pattern %q: %v", i, pattern, err)
		}
	}
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
	if runtime.Embedder.MaxResponseTokens < 0 {
		errs.add("runtime.embedder.max_response_tokens: must be >= 0")
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
	if runtime.Analysis.MaxResponseTokens < 0 {
		errs.add("runtime.analysis.max_response_tokens: must be >= 0")
	}
	validateChunkingKind(&errs, "runtime.chunking.spec", runtime.Chunking.Spec)
	validateChunkingKind(&errs, "runtime.chunking.doc", runtime.Chunking.Doc)
	validateChunkingContextualizer(&errs, "runtime.chunking.contextualizer", runtime.Chunking.Contextualizer)
	validateSearchConfig(&errs, "runtime.search", runtime.Search)
	return errs.err()
}

func buildChunkingConfig(raw rawChunking) ChunkingConfig {
	return ChunkingConfig{
		Spec:           buildChunkingKind(raw.Spec),
		Doc:            buildChunkingKind(raw.Doc),
		Contextualizer: buildChunkingContextualizer(raw.Contextualizer),
	}
}

func buildChunkingContextualizer(raw rawChunkingContextualizer) ChunkingContextualizerConfig {
	return ChunkingContextualizerConfig{
		Format: strings.TrimSpace(raw.Format),
	}
}

func buildChunkingKind(raw rawChunkingKind) ChunkingKindConfig {
	return ChunkingKindConfig{
		Policy:             strings.TrimSpace(raw.Policy),
		MaxTokens:          raw.MaxTokens,
		OverlapTokens:      raw.OverlapTokens,
		MaxSections:        raw.MaxSections,
		ChildMaxTokens:     raw.ChildMaxTokens,
		ChildOverlapTokens: raw.ChildOverlapTokens,
	}
}

func validateChunkingKind(errs *validationErrors, label string, kind ChunkingKindConfig) {
	if kind.IsZero() {
		return
	}
	switch kind.Policy {
	case "", ChunkPolicyMarkdown:
		// Markdown accepts any tuning; late-chunk-only fields here are a user bug.
		if kind.ChildMaxTokens != 0 || kind.ChildOverlapTokens != 0 {
			errs.add("%s: child_max_tokens / child_overlap_tokens only apply to policy %q", label, ChunkPolicyLateChunk)
		}
	case ChunkPolicyLateChunk:
		if kind.ChildMaxTokens <= 0 {
			errs.add("%s.child_max_tokens: must be > 0 for policy %q", label, ChunkPolicyLateChunk)
		}
		if kind.OverlapTokens != 0 {
			errs.add("%s.overlap_tokens: not supported for policy %q (use child_overlap_tokens)", label, ChunkPolicyLateChunk)
		}
	default:
		errs.add("%s.policy: unsupported policy %q (supported: %q, %q)", label, kind.Policy, ChunkPolicyMarkdown, ChunkPolicyLateChunk)
	}
	if kind.MaxTokens < 0 {
		errs.add("%s.max_tokens: must be >= 0", label)
	}
	if kind.OverlapTokens < 0 {
		errs.add("%s.overlap_tokens: must be >= 0", label)
	}
	// max_sections accepts any int here; Resolve applies the 0/negative semantics.
	if kind.ChildMaxTokens < 0 {
		errs.add("%s.child_max_tokens: must be >= 0", label)
	}
	if kind.ChildOverlapTokens < 0 {
		errs.add("%s.child_overlap_tokens: must be >= 0", label)
	}
}

func validateChunkingContextualizer(errs *validationErrors, label string, cfg ChunkingContextualizerConfig) {
	if cfg.IsZero() {
		return
	}
	switch cfg.Format {
	case ChunkContextualizerFormatTitleAncestry,
		ChunkContextualizerFormatRefTitle:
		// valid
	default:
		errs.add("%s.format: unsupported format %q (supported: %q, %q)",
			label, cfg.Format,
			ChunkContextualizerFormatTitleAncestry,
			ChunkContextualizerFormatRefTitle,
		)
	}
}

func buildSearchConfig(raw rawSearch) SearchConfig {
	return SearchConfig{
		Fusion:   buildFusionConfig(raw.Fusion),
		Reranker: strings.TrimSpace(raw.Reranker),
	}
}

func buildFusionConfig(raw rawSearchFusion) FusionConfig {
	return FusionConfig{
		Strategy: strings.TrimSpace(raw.Strategy),
		K:        raw.K,
	}
}

func validateSearchConfig(errs *validationErrors, label string, cfg SearchConfig) {
	validateFusionConfig(errs, label+".fusion", cfg.Fusion)
	switch cfg.Reranker {
	case "", SearchRerankerHistorical, SearchRerankerArmAwareHistorical:
		// valid
	default:
		errs.add("%s.reranker: unsupported reranker %q (supported: %q, %q)",
			label, cfg.Reranker,
			SearchRerankerHistorical,
			SearchRerankerArmAwareHistorical,
		)
	}
}

func validateFusionConfig(errs *validationErrors, label string, cfg FusionConfig) {
	if cfg.IsZero() {
		return
	}
	switch cfg.Strategy {
	case "", SearchFusionStrategyDefault:
		if cfg.K != 0 {
			errs.add("%s.k: only applies to strategy %q", label, SearchFusionStrategyRRF)
		}
	case SearchFusionStrategyRRF:
		if cfg.K <= 0 {
			errs.add("%s.k: must be > 0 for strategy %q", label, SearchFusionStrategyRRF)
		}
	default:
		errs.add("%s.strategy: unsupported strategy %q (supported: %q, %q)",
			label, cfg.Strategy,
			SearchFusionStrategyDefault,
			SearchFusionStrategyRRF,
		)
	}
}

func buildRuntimeProvider(raw rawRuntimeProvider, defaultProvider string) RuntimeProvider {
	return RuntimeProvider{
		Profile:              strings.TrimSpace(raw.Profile),
		Provider:             defaultString(strings.TrimSpace(raw.Provider), defaultProvider),
		Model:                strings.TrimSpace(raw.Model),
		Endpoint:             strings.TrimSpace(raw.Endpoint),
		APIKeyEnv:            strings.TrimSpace(raw.APIKeyEnv),
		TimeoutMS:            defaultOptionalInt(raw.TimeoutMS, 1000),
		MaxRetries:           defaultOptionalInt(raw.MaxRetries, 0),
		MaxResponseTokens:    defaultOptionalInt(raw.MaxResponseTokens, 0),
		providerSet:          strings.TrimSpace(raw.Provider) != "",
		modelSet:             strings.TrimSpace(raw.Model) != "",
		endpointSet:          strings.TrimSpace(raw.Endpoint) != "",
		apiKeyEnvSet:         strings.TrimSpace(raw.APIKeyEnv) != "",
		timeoutMSSet:         raw.TimeoutMS != nil,
		maxRetriesSet:        raw.MaxRetries != nil,
		maxResponseTokensSet: raw.MaxResponseTokens != nil,
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
	maxResponseTokensResolved := provider.maxResponseTokensSet

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
	if !maxResponseTokensResolved && hasProfile && profile.maxResponseTokensSet {
		resolved.MaxResponseTokens = profile.MaxResponseTokens
		maxResponseTokensResolved = true
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
	if !maxResponseTokensResolved {
		resolved.MaxResponseTokens = 0
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
	if profile.MaxResponseTokens < 0 {
		errs.add("%s.max_response_tokens: must be >= 0", label)
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

func pathResolutionDetail(label, raw, base, resolved string) string {
	return fmt.Sprintf(
		"%s %q resolves relative to %q as %q",
		label,
		raw,
		filepath.ToSlash(filepath.Clean(base)),
		filepath.ToSlash(filepath.Clean(resolved)),
	)
}

func sourcePathResolutionDetail(cfg *Config, source *Source, repoRootPath string) string {
	if source == nil {
		return ""
	}
	scope := "workspace.root"
	rootRaw := cfg.Workspace.Root
	if source.Repo != "" {
		scope = "workspace.repos[" + source.Repo + "].root"
		if root, ok := lookupWorkspaceRepoRootRaw(cfg.Workspace, source.Repo); ok {
			rootRaw = root
		}
	}
	return fmt.Sprintf(
		"%s %q resolves relative to config base %q as %q, so source path %q resolves to %q",
		scope,
		rootRaw,
		filepath.ToSlash(filepath.Clean(cfg.ConfigDir)),
		filepath.ToSlash(filepath.Clean(repoRootPath)),
		source.Path,
		filepath.ToSlash(filepath.Clean(source.ResolvedPath)),
	)
}

func lookupWorkspaceRepoRootRaw(workspace Workspace, repoID string) (string, bool) {
	for _, repo := range workspace.Repos {
		if repo.ID == repoID {
			return repo.Root, true
		}
	}
	return "", false
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
