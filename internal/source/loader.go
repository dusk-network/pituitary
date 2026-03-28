package source

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/diag"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/sdk"
)

// LoadResult contains canonical records emitted by configured source adapters.
type LoadResult struct {
	Specs   []model.SpecRecord
	Docs    []model.DocRecord
	Sources []LoadSourceSummary
}

// LoadSourceSummary describes the records contributed by one configured source.
type LoadSourceSummary struct {
	Name      string `json:"name"`
	Adapter   string `json:"adapter"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	ItemCount int    `json:"item_count"`
	SpecCount int    `json:"spec_count,omitempty"`
	DocCount  int    `json:"doc_count,omitempty"`
}

// LoadOptions controls diagnostic behavior while loading configured sources.
type LoadOptions struct {
	Logger *diag.Logger
}

// LoadFromConfig loads and normalizes all configured sources.
func LoadFromConfig(cfg *config.Config) (*LoadResult, error) {
	return LoadFromConfigWithOptions(cfg, LoadOptions{})
}

// LoadFromConfigWithOptions loads and normalizes all configured sources.
func LoadFromConfigWithOptions(cfg *config.Config, options LoadOptions) (*LoadResult, error) {
	logger := options.Logger
	result := &LoadResult{
		Sources: make([]LoadSourceSummary, 0, len(cfg.Sources)),
	}
	seenSpecs := make(map[string]artifactOrigin)
	seenDocs := make(map[string]artifactOrigin)
	logger.Infof("source", "loading %d configured source(s)", len(cfg.Sources))

	for _, source := range cfg.Sources {
		logger.Debugf("source", "loading source %q (%s %s)", source.Name, source.Kind, filepath.ToSlash(source.Path))
		summary := LoadSourceSummary{
			Name:    source.Name,
			Adapter: source.Adapter,
			Kind:    source.Kind,
			Path:    source.Path,
		}

		factory := LookupAdapter(source.Adapter)
		if factory == nil {
			return nil, unknownAdapterError(source.Name, source.Adapter)
		}
		adapterResult, err := factory().Load(context.Background(), sdk.SourceConfig{
			Name:          source.Name,
			Adapter:       source.Adapter,
			Kind:          source.Kind,
			Path:          source.Path,
			Files:         append([]string(nil), source.Files...),
			Include:       append([]string(nil), source.Include...),
			Exclude:       append([]string(nil), source.Exclude...),
			Options:       config.CloneSourceOptions(source.Options),
			WorkspaceRoot: cfg.Workspace.RootPath,
		})
		if err != nil {
			return nil, fmt.Errorf("source %q: %w", source.Name, err)
		}

		if err := appendUniqueSpecRecords(result, seenSpecs, source, adapterResult.Specs); err != nil {
			return nil, err
		}
		if err := appendUniqueDocRecords(result, seenDocs, source, adapterResult.Docs); err != nil {
			return nil, err
		}
		summary.SpecCount = len(adapterResult.Specs)
		summary.DocCount = len(adapterResult.Docs)
		summary.ItemCount = len(adapterResult.Specs) + len(adapterResult.Docs)

		result.Sources = append(result.Sources, summary)
		if summary.ItemCount == 0 {
			logger.Warnf("source", "source %q (%s %s) matched 0 item(s)", source.Name, source.Kind, filepath.ToSlash(source.Path))
			continue
		}
		logger.Infof(
			"source",
			"source %q (%s %s) loaded %d item(s) (%d spec(s), %d doc(s))",
			source.Name,
			source.Kind,
			filepath.ToSlash(source.Path),
			summary.ItemCount,
			summary.SpecCount,
			summary.DocCount,
		)
	}

	sort.Slice(result.Specs, func(i, j int) bool {
		return result.Specs[i].Ref < result.Specs[j].Ref
	})
	sort.Slice(result.Docs, func(i, j int) bool {
		return result.Docs[i].Ref < result.Docs[j].Ref
	})

	return result, nil
}

func unknownAdapterError(sourceName, adapter string) error {
	registered := RegisteredAdapters()
	if len(registered) == 0 {
		return fmt.Errorf("source %q: unknown adapter %q", sourceName, adapter)
	}
	return fmt.Errorf(
		"source %q: unknown adapter %q (registered adapters: %s)",
		sourceName,
		adapter,
		strings.Join(registered, ", "),
	)
}
