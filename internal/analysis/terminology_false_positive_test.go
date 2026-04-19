package analysis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

// TestCheckTerminologyLiteralFindingsMatchCorpus is a regression guard for
// https://github.com/dusk-network/pituitary/issues/289 — check-terminology
// reporting `provenance: literal`, `confidence: 1` findings for governed
// aliases that do not appear anywhere in the cited artifacts. It constructs a
// fixture corpus with a known exact count of literal alias occurrences, runs
// the audit, and asserts:
//
//  1. The number of reported TerminologyTermMatch entries whose Provenance
//     is "literal" equals the number of literal occurrences in the corpus.
//  2. No literal-provenance match is reported against any artifact whose raw
//     body contains zero occurrences of any governed alias.
//
// #289 reproduces at roughly a 73:1 reported-to-real ratio, so a true
// reproduction of the bug on HEAD will fail assertion (1) by a wide margin
// and/or fail assertion (2) by placing literal-provenance matches on the
// zero-alias artifacts.
func TestCheckTerminologyLiteralFindingsMatchCorpus(t *testing.T) {
	t.Parallel()

	cfg, _ := loadTerminologyFalsePositiveFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckTerminology(cfg, TerminologyAuditRequest{
		Scope: "all",
	})
	if err != nil {
		t.Fatalf("CheckTerminology() error = %v", err)
	}

	// Refs whose raw body contains zero literal alias occurrences. The audit
	// must not produce literal-provenance findings against any of them. Doc
	// refs are computed by docRefForPath as "doc://<path-relative-to-source-root>"
	// with the .md suffix trimmed, so files under the `docs` source at
	// docs/unrelated-a/body.md become doc://unrelated-a/body.
	zeroAliasRefs := map[string]struct{}{
		"SPEC-CLEAN":             {},
		"doc://unrelated-a/body": {},
		"doc://unrelated-b/body": {},
	}

	var literalMatchCount int
	for _, finding := range result.Findings {
		for _, section := range finding.Sections {
			for _, match := range section.Matches {
				if match.Provenance != ProvenanceLiteral {
					continue
				}
				literalMatchCount++
				if _, isClean := zeroAliasRefs[finding.Ref]; isClean {
					t.Errorf("literal-provenance match reported against zero-alias artifact %q: term=%q section=%q excerpt=%q",
						finding.Ref, match.Term, section.Section, section.Excerpt)
				}
			}
		}
	}

	// Corpus below contains exactly this many literal occurrences of
	// governed aliases. See loadTerminologyFalsePositiveFixtureConfig.
	const wantLiteralMatches = 4
	if literalMatchCount != wantLiteralMatches {
		t.Errorf("total literal matches = %d, want %d (fabrication ratio ~%.1fx)",
			literalMatchCount, wantLiteralMatches,
			float64(literalMatchCount)/float64(wantLiteralMatches))
		dumpTerminologyFindings(t, result.Findings)
	}
}

// TestCheckTerminologyRebuildIsMonotonic is a regression guard for the most
// alarming signal in https://github.com/dusk-network/pituitary/issues/289:
// after removing literal alias occurrences from the corpus and rebuilding the
// index, the reported match count went UP (366 → 384). A content-derived
// pipeline cannot produce that behavior; the only explanations are stale
// state, neighbor expansion, or non-literal content bleeding into
// literal-provenance findings.
//
// This test indexes a corpus, audits, removes all governed aliases from one
// spec, rebuilds the index, audits again, and asserts:
//
//  1. The literal-match count strictly decreased.
//  2. The post-rebuild count equals the expected remaining total.
//  3. The cleaned spec produces no findings in the second pass.
func TestCheckTerminologyRebuildIsMonotonic(t *testing.T) {
	t.Parallel()

	cfg, root := loadTerminologyFalsePositiveFixtureConfig(t)

	firstResult := indexAndCheckTerminology(t, cfg)
	firstCount := countLiteralMatches(firstResult.Findings)
	const wantFirst = 4
	if firstCount != wantFirst {
		t.Fatalf("pre-rebuild literal matches = %d, want %d", firstCount, wantFirst)
	}

	// Rewrite specs/focus-selection/body.md to remove every governed alias
	// and every heading that would match one. After this edit the file
	// contains zero literal occurrences of any governed alias.
	cleanedFocusSelection := `
# Selection Loop

## Mechanics

The loop runs after every resume.

## History

Prior revisions are not relevant to the current design.
`
	cleanedPath := filepath.Join(root, "specs", "focus-selection", "body.md")
	if err := os.WriteFile(cleanedPath, []byte(strings.TrimSpace(cleanedFocusSelection)+"\n"), 0o644); err != nil {
		t.Fatalf("overwrite %s: %v", cleanedPath, err)
	}

	secondResult := indexAndCheckTerminology(t, cfg)
	secondCount := countLiteralMatches(secondResult.Findings)

	if secondCount >= firstCount {
		t.Errorf("rebuild non-monotonic: pre=%d post=%d (removing aliases did not reduce reported count)", firstCount, secondCount)
		dumpTerminologyFindings(t, secondResult.Findings)
	}
	// We removed two literal occurrences ("focus selection" appeared twice
	// in the original file, collapsed to one match per section × 2 sections).
	const wantSecond = 2
	if secondCount != wantSecond {
		t.Errorf("post-rebuild literal matches = %d, want %d", secondCount, wantSecond)
		dumpTerminologyFindings(t, secondResult.Findings)
	}

	for _, finding := range secondResult.Findings {
		if finding.Ref == "SPEC-FOCUS-SELECTION" {
			t.Errorf("cleaned artifact SPEC-FOCUS-SELECTION still produces finding after rebuild: %+v", finding)
		}
	}
}

func indexAndCheckTerminology(t *testing.T, cfg *config.Config) *TerminologyAuditResult {
	t.Helper()
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}
	result, err := CheckTerminology(cfg, TerminologyAuditRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckTerminology() error = %v", err)
	}
	return result
}

func countLiteralMatches(findings []TerminologyFinding) int {
	var count int
	for _, finding := range findings {
		for _, section := range finding.Sections {
			for _, match := range section.Matches {
				if match.Provenance == ProvenanceLiteral {
					count++
				}
			}
		}
	}
	return count
}

func dumpTerminologyFindings(t *testing.T, findings []TerminologyFinding) {
	t.Helper()
	var b strings.Builder
	b.WriteString("findings dump:\n")
	for _, finding := range findings {
		for _, section := range finding.Sections {
			for _, match := range section.Matches {
				b.WriteString("  ref=")
				b.WriteString(finding.Ref)
				b.WriteString(" section=")
				b.WriteString(section.Section)
				b.WriteString(" term=")
				b.WriteString(match.Term)
				b.WriteString(" provenance=")
				b.WriteString(match.Provenance)
				b.WriteString(" excerpt=")
				b.WriteString(strings.ReplaceAll(section.Excerpt, "\n", " \u23ce "))
				b.WriteString("\n")
			}
		}
	}
	t.Log(b.String())
}

// loadTerminologyFalsePositiveFixtureConfig builds a deterministic corpus
// with exactly four literal occurrences of governed aliases:
//
//   - specs/legacy-handoff/body.md     → 1 × "session handoff"   (SPEC-LEGACY-HANDOFF)
//   - specs/focus-selection/body.md    → 2 × "focus selection"   (SPEC-FOCUS-SELECTION)
//   - docs/changelog/body.md           → 1 × "backlog dispatch"  (doc://changelog/body)
//
// Plus three artifacts that must contain zero occurrences of any governed
// alias: SPEC-CLEAN, doc://unrelated-a/body, doc://unrelated-b/body.
func loadTerminologyFalsePositiveFixtureConfig(tb testing.TB) (*config.Config, string) {
	tb.Helper()

	root := tb.TempDir()

	mustWriteFile(tb, filepath.Join(root, "specs", "legacy-handoff", "spec.toml"), `
id = "SPEC-LEGACY-HANDOFF"
title = "Legacy Session Handoff Notes"
status = "review"
domain = "session"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "legacy-handoff", "body.md"), `
# Legacy Session Handoff Notes

## Historical Context

Operators used to rely on session handoff records to stitch resumption state.
`)

	mustWriteFile(tb, filepath.Join(root, "specs", "focus-selection", "spec.toml"), `
id = "SPEC-FOCUS-SELECTION"
title = "Focus Selection Loop"
status = "review"
domain = "session"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "focus-selection", "body.md"), `
# Focus Selection Loop

## Mechanics

The focus selection loop runs after every resume.

## History

Prior revisions called this focus selection; the preferred term is next step.
`)

	mustWriteFile(tb, filepath.Join(root, "specs", "clean", "spec.toml"), `
id = "SPEC-CLEAN"
title = "Clean Spec"
status = "accepted"
domain = "session"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "clean", "body.md"), `
# Clean Spec

## Overview

This spec deliberately uses only the preferred governed terms throughout.
It describes next step selection, handoff, extension dispatch, dispatch state,
and locality — without any historical alias phrasing.
`)

	mustWriteFile(tb, filepath.Join(root, "docs", "changelog", "body.md"), `
# Changelog

- 2026-03-01 — introduced backlog dispatch semantics; migrated callers.
- 2026-03-15 — tightened dispatch state invariants.
`)

	mustWriteFile(tb, filepath.Join(root, "docs", "unrelated-a", "body.md"), `
# Unrelated Doc A

This document describes handoff, next step, locality, extension dispatch,
and dispatch state exclusively using the preferred governed terms. It must
not trigger any terminology findings.
`)

	mustWriteFile(tb, filepath.Join(root, "docs", "unrelated-b", "body.md"), `
# Unrelated Doc B

An overview of runtime concerns that has no vocabulary overlap with the
governed aliases at all. Nothing in this file should match.
`)

	mustWriteFile(tb, filepath.Join(root, "pituitary.toml"), `
[workspace]
root = "."
index_path = ".pituitary/pituitary.db"

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[terminology.policies]]
preferred = "handoff"
historical_aliases = ["session handoff", "context handoff"]

[[terminology.policies]]
preferred = "extension dispatch"
historical_aliases = ["backlog dispatch", "work selection"]

[[terminology.policies]]
preferred = "dispatch state"
historical_aliases = ["focus state"]

[[terminology.policies]]
preferred = "next step"
historical_aliases = ["focus selection"]

[[terminology.policies]]
preferred = "locality"
historical_aliases = ["repo identity"]

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
include = ["**/body.md"]
`)

	cfg, err := config.Load(filepath.Join(root, "pituitary.toml"))
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg, root
}
