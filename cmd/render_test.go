package cmd

import (
	"bytes"
	"strings"
	"testing"

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
