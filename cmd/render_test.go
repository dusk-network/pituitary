package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestRenderPreviewSourcesResultIncludesFiles(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	renderPreviewSourcesResult(&stdout, &source.PreviewResult{
		Sources: []source.SourcePreview{
			{
				Name:      "docs",
				Kind:      "markdown_docs",
				Path:      ".",
				Files:     []string{"docs/guides/api-rate-limits.md"},
				ItemCount: 1,
				Items: []source.PreviewItem{
					{
						ArtifactKind: "doc",
						Path:         "docs/guides/api-rate-limits.md",
					},
				},
			},
		},
	})

	output := stdout.String()
	if !strings.Contains(output, "files: docs/guides/api-rate-limits.md") {
		t.Fatalf("renderPreviewSourcesResult() output %q does not contain files selector", output)
	}
}

func TestRenderCommandTableSearchSpecs(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer

	err := renderCommandTable(&stdout, "search-specs", &index.SearchSpecResult{
		Matches: []index.SearchSpecMatch{
			{
				Ref:            "SPEC-042",
				Title:          "Tenant-aware rate limiting",
				SectionHeading: "Per-tenant quotas",
				Score:          0.9876,
			},
		},
	})
	if err != nil {
		t.Fatalf("renderCommandTable() error = %v, want nil", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"pituitary search-specs: search spec sections semantically",
		"REF",
		"TITLE",
		"SECTION",
		"SCORE",
		"SPEC-042",
		"Tenant-aware rate limiting",
		"Per-tenant quotas",
		"0.988",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("renderCommandTable() output %q does not contain %q", output, want)
		}
	}
}
