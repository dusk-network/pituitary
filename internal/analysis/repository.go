package analysis

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
)

type analysisRepository struct {
	cfg            *config.Config
	ctx            context.Context
	db             *sql.DB
	specCache      map[string]specDocument
	docCache       map[string]docDocument
	allSpecsLoaded bool
	allDocsLoaded  bool
}

func openAnalysisRepository(cfg *config.Config) (*analysisRepository, error) {
	return openAnalysisRepositoryContext(context.Background(), cfg)
}

func openAnalysisRepositoryContext(ctx context.Context, cfg *config.Config) (*analysisRepository, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	db, err := index.OpenReadOnlyContext(ctx, cfg.Workspace.ResolvedIndexPath)
	if err != nil {
		return nil, fmt.Errorf("open index %s: %w", cfg.Workspace.ResolvedIndexPath, err)
	}

	return &analysisRepository{
		cfg: cfg,
		ctx: ctx,
		db:  db,
	}, nil
}

func (r *analysisRepository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func (r *analysisRepository) loadAllSpecs() (map[string]specDocument, error) {
	if !r.allSpecsLoaded {
		specs, err := loadIndexedSpecsContext(r.ctx, r.db, nil)
		if err != nil {
			return nil, err
		}
		r.specCache = mergeSpecDocuments(r.specCache, specs)
		r.allSpecsLoaded = true
	}
	return copySpecDocuments(r.specCache), nil
}

func (r *analysisRepository) loadSpecs(refs []string) (map[string]specDocument, error) {
	refs = uniqueStrings(refs)
	if len(refs) == 0 {
		return r.loadAllSpecs()
	}
	if r.allSpecsLoaded {
		return selectSpecs(r.specCache, refs), nil
	}
	missing := missingSpecRefs(r.specCache, refs)
	if len(missing) > 0 {
		specs, err := loadIndexedSpecsContext(r.ctx, r.db, missing)
		if err != nil {
			return nil, err
		}
		r.specCache = mergeSpecDocuments(r.specCache, specs)
	}
	return selectSpecs(r.specCache, refs), nil
}

func (r *analysisRepository) loadSelectedSpecs(refs []string) (map[string]specDocument, error) {
	refs = uniqueStrings(refs)
	if len(refs) == 0 {
		return map[string]specDocument{}, nil
	}
	return r.loadSpecs(refs)
}

func (r *analysisRepository) loadAllDocs() (map[string]docDocument, error) {
	if !r.allDocsLoaded {
		docs, err := loadIndexedDocsContext(r.ctx, r.db, nil)
		if err != nil {
			return nil, err
		}
		r.docCache = mergeDocDocuments(r.docCache, docs)
		r.allDocsLoaded = true
	}
	return copyDocDocuments(r.docCache), nil
}

func (r *analysisRepository) loadDocs(refs []string) (map[string]docDocument, error) {
	refs = uniqueStrings(refs)
	if len(refs) == 0 {
		return r.loadAllDocs()
	}
	if r.allDocsLoaded {
		return selectDocs(r.docCache, refs), nil
	}
	missing := missingDocRefs(r.docCache, refs)
	if len(missing) > 0 {
		docs, err := loadIndexedDocsContext(r.ctx, r.db, missing)
		if err != nil {
			return nil, err
		}
		r.docCache = mergeDocDocuments(r.docCache, docs)
	}
	return selectDocs(r.docCache, refs), nil
}

func (r *analysisRepository) loadSelectedDocs(refs []string) (map[string]docDocument, error) {
	refs = uniqueStrings(refs)
	if len(refs) == 0 {
		return map[string]docDocument{}, nil
	}
	return r.loadDocs(refs)
}

func copySpecDocuments(source map[string]specDocument) map[string]specDocument {
	if len(source) == 0 {
		return map[string]specDocument{}
	}
	result := make(map[string]specDocument, len(source))
	for ref, document := range source {
		result[ref] = document
	}
	return result
}

func mergeSpecDocuments(destination, source map[string]specDocument) map[string]specDocument {
	if destination == nil {
		destination = map[string]specDocument{}
	}
	for ref, document := range source {
		destination[ref] = document
	}
	return destination
}

func copyDocDocuments(source map[string]docDocument) map[string]docDocument {
	if len(source) == 0 {
		return map[string]docDocument{}
	}
	result := make(map[string]docDocument, len(source))
	for ref, document := range source {
		result[ref] = document
	}
	return result
}

func mergeDocDocuments(destination, source map[string]docDocument) map[string]docDocument {
	if destination == nil {
		destination = map[string]docDocument{}
	}
	for ref, document := range source {
		destination[ref] = document
	}
	return destination
}

func selectSpecs(source map[string]specDocument, refs []string) map[string]specDocument {
	result := make(map[string]specDocument, len(refs))
	for _, ref := range refs {
		if document, ok := source[ref]; ok {
			result[ref] = document
		}
	}
	return result
}

func selectDocs(source map[string]docDocument, refs []string) map[string]docDocument {
	result := make(map[string]docDocument, len(refs))
	for _, ref := range refs {
		if document, ok := source[ref]; ok {
			result[ref] = document
		}
	}
	return result
}

func missingSpecRefs(source map[string]specDocument, refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	var missing []string
	for _, ref := range refs {
		if _, ok := source[ref]; !ok {
			missing = append(missing, ref)
		}
	}
	return missing
}

func missingDocRefs(source map[string]docDocument, refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	var missing []string
	for _, ref := range refs {
		if _, ok := source[ref]; !ok {
			missing = append(missing, ref)
		}
	}
	return missing
}

func artifactRepoID(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	return strings.TrimSpace(metadata["repo_id"])
}
