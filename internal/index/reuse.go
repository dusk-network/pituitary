package index

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/dusk-network/pituitary/internal/chunk"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
	stindex "github.com/dusk-network/stroma/v2/index"
)

type reuseState struct {
	artifacts map[string]storedArtifact
}

type storedArtifact struct {
	contentHash string
	title       string
	chunks      map[string]struct{}
}

type artifactChunkPlan struct {
	sections           []chunk.Section
	reusedChunkCount   int
	embeddedChunkCount int
	artifactUnchanged  bool
}

func loadReuseStateContext(ctx context.Context, snapshotPath, embedderFingerprint string, embedderDimension int, options RebuildOptions) (*reuseState, error) {
	if options.Full {
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	}

	info, err := os.Stat(snapshotPath)
	switch {
	case os.IsNotExist(err):
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	case err != nil:
		return nil, fmt.Errorf("stat existing snapshot %s: %w", snapshotPath, err)
	case info.IsDir():
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	}

	snapshot, err := stindex.OpenSnapshot(ctx, snapshotPath)
	if err != nil {
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	}
	defer snapshot.Close()

	stats, err := snapshot.Stats(ctx)
	if err != nil {
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	}
	if stats.EmbedderFingerprint != embedderFingerprint || stats.EmbedderDimension != embedderDimension {
		return &reuseState{artifacts: map[string]storedArtifact{}}, nil
	}

	return loadStoredArtifactsContext(ctx, snapshot)
}

func loadStoredArtifactsContext(ctx context.Context, snapshot *stindex.Snapshot) (*reuseState, error) {
	records, err := snapshot.Records(ctx, stindex.RecordQuery{})
	if err != nil {
		return nil, fmt.Errorf("query stored artifacts: %w", err)
	}

	state := &reuseState{artifacts: make(map[string]storedArtifact)}
	for _, record := range records {
		state.artifacts[record.Ref] = storedArtifact{
			contentHash: record.ContentHash,
			title:       record.Title,
			chunks:      map[string]struct{}{},
		}
	}

	sections, err := snapshot.Sections(ctx, stindex.SectionQuery{})
	if err != nil {
		return nil, fmt.Errorf("query stored chunks: %w", err)
	}

	for _, section := range sections {
		artifact := state.artifacts[section.Ref]
		artifact.chunks[reuseChunkKey(artifact.title, section.Heading, section.Content)] = struct{}{}
		state.artifacts[section.Ref] = artifact
	}

	return state, nil
}

func planArtifactReuse(title, contentHash, body string, stored storedArtifact) artifactChunkPlan {
	sections := chunk.Markdown(title, body)
	plan := artifactChunkPlan{sections: sections}
	if stored.contentHash == "" {
		plan.embeddedChunkCount = len(sections)
		return plan
	}

	for _, section := range sections {
		key := reuseChunkKey(title, section.Heading, section.Body)
		if _, ok := stored.chunks[key]; ok {
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
