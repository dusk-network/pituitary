package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dusk-network/pituitary/internal/model"
)

// NewSpecBundleOptions controls creation of one draft spec bundle.
type NewSpecBundleOptions struct {
	WorkspaceRoot string
	SpecRoot      string
	BundleDir     string
	ID            string
	Title         string
	Domain        string
}

// NewSpecBundleWarning reports non-fatal decisions made while scaffolding.
type NewSpecBundleWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewSpecBundleResult reports the generated bundle and rendered files.
type NewSpecBundleResult struct {
	WorkspaceRoot string                    `json:"workspace_root"`
	ConfigPath    string                    `json:"config_path,omitempty"`
	SpecRoot      string                    `json:"spec_root"`
	BundleDir     string                    `json:"bundle_dir"`
	WroteBundle   bool                      `json:"wrote_bundle,omitempty"`
	Spec          model.SpecRecord          `json:"spec"`
	Files         []CanonicalizedBundleFile `json:"files"`
	Warnings      []NewSpecBundleWarning    `json:"warnings,omitempty"`
}

// NewSpecBundle writes one draft spec.toml + body.md bundle.
func NewSpecBundle(options NewSpecBundleOptions) (*NewSpecBundleResult, error) {
	title := strings.TrimSpace(options.Title)
	if title == "" {
		return nil, fmt.Errorf("--title is required")
	}

	workspaceRoot, err := resolveNewWorkspaceRoot(options.WorkspaceRoot)
	if err != nil {
		return nil, err
	}

	specRoot, err := resolveNewSpecRoot(workspaceRoot, options.SpecRoot)
	if err != nil {
		return nil, err
	}

	bundleDir, err := resolveNewBundleDir(workspaceRoot, specRoot, options.BundleDir, title)
	if err != nil {
		return nil, err
	}

	existingBundles, existingIDs, nextID, err := inspectExistingSpecBundles(workspaceRoot, specRoot)
	if err != nil {
		return nil, err
	}
	if err := validateNewBundleDir(workspaceRoot, bundleDir, existingBundles); err != nil {
		return nil, err
	}

	specID := strings.TrimSpace(options.ID)
	if specID == "" {
		specID = nextID
	} else if _, exists := existingIDs[specID]; exists {
		return nil, fmt.Errorf("spec id %q already exists under %s", specID, workspaceRelative(workspaceRoot, specRoot))
	}

	domain := strings.TrimSpace(options.Domain)
	var warnings []NewSpecBundleWarning
	if domain == "" {
		domain = "unknown"
		warnings = append(warnings, NewSpecBundleWarning{
			Code:    "placeholder_domain",
			Message: `domain defaulted to "unknown"; replace it before review or acceptance`,
		})
	}

	specToml := renderNewSpecToml(specID, title, domain)
	bodyMarkdown := renderNewBodyMarkdown(title)
	if err := writeCanonicalizedBundle(bundleDir, specToml, bodyMarkdown); err != nil {
		return nil, err
	}

	specPath := filepath.Join(bundleDir, "spec.toml")
	bodyPath := filepath.Join(bundleDir, "body.md")
	result := &NewSpecBundleResult{
		WorkspaceRoot: workspaceRoot,
		SpecRoot:      workspaceRelative(workspaceRoot, specRoot),
		BundleDir:     workspaceRelative(workspaceRoot, bundleDir),
		WroteBundle:   true,
		Spec: model.SpecRecord{
			Ref:        specID,
			Kind:       model.ArtifactKindSpec,
			Title:      title,
			Status:     model.StatusDraft,
			Domain:     domain,
			SourceRef:  fileSourceRef(workspaceRoot, specPath),
			BodyFormat: model.BodyFormatMarkdown,
			BodyText:   bodyMarkdown,
			Metadata: map[string]string{
				"bundle_path": workspaceRelative(workspaceRoot, bundleDir),
				"body_path":   workspaceRelative(workspaceRoot, bodyPath),
			},
		},
		Files: []CanonicalizedBundleFile{
			{
				Path:    filepath.ToSlash(filepath.Join(workspaceRelative(workspaceRoot, bundleDir), "spec.toml")),
				Content: specToml,
			},
			{
				Path:    filepath.ToSlash(filepath.Join(workspaceRelative(workspaceRoot, bundleDir), "body.md")),
				Content: bodyMarkdown,
			},
		},
		Warnings: warnings,
	}
	return result, nil
}

func resolveNewWorkspaceRoot(raw string) (string, error) {
	workspaceRoot := strings.TrimSpace(raw)
	if workspaceRoot == "" {
		var err error
		workspaceRoot, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
	}
	workspaceRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	info, err := os.Stat(workspaceRoot)
	switch {
	case err == nil && !info.IsDir():
		return "", fmt.Errorf("workspace root %q is not a directory", filepath.ToSlash(workspaceRoot))
	case err != nil:
		return "", fmt.Errorf("workspace root %q: %w", filepath.ToSlash(workspaceRoot), err)
	}
	return workspaceRoot, nil
}

func resolveNewSpecRoot(workspaceRoot, raw string) (string, error) {
	specRoot := strings.TrimSpace(raw)
	if specRoot == "" {
		specRoot = filepath.Join(workspaceRoot, "specs")
	}
	if !filepath.IsAbs(specRoot) {
		specRoot = filepath.Join(workspaceRoot, specRoot)
	}
	specRoot, err := filepath.Abs(specRoot)
	if err != nil {
		return "", fmt.Errorf("resolve spec root %q: %w", raw, err)
	}
	if !pathWithinRoot(workspaceRoot, specRoot) {
		return "", fmt.Errorf("spec root %q resolves outside workspace root %q", raw, filepath.ToSlash(workspaceRoot))
	}
	return specRoot, nil
}

func resolveNewBundleDir(workspaceRoot, specRoot, requested, title string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		slug := newSpecBundleSlug(title)
		if slug == "" {
			return "", fmt.Errorf("cannot derive bundle directory from title %q; pass --bundle-dir explicitly", title)
		}
		requested = filepath.Join(specRoot, slug)
	}
	if !filepath.IsAbs(requested) {
		requested = filepath.Join(workspaceRoot, requested)
	}
	bundleDir, err := filepath.Abs(requested)
	if err != nil {
		return "", fmt.Errorf("resolve bundle directory %q: %w", requested, err)
	}
	if !pathWithinRoot(workspaceRoot, bundleDir) {
		return "", fmt.Errorf("--bundle-dir %q resolves outside workspace root %q", requested, filepath.ToSlash(workspaceRoot))
	}
	return bundleDir, nil
}

func inspectExistingSpecBundles(workspaceRoot, specRoot string) ([]string, map[string]struct{}, string, error) {
	info, err := os.Stat(specRoot)
	switch {
	case os.IsNotExist(err):
		return nil, map[string]struct{}{}, formatSpecID(1, 3), nil
	case err != nil:
		return nil, nil, "", fmt.Errorf("inspect spec root %q: %w", workspaceRelative(workspaceRoot, specRoot), err)
	case !info.IsDir():
		return nil, nil, "", fmt.Errorf("spec root %q is not a directory", workspaceRelative(workspaceRoot, specRoot))
	}

	bundleDirs, err := discoverSpecBundleDirs(specRoot)
	if err != nil {
		return nil, nil, "", fmt.Errorf("scan spec root %q: %w", workspaceRelative(workspaceRoot, specRoot), err)
	}

	existingIDs := make(map[string]struct{}, len(bundleDirs))
	maxNumericID := 0
	width := 3
	for _, bundleDir := range bundleDirs {
		specPath := filepath.Join(bundleDir, "spec.toml")
		// #nosec G304 -- specPath comes from a discovered bundle within the workspace.
		data, err := os.ReadFile(specPath)
		if err != nil {
			return nil, nil, "", fmt.Errorf("read existing spec bundle %q: %w", workspaceRelative(workspaceRoot, specPath), err)
		}
		raw, err := parseSpecBundle(data)
		if err != nil {
			return nil, nil, "", fmt.Errorf("parse existing spec bundle %q: %w", workspaceRelative(workspaceRoot, specPath), err)
		}
		if raw.ID != "" {
			existingIDs[raw.ID] = struct{}{}
		}
		if value, valueWidth, ok := numericSpecID(raw.ID); ok {
			if value > maxNumericID {
				maxNumericID = value
			}
			if valueWidth > width {
				width = valueWidth
			}
		}
	}

	return bundleDirs, existingIDs, formatSpecID(maxNumericID+1, width), nil
}

func validateNewBundleDir(workspaceRoot, bundleDir string, existingBundles []string) error {
	for _, existing := range existingBundles {
		switch {
		case isNestedBundle(existing, bundleDir):
			return fmt.Errorf("bundle %q would be nested inside existing bundle %q", workspaceRelative(workspaceRoot, bundleDir), workspaceRelative(workspaceRoot, existing))
		case isNestedBundle(bundleDir, existing):
			return fmt.Errorf("bundle %q would contain existing bundle %q", workspaceRelative(workspaceRoot, bundleDir), workspaceRelative(workspaceRoot, existing))
		}
	}
	return nil
}

func numericSpecID(id string) (int, int, bool) {
	if !strings.HasPrefix(id, "SPEC-") {
		return 0, 0, false
	}
	suffix := strings.TrimPrefix(id, "SPEC-")
	if suffix == "" {
		return 0, 0, false
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return 0, 0, false
		}
	}
	value, err := strconv.Atoi(suffix)
	if err != nil {
		return 0, 0, false
	}
	return value, len(suffix), true
}

func formatSpecID(value, width int) string {
	if width < 3 {
		width = 3
	}
	return fmt.Sprintf("SPEC-%0*d", width, value)
}

func newSpecBundleSlug(title string) string {
	title = strings.TrimSpace(strings.ToLower(title))
	var builder strings.Builder
	lastHyphen := true
	for _, r := range title {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			lastHyphen = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				builder.WriteRune('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func renderNewSpecToml(id, title, domain string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Generated by pituitary new\n")
	if domain == "unknown" {
		fmt.Fprintf(&builder, "# TODO: replace placeholder domain %q before review or acceptance\n", domain)
	}
	fmt.Fprintf(&builder, "id = %s\n", strconv.Quote(id))
	fmt.Fprintf(&builder, "title = %s\n", strconv.Quote(title))
	fmt.Fprintf(&builder, "status = %s\n", strconv.Quote(model.StatusDraft))
	fmt.Fprintf(&builder, "domain = %s\n", strconv.Quote(domain))
	fmt.Fprintf(&builder, "body = %s\n", strconv.Quote("body.md"))
	fmt.Fprintf(&builder, "\n# Optional fields:\n")
	fmt.Fprintf(&builder, "# authors = [\"your-name\"]\n")
	fmt.Fprintf(&builder, "# tags = [\"area\", \"topic\"]\n")
	fmt.Fprintf(&builder, "# depends_on = [\"SPEC-012\"]\n")
	fmt.Fprintf(&builder, "# supersedes = [\"SPEC-008\"]\n")
	fmt.Fprintf(&builder, "# applies_to = [\"code://path/to/file\"]\n")
	return builder.String()
}

func renderNewBodyMarkdown(title string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# %s\n\n", title)
	fmt.Fprintf(&builder, "## Overview\n\n")
	fmt.Fprintf(&builder, "TODO: describe the problem, scope, and intended outcome.\n\n")
	fmt.Fprintf(&builder, "## Requirements\n\n")
	fmt.Fprintf(&builder, "- TODO: capture the key requirements.\n\n")
	fmt.Fprintf(&builder, "## Design Decisions\n\n")
	fmt.Fprintf(&builder, "- TODO: document the chosen approach.\n\n")
	fmt.Fprintf(&builder, "## Open Questions\n\n")
	fmt.Fprintf(&builder, "- TODO: track unresolved decisions.\n")
	return builder.String()
}
