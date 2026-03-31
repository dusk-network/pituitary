package index

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

// RepoCoverage summarizes indexed artifact coverage for one repo identity.
type RepoCoverage struct {
	Repo      string `json:"repo"`
	ItemCount int    `json:"item_count"`
	SpecCount int    `json:"spec_count,omitempty"`
	DocCount  int    `json:"doc_count,omitempty"`
}

func repoCoverageFromRecords(records *source.LoadResult) []RepoCoverage {
	if records == nil {
		return nil
	}
	counts := map[string]*RepoCoverage{}
	for _, spec := range records.Specs {
		appendRepoCoverage(counts, strings.TrimSpace(spec.Metadata["repo_id"]), model.ArtifactKindSpec)
	}
	for _, doc := range records.Docs {
		appendRepoCoverage(counts, strings.TrimSpace(doc.Metadata["repo_id"]), model.ArtifactKindDoc)
	}
	return sortedRepoCoverage(counts)
}

func repoCoverageFromDBContext(ctx context.Context, db *sql.DB) ([]RepoCoverage, error) {
	rows, err := db.QueryContext(ctx, `SELECT kind, metadata_json FROM artifacts`)
	if err != nil {
		return nil, fmt.Errorf("query repo coverage: %w", err)
	}
	defer rows.Close()

	counts := map[string]*RepoCoverage{}
	for rows.Next() {
		var kind string
		var rawMetadata string
		if err := rows.Scan(&kind, &rawMetadata); err != nil {
			return nil, fmt.Errorf("scan repo coverage: %w", err)
		}
		if strings.TrimSpace(rawMetadata) == "" {
			continue
		}
		metadata := map[string]string{}
		if err := json.Unmarshal([]byte(rawMetadata), &metadata); err != nil {
			return nil, fmt.Errorf("parse repo coverage metadata: %w", err)
		}
		appendRepoCoverage(counts, strings.TrimSpace(metadata["repo_id"]), kind)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repo coverage: %w", err)
	}
	return sortedRepoCoverage(counts), nil
}

func appendRepoCoverage(counts map[string]*RepoCoverage, repoID, kind string) {
	repoID = strings.TrimSpace(repoID)
	if repoID == "" {
		return
	}
	entry, ok := counts[repoID]
	if !ok {
		entry = &RepoCoverage{Repo: repoID}
		counts[repoID] = entry
	}
	entry.ItemCount++
	switch kind {
	case model.ArtifactKindSpec:
		entry.SpecCount++
	case model.ArtifactKindDoc:
		entry.DocCount++
	}
}

func sortedRepoCoverage(counts map[string]*RepoCoverage) []RepoCoverage {
	if len(counts) == 0 {
		return nil
	}
	result := make([]RepoCoverage, 0, len(counts))
	for _, coverage := range counts {
		result = append(result, *coverage)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Repo < result[j].Repo
	})
	return result
}
