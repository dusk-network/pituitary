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
