package index

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dusk-network/pituitary/internal/chunk"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

type reuseState struct {
	artifacts map[string]storedArtifact
}

type storedArtifact struct {
	contentHash string
	title       string
	chunks      map[string]storedChunk
}

type storedChunk struct {
	embedding []byte
}

type artifactChunkPlan struct {
	sections           []chunk.Section
	reusedEmbeddings   map[string][]byte
	reusedChunkCount   int
	embeddedChunkCount int
	artifactUnchanged  bool
}

func loadReuseStateContext(ctx context.Context, indexPath, embedderFingerprint string, embedderDimension int, currentSourceFingerprint string, options RebuildOptions) (*reuseState, error) {
	if options.Full {
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	}

	info, err := os.Stat(indexPath)
	switch {
	case os.IsNotExist(err):
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	case err != nil:
		return nil, fmt.Errorf("stat existing index %s: %w", indexPath, err)
	case info.IsDir():
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	}

	db, err := OpenReadOnlyContext(ctx, indexPath)
	if err != nil {
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	}
	defer db.Close()

	metadata, err := readMetadataContext(ctx, db, "schema_version", "embedder_fingerprint", "embedder_dimension", "source_fingerprint")
	if err != nil {
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	}
	if strings.TrimSpace(metadata["schema_version"]) != fmt.Sprintf("%d", schemaVersion) {
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	}
	if strings.TrimSpace(metadata["embedder_fingerprint"]) != embedderFingerprint {
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	}
	if strings.TrimSpace(metadata["embedder_dimension"]) != strconv.Itoa(embedderDimension) {
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	}
	if strings.TrimSpace(metadata["source_fingerprint"]) != currentSourceFingerprint {
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	}

	return loadStoredArtifactsContext(ctx, db)
}

func loadStoredArtifactsContext(ctx context.Context, db *sql.DB) (*reuseState, error) {
	rows, err := db.QueryContext(ctx, `SELECT ref, title, content_hash FROM artifacts`)
	if err != nil {
		return nil, fmt.Errorf("query stored artifacts: %w", err)
	}
	defer rows.Close()

	state := &reuseState{artifacts: make(map[string]storedArtifact)}
	for rows.Next() {
		var (
			ref         string
			title       sql.NullString
			contentHash string
		)
		if err := rows.Scan(&ref, &title, &contentHash); err != nil {
			return nil, fmt.Errorf("scan stored artifact: %w", err)
		}
		state.artifacts[ref] = storedArtifact{
			contentHash: contentHash,
			title:       title.String,
			chunks:      map[string]storedChunk{},
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stored artifacts: %w", err)
	}

	rows, err = db.QueryContext(ctx, `
SELECT c.record_ref, c.heading, c.content, v.embedding
FROM chunks c
JOIN chunks_vec v ON v.chunk_id = c.id
ORDER BY c.record_ref, c.id`)
	if err != nil {
		return nil, fmt.Errorf("query stored chunks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			ref       string
			section   sql.NullString
			content   string
			embedding []byte
		)
		if err := rows.Scan(&ref, &section, &content, &embedding); err != nil {
			return nil, fmt.Errorf("scan stored chunk: %w", err)
		}
		artifact := state.artifacts[ref]
		artifact.chunks[reuseChunkKey(artifact.title, section.String, content)] = storedChunk{
			embedding: append([]byte(nil), embedding...),
		}
		state.artifacts[ref] = artifact
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stored chunks: %w", err)
	}

	return state, nil
}

func planArtifactReuse(title, contentHash, body string, stored storedArtifact) artifactChunkPlan {
	sections := chunk.Markdown(title, body)
	plan := artifactChunkPlan{
		sections:         sections,
		reusedEmbeddings: make(map[string][]byte),
	}
	if stored.contentHash == "" {
		plan.embeddedChunkCount = len(sections)
		return plan
	}

	for _, section := range sections {
		key := reuseChunkKey(title, section.Heading, section.Body)
		if storedChunk, ok := stored.chunks[key]; ok {
			plan.reusedEmbeddings[key] = storedChunk.embedding
			plan.reusedChunkCount++
		}
	}
	plan.embeddedChunkCount = len(sections) - plan.reusedChunkCount
	plan.artifactUnchanged = stored.contentHash == contentHash && len(sections) == plan.reusedChunkCount
	return plan
}

func reuseChunkKey(title, heading, body string) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(title))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(heading))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(body))
	return hex.EncodeToString(hasher.Sum(nil))
}

func storedArtifactForRecord(state *reuseState, ref string) storedArtifact {
	if state == nil || state.artifacts == nil {
		return storedArtifact{}
	}
	return state.artifacts[ref]
}

func updateRebuildReuseCountsForSpecs(result *RebuildResult, records []model.SpecRecord, state *reuseState) {
	for _, spec := range records {
		plan := planArtifactReuse(spec.Title, spec.ContentHash, spec.BodyText, storedArtifactForRecord(state, spec.Ref))
		if plan.artifactUnchanged {
			result.ReusedArtifactCount++
		}
		result.ReusedChunkCount += plan.reusedChunkCount
		result.EmbeddedChunkCount += plan.embeddedChunkCount
	}
}

func updateRebuildReuseCountsForDocs(result *RebuildResult, records []model.DocRecord, state *reuseState) {
	for _, doc := range records {
		plan := planArtifactReuse(doc.Title, doc.ContentHash, doc.BodyText, storedArtifactForRecord(state, doc.Ref))
		if plan.artifactUnchanged {
			result.ReusedArtifactCount++
		}
		result.ReusedChunkCount += plan.reusedChunkCount
		result.EmbeddedChunkCount += plan.embeddedChunkCount
	}
}

func summarizeRebuild(records *source.LoadResult, dimension int, state *reuseState, options RebuildOptions) *RebuildResult {
	result := &RebuildResult{
		ArtifactCount:     len(records.Specs) + len(records.Docs),
		SpecCount:         len(records.Specs),
		DocCount:          len(records.Docs),
		EmbedderDimension: dimension,
		FullRebuild:       options.Full,
		Repos:             repoCoverageFromRecords(records),
		Sources:           append([]source.LoadSourceSummary(nil), records.Sources...),
	}

	for _, spec := range records.Specs {
		result.ChunkCount += len(chunk.Markdown(spec.Title, spec.BodyText))
		result.EdgeCount += len(spec.Relations) + len(spec.AppliesTo)
	}
	for _, doc := range records.Docs {
		result.ChunkCount += len(chunk.Markdown(doc.Title, doc.BodyText))
	}
	updateRebuildReuseCountsForSpecs(result, records.Specs, state)
	updateRebuildReuseCountsForDocs(result, records.Docs, state)
	result.ContentFingerprint = contentFingerprint(records)
	return result
}
