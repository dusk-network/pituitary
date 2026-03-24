package source

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
)

const (
	discoverDefaultConfigName = "pituitary.toml"
	discoverLocalConfigDir    = ".pituitary"
)

// DiscoverOptions controls how workspace discovery runs.
type DiscoverOptions struct {
	RootPath   string
	ConfigPath string
	Write      bool
}

// DiscoverResult reports a conservative source proposal for one workspace.
type DiscoverResult struct {
	WorkspaceRoot string             `json:"workspace_root"`
	ConfigPath    string             `json:"config_path"`
	WroteConfig   bool               `json:"wrote_config,omitempty"`
	Config        string             `json:"config"`
	Sources       []DiscoveredSource `json:"sources"`
	Preview       *PreviewResult     `json:"preview,omitempty"`
}

// DiscoveredSource describes one proposed source block and the items behind it.
type DiscoveredSource struct {
	Name       string           `json:"name"`
	Adapter    string           `json:"adapter"`
	Kind       string           `json:"kind"`
	Path       string           `json:"path"`
	Files      []string         `json:"files,omitempty"`
	Confidence string           `json:"confidence"`
	Rationale  []string         `json:"rationale,omitempty"`
	ItemCount  int              `json:"item_count"`
	Items      []DiscoveredItem `json:"items,omitempty"`
}

// DiscoveredItem explains why one file was selected during discovery.
type DiscoveredItem struct {
	Path       string   `json:"path"`
	Confidence string   `json:"confidence"`
	Rationale  []string `json:"rationale,omitempty"`
}

type discoveredCandidate struct {
	path       string
	confidence string
	score      int
	rationale  []string
}

// DiscoverWorkspace scans one repo-like directory and proposes a starting
// local config plus a preview of what that config would index.
func DiscoverWorkspace(options DiscoverOptions) (*DiscoverResult, error) {
	rootPath := strings.TrimSpace(options.RootPath)
	if rootPath == "" {
		rootPath = "."
	}

	workspaceRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}
	info, err := os.Stat(workspaceRoot)
	switch {
	case err == nil && !info.IsDir():
		return nil, fmt.Errorf("workspace path %q is not a directory", filepath.ToSlash(rootPath))
	case err != nil:
		return nil, fmt.Errorf("workspace path %q: %w", filepath.ToSlash(rootPath), err)
	}

	specCandidates, specBundleDirs, err := discoverSpecBundleCandidates(workspaceRoot)
	if err != nil {
		return nil, err
	}
	contractCandidates, docCandidates, err := discoverMarkdownCandidates(workspaceRoot, specBundleDirs)
	if err != nil {
		return nil, err
	}

	sources := buildDiscoveredSources(workspaceRoot, specCandidates, contractCandidates, docCandidates)
	if len(sources) == 0 {
		return nil, fmt.Errorf("no likely sources discovered under %s", filepath.ToSlash(workspaceRoot))
	}

	cfg, err := buildDiscoveredConfig(workspaceRoot, strings.TrimSpace(options.ConfigPath), sources)
	if err != nil {
		return nil, err
	}
	preview, err := PreviewFromConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("preview discovered config: %w", err)
	}
	configText, err := config.Render(cfg)
	if err != nil {
		return nil, fmt.Errorf("render discovered config: %w", err)
	}

	result := &DiscoverResult{
		WorkspaceRoot: workspaceRoot,
		ConfigPath:    cfg.ConfigPath,
		Config:        configText,
		Sources:       sources,
		Preview:       preview,
	}

	if options.Write {
		if err := writeDiscoveredConfig(cfg.ConfigPath, configText); err != nil {
			return nil, err
		}
		loaded, err := config.Load(cfg.ConfigPath)
		if err != nil {
			return nil, fmt.Errorf("reload written config: %w", err)
		}
		preview, err := PreviewFromConfig(loaded)
		if err != nil {
			return nil, fmt.Errorf("preview written config: %w", err)
		}
		result.Preview = preview
		result.WroteConfig = true
	}

	return result, nil
}

func discoverSpecBundleCandidates(workspaceRoot string) ([]discoveredCandidate, map[string]struct{}, error) {
	var candidates []discoveredCandidate
	bundleDirs := make(map[string]struct{})

	err := filepath.WalkDir(workspaceRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if shouldSkipDiscoveryDir(workspaceRoot, path, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "spec.toml" {
			return nil
		}

		bundleDir := filepath.Dir(path)
		ok, rationale := isValidDiscoveredSpecBundle(workspaceRoot, bundleDir)
		if !ok {
			return nil
		}
		bundleDirs[bundleDir] = struct{}{}
		candidates = append(candidates, discoveredCandidate{
			path:       workspaceRelative(workspaceRoot, path),
			confidence: "high",
			score:      100,
			rationale:  rationale,
		})
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("scan workspace for spec bundles: %w", err)
	}

	sortDiscoveredCandidates(candidates)
	return candidates, bundleDirs, nil
}

func isValidDiscoveredSpecBundle(workspaceRoot, bundleDir string) (bool, []string) {
	specPath := filepath.Join(bundleDir, "spec.toml")
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		return false, nil
	}
	raw, err := parseSpecBundle(specBytes)
	if err != nil {
		return false, nil
	}
	if err := validateRawSpec(workspaceRoot, "discover", bundleDir, raw); err != nil {
		return false, nil
	}

	bodyPath := filepath.Clean(filepath.Join(bundleDir, raw.Body))
	if !pathWithinRoot(bundleDir, bodyPath) {
		return false, nil
	}
	info, err := os.Stat(bodyPath)
	if err != nil || info.IsDir() {
		return false, nil
	}

	return true, []string{
		"valid spec.toml bundle",
		fmt.Sprintf("body file %s exists", workspaceRelative(workspaceRoot, bodyPath)),
	}
}

func discoverMarkdownCandidates(workspaceRoot string, specBundleDirs map[string]struct{}) ([]discoveredCandidate, []discoveredCandidate, error) {
	var contractCandidates []discoveredCandidate
	var docCandidates []discoveredCandidate

	err := filepath.WalkDir(workspaceRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if shouldSkipDiscoveryDir(workspaceRoot, path, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		if withinAnyDiscoveryRoot(path, specBundleDirs) {
			return nil
		}

		kind, candidate, ok := classifyMarkdownCandidate(workspaceRoot, path)
		if !ok {
			return nil
		}
		if kind == config.SourceKindMarkdownContract {
			contractCandidates = append(contractCandidates, candidate)
			return nil
		}
		docCandidates = append(docCandidates, candidate)
		return nil
	})
	if err != nil {
		return nil, nil, fmt.Errorf("scan workspace for markdown sources: %w", err)
	}

	sortDiscoveredCandidates(contractCandidates)
	sortDiscoveredCandidates(docCandidates)
	return contractCandidates, docCandidates, nil
}

func classifyMarkdownCandidate(workspaceRoot, path string) (string, discoveredCandidate, bool) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", discoveredCandidate{}, false
	}

	relPath := workspaceRelative(workspaceRoot, path)
	title, _ := docTitleWithSource(path, body)
	titleLower := strings.ToLower(title)
	pathTokens := discoveryTokens(relPath)
	pathSet := make(map[string]struct{}, len(pathTokens))
	for _, token := range pathTokens {
		pathSet[token] = struct{}{}
	}

	lines := strings.Split(string(body), "\n")
	contractScore, contractReasons := scoreMarkdownContractCandidate(pathSet, titleLower, lines)
	docScore, docReasons := scoreMarkdownDocCandidate(pathSet, titleLower)

	switch {
	case contractScore >= 3 && contractScore >= docScore+1:
		return config.SourceKindMarkdownContract, discoveredCandidate{
			path:       relPath,
			confidence: discoveryConfidence(contractScore),
			score:      contractScore,
			rationale:  contractReasons,
		}, true
	case docScore >= 2:
		return config.SourceKindMarkdownDocs, discoveredCandidate{
			path:       relPath,
			confidence: discoveryConfidence(docScore),
			score:      docScore,
			rationale:  docReasons,
		}, true
	default:
		return "", discoveredCandidate{}, false
	}
}

func scoreMarkdownContractCandidate(pathTokens map[string]struct{}, titleLower string, lines []string) (int, []string) {
	var (
		score   int
		reasons []string
	)

	metadataSignals := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Ref:"),
			strings.HasPrefix(trimmed, "ID:"),
			strings.HasPrefix(trimmed, "Status:"),
			strings.HasPrefix(trimmed, "Domain:"),
			strings.HasPrefix(trimmed, "Depends On:"),
			strings.HasPrefix(trimmed, "Supersedes:"),
			strings.HasPrefix(trimmed, "Applies To:"):
			metadataSignals++
		}
	}
	if metadataSignals > 0 {
		score += minInt(3, metadataSignals+1)
		reasons = append(reasons, "contains contract-style metadata fields")
	}
	if hasAnyDiscoveryToken(pathTokens, "rfc", "rfcs", "contract", "contracts", "proposal", "proposals", "specification", "specifications") {
		score += 2
		reasons = append(reasons, "lives under a contract-like path")
	}
	if containsAny(titleLower, "contract", "specification", "request for comments") {
		score++
		reasons = append(reasons, "title looks like a contract")
	} else if strings.HasPrefix(titleLower, "rfc ") {
		score++
		reasons = append(reasons, "title starts with RFC")
	}
	if hasAnyDiscoveryToken(pathTokens, "docs", "guides", "guide", "runbooks", "runbook") && metadataSignals == 0 {
		score--
	}

	return score, reasons
}

func scoreMarkdownDocCandidate(pathTokens map[string]struct{}, titleLower string) (int, []string) {
	var (
		score   int
		reasons []string
	)

	if hasAnyDiscoveryToken(pathTokens, "guides", "guide", "runbooks", "runbook", "playbooks", "playbook", "handbook", "manuals", "manual", "operations", "ops", "reference", "references") {
		score += 3
		reasons = append(reasons, "lives under a guide, runbook, operations, or reference-doc path")
	} else if hasAnyDiscoveryToken(pathTokens, "docs", "doc") {
		score++
		reasons = append(reasons, "lives under a docs path")
	}
	if containsAny(titleLower, "guide", "runbook", "playbook", "manual", "operator", "operations", "reference") {
		score++
		reasons = append(reasons, "title looks like a guide, runbook, operations, or reference document")
	}
	if hasAnyDiscoveryToken(pathTokens, "development", "dev", "test", "tests", "testing", "example", "examples", "fixture", "fixtures") {
		score -= 3
		reasons = append(reasons, "skips development or fixture material")
	}

	return score, reasons
}

func buildDiscoveredSources(workspaceRoot string, specs, contracts, docs []discoveredCandidate) []DiscoveredSource {
	var sources []DiscoveredSource
	usedNames := map[string]struct{}{}

	if source, ok := buildDiscoveredSource(workspaceRoot, "specs", config.SourceKindSpecBundle, specs, []string{
		fmt.Sprintf("discovered %d valid spec bundle(s)", len(specs)),
		"uses exact file selectors to stay conservative",
	}, usedNames); ok {
		sources = append(sources, source)
	}
	if source, ok := buildDiscoveredSource(workspaceRoot, "contracts", config.SourceKindMarkdownContract, contracts, []string{
		fmt.Sprintf("discovered %d likely markdown contract file(s)", len(contracts)),
		"uses exact file selectors to avoid overreaching",
	}, usedNames); ok {
		sources = append(sources, source)
	}
	if source, ok := buildDiscoveredSource(workspaceRoot, "docs", config.SourceKindMarkdownDocs, docs, []string{
		fmt.Sprintf("discovered %d likely guide, runbook, operations, or reference-doc file(s)", len(docs)),
		"uses exact file selectors to avoid indexing unrelated markdown",
	}, usedNames); ok {
		sources = append(sources, source)
	}

	return sources
}

func buildDiscoveredSource(workspaceRoot, fallbackName, kind string, candidates []discoveredCandidate, defaultRationale []string, usedNames map[string]struct{}) (DiscoveredSource, bool) {
	if len(candidates) == 0 {
		return DiscoveredSource{}, false
	}

	sourceRoot := discoveryCommonRoot(workspaceRoot, candidates)
	relativeRoot := workspaceRelative(workspaceRoot, sourceRoot)
	if relativeRoot == "" {
		relativeRoot = "."
	}

	files := make([]string, 0, len(candidates))
	items := make([]DiscoveredItem, 0, len(candidates))
	totalScore := 0
	for _, candidate := range candidates {
		relativeFile, err := filepath.Rel(sourceRoot, filepath.Join(workspaceRoot, filepath.FromSlash(candidate.path)))
		if err != nil {
			continue
		}
		files = append(files, filepath.ToSlash(relativeFile))
		items = append(items, DiscoveredItem{
			Path:       candidate.path,
			Confidence: candidate.confidence,
			Rationale:  append([]string(nil), candidate.rationale...),
		})
		totalScore += candidate.score
	}
	sort.Strings(files)
	sort.Slice(items, func(i, j int) bool {
		return items[i].Path < items[j].Path
	})

	name := filepath.Base(sourceRoot)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = fallbackName
	}
	name = uniqueDiscoverySourceName(name, kind, usedNames)

	rationale := append([]string(nil), defaultRationale...)
	averageScore := totalScore / len(candidates)
	return DiscoveredSource{
		Name:       name,
		Adapter:    config.AdapterFilesystem,
		Kind:       kind,
		Path:       filepath.ToSlash(relativeRoot),
		Files:      files,
		Confidence: discoveryConfidence(averageScore),
		Rationale:  rationale,
		ItemCount:  len(items),
		Items:      items,
	}, true
}

func buildDiscoveredConfig(workspaceRoot, requestedConfigPath string, sources []DiscoveredSource) (*config.Config, error) {
	configPath, configDir, workspaceSetting, err := resolveDiscoveredConfigPath(workspaceRoot, requestedConfigPath)
	if err != nil {
		return nil, err
	}
	cfg := &config.Config{
		ConfigPath: configPath,
		ConfigDir:  configDir,
		Workspace: config.Workspace{
			Root:      workspaceSetting,
			RootPath:  workspaceRoot,
			IndexPath: filepath.ToSlash(filepath.Join(".pituitary", "pituitary.db")),
		},
		Runtime: config.Runtime{
			Embedder: config.RuntimeProvider{
				Provider:   "fixture",
				Model:      "fixture-8d",
				TimeoutMS:  1000,
				MaxRetries: 0,
			},
			Analysis: config.RuntimeProvider{
				Provider:   "disabled",
				TimeoutMS:  1000,
				MaxRetries: 0,
			},
		},
		Sources: make([]config.Source, 0, len(sources)),
	}
	for _, source := range sources {
		cfg.Sources = append(cfg.Sources, config.Source{
			Name:    source.Name,
			Adapter: source.Adapter,
			Kind:    source.Kind,
			Path:    source.Path,
			Files:   append([]string(nil), source.Files...),
		})
	}

	if err := config.Validate(cfg); err != nil {
		return nil, fmt.Errorf("validate discovered config: %w", err)
	}
	return cfg, nil
}

func resolveDiscoveredConfigPath(workspaceRoot, requestedConfigPath string) (string, string, string, error) {
	configPath := strings.TrimSpace(requestedConfigPath)
	if configPath == "" {
		configPath = filepath.Join(workspaceRoot, discoverLocalConfigDir, discoverDefaultConfigName)
	} else if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(workspaceRoot, configPath)
	}

	absoluteConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve discovered config path %q: %w", configPath, err)
	}
	if !pathWithinRoot(workspaceRoot, absoluteConfigPath) {
		return "", "", "", fmt.Errorf("discover config path %q resolves outside workspace root %q", requestedConfigPath, filepath.ToSlash(workspaceRoot))
	}

	configDir := discoveredConfigBaseDir(absoluteConfigPath)
	workspaceSetting, err := filepath.Rel(configDir, workspaceRoot)
	if err != nil {
		return "", "", "", fmt.Errorf("relativize workspace root against config path: %w", err)
	}
	workspaceSetting = filepath.ToSlash(workspaceSetting)
	if workspaceSetting == "" {
		workspaceSetting = "."
	}
	return absoluteConfigPath, configDir, workspaceSetting, nil
}

func discoveredConfigBaseDir(configPath string) string {
	configDir := filepath.Dir(configPath)
	if filepath.Base(configPath) == discoverDefaultConfigName && filepath.Base(configDir) == discoverLocalConfigDir {
		return filepath.Dir(configDir)
	}
	return configDir
}

func writeDiscoveredConfig(path, content string) error {
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return fmt.Errorf("refusing to overwrite existing config at %s", filepath.ToSlash(path))
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat config path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write discovered config: %w", err)
	}
	return nil
}

func shouldSkipDiscoveryDir(workspaceRoot, path, base string) bool {
	if filepath.Clean(path) == filepath.Clean(workspaceRoot) {
		return false
	}

	switch strings.ToLower(base) {
	case ".git", ".pituitary", ".claude", ".idea", ".vscode", "node_modules", "vendor", "dist", "build", "target", "tmp", "testdata":
		return true
	}
	return strings.HasPrefix(base, ".")
}

func withinAnyDiscoveryRoot(path string, roots map[string]struct{}) bool {
	for root := range roots {
		if pathWithinRoot(root, path) {
			return true
		}
	}
	return false
}

func discoveryTokens(relPath string) []string {
	fields := strings.FieldsFunc(strings.ToLower(filepath.ToSlash(relPath)), func(r rune) bool {
		return r == '/' || r == '-' || r == '_' || r == '.'
	})
	return fields
}

func hasAnyDiscoveryToken(tokens map[string]struct{}, values ...string) bool {
	for _, value := range values {
		if _, ok := tokens[value]; ok {
			return true
		}
	}
	return false
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func discoveryConfidence(score int) string {
	switch {
	case score >= 4:
		return "high"
	case score >= 2:
		return "medium"
	default:
		return "low"
	}
}

func discoveryCommonRoot(workspaceRoot string, candidates []discoveredCandidate) string {
	common := workspaceRoot
	for i, candidate := range candidates {
		absolute := filepath.Join(workspaceRoot, filepath.FromSlash(candidate.path))
		dir := filepath.Dir(absolute)
		if i == 0 {
			common = dir
			continue
		}
		common = commonDirectory(common, dir)
	}
	if common == "" {
		return workspaceRoot
	}
	return common
}

func commonDirectory(left, right string) string {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	for !pathWithinRoot(left, right) {
		parent := filepath.Dir(left)
		if parent == left {
			return filepath.VolumeName(left) + string(filepath.Separator)
		}
		left = parent
	}
	return left
}

func uniqueDiscoverySourceName(base, kind string, used map[string]struct{}) string {
	name := sanitizeDiscoverySourceName(base)
	if name == "" {
		name = kind
	}
	if _, exists := used[name]; !exists {
		used[name] = struct{}{}
		return name
	}

	suffix := sanitizeDiscoverySourceName(strings.ReplaceAll(kind, "_", "-"))
	candidate := name + "-" + suffix
	if _, exists := used[candidate]; !exists {
		used[candidate] = struct{}{}
		return candidate
	}

	for i := 2; ; i++ {
		candidate = fmt.Sprintf("%s-%d", name+"-"+suffix, i)
		if _, exists := used[candidate]; !exists {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

func sanitizeDiscoverySourceName(value string) string {
	value = strings.TrimSpace(strings.ToLower(filepath.ToSlash(value)))
	value = strings.ReplaceAll(value, "/", "-")
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-':
			builder.WriteRune(r)
		}
	}
	return strings.Trim(builder.String(), "-")
}

func sortDiscoveredCandidates(candidates []discoveredCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].path == candidates[j].path {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].path < candidates[j].path
	})
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func discoveryPathJoin(parts ...string) string {
	return pathpkg.Join(parts...)
}
