package index

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

func BenchmarkRebuildFixture(b *testing.B) {
	cfg := loadFixtureConfig(b)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		b.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		b.Fatalf("warm Rebuild() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Rebuild(cfg, records); err != nil {
			b.Fatalf("Rebuild() error = %v", err)
		}
	}
}

// BenchmarkRebuildLargeCorpus exercises the streaming rebuild pipeline
// against a synthetic corpus large enough to make resident-set memory
// visible in -benchmem output. The corpus is constructed in memory so
// the measurement isolates the rebuild pipeline itself: the streaming
// switch (#396) lets stroma chunk and embed in bounded internal
// batches rather than holding the full chunks-and-vectors plan
// resident before commit, which the B/op delta against pre-#396
// should reflect.
func BenchmarkRebuildLargeCorpus(b *testing.B) {
	const docCount = 200
	const bodyKB = 4

	indexDir := b.TempDir()
	cfg := minimalRebuildConfig(b, filepath.Join(indexDir, "pituitary.db"))
	records := syntheticLargeCorpus(docCount, bodyKB)
	if _, err := Rebuild(cfg, records); err != nil {
		b.Fatalf("warm Rebuild() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Rebuild(cfg, records); err != nil {
			b.Fatalf("Rebuild() error = %v", err)
		}
	}
}

// syntheticLargeCorpus produces a deterministic in-memory LoadResult
// of N markdown docs, each carrying ~bodyKB KB of body text. Bodies
// vary per record so chunk-reuse from the previous iteration's
// snapshot stays bounded — the benchmark stresses the embed-and-flush
// path, not the reuse cache.
func syntheticLargeCorpus(n, bodyKB int) *source.LoadResult {
	docs := make([]model.DocRecord, 0, n)
	for i := 0; i < n; i++ {
		ref := fmt.Sprintf("doc://bench/%04d", i)
		title := fmt.Sprintf("Synthetic Doc %04d", i)
		var body strings.Builder
		body.WriteString("# ")
		body.WriteString(title)
		body.WriteString("\n\nBenchmark fixture body. Doc ordinal: ")
		body.WriteString(fmt.Sprintf("%d.\n\n", i))
		for body.Len() < bodyKB*1024 {
			body.WriteString(fmt.Sprintf("## Section %d-%d\n\n", i, body.Len()))
			body.WriteString("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ")
			body.WriteString(fmt.Sprintf("Doc %04d filler line %d.\n\n", i, body.Len()))
		}
		text := body.String()
		hash := sha256.Sum256([]byte(text))
		docs = append(docs, model.DocRecord{
			Ref:         ref,
			Kind:        model.ArtifactKindDoc,
			Title:       title,
			SourceRef:   fmt.Sprintf("docs/bench/%04d.md", i),
			BodyFormat:  "markdown",
			BodyText:    text,
			ContentHash: hex.EncodeToString(hash[:]),
			Metadata:    map[string]string{"path": fmt.Sprintf("docs/bench/%04d.md", i)},
		})
	}
	return &source.LoadResult{
		Docs: docs,
		Sources: []source.LoadSourceSummary{{
			Name:      "bench",
			Adapter:   "filesystem",
			Kind:      "markdown_docs",
			Path:      "docs/bench",
			ItemCount: n,
			DocCount:  n,
		}},
	}
}

// minimalRebuildConfig builds a workspace config rooted at a temp dir
// without writing fixture files. The streaming benchmark constructs
// its corpus directly in memory; the config only needs the embedder
// runtime and a workspace root the rebuild pipeline can stage into.
func minimalRebuildConfig(tb testing.TB, indexPath string) *config.Config {
	tb.Helper()
	repo := tb.TempDir()
	configPath := filepath.Join(repo, "pituitary.toml")
	mustWriteFile(tb, configPath, `
[workspace]
root = "`+filepath.ToSlash(repo)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"
infer_applies_to = false

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "synthetic"
adapter = "filesystem"
kind = "markdown_docs"
path = "."
include = ["unused-no-files-on-disk.md"]
`)
	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}

func BenchmarkSearchSpecs(b *testing.B) {
	cfg := loadFixtureConfig(b)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		b.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := Rebuild(cfg, records); err != nil {
		b.Fatalf("Rebuild() error = %v", err)
	}

	cases := []struct {
		name  string
		query SearchSpecQuery
	}{
		{
			name:  "default",
			query: SearchSpecQuery{Query: "rate limiting", Limit: 5},
		},
		{
			name: "historical",
			query: SearchSpecQuery{
				Query:    "fixed window rate limiting",
				Statuses: []string{model.StatusSuperseded},
				Limit:    5,
			},
		},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result, err := SearchSpecs(cfg, tc.query)
				if err != nil {
					b.Fatalf("SearchSpecs() error = %v", err)
				}
				if len(result.Matches) == 0 {
					b.Fatal("SearchSpecs() returned no matches")
				}
			}
		})
	}
}
