package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
)

type indexedDocPathRow struct {
	Ref          string
	SourceRef    string
	MetadataJSON string
}

type indexedDocPathResolver struct {
	workspaceRoot string
	repoRoots     map[string]string
	refsByPath    map[string][]string
}

type docPathNotFoundError struct {
	Input      string
	Normalized string
}

func (e *docPathNotFoundError) Error() string {
	if e.Normalized != "" && e.Normalized != e.Input {
		return fmt.Sprintf(
			`unknown --path %q (resolved to %q); run "pituitary preview-sources" to confirm the configured docs and "pituitary index --rebuild" if the workspace changed`,
			e.Input,
			e.Normalized,
		)
	}
	return fmt.Sprintf(
		`unknown --path %q; run "pituitary preview-sources" to confirm the configured docs and "pituitary index --rebuild" if the workspace changed`,
		e.Input,
	)
}

type ambiguousDocPathError struct {
	Input   string
	Matches []string
}

func (e *ambiguousDocPathError) Error() string {
	return fmt.Sprintf(`ambiguous --path %q; matches indexed docs: %s`, e.Input, strings.Join(e.Matches, ", "))
}

type docPathOutsideWorkspaceError struct {
	Input         string
	WorkspaceRoot string
}

func (e *docPathOutsideWorkspaceError) Error() string {
	return fmt.Sprintf(
		`--path %q resolves outside workspace root %q and configured repo roots`,
		e.Input,
		filepath.ToSlash(e.WorkspaceRoot),
	)
}

// IsDocPathNotFound reports whether err is a missing indexed-doc path lookup.
func IsDocPathNotFound(err error) bool {
	var target *docPathNotFoundError
	return errors.As(err, &target)
}

// ResolveIndexedDocRefWithConfigContext resolves one indexed doc ref from a path selector.
func ResolveIndexedDocRefWithConfigContext(ctx context.Context, cfg *config.Config, rawPath string) (string, error) {
	refs, err := ResolveIndexedDocRefsWithConfigContext(ctx, cfg, []string{rawPath})
	if err != nil {
		return "", err
	}
	if len(refs) != 1 {
		return "", fmt.Errorf("expected one resolved doc ref, got %d", len(refs))
	}
	return refs[0], nil
}

// ResolveIndexedDocRefsWithConfigContext resolves indexed doc refs from workspace-relative or absolute paths.
func ResolveIndexedDocRefsWithConfigContext(ctx context.Context, cfg *config.Config, rawPaths []string) ([]string, error) {
	resolver, err := newIndexedDocPathResolverContext(ctx, cfg)
	if err != nil {
		return nil, err
	}

	refs := make([]string, 0, len(rawPaths))
	for _, rawPath := range rawPaths {
		ref, err := resolver.Resolve(rawPath)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func newIndexedDocPathResolverContext(ctx context.Context, cfg *config.Config) (*indexedDocPathResolver, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	db, err := OpenReadOnlyContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := loadIndexedDocPathRowsContext(ctx, db)
	if err != nil {
		return nil, err
	}

	primaryRepoID := config.PrimaryRepoID(cfg)
	repoRoots := map[string]string{
		primaryRepoID: cfg.Workspace.RootPath,
	}
	for _, repo := range cfg.Workspace.Repos {
		repoRoots[strings.TrimSpace(repo.ID)] = repo.RootPath
	}

	refsByPath := make(map[string][]string, len(rows)*3)
	for _, row := range rows {
		candidatePaths, err := indexedDocCandidatePaths(row, primaryRepoID, repoRoots)
		if err != nil {
			return nil, err
		}
		for _, candidatePath := range candidatePaths {
			if candidatePath == "" {
				continue
			}
			refsByPath[candidatePath] = appendUniqueSorted(refsByPath[candidatePath], row.Ref)
		}
	}

	return &indexedDocPathResolver{
		workspaceRoot: cfg.Workspace.RootPath,
		repoRoots:     repoRoots,
		refsByPath:    refsByPath,
	}, nil
}

func loadIndexedDocPathRowsContext(ctx context.Context, db *sql.DB) ([]indexedDocPathRow, error) {
	rows, err := db.QueryContext(ctx, `
SELECT ref, source_ref, metadata_json
FROM artifacts
WHERE kind = ?
ORDER BY ref ASC`, model.ArtifactKindDoc)
	if err != nil {
		return nil, fmt.Errorf("query indexed doc paths: %w", err)
	}
	defer rows.Close()

	var result []indexedDocPathRow
	for rows.Next() {
		var row indexedDocPathRow
		if err := rows.Scan(&row.Ref, &row.SourceRef, &row.MetadataJSON); err != nil {
			return nil, fmt.Errorf("scan indexed doc path: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexed doc paths: %w", err)
	}
	return result, nil
}

func indexedDocCandidatePaths(row indexedDocPathRow, primaryRepoID string, repoRoots map[string]string) ([]string, error) {
	sourcePath := strings.TrimSpace(strings.TrimPrefix(row.SourceRef, "file://"))
	candidates := []string{
		normalizeIndexedSpecLookupPath(row.SourceRef),
		normalizeIndexedSpecLookupPath(sourcePath),
	}

	var metadata map[string]string
	if strings.TrimSpace(row.MetadataJSON) != "" {
		if err := json.Unmarshal([]byte(row.MetadataJSON), &metadata); err != nil {
			return nil, fmt.Errorf("parse indexed metadata for %s: %w", row.Ref, err)
		}
	}

	if sourcePath != "" {
		repoID := strings.TrimSpace(metadata["repo_id"])
		if repoID == "" {
			repoID = primaryRepoID
		}
		repoRoot := strings.TrimSpace(repoRoots[repoID])
		if repoRoot == "" {
			repoRoot = strings.TrimSpace(repoRoots[primaryRepoID])
		}

		absolutePath := sourcePath
		if !filepath.IsAbs(absolutePath) {
			absolutePath = filepath.Join(repoRoot, filepath.FromSlash(sourcePath))
		}
		absolutePath, err := filepath.Abs(absolutePath)
		if err != nil {
			return nil, fmt.Errorf("resolve absolute doc path for %s: %w", row.Ref, err)
		}
		candidates = append(candidates, normalizeIndexedSpecLookupPath(absolutePath))
	}

	return uniqueSortedStrings(candidates), nil
}

func (r *indexedDocPathResolver) Resolve(rawPath string) (string, error) {
	normalized, err := normalizeDocSelectorPath(r.workspaceRoot, r.repoRoots, rawPath)
	if err != nil {
		return "", err
	}

	matches := append([]string(nil), r.refsByPath[normalized]...)
	switch len(matches) {
	case 0:
		return "", &docPathNotFoundError{
			Input:      strings.TrimSpace(rawPath),
			Normalized: normalized,
		}
	case 1:
		return matches[0], nil
	default:
		return "", &ambiguousDocPathError{
			Input:   strings.TrimSpace(rawPath),
			Matches: matches,
		}
	}
}

func normalizeDocSelectorPath(workspaceRoot string, repoRoots map[string]string, rawPath string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", fmt.Errorf("--path must not be empty")
	}
	if !filepath.IsAbs(rawPath) {
		return normalizeIndexedSpecLookupPath(rawPath), nil
	}

	absolutePath, err := filepath.Abs(rawPath)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", rawPath, err)
	}
	if !indexedPathWithinConfiguredRoots(absolutePath, workspaceRoot, repoRoots) {
		return "", &docPathOutsideWorkspaceError{
			Input:         rawPath,
			WorkspaceRoot: workspaceRoot,
		}
	}
	return normalizeIndexedSpecLookupPath(absolutePath), nil
}

func indexedPathWithinConfiguredRoots(path, workspaceRoot string, repoRoots map[string]string) bool {
	if indexedPathWithinRoot(workspaceRoot, path) {
		return true
	}
	for _, root := range repoRoots {
		if indexedPathWithinRoot(root, path) {
			return true
		}
	}
	return false
}

func indexedPathWithinRoot(root, path string) bool {
	root = strings.TrimSpace(root)
	path = strings.TrimSpace(path)
	if root == "" || path == "" {
		return false
	}
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return !pathEscapesRoot(relPath)
}
