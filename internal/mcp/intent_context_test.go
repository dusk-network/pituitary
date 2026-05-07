package mcp

import (
	"context"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
)

func TestIntentContextToolsReturnStructuredResults(t *testing.T) {
	t.Parallel()

	srv := newIntentContextTestServer(t)
	outlineResult, err := srv.Client().CallTool(context.Background(), mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "get_intent_outline",
			Arguments: map[string]any{
				"ref":              "doc://guides/api-rate-limits",
				"kind":             "doc",
				"max_outline_rows": 2,
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(get_intent_outline) error = %v", err)
	}
	if outlineResult.IsError {
		t.Fatalf("CallTool(get_intent_outline) returned tool error: %+v", outlineResult)
	}

	outlinePayload := decodeIntentOutlinePayload(t, outlineResult)
	if outlinePayload.Record.Ref != "doc://guides/api-rate-limits" || outlinePayload.Record.Kind != "doc" {
		t.Fatalf("record = %+v, want API guide doc", outlinePayload.Record)
	}
	if outlinePayload.Record.SourceRef == "" || outlinePayload.SnapshotFingerprint == "" {
		t.Fatalf("outline payload missing provenance: %+v", outlinePayload)
	}
	if len(outlinePayload.Outline) == 0 || outlinePayload.Outline[0].ChunkID == 0 || outlinePayload.Outline[0].Heading == "" {
		t.Fatalf("outline = %+v, want rows with chunk ids and headings", outlinePayload.Outline)
	}

	expandResult, err := srv.Client().CallTool(context.Background(), mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "expand_intent_context",
			Arguments: map[string]any{
				"chunk_id":             outlinePayload.Outline[0].ChunkID,
				"snapshot_fingerprint": outlinePayload.SnapshotFingerprint,
				"include_parent":       true,
				"neighbor_window":      1,
				"max_section_bytes":    120,
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(expand_intent_context) error = %v", err)
	}
	if expandResult.IsError {
		t.Fatalf("CallTool(expand_intent_context) returned tool error: %+v", expandResult)
	}

	var expandPayload intentExpandPayload
	decodeStructuredContent(t, expandResult.StructuredContent, &expandPayload)
	if expandPayload.SnapshotFingerprint != outlinePayload.SnapshotFingerprint {
		t.Fatalf("snapshot_fingerprint = %q, want %q", expandPayload.SnapshotFingerprint, outlinePayload.SnapshotFingerprint)
	}
	if !expandPayload.hasSelectedSection(outlinePayload.Outline[0].ChunkID) {
		t.Fatalf("sections = %+v, want selected section", expandPayload.Sections)
	}
}

func TestExpandIntentContextToolReportsStaleSnapshot(t *testing.T) {
	t.Parallel()

	srv := newIntentContextTestServer(t)
	outlineResult, err := srv.Client().CallTool(context.Background(), mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "get_intent_outline",
			Arguments: map[string]any{
				"ref":  "SPEC-042",
				"kind": "spec",
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(get_intent_outline) error = %v", err)
	}
	if outlineResult.IsError {
		t.Fatalf("CallTool(get_intent_outline) returned tool error: %+v", outlineResult)
	}

	outlinePayload := decodeIntentOutlinePayload(t, outlineResult)
	if len(outlinePayload.Outline) == 0 {
		t.Fatal("get_intent_outline returned no outline rows")
	}

	result, err := srv.Client().CallTool(context.Background(), mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "expand_intent_context",
			Arguments: map[string]any{
				"chunk_id":             outlinePayload.Outline[0].ChunkID,
				"snapshot_fingerprint": "old-snapshot",
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(expand_intent_context) error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("CallTool(expand_intent_context) result = %+v, want tool error", result)
	}
	if !strings.Contains(toolResultText(result), "stale snapshot fingerprint") {
		t.Fatalf("tool error = %q, want stale snapshot detail", toolResultText(result))
	}
}

func TestGetIntentOutlineToolReportsFilteredRecord(t *testing.T) {
	t.Parallel()

	srv := newIntentContextTestServer(t)
	result, err := srv.Client().CallTool(context.Background(), mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "get_intent_outline",
			Arguments: map[string]any{
				"ref":  "SPEC-008",
				"kind": "spec",
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(get_intent_outline) error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("CallTool(get_intent_outline) result = %+v, want tool error", result)
	}
	if !strings.Contains(toolResultText(result), "excluded by the requested filters") {
		t.Fatalf("tool error = %q, want filtered-record detail", toolResultText(result))
	}
}

type intentOutlinePayload struct {
	Record struct {
		Ref       string `json:"ref"`
		Kind      string `json:"kind"`
		SourceRef string `json:"source_ref"`
	} `json:"record"`
	SnapshotFingerprint string `json:"snapshot_fingerprint"`
	Outline             []struct {
		ChunkID int64  `json:"chunk_id"`
		Heading string `json:"heading"`
	} `json:"outline"`
}

type intentExpandPayload struct {
	SnapshotFingerprint string `json:"snapshot_fingerprint"`
	Sections            []struct {
		ChunkID   int64  `json:"chunk_id"`
		Ref       string `json:"ref"`
		Kind      string `json:"kind"`
		Heading   string `json:"heading"`
		Role      string `json:"role"`
		SourceRef string `json:"source_ref"`
		Content   string `json:"content"`
	} `json:"sections"`
}

func newIntentContextTestServer(t *testing.T) *mcptest.Server {
	t.Helper()

	configPath := writeMCPWorkspace(t)
	srv, err := mcptest.NewServer(t, Tools(Options{ConfigPath: configPath})...)
	if err != nil {
		t.Fatalf("mcptest.NewServer() error = %v", err)
	}
	t.Cleanup(srv.Close)
	return srv
}

func decodeIntentOutlinePayload(t *testing.T, result *mcpgo.CallToolResult) intentOutlinePayload {
	t.Helper()

	var payload intentOutlinePayload
	decodeStructuredContent(t, result.StructuredContent, &payload)
	return payload
}

func (p intentExpandPayload) hasSelectedSection(chunkID int64) bool {
	for _, section := range p.Sections {
		if section.ChunkID == 0 || section.Ref == "" || section.Kind == "" || section.SourceRef == "" {
			return false
		}
		if section.Role == "selected" && section.ChunkID == chunkID {
			return section.Heading != "" && section.Content != ""
		}
	}
	return false
}
