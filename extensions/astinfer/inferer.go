package astinfer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dusk-network/pituitary/internal/ast"
	"github.com/dusk-network/pituitary/internal/codeinfer"
)

const defaultMaxFileSizeBytes = 1 << 20 // 1 MB

func init() {
	codeinfer.Register(codeinfer.DefaultInfererName, func() codeinfer.AppliesToInferer {
		return Inferer{}
	})
}

// Inferer is the first-party tree-sitter-backed applies_to inferer.
type Inferer struct{}

func (Inferer) Name() string {
	return codeinfer.DefaultInfererName
}

func (Inferer) InferAppliesTo(ctx context.Context, req codeinfer.Request) (*codeinfer.Result, error) {
	if req.WorkspaceRoot == "" {
		return &codeinfer.Result{}, nil
	}
	if !hasMatchableSpecIdentifiers(req.Specs) {
		return &codeinfer.Result{}, nil
	}

	codePaths, err := ast.WalkWorkspace(req.WorkspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("walk workspace for AST extraction: %w", err)
	}
	if len(codePaths) == 0 {
		return &codeinfer.Result{}, nil
	}

	cachedData := previousCacheByHash(req.PreviousCache)
	maxFileSize := req.Limits.MaxFileSizeBytes
	if maxFileSize <= 0 {
		maxFileSize = defaultMaxFileSizeBytes
	}

	fileSymbols := make(map[string][]codeinfer.Symbol, len(codePaths))
	cacheEntries := make([]codeinfer.CacheEntry, 0, len(codePaths))
	for _, relPath := range codePaths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		fullPath := filepath.Join(req.WorkspaceRoot, relPath)
		info, err := os.Stat(fullPath)
		if err != nil || info.Size() > maxFileSize {
			continue
		}
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		hash := ast.ContentHash(content, relPath)
		if cached, ok := cachedData[hash]; ok {
			fileSymbols[relPath] = cached.Symbols
			cacheEntries = append(cacheEntries, codeinfer.CacheEntry{
				ContentHash: hash,
				Path:        relPath,
				Symbols:     cached.Symbols,
				Rationale:   cached.Rationale,
			})
			continue
		}

		lang := ast.DetectLanguage(relPath)
		if lang == "" {
			continue
		}
		symbols, err := ast.ExtractSymbols(content, lang)
		if err != nil {
			continue
		}
		rationale := ast.ExtractRationale(content, symbols, lang)
		fileSymbols[relPath] = symbols
		cacheEntries = append(cacheEntries, codeinfer.CacheEntry{
			ContentHash: hash,
			Path:        relPath,
			Symbols:     symbols,
			Rationale:   rationale,
		})
	}

	return &codeinfer.Result{
		CacheEntries: cacheEntries,
		Edges:        ast.InferEdges(fileSymbols, req.Specs),
	}, nil
}

func hasMatchableSpecIdentifiers(specs []codeinfer.SpecInput) bool {
	for _, spec := range specs {
		if len(ast.ScanSpecIdentifiers(spec.BodyText)) > 0 {
			return true
		}
	}
	return false
}

func previousCacheByHash(entries []codeinfer.CacheEntry) map[string]codeinfer.CacheEntry {
	result := make(map[string]codeinfer.CacheEntry, len(entries))
	for _, entry := range entries {
		if entry.ContentHash == "" {
			continue
		}
		result[entry.ContentHash] = entry
	}
	return result
}
