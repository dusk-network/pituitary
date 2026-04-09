package jsonadapter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/sdk"
)

func TestAdapterLoadsJSONSpecWithMappedFields(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "schemas", "rate-limit.json"), `{
  "meta": {
    "ref": "SPEC-JSON-001",
    "status": "accepted",
    "domain": "api",
    "authors": ["alice", "bob"],
    "tags": ["rate-limit", "api"],
    "depends_on": ["SPEC-BASE-001"],
    "applies_to": ["config://api/rate-limit.json"]
  },
  "info": {
    "title": "JSON Rate Limits"
  },
  "description": "JSON-backed rate limiting policy."
}`)

	adapter := &adapter{}
	result, err := adapter.Load(context.Background(), sdk.SourceConfig{
		Name:          "api-json",
		Adapter:       adapterName,
		Kind:          kindSpec,
		Path:          "schemas",
		WorkspaceRoot: root,
		Options: map[string]any{
			"ref_pointer":        "/meta/ref",
			"title_pointer":      "/info/title",
			"body_pointer":       "/description",
			"status_pointer":     "/meta/status",
			"domain_pointer":     "/meta/domain",
			"authors_pointer":    "/meta/authors",
			"tags_pointer":       "/meta/tags",
			"depends_on_pointer": "/meta/depends_on",
			"applies_to_pointer": "/meta/applies_to",
		},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := len(result.Specs), 1; got != want {
		t.Fatalf("spec count = %d, want %d", got, want)
	}
	spec := result.Specs[0]
	if got, want := spec.Ref, "SPEC-JSON-001"; got != want {
		t.Fatalf("spec ref = %q, want %q", got, want)
	}
	if got, want := spec.Status, sdk.StatusAccepted; got != want {
		t.Fatalf("spec status = %q, want %q", got, want)
	}
	if got, want := spec.Domain, "api"; got != want {
		t.Fatalf("spec domain = %q, want %q", got, want)
	}
	if got, want := spec.Metadata["path"], "schemas/rate-limit.json"; got != want {
		t.Fatalf("spec metadata path = %q, want %q", got, want)
	}
	if !strings.Contains(spec.BodyText, "JSON-backed rate limiting policy.") {
		t.Fatalf("spec body = %q, want mapped body text", spec.BodyText)
	}
	if got, want := len(spec.Relations), 1; got != want {
		t.Fatalf("relation count = %d, want %d", got, want)
	}
}

func TestAdapterLoadsJSONDocWithPathFallbacks(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "artifacts", "schema.json"), `{
  "title": "Event Schema",
  "type": "object",
  "properties": {
    "id": {"type": "string"}
  }
}`)

	adapter := &adapter{}
	result, err := adapter.Load(context.Background(), sdk.SourceConfig{
		Name:          "json-docs",
		Adapter:       adapterName,
		Kind:          kindDoc,
		Path:          "artifacts",
		WorkspaceRoot: root,
		Options: map[string]any{
			"title_pointer": "/title",
		},
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := len(result.Docs), 1; got != want {
		t.Fatalf("doc count = %d, want %d", got, want)
	}
	doc := result.Docs[0]
	if got, want := doc.Ref, "json-doc://schema"; got != want {
		t.Fatalf("doc ref = %q, want %q", got, want)
	}
	if got, want := doc.Title, "Event Schema"; got != want {
		t.Fatalf("doc title = %q, want %q", got, want)
	}
	if !strings.Contains(doc.BodyText, "properties") {
		t.Fatalf("doc body = %q, want rendered JSON body", doc.BodyText)
	}
}

func TestAdapterPreviewListsSelectedJSONFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "schemas", "keep.json"), `{"title":"Keep"}`)
	writeFile(t, filepath.Join(root, "schemas", "skip.json"), `{"title":"Skip"}`)

	adapter := &adapter{}
	items, err := adapter.Preview(context.Background(), sdk.SourceConfig{
		Name:          "json-docs",
		Adapter:       adapterName,
		Kind:          kindDoc,
		Path:          "schemas",
		WorkspaceRoot: root,
		Files:         []string{"keep.json"},
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if got, want := len(items), 1; got != want {
		t.Fatalf("preview item count = %d, want %d", got, want)
	}
	if got, want := items[0].Path, "schemas/keep.json"; got != want {
		t.Fatalf("preview path = %q, want %q", got, want)
	}
	if got, want := items[0].ArtifactKind, sdk.ArtifactKindDoc; got != want {
		t.Fatalf("preview artifact kind = %q, want %q", got, want)
	}
}

func TestAdapterRejectsDocOnlySpecFields(t *testing.T) {
	t.Parallel()

	adapter := &adapter{}
	_, err := adapter.Load(context.Background(), sdk.SourceConfig{
		Name:          "json-docs",
		Adapter:       adapterName,
		Kind:          kindDoc,
		Path:          ".",
		WorkspaceRoot: t.TempDir(),
		Options: map[string]any{
			"status_pointer": "/status",
		},
	})
	if err == nil {
		t.Fatal("Load() error = nil, want doc/spec option failure")
	}
	if !strings.Contains(err.Error(), `options.status_pointer is only supported for kind "json_spec"`) {
		t.Fatalf("Load() error = %q, want spec-only option detail", err)
	}
}

func TestAdapterRejectsMissingMappedField(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "schemas", "broken.json"), `{"title":"Broken"}`)

	adapter := &adapter{}
	_, err := adapter.Load(context.Background(), sdk.SourceConfig{
		Name:          "broken-json",
		Adapter:       adapterName,
		Kind:          kindSpec,
		Path:          "schemas",
		WorkspaceRoot: root,
		Options: map[string]any{
			"status_pointer": "/status",
		},
	})
	if err == nil {
		t.Fatal("Load() error = nil, want missing pointer failure")
	}
	if !strings.Contains(err.Error(), `json "schemas/broken.json"`) {
		t.Fatalf("Load() error = %q, want workspace-relative path", err)
	}
	if !strings.Contains(err.Error(), `pointer "/status" does not exist`) {
		t.Fatalf("Load() error = %q, want pointer detail", err)
	}
	if strings.Contains(err.Error(), root) {
		t.Fatalf("Load() error = %q, should not leak absolute workspace path", err)
	}
}

func TestAdapterRejectsEscapingExplicitJSONFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "schemas", "rate-limit.json"), `{"title":"Rate Limit"}`)

	adapter := &adapter{}
	_, err := adapter.Preview(context.Background(), sdk.SourceConfig{
		Name:          "json-docs",
		Adapter:       adapterName,
		Kind:          kindDoc,
		Path:          "schemas",
		WorkspaceRoot: root,
		Files:         []string{"../rate-limit.json"},
	})
	if err == nil {
		t.Fatal("Preview() error = nil, want escaping file selector failure")
	}
	if !strings.Contains(err.Error(), `files[0]: "../rate-limit.json" escapes the source root`) {
		t.Fatalf("Preview() error = %q, want escaping path detail", err)
	}
}

func TestParsePointerIndexRejectsLeadingZero(t *testing.T) {
	t.Parallel()

	_, err := parsePointerIndex("007", 8)
	if err == nil {
		t.Fatal("parsePointerIndex() error = nil, want leading-zero failure")
	}
	if !strings.Contains(err.Error(), `array index "007" has invalid leading zero`) {
		t.Fatalf("parsePointerIndex() error = %q, want leading-zero detail", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
