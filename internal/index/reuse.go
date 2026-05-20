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
	stindex "github.com/dusk-network/stroma/v3/index"
)

type reuseState struct {
	artifacts      map[string]storedArtifact
	disabledReason string
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
		return emptyReuseState("no existing stroma snapshot"), nil
	case err != nil:
		return nil, fmt.Errorf("stat existing snapshot %s: %w", snapshotPath, err)
	case info.IsDir():
		return emptyReuseState(fmt.Sprintf("existing stroma snapshot %s is a directory", snapshotPath)), nil
	}

	snapshot, err := stindex.OpenSnapshot(ctx, snapshotPath)
	if err != nil {
		return emptyReuseState(fmt.Sprintf("open existing stroma snapshot: %v", err)), nil
	}
	defer snapshot.Close()

	stats, err := snapshot.Stats(ctx)
	if err != nil {
		return emptyReuseState(fmt.Sprintf("read existing stroma snapshot stats: %v", err)), nil
	}
	if stats.EmbedderFingerprint != embedderFingerprint || stats.EmbedderDimension != embedderDimension {
		return emptyReuseState("existing stroma snapshot embedder does not match current runtime"), nil
	}
	storedFormat, err := readStoredSnapshotVectorFormatContext(ctx, snapshotPath)
	if err != nil {
		return emptyReuseState(fmt.Sprintf("read existing stroma snapshot vector format: %v", err)), nil
	}
	if storedFormat != defaultStromaVectorFormat {
		return emptyReuseState(fmt.Sprintf("existing stroma snapshot vector storage format %q requires full rebuild", storedFormat)), nil
	}

	return loadStoredArtifactsContext(ctx, snapshot)
}

func emptyReuseState(reason string) *reuseState {
	return &reuseState{
		artifacts:      map[string]storedArtifact{},
		disabledReason: reason,
	}
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

func updateRebuildReuseCountsForSpecs(result *RebuildResult, records []model.SpecRecord, state *reuseState) error {
	for _, spec := range records {
		record, err := source.CorpusRecordFromSpec(spec)
		if err != nil {
			return fmt.Errorf("normalize spec %s for reuse accounting: %w", spec.Ref, err)
		}
		plan := planArtifactReuse(spec.Title, record.ContentHash, spec.BodyText, storedArtifactForRecord(state, spec.Ref))
		if plan.artifactUnchanged {
			result.ReusedArtifactCount++
		}
		result.ReusedChunkCount += plan.reusedChunkCount
		result.EmbeddedChunkCount += plan.embeddedChunkCount
	}
	return nil
}

func updateRebuildReuseCountsForDocs(result *RebuildResult, records []model.DocRecord, state *reuseState) error {
	for _, doc := range records {
		record, err := source.CorpusRecordFromDoc(doc)
		if err != nil {
			return fmt.Errorf("normalize doc %s for reuse accounting: %w", doc.Ref, err)
		}
		plan := planArtifactReuse(doc.Title, record.ContentHash, doc.BodyText, storedArtifactForRecord(state, doc.Ref))
		if plan.artifactUnchanged {
			result.ReusedArtifactCount++
		}
		result.ReusedChunkCount += plan.reusedChunkCount
		result.EmbeddedChunkCount += plan.embeddedChunkCount
	}
	return nil
}

func summarizeRebuild(records *source.LoadResult, dimension int, state *reuseState, options RebuildOptions) (*RebuildResult, error) {
	result := &RebuildResult{
		ArtifactCount:     len(records.Specs) + len(records.Docs),
		SpecCount:         len(records.Specs),
		DocCount:          len(records.Docs),
		EmbedderDimension: dimension,
		FullRebuild:       options.Full,
		Repos:             repoCoverageFromRecords(records),
		Sources:           append([]source.LoadSourceSummary(nil), records.Sources...),
	}
	if state != nil {
		result.ReuseDisabledReason = state.disabledReason
	}

	for _, spec := range records.Specs {
		result.ChunkCount += len(chunk.Markdown(spec.Title, spec.BodyText))
		result.EdgeCount += len(spec.Relations) + len(spec.AppliesTo)
	}
	for _, doc := range records.Docs {
		result.ChunkCount += len(chunk.Markdown(doc.Title, doc.BodyText))
	}
	if err := updateRebuildReuseCountsForSpecs(result, records.Specs, state); err != nil {
		return nil, err
	}
	if err := updateRebuildReuseCountsForDocs(result, records.Docs, state); err != nil {
		return nil, err
	}
	result.ContentFingerprint = contentFingerprint(records)
	return result, nil
}
