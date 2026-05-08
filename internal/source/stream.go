package source

import (
	"context"
	"fmt"

	"github.com/dusk-network/pituitary/internal/model"
	stcorpus "github.com/dusk-network/stroma/v3/corpus"
)

// RecordSource streams normalized stroma corpus records one at a time.
//
// It mirrors stroma's index.RecordSource shape so loaders can yield
// records lazily into stindex.RebuildFromSource / UpdateFromSource /
// SyncFromSource without materializing the full corpus first. The
// stroma pipeline consumes records serially, chunks and embeds in
// bounded internal batches, and never holds the full BodyText of every
// record resident at once.
//
// Implementations must:
//   - return (record, true, nil) while input remains;
//   - return (zero, false, nil) once the stream is exhausted;
//   - return (zero, false, err) on a loading or normalization failure.
//
// Calling Next after exhaustion or an error must keep returning the
// same exhausted/error state.
type RecordSource interface {
	Next(ctx context.Context) (stcorpus.Record, bool, error)
}

// RecordSourceFunc adapts a function value to RecordSource. Mirrors
// stroma's index.RecordSourceFunc contract.
type RecordSourceFunc func(ctx context.Context) (stcorpus.Record, bool, error)

// Next implements RecordSource by delegating to the underlying function.
func (f RecordSourceFunc) Next(ctx context.Context) (stcorpus.Record, bool, error) {
	return f(ctx)
}

// NewLoadResultRecordSource yields stroma corpus records from an
// already-materialized LoadResult, in spec-then-doc order. This is the
// backward-compatible adapter for callers that produce a LoadResult
// today: the bodies are still resident in the slices, but stroma's
// pipeline becomes incremental on the index side, which removes the
// previous "every chunk and embedding stays resident until commit"
// peak from the rebuild pipeline.
//
// True end-to-end streaming (bodies decoded inside Next) is a separate
// loader-side change that builds on this adapter contract.
func NewLoadResultRecordSource(records *LoadResult) RecordSource {
	if records == nil {
		return RecordSourceFunc(func(context.Context) (stcorpus.Record, bool, error) {
			return stcorpus.Record{}, false, nil
		})
	}

	specIdx := 0
	docIdx := 0
	exhausted := false
	return RecordSourceFunc(func(_ context.Context) (stcorpus.Record, bool, error) {
		if exhausted {
			return stcorpus.Record{}, false, nil
		}
		if specIdx < len(records.Specs) {
			spec := records.Specs[specIdx]
			specIdx++
			record, err := corpusRecordFromSpec(spec)
			if err != nil {
				exhausted = true
				return stcorpus.Record{}, false, err
			}
			return record, true, nil
		}
		if docIdx < len(records.Docs) {
			doc := records.Docs[docIdx]
			docIdx++
			record, err := corpusRecordFromDoc(doc)
			if err != nil {
				exhausted = true
				return stcorpus.Record{}, false, err
			}
			return record, true, nil
		}
		exhausted = true
		return stcorpus.Record{}, false, nil
	})
}

// CorpusRecordFromSpec converts a normalized spec record into a stroma
// corpus record, preserving the metadata and content-hash provenance
// the rebuild and update pipelines rely on.
func CorpusRecordFromSpec(spec model.SpecRecord) (stcorpus.Record, error) {
	return corpusRecordFromSpec(spec)
}

// CorpusRecordFromDoc converts a normalized doc record into a stroma
// corpus record. See CorpusRecordFromSpec for the metadata contract.
func CorpusRecordFromDoc(doc model.DocRecord) (stcorpus.Record, error) {
	return corpusRecordFromDoc(doc)
}

func corpusRecordFromSpec(spec model.SpecRecord) (stcorpus.Record, error) {
	record := stcorpus.Record{
		Ref:         spec.Ref,
		Kind:        spec.Kind,
		Title:       spec.Title,
		SourceRef:   spec.SourceRef,
		BodyFormat:  spec.BodyFormat,
		BodyText:    spec.BodyText,
		ContentHash: spec.ContentHash,
		Metadata:    cloneMetadata(spec.Metadata),
	}
	normalized, err := record.Normalize()
	if err != nil {
		return stcorpus.Record{}, fmt.Errorf("normalize spec %s: %w", spec.Ref, err)
	}
	return normalized, nil
}

func corpusRecordFromDoc(doc model.DocRecord) (stcorpus.Record, error) {
	record := stcorpus.Record{
		Ref:         doc.Ref,
		Kind:        doc.Kind,
		Title:       doc.Title,
		SourceRef:   doc.SourceRef,
		BodyFormat:  doc.BodyFormat,
		BodyText:    doc.BodyText,
		ContentHash: doc.ContentHash,
		Metadata:    cloneMetadata(doc.Metadata),
	}
	normalized, err := record.Normalize()
	if err != nil {
		return stcorpus.Record{}, fmt.Errorf("normalize doc %s: %w", doc.Ref, err)
	}
	return normalized, nil
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}
