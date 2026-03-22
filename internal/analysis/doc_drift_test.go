package analysis

import (
	"testing"

	"github.com/dusk-network/pituitary/internal/index"
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
	if top.SpecRef == "" || top.Evidence.SpecSection == "" || top.SuggestedEdit.Action == "" {
		t.Fatalf("top remediation suggestion = %+v, want evidence and suggested edit", top)
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
}
