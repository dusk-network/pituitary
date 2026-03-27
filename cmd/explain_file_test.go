package cmd

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExplainFileJSON(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runExplainFile([]string{"docs/development/testing-guide.md", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runExplainFile() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runExplainFile() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct {
			Path string `json:"path"`
		} `json:"request"`
		Result struct {
			WorkspacePath string `json:"workspace_path"`
			Summary       struct {
				Status string `json:"status"`
			} `json:"summary"`
			Sources []explainFileSourceJSON `json:"sources"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal explain payload: %v", err)
	}
	if got, want := payload.Request.Path, "docs/development/testing-guide.md"; got != want {
		t.Fatalf("request path = %q, want %q", got, want)
	}
	if got, want := payload.Result.Summary.Status, "excluded"; got != want {
		t.Fatalf("summary status = %q, want %q", got, want)
	}
	if got, want := payload.Result.WorkspacePath, "docs/development/testing-guide.md"; got != want {
		t.Fatalf("workspace path = %q, want %q", got, want)
	}
	if len(payload.Result.Sources) != 2 {
		t.Fatalf("sources = %+v, want 2 explanations", payload.Result.Sources)
	}
	docsSource, ok := findExplainFileSource(payload.Result.Sources, func(src explainFileSourceJSON) bool {
		return src.Name == "docs"
	})
	if !ok {
		t.Fatal("did not find docs source in payload result")
	}
	if docsSource.Reason != "not_matched_by_include" || docsSource.Selected {
		t.Fatalf("docs explanation = %+v, want excluded docs explanation", docsSource)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
}

func TestRunExplainFileContractJSON(t *testing.T) {
	repo := writePathFirstWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runExplainFile([]string{"rfcs/service-sla.md", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runExplainFile() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runExplainFile() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result struct {
			Summary struct {
				Status string `json:"status"`
			} `json:"summary"`
			Sources []explainFileSourceJSON `json:"sources"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal explain contract payload: %v", err)
	}
	if got, want := payload.Result.Summary.Status, "indexed"; got != want {
		t.Fatalf("summary status = %q, want %q", got, want)
	}
	if len(payload.Result.Sources) != 3 {
		t.Fatalf("sources = %+v, want 3 explanations", payload.Result.Sources)
	}
	contract, ok := findExplainFileSource(payload.Result.Sources, func(src explainFileSourceJSON) bool {
		return src.Name == "contracts"
	})
	if !ok {
		t.Fatal("did not find contracts source in payload result")
	}
	if got, want := contract.Name, "contracts"; got != want {
		t.Fatalf("contract source name = %q, want %q", got, want)
	}
	if got, want := contract.Reason, "indexed_markdown_contract"; got != want {
		t.Fatalf("contract reason = %q, want %q", got, want)
	}
	if got, want := contract.ArtifactKind, "spec"; got != want {
		t.Fatalf("contract artifact_kind = %q, want %q", got, want)
	}
	if got, want := contract.InferredSpec.Ref, "contract://rfcs/service-sla"; got != want {
		t.Fatalf("contract inferred ref = %q, want %q", got, want)
	}
	if got, want := contract.InferredSpec.Title, "Service Rate Limiting Contract"; got != want {
		t.Fatalf("contract inferred title = %q, want %q", got, want)
	}
}

func TestRunExplainFileRejectsMissingPath(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runExplainFile([]string{"--format", "json"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("runExplainFile() exit code = %d, want 2", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runExplainFile() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Result any        `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal explain error payload: %v", err)
	}
	if payload.Result != nil {
		t.Fatalf("result = %#v, want nil", payload.Result)
	}
	if len(payload.Errors) != 1 || payload.Errors[0].Code != "validation_error" {
		t.Fatalf("errors = %+v, want one validation_error", payload.Errors)
	}
}

func TestResolveExplainPathResolvesRelativePathFromWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	exitCode := withWorkingDir(t, root, func() int {
		mustWriteFileCmd(t, filepath.Join(root, "docs", "guide.md"), "# guide\n")
		got, err := resolveExplainPath(root, filepath.Join("docs", "guide.md"))
		if err != nil {
			t.Fatalf("resolveExplainPath() error = %v", err)
		}
		want, err := filepath.Abs(filepath.Join(root, "docs", "guide.md"))
		if err != nil {
			t.Fatalf("filepath.Abs() error = %v", err)
		}
		if got != want {
			t.Fatalf("resolveExplainPath() = %q, want %q", got, want)
		}
		return 0
	})
	if exitCode != 0 {
		t.Fatalf("withWorkingDir() exit code = %d, want 0", exitCode)
	}
}

func TestRunExplainFileRejectsOutsideWorkspacePath(t *testing.T) {
	repo := writeSearchWorkspace(t)
	outside := filepath.Join(t.TempDir(), "outside.md")
	mustWriteFileCmd(t, outside, "# outside\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, repo, func() int {
		return runExplainFile([]string{outside, "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 2 {
		t.Fatalf("runExplainFile() exit code = %d, want 2", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runExplainFile() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal explain error payload: %v", err)
	}
	if len(payload.Errors) != 1 || payload.Errors[0].Code != "validation_error" {
		t.Fatalf("errors = %+v, want one validation_error", payload.Errors)
	}
	if got := payload.Errors[0].Message; !strings.Contains(got, "outside workspace root") {
		t.Fatalf("error message = %q, want workspace-root validation", got)
	}
}

type explainFileSourceJSON struct {
	Name         string `json:"name"`
	Reason       string `json:"reason"`
	Selected     bool   `json:"selected"`
	RelativePath string `json:"relative_path"`
	ArtifactKind string `json:"artifact_kind"`
	InferredSpec struct {
		Ref    string `json:"ref"`
		Title  string `json:"title"`
		Status string `json:"status"`
	} `json:"inferred_spec"`
}

func findExplainFileSource(sources []explainFileSourceJSON, match func(explainFileSourceJSON) bool) (explainFileSourceJSON, bool) {
	for _, source := range sources {
		if match(source) {
			return source, true
		}
	}
	return explainFileSourceJSON{}, false
}
