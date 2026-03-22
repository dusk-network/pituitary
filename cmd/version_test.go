package cmd

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

func TestRunVersionText(t *testing.T) {
	oldVersion := Version
	oldCommit := Commit
	oldBuildDate := BuildDate
	Version = "test-version"
	Commit = "abc123"
	BuildDate = "2026-03-21T18:30:00Z"
	t.Cleanup(func() {
		Version = oldVersion
		Commit = oldCommit
		BuildDate = oldBuildDate
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runVersion(nil, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("runVersion() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runVersion() wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "pituitary version") {
		t.Fatalf("runVersion() output %q does not mention command", out)
	}
	if !strings.Contains(out, "version: test-version") {
		t.Fatalf("runVersion() output %q does not contain build version", out)
	}
	if !strings.Contains(out, "go version: "+runtime.Version()) {
		t.Fatalf("runVersion() output %q does not contain Go version", out)
	}
	if !strings.Contains(out, "commit: abc123") {
		t.Fatalf("runVersion() output %q does not contain commit", out)
	}
	if !strings.Contains(out, "build date: 2026-03-21T18:30:00Z") {
		t.Fatalf("runVersion() output %q does not contain build date", out)
	}
}

func TestRunVersionJSON(t *testing.T) {
	oldVersion := Version
	oldCommit := Commit
	oldBuildDate := BuildDate
	Version = "test-version"
	Commit = ""
	BuildDate = ""
	t.Cleanup(func() {
		Version = oldVersion
		Commit = oldCommit
		BuildDate = oldBuildDate
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runVersion([]string{"--format", "json"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("runVersion() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runVersion() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request struct{} `json:"request"`
		Result  struct {
			Version   string `json:"version"`
			GoVersion string `json:"go_version"`
			Commit    string `json:"commit"`
			BuildDate string `json:"build_date"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal version payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if payload.Result.Version != "test-version" {
		t.Fatalf("result.version = %q, want test-version", payload.Result.Version)
	}
	if payload.Result.GoVersion != runtime.Version() {
		t.Fatalf("result.go_version = %q, want %q", payload.Result.GoVersion, runtime.Version())
	}
	if payload.Result.Commit != "" || payload.Result.BuildDate != "" {
		t.Fatalf("result = %+v, want empty optional build metadata", payload.Result)
	}
}

func TestRunVersionHelpDoesNotAdvertiseConfigResolution(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"version", "--help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("Run(version, --help) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(version, --help) wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "usage: pituitary version [--format FORMAT]") {
		t.Fatalf("Run(version, --help) output %q does not contain version usage", out)
	}
	if strings.Contains(out, "shared config resolution:") {
		t.Fatalf("Run(version, --help) output %q unexpectedly advertises shared config resolution", out)
	}
	if strings.Contains(out, "command-local --config PATH") {
		t.Fatalf("Run(version, --help) output %q unexpectedly advertises command-local config", out)
	}
}

func TestRunVersionRejectsTableFormat(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runVersion([]string{"--format", "table"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("runVersion(--format table) exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runVersion(--format table) wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `pituitary version: format "table" is only supported for search-specs`) {
		t.Fatalf("runVersion(--format table) stderr = %q, want explicit table-format error", stderr.String())
	}
}

func TestRunVersionRejectsMarkdownFormat(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runVersion([]string{"--format", "markdown"}, &stdout, &stderr)
	if exitCode != 2 {
		t.Fatalf("runVersion(--format markdown) exit code = %d, want 2", exitCode)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runVersion(--format markdown) wrote unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), `pituitary version: format "markdown" is only supported for review-spec`) {
		t.Fatalf("runVersion(--format markdown) stderr = %q, want explicit markdown-format error", stderr.String())
	}
}
