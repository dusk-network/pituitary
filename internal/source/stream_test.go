package source

import (
	"context"
	"errors"
	"testing"

	"github.com/dusk-network/pituitary/internal/model"
	stcorpus "github.com/dusk-network/stroma/v3/corpus"
)

func TestLoadResultRecordSourceYieldsSpecsThenDocs(t *testing.T) {
	t.Parallel()

	result := &LoadResult{
		Specs: []model.SpecRecord{
			{Ref: "SPEC-1", Kind: model.ArtifactKindSpec, Title: "One", BodyText: "spec one body", BodyFormat: "markdown", ContentHash: "h1", SourceRef: "specs/one"},
			{Ref: "SPEC-2", Kind: model.ArtifactKindSpec, Title: "Two", BodyText: "spec two body", BodyFormat: "markdown", ContentHash: "h2", SourceRef: "specs/two"},
		},
		Docs: []model.DocRecord{
			{Ref: "DOC-1", Kind: model.ArtifactKindDoc, Title: "Doc", BodyText: "doc body", BodyFormat: "markdown", ContentHash: "h3", SourceRef: "docs/one"},
		},
	}

	src := NewLoadResultRecordSource(result)
	ctx := context.Background()
	var refs []string
	for {
		record, ok, err := src.Next(ctx)
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if !ok {
			break
		}
		refs = append(refs, record.Ref)
	}
	want := []string{"SPEC-1", "SPEC-2", "DOC-1"}
	if len(refs) != len(want) {
		t.Fatalf("yielded refs = %v, want %v", refs, want)
	}
	for i, ref := range want {
		if refs[i] != ref {
			t.Fatalf("yielded refs[%d] = %q, want %q", i, refs[i], ref)
		}
	}

	// After exhaustion the iterator must keep returning false.
	if _, ok, err := src.Next(ctx); ok || err != nil {
		t.Fatalf("Next() after exhaustion = (_, %v, %v), want (zero, false, nil)", ok, err)
	}
}

func TestLoadResultRecordSourceHandlesNilLoadResult(t *testing.T) {
	t.Parallel()

	src := NewLoadResultRecordSource(nil)
	if _, ok, err := src.Next(context.Background()); ok || err != nil {
		t.Fatalf("Next() on nil source = (_, %v, %v), want (zero, false, nil)", ok, err)
	}
}

func TestLoadResultRecordSourceStickyErrorAfterNormalizationFailure(t *testing.T) {
	t.Parallel()

	// A spec record with no Ref fails stcorpus.Record.Normalize. The
	// adapter must surface that error on the failing Next and on every
	// subsequent Next, not silently flip to (zero, false, nil) — that
	// would let stroma's RebuildFromSource treat the stream as cleanly
	// exhausted and commit a partial snapshot.
	bad := model.SpecRecord{Kind: model.ArtifactKindSpec, Title: "no ref", BodyText: "x", BodyFormat: "markdown", ContentHash: "h"}
	src := NewLoadResultRecordSource(&LoadResult{Specs: []model.SpecRecord{bad}})
	ctx := context.Background()

	_, ok, firstErr := src.Next(ctx)
	if firstErr == nil {
		t.Fatal("Next() error = nil, want normalization failure on bad spec")
	}
	if ok {
		t.Fatal("Next() ok = true, want false on the failing record")
	}
	for i := 0; i < 3; i++ {
		_, ok, err := src.Next(ctx)
		if ok {
			t.Fatalf("Next() #%d ok = true, want sticky terminal state", i)
		}
		if !errors.Is(err, firstErr) {
			t.Fatalf("Next() #%d error = %v, want sticky %v", i, err, firstErr)
		}
	}
}

func TestRecordSourceFuncImplementsRecordSource(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("loader exploded")
	src := RecordSourceFunc(func(context.Context) (stcorpus.Record, bool, error) {
		return stcorpus.Record{}, false, wantErr
	})
	if _, _, err := src.Next(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("Next() error = %v, want %v", err, wantErr)
	}
}
