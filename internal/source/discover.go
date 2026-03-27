package source

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/diag"
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
	Logger     *diag.Logger
}

// DiscoverResult reports a conservative source proposal for one workspace.
type DiscoverResult struct {
	WorkspaceRoot string             `json:"workspace_root"`
	ConfigPath    string             `json:"config_path"`
	WroteConfig   bool               `json:"wrote_config,omitempty"`
	Config        string             `json:"config"`
	Sources       []DiscoveredSource `json:"sources"`
	Preview       *PreviewResult     `json:"preview,omitempty"`
	Warnings      []DiscoverWarning  `json:"warnings,omitempty"`
}

// DiscoverWarning reports non-fatal discovery decisions that changed the
// generated source selection.
type DiscoverWarning struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Ref         string `json:"ref,omitempty"`
	KeptPath    string `json:"kept_path,omitempty"`
	SkippedPath string `json:"skipped_path,omitempty"`
	Reason      string `json:"reason,omitempty"`
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
	path        string
	kind        string
	ref         string
	contentHash string
	confidence  string
	score       int
	modifiedAt  time.Time
	rationale   []string
}

type markdownCandidateAssessment struct {
	Kind         string
	Candidate    discoveredCandidate
	RelativePath string
	Selected     bool
	Reason       string
}

// DiscoverWorkspace scans one repo-like directory and proposes a starting
// local config plus a preview of what that config would index.
func DiscoverWorkspace(options DiscoverOptions) (*DiscoverResult, error) {
	logger := options.Logger
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
	logger.Infof("discover", "scanning workspace %s", filepath.ToSlash(workspaceRoot))

	specCandidates, specBundleDirs, rejectedSpecBundles, err := discoverSpecBundleCandidates(workspaceRoot, logger)
	if err != nil {
		return nil, err
	}
	contractCandidates, docCandidates, rejectedMarkdown, err := discoverMarkdownCandidates(workspaceRoot, specBundleDirs, logger)
	if err != nil {
		return nil, err
	}
	specCandidates, contractCandidates, warnings := dedupeDiscoveredSpecCandidates(specCandidates, contractCandidates)
	logger.Infof(
		"discover",
		"accepted %d spec bundle(s), %d markdown contract(s), and %d markdown doc file(s); rejected %d spec bundle candidate(s) and %d markdown file(s)",
		len(specCandidates),
		len(contractCandidates),
		len(docCandidates),
		rejectedSpecBundles,
		rejectedMarkdown,
	)
	if len(specCandidates) == 0 {
		logger.Warnf("discover", "discovery selected no spec bundles")
	}
	for _, warning := range warnings {
		logger.Warnf("discover", "%s", warning.Message)
	}

	sources := buildDiscoveredSources(workspaceRoot, specCandidates, contractCandidates, docCandidates)
	if len(sources) == 0 {
		logger.Warnf("discover", "discovery selected no source blocks under %s", filepath.ToSlash(workspaceRoot))
		return nil, fmt.Errorf("no likely sources discovered under %s", filepath.ToSlash(workspaceRoot))
	}
	logger.Infof("discover", "generated %d source block(s)", len(sources))

	cfg, err := buildDiscoveredConfig(workspaceRoot, strings.TrimSpace(options.ConfigPath), sources)
	if err != nil {
		return nil, err
	}
	preview, err := PreviewFromConfigWithOptions(cfg, PreviewOptions{Logger: logger})
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
		Warnings:      warnings,
	}

	if options.Write {
		if err := writeDiscoveredConfig(cfg.ConfigPath, configText); err != nil {
			return nil, err
		}
		loaded, err := config.Load(cfg.ConfigPath)
		if err != nil {
			return nil, fmt.Errorf("reload written config: %w", err)
		}
		preview, err := PreviewFromConfigWithOptions(loaded, PreviewOptions{Logger: logger})
		if err != nil {
			return nil, fmt.Errorf("preview written config: %w", err)
		}
		result.Preview = preview
		result.WroteConfig = true
		logger.Infof("discover", "wrote generated config to %s", filepath.ToSlash(cfg.ConfigPath))
	}

	return result, nil
}

// WriteDiscoveredConfig writes a rendered discovery config to disk.
func WriteDiscoveredConfig(path, content string) error {
	return writeDiscoveredConfig(path, content)
}

func discoverSpecBundleCandidates(workspaceRoot string, logger *diag.Logger) ([]discoveredCandidate, map[string]struct{}, int, error) {
	var candidates []discoveredCandidate
	bundleDirs := make(map[string]struct{})
	rejected := 0

	err := filepath.WalkDir(workspaceRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if shouldSkipDiscoveryDir(workspaceRoot, path, d.Name()) {
				logger.Debugf("discover", "skipping directory %s", workspaceRelative(workspaceRoot, path))
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "spec.toml" {
			return nil
		}

		bundleDir := filepath.Dir(path)
		ok, rationale, rejectionReason := assessDiscoveredSpecBundle(workspaceRoot, bundleDir)
		if !ok {
			rejected++
			logger.Debugf("discover", "rejected spec bundle %s: %s", workspaceRelative(workspaceRoot, path), rejectionReason)
			return nil
		}
		bundleDirs[bundleDir] = struct{}{}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("inspect discovered spec bundle %q: %w", workspaceRelative(workspaceRoot, bundleDir), err)
		}
		ref, contentHash, err := discoverSpecBundleIdentity(bundleDir)
		if err != nil {
			return err
		}
		candidates = append(candidates, discoveredCandidate{
			path:        workspaceRelative(workspaceRoot, path),
			kind:        config.SourceKindSpecBundle,
			ref:         ref,
			contentHash: contentHash,
			confidence:  "high",
			score:       100,
			modifiedAt:  info.ModTime(),
			rationale:   rationale,
		})
		logger.Debugf("discover", "accepted spec bundle %s (%s)", workspaceRelative(workspaceRoot, path), strings.Join(rationale, "; "))
		return nil
	})
	if err != nil {
		return nil, nil, 0, fmt.Errorf("scan workspace for spec bundles: %w", err)
	}

	sortDiscoveredCandidates(candidates)
	return candidates, bundleDirs, rejected, nil
}

func discoverSpecBundleIdentity(bundleDir string) (string, string, error) {
	specPath := filepath.Join(bundleDir, "spec.toml")
	// #nosec G304 -- specPath is the fixed bundle manifest inside a discovered workspace directory.
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		return "", "", fmt.Errorf("read discovered spec bundle %q: %w", filepath.ToSlash(bundleDir), err)
	}
	raw, err := parseSpecBundle(specBytes)
	if err != nil {
		return "", "", fmt.Errorf("parse discovered spec bundle %q: %w", filepath.ToSlash(specPath), err)
	}
	bodyPath := filepath.Clean(filepath.Join(bundleDir, raw.Body))
	if !pathWithinRoot(bundleDir, bodyPath) {
		return "", "", fmt.Errorf("read discovered spec body %q: path escapes bundle", filepath.ToSlash(bodyPath))
	}
	// #nosec G304 -- bodyPath is validated to remain inside the discovered bundle directory.
	bodyBytes, err := os.ReadFile(bodyPath)
	if err != nil {
		return "", "", fmt.Errorf("read discovered spec body %q: %w", filepath.ToSlash(bodyPath), err)
	}
	return raw.ID, joinedContentHash(specBytes, bodyBytes), nil
}

func assessDiscoveredSpecBundle(workspaceRoot, bundleDir string) (bool, []string, string) {
	specPath := filepath.Join(bundleDir, "spec.toml")
	// #nosec G304 -- specPath is the fixed bundle manifest inside a discovered workspace directory.
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		return false, nil, fmt.Sprintf("read spec.toml: %v", err)
	}
	raw, err := parseSpecBundle(specBytes)
	if err != nil {
		return false, nil, "invalid spec.toml: " + err.Error()
	}
	if err := validateRawSpec(workspaceRoot, "discover", bundleDir, raw); err != nil {
		return false, nil, err.Error()
	}

	bodyPath := filepath.Clean(filepath.Join(bundleDir, raw.Body))
	if !pathWithinRoot(bundleDir, bodyPath) {
		return false, nil, fmt.Sprintf("body path %q escapes the bundle directory", raw.Body)
	}
	info, err := os.Stat(bodyPath)
	switch {
	case err != nil:
		return false, nil, fmt.Sprintf("body file %s does not exist", workspaceRelative(workspaceRoot, bodyPath))
	case info.IsDir():
		return false, nil, fmt.Sprintf("body path %s is a directory", workspaceRelative(workspaceRoot, bodyPath))
	}

	return true, []string{
		"valid spec.toml bundle",
		fmt.Sprintf("body file %s exists", workspaceRelative(workspaceRoot, bodyPath)),
	}, ""
}

func discoverMarkdownCandidates(workspaceRoot string, specBundleDirs map[string]struct{}, logger *diag.Logger) ([]discoveredCandidate, []discoveredCandidate, int, error) {
	var contractCandidates []discoveredCandidate
	var docCandidates []discoveredCandidate
	rejected := 0

	err := filepath.WalkDir(workspaceRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if shouldSkipDiscoveryDir(workspaceRoot, path, d.Name()) {
				logger.Debugf("discover", "skipping directory %s", workspaceRelative(workspaceRoot, path))
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		if withinAnyDiscoveryRoot(path, specBundleDirs) {
			logger.Debugf("discover", "skipping markdown %s: inside a discovered spec bundle", workspaceRelative(workspaceRoot, path))
			return nil
		}

		assessment, err := classifyMarkdownCandidate(workspaceRoot, path)
		if err != nil {
			return err
		}
		if !assessment.Selected {
			rejected++
			logger.Debugf("discover", "rejected markdown %s: %s", assessment.RelativePath, assessment.Reason)
			return nil
		}
		if assessment.Kind == config.SourceKindMarkdownContract {
			contractCandidates = append(contractCandidates, assessment.Candidate)
			logger.Debugf("discover", "accepted markdown contract %s (%s)", assessment.Candidate.path, strings.Join(assessment.Candidate.rationale, "; "))
			return nil
		}
		docCandidates = append(docCandidates, assessment.Candidate)
		logger.Debugf("discover", "accepted markdown doc %s (%s)", assessment.Candidate.path, strings.Join(assessment.Candidate.rationale, "; "))
		return nil
	})
	if err != nil {
		return nil, nil, 0, fmt.Errorf("scan workspace for markdown sources: %w", err)
	}

	sortDiscoveredCandidates(contractCandidates)
	sortDiscoveredCandidates(docCandidates)
	return contractCandidates, docCandidates, rejected, nil
}

func classifyMarkdownCandidate(workspaceRoot, path string) (markdownCandidateAssessment, error) {
	relPath := workspaceRelative(workspaceRoot, path)
	// #nosec G304 -- path comes from filepath.WalkDir rooted at the workspace.
	body, err := os.ReadFile(path)
	if err != nil {
		return markdownCandidateAssessment{
			RelativePath: relPath,
			Reason:       fmt.Sprintf("read markdown: %v", err),
		}, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return markdownCandidateAssessment{
			RelativePath: relPath,
			Reason:       fmt.Sprintf("stat markdown: %v", err),
		}, nil
	}

	title, _ := docTitleWithSource(path, body)
	titleLower := strings.ToLower(title)
	pathTokens := discoveryTokens(relPath)
	pathSet := make(map[string]struct{}, len(pathTokens))
	for _, token := range pathTokens {
		pathSet[token] = struct{}{}
	}
	titleTokens := make(map[string]struct{}, len(discoveryTokens(titleLower)))
	for _, token := range discoveryTokens(titleLower) {
		titleTokens[token] = struct{}{}
	}

	lines := strings.Split(string(body), "\n")
	contractScore, contractReasons := scoreMarkdownContractCandidate(pathSet, titleLower, lines)
	docScore, docReasons := scoreMarkdownDocCandidate(pathSet, titleLower, titleTokens, filepath.Base(relPath))

	switch {
	case contractScore >= 3 && contractScore >= docScore+1:
		ref, contentHash, err := discoverMarkdownContractIdentity(workspaceRoot, path, body)
		if err != nil {
			return markdownCandidateAssessment{}, err
		}
		return markdownCandidateAssessment{
			Kind:         config.SourceKindMarkdownContract,
			RelativePath: relPath,
			Selected:     true,
			Candidate: discoveredCandidate{
				path:        relPath,
				kind:        config.SourceKindMarkdownContract,
				ref:         ref,
				contentHash: contentHash,
				confidence:  discoveryConfidence(contractScore),
				score:       contractScore,
				modifiedAt:  info.ModTime(),
				rationale:   contractReasons,
			},
		}, nil
	case docScore >= 2:
		return markdownCandidateAssessment{
			Kind:         config.SourceKindMarkdownDocs,
			RelativePath: relPath,
			Selected:     true,
			Candidate: discoveredCandidate{
				path:       relPath,
				kind:       config.SourceKindMarkdownDocs,
				confidence: discoveryConfidence(docScore),
				score:      docScore,
				modifiedAt: info.ModTime(),
				rationale:  docReasons,
			},
		}, nil
	default:
		return markdownCandidateAssessment{
			RelativePath: relPath,
			Reason: fmt.Sprintf(
				"did not meet discovery thresholds (%s; %s)",
				discoveryScoreSummary("contract", contractScore, contractReasons),
				discoveryScoreSummary("doc", docScore, docReasons),
			),
		}, nil
	}
}

func discoverMarkdownContractIdentity(workspaceRoot, path string, body []byte) (string, string, error) {
	fields := inferMarkdownContractFields(body)
	ref := strings.TrimSpace(fields.Ref)
	if ref == "" {
		var err error
		ref, err = markdownContractRefForPath(workspaceRoot, path)
		if err != nil {
			return "", "", err
		}
	}
	return ref, contentHash(body), nil
}

func dedupeDiscoveredSpecCandidates(specs, contracts []discoveredCandidate) ([]discoveredCandidate, []discoveredCandidate, []DiscoverWarning) {
	all := append(append([]discoveredCandidate(nil), specs...), contracts...)
	if len(all) == 0 {
		return nil, nil, nil
	}

	groups := make(map[string][]discoveredCandidate)
	var passthrough []discoveredCandidate
	for _, candidate := range all {
		ref := strings.TrimSpace(candidate.ref)
		if ref == "" {
			passthrough = append(passthrough, candidate)
			continue
		}
		groups[ref] = append(groups[ref], candidate)
	}

	refs := make([]string, 0, len(groups))
	for ref := range groups {
		refs = append(refs, ref)
	}
	sort.Strings(refs)

	deduped := append([]discoveredCandidate(nil), passthrough...)
	var warnings []DiscoverWarning
	for _, ref := range refs {
		group := groups[ref]
		preferred := group[0]
		for _, candidate := range group[1:] {
			if prefersDiscoveredCandidate(candidate, preferred) {
				preferred = candidate
			}
		}
		deduped = append(deduped, preferred)
		for _, candidate := range group {
			if candidate.path == preferred.path {
				continue
			}
			if candidate.contentHash != "" && preferred.contentHash != "" && candidate.contentHash == preferred.contentHash {
				continue
			}
			reason := discoverConflictPreferenceReason(preferred, candidate)
			warnings = append(warnings, DiscoverWarning{
				Code:        "duplicate_spec_ref_skipped",
				Message:     fmt.Sprintf("skipped %s during discovery: ref %q conflicts with %s; kept %s (%s)", candidate.path, ref, preferred.path, preferred.path, reason),
				Ref:         ref,
				KeptPath:    preferred.path,
				SkippedPath: candidate.path,
				Reason:      reason,
			})
		}
	}

	sortDiscoveredCandidates(deduped)
	var dedupedSpecs []discoveredCandidate
	var dedupedContracts []discoveredCandidate
	for _, candidate := range deduped {
		switch candidate.kind {
		case config.SourceKindSpecBundle:
			dedupedSpecs = append(dedupedSpecs, candidate)
		case config.SourceKindMarkdownContract:
			dedupedContracts = append(dedupedContracts, candidate)
		}
	}
	return dedupedSpecs, dedupedContracts, warnings
}

func prefersDiscoveredCandidate(left, right discoveredCandidate) bool {
	if left.score != right.score {
		return left.score > right.score
	}
	if !left.modifiedAt.Equal(right.modifiedAt) {
		return left.modifiedAt.After(right.modifiedAt)
	}
	return left.path < right.path
}

func discoverConflictPreferenceReason(preferred, skipped discoveredCandidate) string {
	switch {
	case preferred.score != skipped.score:
		return "higher discovery score"
	case !preferred.modifiedAt.Equal(skipped.modifiedAt):
		return "newer file"
	default:
		return "stable path tie-breaker"
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

func scoreMarkdownDocCandidate(pathTokens map[string]struct{}, titleLower string, titleTokens map[string]struct{}, filename string) (int, []string) {
	var (
		score   int
		reasons []string
	)

	// Well-known intent artifacts get a strong doc signal regardless of directory.
	wellKnownDocFiles := map[string]bool{
		"claude.md": true, "agents.md": true, "architecture.md": true,
		"contributing.md": true, "readme.md": true,
	}
	if wellKnownDocFiles[strings.ToLower(filename)] {
		score += 3
		reasons = append(reasons, "well-known intent artifact")
	}

	if hasAnyDiscoveryToken(pathTokens, "guides", "guide", "runbooks", "runbook", "playbooks", "playbook", "handbook", "manuals", "manual", "operations", "ops", "reference", "references") {
		score += 3
		reasons = append(reasons, "lives under a guide, runbook, operations, or reference-doc path")
	} else if hasAnyDiscoveryToken(pathTokens, "docs", "doc") {
		score++
		reasons = append(reasons, "lives under a docs path")
	}
	if hasAnyDiscoveryToken(titleTokens, "guide", "runbook", "playbook", "manual", "operator", "operations", "reference") {
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

	// Partition docs: root-level well-known files go into their own source
	// to avoid shifting the common root for regular docs.
	var intentDocs, regularDocs []discoveredCandidate
	for _, doc := range docs {
		if !strings.Contains(doc.path, "/") && isWellKnownIntentArtifact(filepath.Base(doc.path)) {
			intentDocs = append(intentDocs, doc)
		} else {
			regularDocs = append(regularDocs, doc)
		}
	}

	if source, ok := buildDiscoveredSource(workspaceRoot, "docs", config.SourceKindMarkdownDocs, regularDocs, []string{
		fmt.Sprintf("discovered %d likely guide, runbook, operations, or reference-doc file(s)", len(regularDocs)),
		"uses exact file selectors to avoid indexing unrelated markdown",
	}, usedNames); ok {
		sources = append(sources, source)
	}
	if source, ok := buildDiscoveredSource(workspaceRoot, "project-docs", config.SourceKindMarkdownDocs, intentDocs, []string{
		fmt.Sprintf("discovered %d well-known intent artifact(s)", len(intentDocs)),
		"root-level project documentation kept as separate source to preserve doc ref stability",
	}, usedNames); ok {
		sources = append(sources, source)
	}

	return sources
}

func isWellKnownIntentArtifact(filename string) bool {
	wellKnown := map[string]bool{
		"claude.md": true, "agents.md": true, "architecture.md": true,
		"contributing.md": true, "readme.md": true,
	}
	return wellKnown[strings.ToLower(filename)]
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
	if name == "." || name == string(filepath.Separator) || name == "" || relativeRoot == "." {
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

	// #nosec G301 -- generated config directories use normal checkout permissions for repo files.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	// #nosec G306 -- discovered config files are non-secret repo files intended to be readable by standard tooling.
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

func discoveryScoreSummary(label string, score int, reasons []string) string {
	if len(reasons) == 0 {
		return fmt.Sprintf("%s score %d", label, score)
	}
	return fmt.Sprintf("%s score %d [%s]", label, score, strings.Join(reasons, "; "))
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
