package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunIndexSupportsGlobalConfigFlag(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))

	configPath := filepath.Join(repo, "pituitary.toml")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, t.TempDir(), func() int {
		return Run([]string{"--config", configPath, "index", "--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("Run() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 && !strings.Contains(stderr.String(), "pituitary index: chunking") {
		t.Fatalf("Run() wrote unexpected stderr: %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); err != nil {
		t.Fatalf("Run() did not create database via global config: %v", err)
	}
}

func TestRunIndexSupportsPerCommandConfigFlag(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))

	configPath := filepath.Join(repo, "pituitary.toml")

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, t.TempDir(), func() int {
		return runIndex([]string{"--config", configPath, "--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 && !strings.Contains(stderr.String(), "pituitary index: chunking") {
		t.Fatalf("runIndex() wrote unexpected stderr: %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); err != nil {
		t.Fatalf("runIndex() did not create database via per-command config: %v", err)
	}
}

func TestRunIndexUsesPituitaryConfigEnv(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))

	t.Setenv(configEnvVar, filepath.Join(repo, "pituitary.toml"))

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, t.TempDir(), func() int {
		return runIndex([]string{"--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 && !strings.Contains(stderr.String(), "pituitary index: chunking") {
		t.Fatalf("runIndex() wrote unexpected stderr: %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); err != nil {
		t.Fatalf("runIndex() did not create database via %s: %v", configEnvVar, err)
	}
}

func TestRunIndexDiscoversLocalConfigFromParentDirectoryAndPrefersIt(t *testing.T) {
	repo := t.TempDir()
	mustWriteIndexFixture(t, repo, `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "github"
kind = "spec_bundle"
path = "specs"
`)
	mustMkdirAllCmd(t, filepath.Join(repo, ".pituitary"))
	mustWriteIndexFixture(t, filepath.Join(repo, ".pituitary"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = "specs"
`)
	mustMkdirAllCmd(t, filepath.Join(repo, "specs"))

	nested := filepath.Join(repo, "pkg", "nested")
	mustMkdirAllCmd(t, nested)

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := withWorkingDir(t, nested, func() int {
		return runIndex([]string{"--rebuild"}, &stdout, &stderr)
	})
	if exitCode != 0 {
		t.Fatalf("runIndex() exit code = %d, want 0 (stderr: %q)", exitCode, stderr.String())
	}
	if stderr.Len() != 0 && !strings.Contains(stderr.String(), "pituitary index: chunking") {
		t.Fatalf("runIndex() wrote unexpected stderr: %q", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".pituitary", "pituitary.db")); err != nil {
		t.Fatalf("runIndex() did not create database via discovered local config: %v", err)
	}
}

func TestParseGlobalCLIOptionsSupportsColorMode(t *testing.T) {
	t.Parallel()

	options, remaining, err := parseGlobalCLIOptions([]string{"--color", "always", "status"})
	if err != nil {
		t.Fatalf("parseGlobalCLIOptions() error = %v, want nil", err)
	}
	if got, want := options.ColorMode, colorModeAlways; got != want {
		t.Fatalf("color mode = %q, want %q", got, want)
	}
	if len(remaining) != 1 || remaining[0] != "status" {
		t.Fatalf("remaining args = %#v, want [status]", remaining)
	}
}

func TestParseGlobalCLIOptionsSupportsLogLevel(t *testing.T) {
	t.Parallel()

	options, remaining, err := parseGlobalCLIOptions([]string{"--log-level", "debug", "status"})
	if err != nil {
		t.Fatalf("parseGlobalCLIOptions() error = %v, want nil", err)
	}
	if got, want := options.LogLevel, "debug"; got != want {
		t.Fatalf("log level = %q, want %q", got, want)
	}
	if len(remaining) != 1 || remaining[0] != "status" {
		t.Fatalf("remaining args = %#v, want [status]", remaining)
	}
}

func TestParseGlobalCLIOptionsUsesLogLevelEnv(t *testing.T) {
	t.Setenv(logLevelEnvVar, "info")

	options, remaining, err := parseGlobalCLIOptions([]string{"status"})
	if err != nil {
		t.Fatalf("parseGlobalCLIOptions() error = %v, want nil", err)
	}
	if got, want := options.LogLevel, "info"; got != want {
		t.Fatalf("log level = %q, want %q", got, want)
	}
	if len(remaining) != 1 || remaining[0] != "status" {
		t.Fatalf("remaining args = %#v, want [status]", remaining)
	}
}

func TestParseGlobalCLIOptionsRejectsInvalidColorMode(t *testing.T) {
	t.Parallel()

	_, _, err := parseGlobalCLIOptions([]string{"--color", "violet", "status"})
	if err == nil || !strings.Contains(err.Error(), "invalid --color value") {
		t.Fatalf("parseGlobalCLIOptions() error = %v, want invalid --color value", err)
	}
}

func TestParseGlobalCLIOptionsRejectsInvalidLogLevel(t *testing.T) {
	t.Parallel()

	_, _, err := parseGlobalCLIOptions([]string{"--log-level", "trace", "status"})
	if err == nil || !strings.Contains(err.Error(), "invalid --log-level value") {
		t.Fatalf("parseGlobalCLIOptions() error = %v, want invalid --log-level value", err)
	}
}

func TestResolveCommandConfigPathPrefersCommandFlagAndExplainsCandidates(t *testing.T) {
	repo := t.TempDir()
	commandConfigPath := filepath.Join(repo, "configs", "pituitary.local.toml")
	globalConfigPath := filepath.Join(repo, "global", "pituitary.toml")
	envConfigPath := filepath.Join(repo, "env", "pituitary.toml")

	t.Setenv(configEnvVar, envConfigPath)

	var (
		resolvedPath string
		resolution   *configResolution
		err          error
	)
	withWorkingDir(t, repo, func() int {
		resolvedPath, resolution, err = resolveCommandConfigPathWithResolution(
			withCLIConfigPath(context.Background(), globalConfigPath),
			commandConfigPath,
		)
		return 0
	})
	if err != nil {
		t.Fatalf("resolveCommandConfigPathWithResolution() error = %v", err)
	}
	if got, want := resolvedPath, commandConfigPath; got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
	if resolution == nil {
		t.Fatal("resolution = nil, want explanation payload")
	}
	if got, want := resolution.SelectedBy, configSourceCommandFlag; got != want {
		t.Fatalf("selected_by = %q, want %q", got, want)
	}
	if len(resolution.Candidates) < 5 {
		t.Fatalf("candidates = %+v, want explicit/env/discovery entries", resolution.Candidates)
	}
	if got, want := resolution.Candidates[0].Status, "selected"; got != want {
		t.Fatalf("command candidate status = %q, want %q", got, want)
	}
	if got, want := resolution.Candidates[1].Status, "shadowed"; got != want {
		t.Fatalf("global candidate status = %q, want %q", got, want)
	}
	if got, want := resolution.Candidates[2].Status, "shadowed"; got != want {
		t.Fatalf("env candidate status = %q, want %q", got, want)
	}
	if got, want := resolution.Candidates[3].Status, "missing"; got != want {
		t.Fatalf("first discovery candidate status = %q, want %q", got, want)
	}
	if !strings.Contains(resolution.Reason, "command-local --config") {
		t.Fatalf("reason = %q, want command precedence detail", resolution.Reason)
	}
}

func TestResolveCommandConfigPathExplainsDiscoveredShadowedConfig(t *testing.T) {
	repo := t.TempDir()
	resolvedRepo, resolveErr := filepath.EvalSymlinks(repo)
	if resolveErr != nil {
		t.Fatalf("filepath.EvalSymlinks(%q) error = %v", repo, resolveErr)
	}
	mustMkdirAllCmd(t, filepath.Join(repo, ".pituitary"))
	mustWriteFileCmd(t, filepath.Join(repo, ".pituitary", "pituitary.toml"), "[workspace]\nroot = \".\"\nindex_path = \".pituitary/pituitary.db\"\n")
	mustWriteFileCmd(t, filepath.Join(repo, "pituitary.toml"), "[workspace]\nroot = \".\"\nindex_path = \".pituitary/pituitary.db\"\n")

	nested := filepath.Join(repo, "pkg", "nested")
	mustMkdirAllCmd(t, nested)

	var (
		resolvedPath string
		resolution   *configResolution
		err          error
	)
	withWorkingDir(t, nested, func() int {
		resolvedPath, resolution, err = resolveCommandConfigPathWithResolution(context.Background(), "")
		return 0
	})
	if err != nil {
		t.Fatalf("resolveCommandConfigPathWithResolution() error = %v", err)
	}
	if got, want := resolvedPath, filepath.Join(resolvedRepo, ".pituitary", "pituitary.toml"); got != want {
		t.Fatalf("resolved path = %q, want %q", got, want)
	}
	if resolution == nil {
		t.Fatal("resolution = nil, want explanation payload")
	}
	if got, want := resolution.SelectedBy, configSourceDiscovery; got != want {
		t.Fatalf("selected_by = %q, want %q", got, want)
	}
	var foundSelected, foundShadowed bool
	for _, candidate := range resolution.Candidates {
		switch {
		case candidate.Path == filepath.Join(resolvedRepo, ".pituitary", "pituitary.toml") && candidate.Status == "selected":
			foundSelected = true
		case candidate.Path == filepath.Join(resolvedRepo, "pituitary.toml") && candidate.Status == "shadowed":
			foundShadowed = true
		}
	}
	if !foundSelected || !foundShadowed {
		t.Fatalf("candidates = %+v, want selected local config and shadowed root config", resolution.Candidates)
	}
	if !strings.Contains(resolution.Reason, filepath.ToSlash(filepath.Join(resolvedRepo, ".pituitary", "pituitary.toml"))) ||
		!strings.Contains(resolution.Reason, filepath.ToSlash(filepath.Join(resolvedRepo, "pituitary.toml"))) {
		t.Fatalf("reason = %q, want selected and shadowed discovered paths", resolution.Reason)
	}
}

func TestConfigResolutionDetectsShadowedMultirepoConfig(t *testing.T) {
	root := t.TempDir()
	childRepo := filepath.Join(root, "child")

	// Parent multirepo config.
	mustWriteFileCmd(t, filepath.Join(root, ".pituitary", "pituitary.toml"), `
[workspace]
root = "`+filepath.ToSlash(childRepo)+`"
repo_id = "child"
index_path = ".pituitary/pituitary.db"

[[workspace.repos]]
id = "shared"
root = "`+filepath.ToSlash(filepath.Join(root, "shared"))+`"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["*.md"]
`)

	// Child repo-local config (no repos).
	mustWriteFileCmd(t, filepath.Join(childRepo, ".pituitary", "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = "docs"
include = ["*.md"]
`)

	mustWriteFileCmd(t, filepath.Join(childRepo, "docs", "readme.md"), "# Hello\n")
	mustWriteFileCmd(t, filepath.Join(root, "shared", "docs", "api.md"), "# API\n")

	_, resolution, err := resolveCLIConfigPathFromWorkingDir(childRepo)
	if err != nil {
		t.Fatalf("resolveCLIConfigPathFromWorkingDir() error = %v", err)
	}
	if resolution.ShadowedMultirepoConfig == "" {
		t.Fatal("ShadowedMultirepoConfig is empty, want path to parent multirepo config")
	}
	want := filepath.Join(root, ".pituitary", "pituitary.toml")
	if got := resolution.ShadowedMultirepoConfig; got != want {
		t.Fatalf("ShadowedMultirepoConfig = %q, want %q", got, want)
	}
}
