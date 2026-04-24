package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
	mcpclient "github.com/mark3labs/mcp-go/client"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/mcptest"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestValidateStartupAcceptsReadyWorkspace(t *testing.T) {
	t.Parallel()

	configPath := writeMCPWorkspace(t)
	if err := validateStartup(Options{ConfigPath: configPath}); err != nil {
		t.Fatalf("validateStartup() error = %v", err)
	}
}

func TestValidateStartupDoesNotRequireLiveEmbedderEndpointWhenIndexAlreadyExists(t *testing.T) {
	server := newOpenAICompatibleEmbeddingServer(t)
	configPath := writeMCPWorkspaceWithRuntime(t, fmt.Sprintf(`
[runtime.embedder]
provider = "openai_compatible"
model = "pituitary-embed"
endpoint = %q
timeout_ms = 1000
max_retries = 0
`, server.URL+"/v1"))
	server.Close()

	if err := validateStartup(Options{ConfigPath: configPath}); err != nil {
		t.Fatalf("validateStartup() error = %v, want startup to rely on stored metadata", err)
	}
}

func TestValidateStartupRejectsMissingConfig(t *testing.T) {
	t.Parallel()

	missingPath := filepath.Join(t.TempDir(), "pituitary.toml")
	err := validateStartup(Options{ConfigPath: missingPath})
	if err == nil {
		t.Fatal("validateStartup() error = nil, want missing-config failure")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Fatalf("validateStartup() error = %v, want load-config detail", err)
	}
}

func TestValidateStartupRejectsMissingIndex(t *testing.T) {
	t.Parallel()

	configPath := writeMCPServeWorkspace(t, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md", "runbooks/*.md"]
`)

	err := validateStartup(Options{ConfigPath: configPath})
	if err == nil {
		t.Fatal("validateStartup() error = nil, want missing-index failure")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("validateStartup() error = %v, want missing-index detail", err)
	}
}

func TestValidateStartupRejectsOpenAICompatibleEmbedderWithoutEndpoint(t *testing.T) {
	configPath := writeMCPServeWorkspace(t, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "openai_compatible"
model = "text-embedding-3-small"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md", "runbooks/*.md"]
`)

	err := validateStartup(Options{ConfigPath: configPath})
	if err == nil {
		t.Fatal("validateStartup() error = nil, want embedder failure")
	}
	if !strings.Contains(err.Error(), `runtime.embedder.endpoint: value is required for provider "openai_compatible"`) {
		t.Fatalf("validateStartup() error = %v, want missing-endpoint detail", err)
	}
}

func TestValidateStartupRejectsSQLiteReadinessFailure(t *testing.T) {
	configPath := writeMCPServeWorkspace(t, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
`)

	previous := sqliteReadyCheck
	sqliteReadyCheck = func() error {
		return errors.New("sqlite-vec smoke failed")
	}
	t.Cleanup(func() {
		sqliteReadyCheck = previous
	})

	err := validateStartup(Options{ConfigPath: configPath})
	if err == nil {
		t.Fatal("validateStartup() error = nil, want sqlite readiness failure")
	}
	if !strings.Contains(err.Error(), "sqlite readiness check failed") {
		t.Fatalf("validateStartup() error = %v, want sqlite readiness detail", err)
	}
}

func TestServeStdioSubprocessSmoke(t *testing.T) {
	configPath := writeMCPWorkspace(t)
	binaryPath := buildPituitaryBinary(t)
	client, stderrLogs := newStdioSmokeClient(t, binaryPath, configPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var initRequest mcpgo.InitializeRequest
	initRequest.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcpgo.Implementation{
		Name:    "pituitary-mcp-smoke",
		Version: "1.0.0",
	}

	initResult, err := client.Initialize(ctx, initRequest)
	if err != nil {
		t.Fatalf("client.Initialize() error = %v\nstderr:\n%s", err, stderrLogs.detail())
	}
	if initResult.ServerInfo.Name != serverName {
		t.Fatalf("server name = %q, want %q\nstderr:\n%s", initResult.ServerInfo.Name, serverName, stderrLogs.detail())
	}
	if initResult.ServerInfo.Version != serverVersion {
		t.Fatalf("server version = %q, want %q\nstderr:\n%s", initResult.ServerInfo.Version, serverVersion, stderrLogs.detail())
	}

	toolsResult, err := client.ListTools(ctx, mcpgo.ListToolsRequest{})
	if err != nil {
		t.Fatalf("client.ListTools() error = %v\nstderr:\n%s", err, stderrLogs.detail())
	}

	names := make([]string, 0, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)

	wantTools := shippedToolNames()
	if len(names) != len(wantTools) {
		t.Fatalf("tool names = %v, want %v\nstderr:\n%s", names, wantTools, stderrLogs.detail())
	}
	for i := range wantTools {
		if names[i] != wantTools[i] {
			t.Fatalf("tool names = %v, want %v\nstderr:\n%s", names, wantTools, stderrLogs.detail())
		}
	}

	callResult, err := client.CallTool(ctx, mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "search_specs",
			Arguments: map[string]any{
				"query": "rate limiting",
			},
		},
	})
	if err != nil {
		t.Fatalf("client.CallTool(search_specs) error = %v\nstderr:\n%s", err, stderrLogs.detail())
	}
	if callResult.IsError {
		t.Fatalf("client.CallTool(search_specs) returned tool error: %+v\nstderr:\n%s", callResult, stderrLogs.detail())
	}

	var payload struct {
		Matches []struct {
			Ref string `json:"ref"`
		} `json:"matches"`
	}
	decodeStructuredContent(t, callResult.StructuredContent, &payload)
	if len(payload.Matches) == 0 {
		t.Fatalf("search_specs returned no matches\nstderr:\n%s", stderrLogs.detail())
	}
	if payload.Matches[0].Ref == "" {
		t.Fatalf("top match = %+v, want stable ref\nstderr:\n%s", payload.Matches[0], stderrLogs.detail())
	}
}

func TestToolsExposeOnlyShippedOperations(t *testing.T) {
	t.Parallel()

	configPath := writeMCPWorkspace(t)

	srv, err := mcptest.NewServer(t, Tools(Options{ConfigPath: configPath})...)
	if err != nil {
		t.Fatalf("mcptest.NewServer() error = %v", err)
	}
	t.Cleanup(srv.Close)

	result, err := srv.Client().ListTools(context.Background(), mcpgo.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)

	want := shippedToolNames()
	if len(names) != len(want) {
		t.Fatalf("tool names = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("tool names = %v, want %v", names, want)
		}
	}
}

func TestSearchSpecsToolReturnsStructuredResult(t *testing.T) {
	t.Parallel()

	configPath := writeMCPWorkspace(t)

	srv, err := mcptest.NewServer(t, Tools(Options{ConfigPath: configPath})...)
	if err != nil {
		t.Fatalf("mcptest.NewServer() error = %v", err)
	}
	t.Cleanup(srv.Close)

	result, err := srv.Client().CallTool(context.Background(), mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "search_specs",
			Arguments: map[string]any{
				"query": "rate limiting",
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(search_specs) error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool(search_specs) returned tool error: %+v", result)
	}

	var payload struct {
		Matches []struct {
			Ref string `json:"ref"`
		} `json:"matches"`
	}
	decodeStructuredContent(t, result.StructuredContent, &payload)
	if len(payload.Matches) == 0 {
		t.Fatal("search_specs returned no matches")
	}
	if payload.Matches[0].Ref == "" {
		t.Fatalf("top match = %+v, want stable ref", payload.Matches[0])
	}
}

func TestReviewSpecToolReturnsStructuredResult(t *testing.T) {
	t.Parallel()

	configPath := writeMCPWorkspace(t)

	srv, err := mcptest.NewServer(t, Tools(Options{ConfigPath: configPath})...)
	if err != nil {
		t.Fatalf("mcptest.NewServer() error = %v", err)
	}
	t.Cleanup(srv.Close)

	result, err := srv.Client().CallTool(context.Background(), mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "review_spec",
			Arguments: map[string]any{
				"spec_ref": "SPEC-042",
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(review_spec) error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool(review_spec) returned tool error: %+v", result)
	}

	var payload struct {
		SpecRef string `json:"spec_ref"`
		Overlap struct {
			Overlaps []struct {
				Ref string `json:"ref"`
			} `json:"overlaps"`
		} `json:"overlap"`
		Impact struct {
			AffectedSpecs []struct {
				Ref string `json:"ref"`
			} `json:"affected_specs"`
		} `json:"impact"`
		DocDrift struct {
			DriftItems []struct {
				DocRef string `json:"doc_ref"`
			} `json:"drift_items"`
		} `json:"doc_drift"`
	}
	decodeStructuredContent(t, result.StructuredContent, &payload)
	if payload.SpecRef != "SPEC-042" {
		t.Fatalf("spec_ref = %q, want SPEC-042", payload.SpecRef)
	}
	if len(payload.Overlap.Overlaps) == 0 || payload.Overlap.Overlaps[0].Ref != "SPEC-008" {
		t.Fatalf("overlap = %+v, want SPEC-008 first", payload.Overlap)
	}
	if len(payload.Impact.AffectedSpecs) == 0 || payload.Impact.AffectedSpecs[0].Ref != "SPEC-055" {
		t.Fatalf("impact = %+v, want SPEC-055 impacted", payload.Impact)
	}
	if len(payload.DocDrift.DriftItems) != 1 || payload.DocDrift.DriftItems[0].DocRef != "doc://guides/api-rate-limits" {
		t.Fatalf("doc_drift = %+v, want guide drift only", payload.DocDrift)
	}
}

func TestCheckComplianceToolReturnsStructuredResult(t *testing.T) {
	t.Parallel()

	configPath := writeMCPWorkspace(t)

	srv, err := mcptest.NewServer(t, Tools(Options{ConfigPath: configPath})...)
	if err != nil {
		t.Fatalf("mcptest.NewServer() error = %v", err)
	}
	t.Cleanup(srv.Close)

	result, err := srv.Client().CallTool(context.Background(), mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "check_compliance",
			Arguments: map[string]any{
				"diff_text": "--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,3 @@\n-old line\n+new line\n context\n",
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(check_compliance) error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool(check_compliance) returned tool error: %+v", result)
	}

	var payload struct {
		Paths []string `json:"paths"`
	}
	decodeStructuredContent(t, result.StructuredContent, &payload)
	if len(payload.Paths) == 0 {
		t.Fatal("check_compliance returned no paths")
	}
}

func TestGovernedByToolReturnsStructuredResult(t *testing.T) {
	t.Parallel()

	configPath := writeMCPWorkspace(t)

	srv, err := mcptest.NewServer(t, Tools(Options{ConfigPath: configPath})...)
	if err != nil {
		t.Fatalf("mcptest.NewServer() error = %v", err)
	}
	t.Cleanup(srv.Close)

	result, err := srv.Client().CallTool(context.Background(), mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "governed_by",
			Arguments: map[string]any{
				"path": "src/api/middleware/ratelimiter.go",
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(governed_by) error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool(governed_by) returned tool error: %+v", result)
	}

	var payload struct {
		Path  string `json:"path"`
		Specs []struct {
			Ref string `json:"ref"`
		} `json:"specs"`
	}
	decodeStructuredContent(t, result.StructuredContent, &payload)
	if payload.Path == "" {
		t.Fatal("governed_by returned empty path")
	}
}

func TestStatusToolReturnsStructuredResult(t *testing.T) {
	t.Parallel()

	configPath := writeMCPWorkspace(t)

	srv, err := mcptest.NewServer(t, Tools(Options{ConfigPath: configPath})...)
	if err != nil {
		t.Fatalf("mcptest.NewServer() error = %v", err)
	}
	t.Cleanup(srv.Close)

	result, err := srv.Client().CallTool(context.Background(), mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "status",
		},
	})
	if err != nil {
		t.Fatalf("CallTool(status) error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool(status) returned tool error: %+v", result)
	}

	var payload struct {
		IndexExists bool `json:"index_exists"`
		SpecCount   int  `json:"spec_count"`
		DocCount    int  `json:"doc_count"`
	}
	decodeStructuredContent(t, result.StructuredContent, &payload)
	if !payload.IndexExists {
		t.Fatal("status reports index does not exist")
	}
	if payload.SpecCount == 0 {
		t.Fatal("status reports 0 specs")
	}
}

func TestCheckTerminologyToolReturnsStructuredResult(t *testing.T) {
	t.Parallel()

	configPath := writeMCPWorkspace(t)

	srv, err := mcptest.NewServer(t, Tools(Options{ConfigPath: configPath})...)
	if err != nil {
		t.Fatalf("mcptest.NewServer() error = %v", err)
	}
	t.Cleanup(srv.Close)

	result, err := srv.Client().CallTool(context.Background(), mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "check_terminology",
			Arguments: map[string]any{
				"terms": []any{"rate limit"},
				"scope": "all",
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(check_terminology) error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool(check_terminology) returned tool error: %+v", result)
	}

	var payload struct {
		Scope struct {
			Mode string `json:"mode"`
		} `json:"scope"`
		Terms []string `json:"terms"`
	}
	decodeStructuredContent(t, result.StructuredContent, &payload)
	if len(payload.Terms) == 0 {
		t.Fatal("check_terminology returned no terms")
	}
}

func TestCompilePreviewToolReturnsToolResult(t *testing.T) {
	t.Parallel()

	configPath := writeMCPWorkspace(t)

	srv, err := mcptest.NewServer(t, Tools(Options{ConfigPath: configPath})...)
	if err != nil {
		t.Fatalf("mcptest.NewServer() error = %v", err)
	}
	t.Cleanup(srv.Close)

	// The test workspace has no terminology policies, so compile_preview returns
	// a tool error. Verify the tool responds without a transport error.
	result, err := srv.Client().CallTool(context.Background(), mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "compile_preview",
			Arguments: map[string]any{
				"scope": "all",
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(compile_preview) error = %v", err)
	}
	// Tool error is expected (no terminology policies) — verify it's not a crash.
	if !result.IsError {
		var payload struct {
			Applied bool `json:"applied"`
		}
		decodeStructuredContent(t, result.StructuredContent, &payload)
		if payload.Applied {
			t.Fatal("compile_preview should not apply edits")
		}
	}
}

func TestFixPreviewToolReturnsStructuredResult(t *testing.T) {
	t.Parallel()

	configPath := writeMCPWorkspace(t)

	srv, err := mcptest.NewServer(t, Tools(Options{ConfigPath: configPath})...)
	if err != nil {
		t.Fatalf("mcptest.NewServer() error = %v", err)
	}
	t.Cleanup(srv.Close)

	result, err := srv.Client().CallTool(context.Background(), mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "fix_preview",
			Arguments: map[string]any{
				"scope": "all",
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(fix_preview) error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool(fix_preview) returned tool error: %+v", result)
	}

	var payload struct {
		Selector string `json:"selector"`
		Applied  bool   `json:"applied"`
	}
	decodeStructuredContent(t, result.StructuredContent, &payload)
	if payload.Applied {
		t.Fatal("fix_preview should not apply edits")
	}
}

func TestExplainFileToolReturnsStructuredResult(t *testing.T) {
	t.Parallel()

	configPath := writeMCPWorkspace(t)
	workspaceRoot := filepath.Dir(configPath)

	srv, err := mcptest.NewServer(t, Tools(Options{ConfigPath: configPath})...)
	if err != nil {
		t.Fatalf("mcptest.NewServer() error = %v", err)
	}
	t.Cleanup(srv.Close)

	result, err := srv.Client().CallTool(context.Background(), mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Name: "explain_file",
			Arguments: map[string]any{
				"path": filepath.Join(workspaceRoot, "specs", "rate-limit-v2", "spec.toml"),
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(explain_file) error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool(explain_file) returned tool error: %+v", result)
	}

	var payload struct {
		Summary struct {
			Status string `json:"status"`
		} `json:"summary"`
	}
	decodeStructuredContent(t, result.StructuredContent, &payload)
	if payload.Summary.Status == "" {
		t.Fatal("explain_file returned empty summary status")
	}
}

func TestSearchSpecsToolHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	configPath := writeMCPWorkspace(t)
	started := make(chan struct{})
	client := newWrappedInProcessClient(t, Options{ConfigPath: configPath}, "search_specs", started)

	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan callToolOutcome, 1)
	go func() {
		result, err := client.CallTool(ctx, mcpgo.CallToolRequest{
			Params: mcpgo.CallToolParams{
				Name: "search_specs",
				Arguments: map[string]any{
					"query": "rate limiting",
				},
			},
		})
		results <- callToolOutcome{result: result, err: err}
	}()

	waitForToolInvocation(t, started)
	cancel()

	outcome := waitForCallToolOutcome(t, results)
	if outcome.err != nil {
		t.Fatalf("CallTool(search_specs) error = %v, want nil transport error", outcome.err)
	}
	if outcome.result == nil {
		t.Fatal("CallTool(search_specs) result = nil, want tool error result")
	}
	if !outcome.result.IsError {
		t.Fatalf("CallTool(search_specs) result = %+v, want tool error", outcome.result)
	}
	if !strings.Contains(toolResultText(outcome.result), "context canceled") {
		t.Fatalf("CallTool(search_specs) text = %q, want context canceled detail", toolResultText(outcome.result))
	}
}

func TestReviewSpecToolHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	configPath := writeMCPWorkspace(t)
	started := make(chan struct{})
	client := newWrappedInProcessClient(t, Options{ConfigPath: configPath}, "review_spec", started)

	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan callToolOutcome, 1)
	go func() {
		result, err := client.CallTool(ctx, mcpgo.CallToolRequest{
			Params: mcpgo.CallToolParams{
				Name: "review_spec",
				Arguments: map[string]any{
					"spec_ref": "SPEC-042",
				},
			},
		})
		results <- callToolOutcome{result: result, err: err}
	}()

	waitForToolInvocation(t, started)
	cancel()

	outcome := waitForCallToolOutcome(t, results)
	if outcome.err != nil {
		t.Fatalf("CallTool(review_spec) error = %v, want nil transport error", outcome.err)
	}
	if outcome.result == nil {
		t.Fatal("CallTool(review_spec) result = nil, want tool error result")
	}
	if !outcome.result.IsError {
		t.Fatalf("CallTool(review_spec) result = %+v, want tool error", outcome.result)
	}
	if !strings.Contains(toolResultText(outcome.result), "context canceled") {
		t.Fatalf("CallTool(review_spec) text = %q, want context canceled detail", toolResultText(outcome.result))
	}
}

func decodeStructuredContent(t *testing.T, input any, target any) {
	t.Helper()

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal(structuredContent) error = %v", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("json.Unmarshal(structuredContent) error = %v", err)
	}
}

type callToolOutcome struct {
	result *mcpgo.CallToolResult
	err    error
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) detail() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.buf.Len() == 0 {
		return "<empty>"
	}
	return strings.TrimSpace(b.buf.String())
}

func newWrappedInProcessClient(t *testing.T, options Options, toolName string, started chan struct{}) *mcpclient.Client {
	t.Helper()

	server := mcpserver.NewMCPServer(
		serverName,
		serverVersion,
		mcpserver.WithToolCapabilities(false),
		mcpserver.WithRecovery(),
	)
	server.AddTools(wrapToolForCancellation(t, Tools(options), toolName, started)...)

	client, err := mcpclient.NewInProcessClient(server)
	if err != nil {
		t.Fatalf("NewInProcessClient() error = %v", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Fatalf("client.Close() error = %v", err)
		}
	})

	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("client.Start() error = %v", err)
	}

	var initRequest mcpgo.InitializeRequest
	initRequest.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcpgo.Implementation{
		Name:    "pituitary-mcp-test-client",
		Version: "1.0.0",
	}
	if _, err := client.Initialize(context.Background(), initRequest); err != nil {
		t.Fatalf("client.Initialize() error = %v", err)
	}

	return client
}

func newStdioSmokeClient(t *testing.T, binaryPath, configPath string) (*mcpclient.Client, *lockedBuffer) {
	t.Helper()

	client, err := mcpclient.NewStdioMCPClient(binaryPath, nil, "--config", configPath, "serve", "--transport", "stdio")
	if err != nil {
		t.Fatalf("NewStdioMCPClient() error = %v", err)
	}

	stderrLogs := &lockedBuffer{}
	stderrDone := make(chan struct{})
	if stderr, ok := mcpclient.GetStderr(client); ok {
		go func() {
			_, _ = io.Copy(stderrLogs, stderr)
			close(stderrDone)
		}()
	} else {
		t.Log("subprocess stderr unavailable; diagnostic output will be missing")
		close(stderrDone)
	}

	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("client.Close() error = %v", err)
		}
		<-stderrDone
	})

	return client, stderrLogs
}

func wrapToolForCancellation(t *testing.T, tools []mcpserver.ServerTool, toolName string, started chan struct{}) []mcpserver.ServerTool {
	t.Helper()

	wrapped := append([]mcpserver.ServerTool(nil), tools...)
	var (
		found bool
		once  sync.Once
	)
	for i := range wrapped {
		if wrapped[i].Tool.Name != toolName {
			continue
		}
		found = true
		next := wrapped[i].Handler
		wrapped[i].Handler = func(ctx context.Context, request mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			once.Do(func() {
				close(started)
			})
			<-ctx.Done()
			return next(ctx, request)
		}
	}
	if !found {
		t.Fatalf("tool %q not found", toolName)
	}
	return wrapped
}

func waitForToolInvocation(t *testing.T, started <-chan struct{}) {
	t.Helper()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for wrapped MCP tool invocation")
	}
}

func waitForCallToolOutcome(t *testing.T, results <-chan callToolOutcome) callToolOutcome {
	t.Helper()

	select {
	case outcome := <-results:
		return outcome
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for CallTool result")
		return callToolOutcome{}
	}
}

func toolResultText(result *mcpgo.CallToolResult) string {
	if result == nil {
		return ""
	}
	for _, content := range result.Content {
		if text, ok := content.(mcpgo.TextContent); ok {
			return text.Text
		}
	}
	return ""
}

func shippedToolNames() []string {
	return []string{
		"analyze_impact",
		"check_compliance",
		"check_doc_drift",
		"check_overlap",
		"check_terminology",
		"compare_specs",
		"compile_preview",
		"explain_file",
		"fix_preview",
		"governed_by",
		"review_spec",
		"search_specs",
		"status",
	}
}

func writeMCPWorkspace(t *testing.T) string {
	return writeMCPWorkspaceWithRuntime(t, `
[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0
`)
}

func writeMCPWorkspaceWithRuntime(t *testing.T, runtimeEmbedder string) string {
	t.Helper()

	repoRoot := mcpRepoRoot(t)
	root := t.TempDir()
	copyTreeMCP(t, filepath.Join(repoRoot, "specs"), filepath.Join(root, "specs"))
	copyTreeMCP(t, filepath.Join(repoRoot, "docs"), filepath.Join(root, "docs"))

	configPath := filepath.Join(root, "pituitary.toml")
	mustWriteMCPFile(t, configPath, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"
infer_applies_to = false

`+strings.TrimSpace(runtimeEmbedder)+`

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["guides/*.md", "runbooks/*.md"]
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	return configPath
}

func newOpenAICompatibleEmbeddingServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("request path = %q, want /v1/embeddings", r.URL.Path)
		}

		var request struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		response := map[string]any{"data": []map[string]any{}}
		for i := range request.Input {
			response["data"] = append(response["data"].([]map[string]any), map[string]any{
				"index":     i,
				"embedding": []float64{float64(i + 1), float64(i + 2), float64(i + 3)},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
}

func writeMCPServeWorkspace(t *testing.T, content string) string {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "specs"), 0o755); err != nil {
		t.Fatalf("mkdir specs: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	configPath := filepath.Join(root, "pituitary.toml")
	mustWriteMCPFile(t, configPath, content)
	return configPath
}

func mcpRepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

func copyTreeMCP(t *testing.T, src, dst string) {
	t.Helper()

	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy %s -> %s: %v", src, dst, err)
	}
}

func mustWriteMCPFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func buildPituitaryBinary(t *testing.T) string {
	t.Helper()

	repoRoot := mcpRepoRoot(t)
	binaryPath := filepath.Join(t.TempDir(), "pituitary")

	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=1",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build . error = %v\n%s", err, output)
	}

	return binaryPath
}
