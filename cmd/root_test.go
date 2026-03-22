package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunKnownCommandsStayCallable(t *testing.T) {
	for name, description := range commands {
		if name == "help" {
			continue
		}

		t.Run(name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			args := []string{name}
			expectBootstrapStatus := true

			run := func() int {
				return Run(args, &stdout, &stderr)
			}

			if name == "serve" {
				args = []string{name, "--help"}
				expectBootstrapStatus = false
				exitCode := run()
				assertKnownCommandResult(t, name, description, exitCode, stdout.String(), stderr.String(), expectBootstrapStatus)
				return
			}

			if name == "canonicalize" || name == "discover" || name == "index" || name == "status" || name == "version" || name == "preview-sources" || name == "search-specs" || name == "check-overlap" || name == "compare-specs" || name == "analyze-impact" || name == "check-compliance" || name == "check-doc-drift" || name == "review-spec" {
				repoRoot := writeSearchWorkspace(t)
				if name == "discover" || name == "canonicalize" {
					repoRoot = writeDiscoveryWorkspace(t)
				}
				if name == "canonicalize" {
					args = []string{name, "--path", "rfcs/service-sla.md"}
					expectBootstrapStatus = false
					exitCode := withWorkingDir(t, repoRoot, run)
					assertKnownCommandResult(t, name, description, exitCode, stdout.String(), stderr.String(), expectBootstrapStatus)
					return
				}
				if name == "discover" {
					args = []string{name, "--path", "."}
					expectBootstrapStatus = false
					exitCode := withWorkingDir(t, repoRoot, run)
					assertKnownCommandResult(t, name, description, exitCode, stdout.String(), stderr.String(), expectBootstrapStatus)
					return
				}
				if name == "index" {
					args = []string{name, "--rebuild"}
				} else if name == "status" {
					args = []string{name}
				} else if name == "version" {
					args = []string{name}
				} else if name == "preview-sources" {
					args = []string{name}
				} else if name == "search-specs" {
					indexStdout := bytes.Buffer{}
					indexStderr := bytes.Buffer{}
					exitCode := withWorkingDir(t, repoRoot, func() int {
						return runIndex([]string{"--rebuild"}, &indexStdout, &indexStderr)
					})
					if exitCode != 0 {
						t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, indexStderr.String())
					}
					args = []string{name, "--query", "rate limiting"}
				} else {
					indexStdout := bytes.Buffer{}
					indexStderr := bytes.Buffer{}
					if name == "check-compliance" {
						writeComplianceSourceFile(t, repoRoot, "src/api/middleware/ratelimiter.go", `
package middleware

// Apply limits per tenant rather than per API key.
// Enforce a default limit of 200 requests per minute.
// Use a sliding-window limiter.
func buildLimiter() {}
`)
					}
					exitCode := withWorkingDir(t, repoRoot, func() int {
						return runIndex([]string{"--rebuild"}, &indexStdout, &indexStderr)
					})
					if exitCode != 0 {
						t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, indexStderr.String())
					}
					if name == "check-overlap" {
						args = []string{name, "--spec-ref", "SPEC-042"}
					} else if name == "compare-specs" {
						args = []string{name, "--spec-ref", "SPEC-008", "--spec-ref", "SPEC-042"}
					} else if name == "analyze-impact" {
						args = []string{name, "--spec-ref", "SPEC-042"}
					} else if name == "check-compliance" {
						args = []string{name, "--path", "src/api/middleware/ratelimiter.go"}
					} else if name == "review-spec" {
						args = []string{name, "--spec-ref", "SPEC-042"}
					} else {
						args = []string{name, "--scope", "all"}
					}
				}
				expectBootstrapStatus = false
				exitCode := withWorkingDir(t, repoRoot, run)
				assertKnownCommandResult(t, name, description, exitCode, stdout.String(), stderr.String(), expectBootstrapStatus)
				return
			}

			exitCode := run()
			assertKnownCommandResult(t, name, description, exitCode, stdout.String(), stderr.String(), expectBootstrapStatus)
		})
	}
}

func assertKnownCommandResult(t *testing.T, name, description string, exitCode int, stdout, stderr string, expectBootstrapStatus bool) {
	t.Helper()

	if exitCode != 0 {
		t.Fatalf("Run(%q) exit code = %d, want 0", name, exitCode)
	}
	if stderr != "" && name != "index" {
		t.Fatalf("Run(%q) wrote unexpected stderr: %q", name, stderr)
	}
	if name == "index" && stderr != "" && !strings.Contains(stderr, "pituitary index: chunking") {
		t.Fatalf("Run(%q) stderr %q does not contain rebuild progress", name, stderr)
	}

	if !strings.Contains(stdout, description) {
		t.Fatalf("Run(%q) output %q does not contain description %q", name, stdout, description)
	}
	if expectBootstrapStatus && !strings.Contains(stdout, "status: bootstrap only, not implemented yet") {
		t.Fatalf("Run(%q) output %q does not contain bootstrap status", name, stdout)
	}
}

func TestRunHelpIncludesCommandSurface(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run([]string{"help"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("Run(help) exit code = %d, want 0", exitCode)
	}
	if stderr.Len() != 0 {
		t.Fatalf("Run(help) wrote unexpected stderr: %q", stderr.String())
	}

	out := stdout.String()
	for name := range commands {
		if name == "help" {
			continue
		}
		if !strings.Contains(out, name) {
			t.Fatalf("help output %q does not include command %q", out, name)
		}
	}
}
