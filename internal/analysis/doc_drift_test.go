package analysis

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/model"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestCheckDocDriftFlagsGuideButNotRunbook(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckDocDrift(cfg, DocDriftRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}
	if result.Scope.Mode != "all" {
		t.Fatalf("scope = %+v, want mode all", result.Scope)
	}

	var foundGuide, foundRunbook bool
	for _, item := range result.DriftItems {
		switch item.DocRef {
		case "doc://guides/api-rate-limits":
			foundGuide = true
			if len(item.Findings) == 0 {
				t.Fatalf("guide drift item = %+v, want findings", item)
			}
			top := item.Findings[0]
			if top.Rationale == "" || top.Evidence == nil || top.Evidence.SpecSection == "" || top.Evidence.DocSection == "" || top.Evidence.SpecSourceRef == "" || top.Evidence.DocSourceRef == "" || top.Evidence.LinkReason == "" || top.Confidence == nil || top.Confidence.Level == "" {
				t.Fatalf("top finding = %+v, want rationale, source-linked evidence, and confidence", top)
			}
		case "doc://runbooks/rate-limit-rollout":
			foundRunbook = true
		}
	}
	if !foundGuide {
		t.Fatalf("drift_items = %+v, want guide drift", result.DriftItems)
	}
	if foundRunbook {
		t.Fatalf("drift_items = %+v, did not expect aligned runbook", result.DriftItems)
	}
	if result.Remediation == nil || len(result.Remediation.Items) != 1 {
		t.Fatalf("remediation = %+v, want one remediation item", result.Remediation)
	}
	if result.Remediation.Items[0].DocRef != "doc://guides/api-rate-limits" {
		t.Fatalf("remediation item = %+v, want guide remediation", result.Remediation.Items[0])
	}
	if len(result.Remediation.Items[0].Suggestions) < 3 {
		t.Fatalf("remediation suggestions = %+v, want multiple actionable suggestions", result.Remediation.Items[0].Suggestions)
	}
	top := result.Remediation.Items[0].Suggestions[0]
	if top.SpecRef == "" || top.Classification == "" || top.Evidence.SpecSection == "" || top.Evidence.SpecSourceRef == "" || top.LinkReason == "" || top.TargetSourceRef == "" || top.TargetSection == "" || len(top.SuggestedBullets) == 0 || top.SuggestedEdit.Action == "" {
		t.Fatalf("top remediation suggestion = %+v, want classified evidence chain, target, bullets, and suggested edit", top)
	}

	var foundGuideAssessment, foundRunbookAssessment bool
	for _, assessment := range result.Assessments {
		switch assessment.DocRef {
		case "doc://guides/api-rate-limits":
			foundGuideAssessment = true
			if assessment.Status != "drift" {
				t.Fatalf("guide assessment = %+v, want drift status", assessment)
			}
			if assessment.Rationale == "" || assessment.Evidence == nil || assessment.Evidence.LinkReason == "" || assessment.Confidence == nil || assessment.Confidence.Level == "" {
				t.Fatalf("guide assessment = %+v, want rationale, linked evidence, and confidence", assessment)
			}
		case "doc://runbooks/rate-limit-rollout":
			foundRunbookAssessment = true
			if assessment.Status != "aligned" {
				t.Fatalf("runbook assessment = %+v, want aligned status", assessment)
			}
			if assessment.Rationale == "" || assessment.Evidence == nil || assessment.Evidence.LinkReason == "" || assessment.Confidence == nil || assessment.Confidence.Level == "" {
				t.Fatalf("runbook assessment = %+v, want rationale, linked evidence, and confidence", assessment)
			}
		}
	}
	if !foundGuideAssessment {
		t.Fatalf("assessments = %+v, want guide assessment", result.Assessments)
	}
	if !foundRunbookAssessment {
		t.Fatalf("assessments = %+v, want aligned runbook assessment", result.Assessments)
	}
}

func TestCheckDocDriftSupportsTargetedDocRefs(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckDocDrift(cfg, DocDriftRequest{
		DocRefs: []string{"doc://guides/api-rate-limits", "doc://runbooks/rate-limit-rollout"},
	})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}
	if result.Scope.Mode != "doc_refs" {
		t.Fatalf("scope = %+v, want mode doc_refs", result.Scope)
	}
	if len(result.Scope.DocRefs) != 2 {
		t.Fatalf("scope.doc_refs = %v, want 2 refs", result.Scope.DocRefs)
	}
	if len(result.DriftItems) != 1 || result.DriftItems[0].DocRef != "doc://guides/api-rate-limits" {
		t.Fatalf("drift_items = %+v, want only guide drift", result.DriftItems)
	}
	var foundAligned bool
	for _, assessment := range result.Assessments {
		if assessment.DocRef == "doc://runbooks/rate-limit-rollout" {
			foundAligned = true
			if got, want := assessment.Status, "aligned"; got != want {
				t.Fatalf("assessment.status = %q, want %q", got, want)
			}
		}
	}
	if !foundAligned {
		t.Fatalf("assessments = %+v, want aligned runbook assessment", result.Assessments)
	}
}

func TestCheckDocDriftDiffTextShortlistsChangedFilesAndDocs(t *testing.T) {
	t.Parallel()

	cfg := loadFixtureConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckDocDrift(cfg, DocDriftRequest{
		DiffText: `
diff --git a/src/api/middleware/ratelimiter.go b/src/api/middleware/ratelimiter.go
--- a/src/api/middleware/ratelimiter.go
+++ b/src/api/middleware/ratelimiter.go
@@ -1,2 +1,2 @@
-const defaultLimit = 100
+const defaultLimit = 200
`,
	})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}
	if got, want := result.Scope.Mode, "diff"; got != want {
		t.Fatalf("scope.mode = %q, want %q", got, want)
	}
	if len(result.ChangedFiles) != 1 || result.ChangedFiles[0].Path != "src/api/middleware/ratelimiter.go" {
		t.Fatalf("changed_files = %+v, want ratelimiter.go", result.ChangedFiles)
	}
	if len(result.ImplicatedSpecs) == 0 || result.ImplicatedSpecs[0].Ref != "SPEC-042" {
		t.Fatalf("implicated_specs = %+v, want SPEC-042 first", result.ImplicatedSpecs)
	}
	if len(result.ImplicatedSpecs[0].Reasons) == 0 || !strings.Contains(result.ImplicatedSpecs[0].Reasons[0], "applies_to matched changed path") {
		t.Fatalf("implicated spec reasons = %+v, want applies_to basis", result.ImplicatedSpecs[0].Reasons)
	}
	var foundGuide bool
	for _, item := range result.ImplicatedDocs {
		if item.DocRef == "doc://guides/api-rate-limits" {
			foundGuide = true
			if len(item.Reasons) == 0 || !strings.Contains(item.Reasons[0], "SPEC-042") {
				t.Fatalf("implicated doc reasons = %+v, want SPEC-042 linkage", item.Reasons)
			}
		}
	}
	if !foundGuide {
		t.Fatalf("implicated_docs = %+v, want guide doc", result.ImplicatedDocs)
	}
	if len(result.DriftItems) == 0 || result.DriftItems[0].DocRef != "doc://guides/api-rate-limits" {
		t.Fatalf("drift_items = %+v, want guide drift", result.DriftItems)
	}
}

func TestCheckDocDriftFlagsStaleNamedArtifacts(t *testing.T) {
	t.Parallel()

	cfg := writeArtifactContractWorkspace(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckDocDrift(cfg, DocDriftRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}

	var stale, aligned, native *DriftItem
	for i := range result.DriftItems {
		item := &result.DriftItems[i]
		switch item.DocRef {
		case "doc://guides/runtime-cache":
			stale = item
		case "doc://guides/runtime-derived":
			aligned = item
		case "doc://guides/runtime-native":
			native = item
		}
	}
	if stale == nil {
		t.Fatalf("drift_items = %+v, want stale runtime-cache doc", result.DriftItems)
	}
	if aligned != nil {
		t.Fatalf("drift_items = %+v, did not expect aligned derived doc", result.DriftItems)
	}
	if native != nil {
		t.Fatalf("drift_items = %+v, did not expect canonical state.db doc", result.DriftItems)
	}

	var foundWorkQueue, foundCompiledState bool
	for _, finding := range stale.Findings {
		switch {
		case finding.Artifact == "work_queue.json" && finding.Code == "artifact_runtime_input_mismatch":
			foundWorkQueue = true
		case finding.Artifact == "compiled_state.json" && finding.Code == "artifact_contract_mismatch":
			foundCompiledState = true
		}
	}
	if !foundWorkQueue || !foundCompiledState {
		t.Fatalf("findings = %+v, want work_queue.json runtime-input drift and compiled_state.json contract drift", stale.Findings)
	}

	if result.Remediation == nil || len(result.Remediation.Items) != 1 || result.Remediation.Items[0].DocRef != "doc://guides/runtime-cache" {
		t.Fatalf("remediation = %+v, want runtime-cache remediation only", result.Remediation)
	}
}

func TestCheckDocDriftIncludesRepoIdentity(t *testing.T) {
	t.Parallel()

	cfg := loadMultiRepoAnalysisConfig(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckDocDrift(cfg, DocDriftRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}

	var foundSharedDrift, foundSharedAssessment, foundSharedRemediation bool
	for _, item := range result.DriftItems {
		if item.DocRef == "doc://shared/guides/api-rate-limits" {
			foundSharedDrift = true
			if got, want := item.Repo, "shared"; got != want {
				t.Fatalf("shared drift repo = %q, want %q", got, want)
			}
		}
	}
	for _, assessment := range result.Assessments {
		if assessment.DocRef == "doc://shared/guides/api-rate-limits" {
			foundSharedAssessment = true
			if got, want := assessment.Repo, "shared"; got != want {
				t.Fatalf("shared assessment repo = %q, want %q", got, want)
			}
		}
	}
	if result.Remediation != nil {
		for _, item := range result.Remediation.Items {
			if item.DocRef == "doc://shared/guides/api-rate-limits" {
				foundSharedRemediation = true
				if got, want := item.Repo, "shared"; got != want {
					t.Fatalf("shared remediation repo = %q, want %q", got, want)
				}
			}
		}
	}
	if !foundSharedDrift || !foundSharedAssessment || !foundSharedRemediation {
		t.Fatalf("doc drift result = %+v, want shared repo drift, assessment, and remediation", result)
	}
}

func TestCheckDocDriftUsesAnalysisProviderWhenEnabled(t *testing.T) {
	t.Parallel()

	cfg := writeArtifactContractWorkspace(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	configureOpenAIAnalysisProvider(t, cfg, func(t *testing.T, request openAICompatibleChatRequest) string {
		t.Helper()
		if got, want := request.MaxTokens, 2048; got != want {
			t.Fatalf("request.max_tokens = %d, want %d", got, want)
		}
		var prompt docDriftAnalysisPrompt
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &prompt); err != nil {
			t.Fatalf("unmarshal prompt: %v", err)
		}
		if got, want := prompt.Command, "check-doc-drift"; got != want {
			t.Fatalf("command = %q, want %q", got, want)
		}
		if got, want := prompt.Doc.Ref, "doc://guides/runtime-cache"; got != want {
			t.Fatalf("doc.ref = %q, want %q", got, want)
		}

		return `{
			"findings": [
				{
					"spec_ref": "SPEC-200",
					"artifact": "work_queue.json",
					"code": "artifact_runtime_input_mismatch",
					"message": "runtime-cache guide still presents work_queue.json as the canonical startup input",
					"expected": "not a canonical runtime input",
					"observed": "documented as the canonical startup input"
				}
			],
			"suggestions": [
				{
					"spec_ref": "SPEC-200",
					"code": "artifact_runtime_input_mismatch",
					"summary": "Rewrite the runtime-cache guide so work_queue.json is clearly described as a derived cache, not a required runtime input.",
					"evidence": {
						"expected": "not a canonical runtime input",
						"observed": "documented as the canonical startup input"
					},
					"suggested_edit": {
						"action": "replace_statement",
						"note": "Point readers at state.db as the canonical store and downgrade work_queue.json to optional cache status."
					}
				}
			]
		}`
	})

	result, err := CheckDocDrift(cfg, DocDriftRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}

	var stale *DriftItem
	for i := range result.DriftItems {
		if result.DriftItems[i].DocRef == "doc://guides/runtime-cache" {
			stale = &result.DriftItems[i]
			break
		}
	}
	if stale == nil {
		t.Fatalf("drift_items = %+v, want runtime-cache drift", result.DriftItems)
	}

	var refined bool
	for _, finding := range stale.Findings {
		if finding.Artifact == "work_queue.json" && finding.Code == "artifact_runtime_input_mismatch" && finding.Message == "runtime-cache guide still presents work_queue.json as the canonical startup input" {
			refined = true
			break
		}
	}
	if !refined {
		t.Fatalf("findings = %+v, want provider-refined work_queue.json message", stale.Findings)
	}

	if result.Remediation == nil || len(result.Remediation.Items) != 1 {
		t.Fatalf("remediation = %+v, want one remediation item", result.Remediation)
	}
	suggestion := result.Remediation.Items[0].Suggestions[0]
	if got, want := suggestion.Summary, "Rewrite the runtime-cache guide so work_queue.json is clearly described as a derived cache, not a required runtime input."; got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if got, want := suggestion.SuggestedEdit.Note, "Point readers at state.db as the canonical store and downgrade work_queue.json to optional cache status."; got != want {
		t.Fatalf("suggested_edit.note = %q, want %q", got, want)
	}
	if result.Runtime == nil || result.Runtime.Analysis == nil {
		t.Fatalf("runtime = %+v, want analysis provenance", result.Runtime)
	}
	if !result.Runtime.Analysis.Used {
		t.Fatalf("runtime.analysis.used = false, want true")
	}
	if got, want := result.Runtime.Analysis.Model, "pituitary-analysis"; got != want {
		t.Fatalf("runtime.analysis.model = %q, want %q", got, want)
	}
}

func TestCheckDocDriftSurfacesPossibleDriftForConceptualNearMatch(t *testing.T) {
	t.Parallel()

	cfg := writePossibleDocDriftWorkspace(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckDocDrift(cfg, DocDriftRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}
	if len(result.DriftItems) != 0 {
		t.Fatalf("drift_items = %+v, want no deterministic drift items", result.DriftItems)
	}
	if len(result.Assessments) != 1 {
		t.Fatalf("assessments = %+v, want one assessment", result.Assessments)
	}
	assessment := result.Assessments[0]
	if got, want := assessment.Status, "possible_drift"; got != want {
		t.Fatalf("assessment.status = %q, want %q", got, want)
	}
	if assessment.Confidence == nil || assessment.Confidence.Level != "low" {
		t.Fatalf("assessment.confidence = %+v, want low confidence", assessment.Confidence)
	}
	if assessment.Evidence == nil || assessment.Evidence.SpecSection == "" || assessment.Evidence.DocSection == "" {
		t.Fatalf("assessment = %+v, want evidence sections", assessment)
	}
}

func TestCheckDocDriftKeepsPossibleDriftInMixedBatches(t *testing.T) {
	t.Parallel()

	cfg := writeMixedDocDriftWorkspace(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckDocDrift(cfg, DocDriftRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}

	var foundDrift, foundPossible bool
	for _, assessment := range result.Assessments {
		switch assessment.DocRef {
		case "doc://guides/api-rate-limits":
			if got, want := assessment.Status, "drift"; got != want {
				t.Fatalf("api-rate-limits assessment.status = %q, want %q", got, want)
			}
			foundDrift = true
		case "doc://guides/kernel-migration":
			if got, want := assessment.Status, "possible_drift"; got != want {
				t.Fatalf("migration assessment.status = %q, want %q", got, want)
			}
			foundPossible = true
		}
	}
	if !foundDrift {
		t.Fatalf("assessments = %+v, want deterministic drift doc", result.Assessments)
	}
	if !foundPossible {
		t.Fatalf("assessments = %+v, want possible_drift doc in mixed batch", result.Assessments)
	}
}

func TestCheckDocDriftToleratesHistoricalDocs(t *testing.T) {
	t.Parallel()

	cfg := writeRoleAwareDocDriftWorkspace(t, config.SourceRoleCanonical, `
# Rate Limit Contract

Use a sliding-window limiter with a default limit of 200 requests per minute.
Apply limits per tenant.
`, config.SourceRoleHistorical, `
# 2024 Rollout Notes

Use a fixed-window limiter with a default limit of 100 requests per minute.
Apply limits per API key.
`)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckDocDrift(cfg, DocDriftRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}
	if len(result.DriftItems) != 0 {
		t.Fatalf("drift_items = %+v, want historical doc to be tolerated", result.DriftItems)
	}
	if len(result.Assessments) != 1 {
		t.Fatalf("assessments = %+v, want one historical assessment", result.Assessments)
	}
	assessment := result.Assessments[0]
	if got, want := assessment.Status, "aligned"; got != want {
		t.Fatalf("assessment.status = %q, want %q", got, want)
	}
	if assessment.Rationale == "" || assessment.Confidence == nil {
		t.Fatalf("assessment = %+v, want rationale and confidence", assessment)
	}
	if want := "historical"; !containsSubstringFold(assessment.Rationale, want) {
		t.Fatalf("assessment.rationale = %q, want substring %q", assessment.Rationale, want)
	}
}

func TestCheckDocDriftClassifiesRoleMismatchAgainstRuntimeAuthority(t *testing.T) {
	t.Parallel()

	cfg := writeRoleAwareDocDriftWorkspace(t, config.SourceRoleRuntimeAuth, `
# Runtime Rate Limit Contract

Use a sliding-window limiter with a default limit of 200 requests per minute.
Apply limits per tenant.
`, config.SourceRoleCurrentState, `
# Public API Runtime Guide

Use a fixed-window limiter with a default limit of 100 requests per minute.
Apply limits per API key.
`)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckDocDrift(cfg, DocDriftRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}
	if len(result.DriftItems) != 1 {
		t.Fatalf("drift_items = %+v, want one role mismatch doc", result.DriftItems)
	}
	finding := result.DriftItems[0].Findings[0]
	if got, want := finding.Classification, driftClassificationRole; got != want {
		t.Fatalf("finding.classification = %q, want %q", got, want)
	}
	if got, want := finding.DocRole, config.SourceRoleCurrentState; got != want {
		t.Fatalf("finding.doc_role = %q, want %q", got, want)
	}
	if got, want := finding.SpecRole, config.SourceRoleRuntimeAuth; got != want {
		t.Fatalf("finding.spec_role = %q, want %q", got, want)
	}
}

func TestCheckDocDriftIgnoresPlanningSpecsWhenAuthoritativeSpecExists(t *testing.T) {
	t.Parallel()

	cfg := writePlanningConflictDocDriftWorkspace(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	result, err := CheckDocDrift(cfg, DocDriftRequest{Scope: "all"})
	if err != nil {
		t.Fatalf("CheckDocDrift() error = %v", err)
	}
	if len(result.DriftItems) != 0 {
		t.Fatalf("drift_items = %+v, want planning spec excluded from authoritative drift checks", result.DriftItems)
	}
	if len(result.Assessments) != 1 {
		t.Fatalf("assessments = %+v, want one aligned assessment", result.Assessments)
	}
	assessment := result.Assessments[0]
	if assessment.Status != "aligned" && assessment.Status != "possible_drift" {
		t.Fatalf("assessment.status = %q, want aligned or possible_drift", assessment.Status)
	}
	for _, specRef := range assessment.SpecRefs {
		if specRef == "SPEC-PLAN" {
			t.Fatalf("assessment.spec_refs = %+v, did not expect planning spec", assessment.SpecRefs)
		}
	}
	if len(assessment.SpecRefs) > 0 && assessment.SpecRefs[0] != "SPEC-CANON" {
		t.Fatalf("assessment.spec_refs = %+v, want canonical spec when a spec ref is surfaced", assessment.SpecRefs)
	}
}

func TestClassifyArtifactConstraintScopesRuntimeInputToLocalArtifact(t *testing.T) {
	t.Parallel()

	line := "Prefer `state.db` for canonical runtime state, and the kernel must not read `work_queue.json` as canonical runtime input."

	if kind, _, ok := classifyArtifactConstraint(line, "state.db"); ok || kind != "" {
		t.Fatalf("classifyArtifactConstraint(state.db) = %q, %t, want no constraint", kind, ok)
	}
	if kind, expected, ok := classifyArtifactConstraint(line, "work_queue.json"); !ok || kind != "runtime_input" || expected != "not a canonical runtime input" {
		t.Fatalf("classifyArtifactConstraint(work_queue.json) = kind=%q expected=%q ok=%t, want runtime_input/not a canonical runtime input/true", kind, expected, ok)
	}
}

func writeArtifactContractWorkspace(tb testing.TB) *config.Config {
	tb.Helper()

	root := tb.TempDir()
	indexPath := filepath.Join(root, ".pituitary", "pituitary.db")
	configPath := filepath.Join(root, "pituitary.toml")

	mustWriteFile(tb, filepath.Join(root, "specs", "runtime-contract", "spec.toml"), `
id = "SPEC-200"
title = "Runtime Contract"
status = "accepted"
domain = "runtime"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "runtime-contract", "body.md"), "# Runtime Contract\n\n"+
		"- Legacy derived files such as `handoff.md`, `compiled_state.json`, and `work_queue.json` are not part of the accepted runtime contract.\n"+
		"- The kernel must not read `work_queue.json` as canonical runtime input.\n"+
		"- `compiled_state.json` is not a required artifact in the accepted runtime contract.\n")

	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "runtime-cache.md"), "# Runtime Cache Guide\n\n"+
		"`ccd start` writes `work_queue.json` for the active clone and reads it on the next startup.\n\n"+
		"The clone-local runtime layout also keeps `compiled_state.json` alongside that cache.\n")

	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "runtime-derived.md"), "# Runtime Derived Exports\n\n"+
		"- `handoff.md` remains an optional derived export rendered from canonical state.\n")

	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "runtime-native.md"), "# Runtime Native State\n\n"+
		"- `state.db` is the canonical clone-local runtime store.\n")

	mustWriteFile(tb, configPath, `
[workspace]
root = "`+filepath.ToSlash(root)+`"
index_path = "`+filepath.ToSlash(indexPath)+`"

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
include = ["guides/*.md"]
`)

	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}

func writeRoleAwareDocDriftWorkspace(tb testing.TB, specRole, specBody, docRole, docBody string) *config.Config {
	tb.Helper()

	root := tb.TempDir()
	indexPath := filepath.Join(root, ".pituitary", "pituitary.db")
	configPath := filepath.Join(root, "pituitary.toml")

	mustWriteFile(tb, filepath.Join(root, "specs", "rate-limit", "spec.toml"), `
id = "SPEC-ROLE"
title = "Role Aware Rate Limit Contract"
status = "accepted"
domain = "api"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "rate-limit", "body.md"), specBody)
	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "rate-limit.md"), docBody)

	mustWriteFile(tb, configPath, fmt.Sprintf(`
[workspace]
root = %q
index_path = %q

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
role = %q
path = %q

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
role = %q
path = %q
`, root, indexPath, specRole, filepath.Join(root, "specs"), docRole, filepath.Join(root, "docs")))

	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}

func writePlanningConflictDocDriftWorkspace(tb testing.TB) *config.Config {
	tb.Helper()

	root := tb.TempDir()
	indexPath := filepath.Join(root, ".pituitary", "pituitary.db")
	configPath := filepath.Join(root, "pituitary.toml")

	mustWriteFile(tb, filepath.Join(root, "specs", "canonical", "spec.toml"), `
id = "SPEC-CANON"
title = "Canonical Rate Limit Contract"
status = "accepted"
domain = "api"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "canonical", "body.md"), `
# Canonical Rate Limit Contract

Use a sliding-window limiter with a default limit of 200 requests per minute.
Apply limits per tenant.
`)

	mustWriteFile(tb, filepath.Join(root, "plans", "future-rollout", "spec.toml"), `
id = "SPEC-PLAN"
title = "Future Rate Limit Rollout"
status = "accepted"
domain = "api"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(root, "plans", "future-rollout", "body.md"), `
# Future Rate Limit Rollout

Use a fixed-window limiter with a default limit of 100 requests per minute.
Apply limits per API key.
`)

	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "rate-limit.md"), `
# Public API Runtime Guide

Use a sliding-window limiter with a default limit of 200 requests per minute.
Apply limits per tenant.
`)

	mustWriteFile(tb, configPath, fmt.Sprintf(`
[workspace]
root = %q
index_path = %q

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
role = "canonical"
path = %q

[[sources]]
name = "plans"
adapter = "filesystem"
kind = "spec_bundle"
role = "planning"
path = %q

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
role = "current_state"
path = %q
`, root, indexPath, filepath.Join(root, "specs"), filepath.Join(root, "plans"), filepath.Join(root, "docs")))

	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}

func writePossibleDocDriftWorkspace(tb testing.TB) *config.Config {
	tb.Helper()

	root := tb.TempDir()
	indexPath := filepath.Join(root, ".pituitary", "pituitary.db")
	configPath := filepath.Join(root, "pituitary.toml")

	mustWriteFile(tb, filepath.Join(root, "specs", "kernel-locality", "spec.toml"), `
id = "SPEC-LOCALITY"
title = "Kernel Locality Contract"
status = "accepted"
domain = "kernel"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "kernel-locality", "body.md"), `
# Kernel Locality Contract

## Core Model

The kernel keeps continuity in clone-local state and treats locality as the primary runtime boundary.

## Operator Guidance

Use locality and continuity language in operator guidance.
`)

	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "kernel-migration.md"), `
# Kernel Migration Guide

## Working Notes

The kernel keeps continuity in local state during migration.
Operators should map old repository language to the new locality model while updating guides.
`)

	mustWriteFile(tb, configPath, fmt.Sprintf(`
[workspace]
root = %q
index_path = %q

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = %q

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = %q
`, root, indexPath, filepath.Join(root, "specs"), filepath.Join(root, "docs")))

	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}

func writeMixedDocDriftWorkspace(tb testing.TB) *config.Config {
	tb.Helper()

	root := tb.TempDir()
	indexPath := filepath.Join(root, ".pituitary", "pituitary.db")
	configPath := filepath.Join(root, "pituitary.toml")

	mustWriteFile(tb, filepath.Join(root, "specs", "kernel-locality", "spec.toml"), `
id = "SPEC-LOCALITY"
title = "Kernel Locality Contract"
status = "accepted"
domain = "kernel"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "kernel-locality", "body.md"), `
# Kernel Locality Contract

## Core Model

The kernel keeps continuity in clone-local state and treats locality as the primary runtime boundary.

## Operator Guidance

Use locality and continuity language in operator guidance.
`)

	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "kernel-migration.md"), `
# Kernel Migration Guide

## Working Notes

The kernel keeps continuity in local state during migration.
Operators should map old repository language to the new locality model while updating guides.
`)

	mustWriteFile(tb, filepath.Join(root, "specs", "rate-limit", "spec.toml"), `
id = "SPEC-042"
title = "Per-Tenant Rate Limiting"
status = "accepted"
domain = "api"
body = "body.md"
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "rate-limit", "body.md"), `
# Per-Tenant Rate Limiting

## Enforcement

Use a sliding-window limiter with a default limit of 200 requests per minute.

## Subject

Apply limits per tenant rather than per API key.
`)

	mustWriteFile(tb, filepath.Join(root, "docs", "guides", "api-rate-limits.md"), `
# Public API Rate Limits

## Current Policy

Apply limits per API key using a fixed-window limiter with a default limit of 100 requests per minute.
`)

	mustWriteFile(tb, configPath, fmt.Sprintf(`
[workspace]
root = %q
index_path = %q

[runtime.embedder]
provider = "fixture"
model = "fixture-8d"
timeout_ms = 1000
max_retries = 0

[[sources]]
name = "specs"
adapter = "filesystem"
kind = "spec_bundle"
path = %q

[[sources]]
name = "docs"
adapter = "filesystem"
kind = "markdown_docs"
path = %q
`, root, indexPath, filepath.Join(root, "specs"), filepath.Join(root, "docs")))

	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}

func containsSubstringFold(value, needle string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(needle))
}

func TestRelevantAcceptedSpecsRespectsThresholdAndLimit(t *testing.T) {
	t.Parallel()

	// Build a doc with a single section containing a known embedding.
	docEmbed := []float64{1, 0, 0, 0, 0, 0, 0, 0}
	doc := docDocument{
		Record: model.DocRecord{Ref: "doc://test"},
		Sections: []embeddedSection{
			{Heading: "Test", Content: "test content", Embedding: docEmbed},
		},
	}

	// Build specs with varying similarity to the doc.
	// Cosine similarity of normalized vectors: identical=1.0, orthogonal=0.0.
	makeSpec := func(ref string, embed []float64) specDocument {
		return specDocument{
			Record: model.SpecRecord{Ref: ref, Status: model.StatusAccepted},
			Sections: []embeddedSection{
				{Heading: "Spec", Content: "spec content", Embedding: embed},
			},
		}
	}

	specs := map[string]specDocument{
		"high-1": makeSpec("high-1", []float64{1, 0, 0, 0, 0, 0, 0, 0}),     // similarity ≈ 1.0
		"high-2": makeSpec("high-2", []float64{0.9, 0.1, 0, 0, 0, 0, 0, 0}), // high
		"high-3": makeSpec("high-3", []float64{0.8, 0.2, 0, 0, 0, 0, 0, 0}), // high
		"high-4": makeSpec("high-4", []float64{0.7, 0.3, 0, 0, 0, 0, 0, 0}), // high
		"high-5": makeSpec("high-5", []float64{0.6, 0.4, 0, 0, 0, 0, 0, 0}), // medium-high
		"high-6": makeSpec("high-6", []float64{0.5, 0.5, 0, 0, 0, 0, 0, 0}), // medium
		"low":    makeSpec("low", []float64{0, 0, 0, 0, 0, 0, 0, 1}),        // orthogonal → 0.0
		"draft":  {Record: model.SpecRecord{Ref: "draft", Status: model.StatusDraft}, Sections: []embeddedSection{{Heading: "Spec", Content: "spec content", Embedding: docEmbed}}},
	}

	result := relevantAcceptedSpecs(doc, specs)

	// Draft and low-similarity specs should be excluded.
	for _, spec := range result {
		if spec.Record.Ref == "draft" {
			t.Error("relevantAcceptedSpecs included a draft spec")
		}
		if spec.Record.Ref == "low" {
			t.Error("relevantAcceptedSpecs included a low-similarity spec")
		}
	}

	// Should be sorted by similarity descending.
	if len(result) > 1 && result[0].Record.Ref != "high-1" {
		t.Errorf("first result = %q, want highest-similarity spec", result[0].Record.Ref)
	}
}
