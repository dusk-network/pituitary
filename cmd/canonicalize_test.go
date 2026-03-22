package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCanonicalizeJSON(t *testing.T) {
	repo := writeDiscoveryWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runCanonicalize([]string{"--path", "rfcs/service-sla.md", "--format", "json"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCanonicalize() exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("runCanonicalize() wrote unexpected stderr: %q", stderr.String())
	}

	var payload struct {
		Request canonicalizeRequest `json:"request"`
		Result  struct {
			BundleDir string `json:"bundle_dir"`
			Spec      struct {
				Ref string `json:"ref"`
			} `json:"spec"`
			Provenance struct {
				SourceRef string `json:"source_ref"`
			} `json:"provenance"`
			Files []struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			} `json:"files"`
		} `json:"result"`
		Errors []cliIssue `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal canonicalize payload: %v", err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("errors = %+v, want none", payload.Errors)
	}
	if payload.Request.Path != "rfcs/service-sla.md" || payload.Request.Write {
		t.Fatalf("request = %+v, want preview canonicalize request", payload.Request)
	}
	if got, want := payload.Result.Spec.Ref, "contract://rfcs/service-sla"; got != want {
		t.Fatalf("spec ref = %q, want %q", got, want)
	}
	if !strings.HasSuffix(filepath.ToSlash(payload.Result.BundleDir), ".pituitary/canonicalized/service-sla") {
		t.Fatalf("bundle dir = %q, want canonicalized path suffix", payload.Result.BundleDir)
	}
	if got, want := payload.Result.Provenance.SourceRef, "file://rfcs/service-sla.md"; got != want {
		t.Fatalf("source ref = %q, want %q", got, want)
	}
	if len(payload.Result.Files) != 2 {
		t.Fatalf("generated file count = %d, want 2", len(payload.Result.Files))
	}
	if strings.Contains(payload.Result.Files[1].Content, "Status: review") {
		t.Fatalf("normalized body %q still contains lifted metadata", payload.Result.Files[1].Content)
	}
}

func TestRunCanonicalizeWriteProducesBundle(t *testing.T) {
	repo := writeDiscoveryWorkspace(t)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := withWorkingDir(t, repo, func() int {
		return runCanonicalize([]string{"--path", "rfcs/service-sla.md", "--write"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runCanonicalize(--write) exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	for _, path := range []string{
		filepath.Join(repo, ".pituitary", "canonicalized", "service-sla", "spec.toml"),
		filepath.Join(repo, ".pituitary", "canonicalized", "service-sla", "body.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("generated file %s missing: %v", path, err)
		}
	}
}

func TestRunCanonicalizeHelpDoesNotAdvertiseConfigResolution(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"canonicalize", "--help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("Run(canonicalize, --help) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(canonicalize, --help) wrote unexpected stderr: %q", stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "shared config resolution:") {
		t.Fatalf("canonicalize help %q unexpectedly advertises config resolution", out)
	}
	if !strings.Contains(out, "usage: pituitary canonicalize --path PATH [--bundle-dir PATH] [--write] [--format FORMAT]") {
		t.Fatalf("canonicalize help %q missing usage line", out)
	}
}
