package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
)

type indexedSpecPathRow struct {
	Ref          string
	SourceRef    string
	MetadataJSON string
}

type indexedSpecPathResolver struct {
	workspaceRoot string
	refsByPath    map[string][]string
}

type specPathNotFoundError struct {
	Input      string
	Normalized string
}

func (e *specPathNotFoundError) Error() string {
	if e.Normalized != "" && e.Normalized != e.Input {
		return fmt.Sprintf(
			`unknown --path %q (resolved to workspace path %q); run pituitary index --rebuild if the file should be indexed`,
			e.Input,
			e.Normalized,
		)
	}
	return fmt.Sprintf(`unknown --path %q; run pituitary index --rebuild if the file should be indexed`, e.Input)
}

type ambiguousSpecPathError struct {
	Input   string
	Matches []string
}

func (e *ambiguousSpecPathError) Error() string {
	return fmt.Sprintf(`ambiguous --path %q; matches indexed specs: %s`, e.Input, strings.Join(e.Matches, ", "))
}

type specPathOutsideWorkspaceError struct {
	Input         string
	WorkspaceRoot string
}

func (e *specPathOutsideWorkspaceError) Error() string {
	return fmt.Sprintf(`--path %q resolves outside workspace root %q`, e.Input, filepath.ToSlash(e.WorkspaceRoot))
}

func isSpecPathNotFound(err error) bool {
	var target *specPathNotFoundError
	return errors.As(err, &target)
}

func resolveIndexedSpecRefWithConfigContext(ctx context.Context, cfg *config.Config, rawPath string) (string, error) {
	refs, err := resolveIndexedSpecRefsWithConfigContext(ctx, cfg, []string{rawPath})
	if err != nil {
		return "", err
	}
	if len(refs) != 1 {
		return "", fmt.Errorf("expected one resolved spec ref, got %d", len(refs))
	}
	return refs[0], nil
}

func resolveIndexedSpecRefsWithConfigContext(ctx context.Context, cfg *config.Config, rawPaths []string) ([]string, error) {
	resolver, err := newIndexedSpecPathResolverContext(ctx, cfg)
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

func newIndexedSpecPathResolverContext(ctx context.Context, cfg *config.Config) (*indexedSpecPathResolver, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	db, err := index.OpenReadOnlyContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := loadIndexedSpecPathRowsContext(ctx, db)
	if err != nil {
		return nil, err
	}

	refsByPath := make(map[string][]string, len(rows)*2)
	for _, row := range rows {
		candidatePaths, err := indexedSpecCandidatePaths(row)
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

	return &indexedSpecPathResolver{
		workspaceRoot: cfg.Workspace.RootPath,
		refsByPath:    refsByPath,
	}, nil
}

func loadIndexedSpecPathRowsContext(ctx context.Context, db *sql.DB) ([]indexedSpecPathRow, error) {
	rows, err := db.QueryContext(ctx, `
SELECT ref, source_ref, metadata_json
FROM artifacts
WHERE kind = ?
ORDER BY ref ASC`, model.ArtifactKindSpec)
	if err != nil {
		return nil, fmt.Errorf("query indexed spec paths: %w", err)
	}
	defer rows.Close()

	var result []indexedSpecPathRow
	for rows.Next() {
		var row indexedSpecPathRow
		if err := rows.Scan(&row.Ref, &row.SourceRef, &row.MetadataJSON); err != nil {
			return nil, fmt.Errorf("scan indexed spec path: %w", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate indexed spec paths: %w", err)
	}
	return result, nil
}

func indexedSpecCandidatePaths(row indexedSpecPathRow) ([]string, error) {
	candidates := []string{
		normalizeIndexedSpecPath(row.SourceRef),
		normalizeIndexedSpecPath(strings.TrimPrefix(strings.TrimSpace(row.SourceRef), "file://")),
	}

	var metadata map[string]string
	if strings.TrimSpace(row.MetadataJSON) != "" {
		if err := json.Unmarshal([]byte(row.MetadataJSON), &metadata); err != nil {
			return nil, fmt.Errorf("parse indexed metadata for %s: %w", row.Ref, err)
		}
	}
	for _, key := range []string{"path", "body_path", "bundle_path"} {
		if value := normalizeIndexedSpecPath(metadata[key]); value != "" {
			candidates = append(candidates, value)
		}
	}
	return uniqueSortedStrings(candidates), nil
}

func (r *indexedSpecPathResolver) Resolve(rawPath string) (string, error) {
	normalized, err := normalizeSpecSelectorPath(r.workspaceRoot, rawPath)
	if err != nil {
		return "", err
	}

	matches := append([]string(nil), r.refsByPath[normalized]...)
	switch len(matches) {
	case 0:
		return "", &specPathNotFoundError{
			Input:      strings.TrimSpace(rawPath),
			Normalized: normalized,
		}
	case 1:
		return matches[0], nil
	default:
		return "", &ambiguousSpecPathError{
			Input:   strings.TrimSpace(rawPath),
			Matches: matches,
		}
	}
}

func normalizeSpecSelectorPath(workspaceRoot, rawPath string) (string, error) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", fmt.Errorf("--path must not be empty")
	}

	absPath := rawPath
	if !filepath.IsAbs(absPath) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
		absPath = filepath.Join(cwd, absPath)
	}
	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", rawPath, err)
	}

	relPath, err := filepath.Rel(workspaceRoot, absPath)
	if err != nil {
		return "", fmt.Errorf("resolve workspace-relative path for %q: %w", rawPath, err)
	}
	if pathEscapesRoot(relPath) {
		return "", &specPathOutsideWorkspaceError{
			Input:         rawPath,
			WorkspaceRoot: workspaceRoot,
		}
	}
	return normalizeIndexedSpecPath(relPath), nil
}

func normalizeIndexedSpecPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func pathEscapesRoot(relPath string) bool {
	relPath = filepath.Clean(relPath)
	if relPath == ".." {
		return true
	}
	return strings.HasPrefix(relPath, ".."+string(filepath.Separator))
}

func appendUniqueSorted(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	values = append(values, value)
	sort.Strings(values)
	return values
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		result = appendUniqueSorted(result, value)
	}
	return result
}

func writeSpecPathResolutionError(stdout, stderr io.Writer, format, command string, request any, err error) int {
	code := "validation_error"
	switch {
	case index.IsMissingIndex(err):
		code = "config_error"
	case isSpecPathNotFound(err):
		code = "not_found"
	}
	return writeCLIError(stdout, stderr, format, command, request, cliIssue{
		Code:    code,
		Message: err.Error(),
	}, 2)
}

func nonEmptyCount(values ...string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}
