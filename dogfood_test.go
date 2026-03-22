package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestDogfoodPhase1Workspace(t *testing.T) {
	cfg, err := config.Load(filepath.Join("dogfood", "pituitary.toml"))
	if err != nil {
		t.Fatalf("config.Load(dogfood) error = %v", err)
	}

	preview, err := source.PreviewFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.PreviewFromConfig(dogfood) error = %v", err)
	}
	if got, want := len(preview.Sources), 2; got != want {
		t.Fatalf("preview source count = %d, want %d", got, want)
	}

	contracts := preview.Sources[0]
	if got, want := contracts.Name, "contracts"; got != want {
		t.Fatalf("contracts source name = %q, want %q", got, want)
	}
	if got, want := contracts.ItemCount, 2; got != want {
		t.Fatalf("contracts item count = %d, want %d", got, want)
	}

	docs := preview.Sources[1]
	if got, want := docs.Name, "docs"; got != want {
		t.Fatalf("docs source name = %q, want %q", got, want)
	}
	if got, want := docs.ItemCount, 6; got != want {
		t.Fatalf("docs item count = %d, want %d", got, want)
	}

	docPaths := make([]string, 0, len(docs.Items))
	for _, item := range docs.Items {
		docPaths = append(docPaths, item.Path)
	}
	joined := strings.Join(docPaths, "\n")
	for _, unexpected := range []string{"IMPLEMENTATION_BACKLOG.md", "tasklist.md"} {
		if strings.Contains(joined, unexpected) {
			t.Fatalf("dogfood docs unexpectedly include %q", unexpected)
		}
	}

	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig(dogfood) error = %v", err)
	}
	if got, want := len(records.Specs), 2; got != want {
		t.Fatalf("loaded dogfood spec count = %d, want %d", got, want)
	}
	if got, want := len(records.Docs), 6; got != want {
		t.Fatalf("loaded dogfood doc count = %d, want %d", got, want)
	}

	result, err := index.PrepareRebuild(cfg, records)
	if err != nil {
		t.Fatalf("index.PrepareRebuild(dogfood) error = %v", err)
	}
	if result == nil || result.ArtifactCount == 0 || result.ChunkCount == 0 {
		t.Fatalf("dogfood dry-run result = %+v, want indexed artifacts and chunks", result)
	}
}
