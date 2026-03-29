package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunNewJSONCreatesBundleThatIndexes(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runNew([]string{"--title", "Queue shaping", "--domain", "api", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runNew() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("runNew() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request newRequest `json:"request"`
		Result  struct {
			BundleDir string `json:"bundle_dir"`
			Spec      struct {
				Ref    string `json:"ref"`
				Status string `json:"status"`
				Domain string `json:"domain"`
			} `json:"spec"`
			Files []struct {
				Path string `json:"path"`
			} `json:"files"`
		} `json:"result"`
		Warnings []cliIssue `json:"warnings"`
		Errors   []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal new payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if got, want := payload.Request.Title, "Queue shaping"; got != want {
		t.Fatalf("request.title = %q, want %q", got, want)
	}
	if got, want := payload.Result.BundleDir, "specs/queue-shaping"; got != want {
		t.Fatalf("bundle_dir = %q, want %q", got, want)
	}
	if got, want := payload.Result.Spec.Status, "draft"; got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
	if got, want := payload.Result.Spec.Domain, "api"; got != want {
		t.Fatalf("domain = %q, want %q", got, want)
	}
	if payload.Result.Spec.Ref == "" {
		t.Fatal("spec.ref is empty, want generated id")
	}
	for _, path := range []string{
		filepath.Join(repo, "specs", "queue-shaping", "spec.toml"),
		filepath.Join(repo, "specs", "queue-shaping", "body.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected scaffold file %s: %v", path, err)
		}
	}

	var indexStdout bytes.Buffer
	var indexStderr bytes.Buffer
	exitCode = withWorkingDir(t, repo, func() int {
		return runIndex([]string{"--rebuild"}, &indexStdout, &indexStderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex(--rebuild) exit code = %d, want 0 (stderr: %q)", exitCode, indexStderr.String())
	}
}

func TestRunNewDefaultsDomainToUnknown(t *testing.T) {
	repo := writeSearchWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runNew([]string{"--title", "Queue shaping", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runNew() exit code = %d, want 0", exitCode)
	}

	var payload struct {
		Result struct {
			Spec struct {
				Domain string `json:"domain"`
			} `json:"spec"`
		} `json:"result"`
		Warnings []cliIssue `json:"warnings"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal new payload: %v", err)
	}
	if got, want := payload.Result.Spec.Domain, "unknown"; got != want {
		t.Fatalf("domain = %q, want %q", got, want)
	}
	if len(payload.Warnings) == 0 || !strings.Contains(payload.Warnings[0].Message, `domain defaulted to "unknown"`) {
		t.Fatalf("warnings = %+v, want placeholder domain warning", payload.Warnings)
	}
}
