package analysis

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
	"github.com/dusk-network/pituitary/internal/source"
)

func TestCheckComplianceIncludesAnalysisRuntimeProvenance(t *testing.T) {
	t.Parallel()

	cfg := writeComplianceRuntimeWorkspace(t)
	records, err := source.LoadFromConfig(cfg)
	if err != nil {
		t.Fatalf("source.LoadFromConfig() error = %v", err)
	}
	if _, err := index.Rebuild(cfg, records); err != nil {
		t.Fatalf("index.Rebuild() error = %v", err)
	}

	configureOpenAIAnalysisProvider(t, cfg, func(t *testing.T, request openAICompatibleChatRequest) string {
		t.Helper()
		var prompt complianceAdjudicatePrompt
		if err := json.Unmarshal([]byte(request.Messages[1].Content), &prompt); err != nil {
			t.Fatalf("unmarshal prompt: %v", err)
		}
		if got, want := prompt.Command, "check-compliance-adjudicate"; got != want {
			t.Fatalf("command = %q, want %q", got, want)
		}
		if got, want := prompt.Targets[0].Path, "src/service/handler.go"; got != want {
			t.Fatalf("target path = %q, want %q", got, want)
		}

		return `{
			"adjudications": [
				{
					"path": "src/service/handler.go",
					"classification": "conflict",
					"violated_section": "Requirements",
					"evidence": "handler omits the required audit log call",
					"confidence": 0.87,
					"message": "handler omits the audit log required by the accepted spec",
					"expected": "emit an audit log before mutating state",
					"observed": "mutates state without any audit log"
				}
			]
		}`
	})

	result, err := CheckCompliance(cfg, ComplianceRequest{Paths: []string{"src/service/handler.go"}})
	if err != nil {
		t.Fatalf("CheckCompliance() error = %v", err)
	}
	if result.Runtime == nil || result.Runtime.Analysis == nil {
		t.Fatalf("runtime = %+v, want analysis provenance", result.Runtime)
	}
	if got, want := result.Runtime.Analysis.Provider, config.RuntimeProviderOpenAI; got != want {
		t.Fatalf("runtime.analysis.provider = %q, want %q", got, want)
	}
	if got, want := result.Runtime.Analysis.Model, "pituitary-analysis"; got != want {
		t.Fatalf("runtime.analysis.model = %q, want %q", got, want)
	}
	if !result.Runtime.Analysis.Used {
		t.Fatalf("runtime.analysis.used = false, want true")
	}

	var found bool
	for _, finding := range result.Conflicts {
		if finding.Path == "src/service/handler.go" && finding.Provenance == ProvenanceModelAdjudication {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("conflicts = %+v, want model-adjudicated conflict", result.Conflicts)
	}
}

func writeComplianceRuntimeWorkspace(tb testing.TB) *config.Config {
	tb.Helper()

	root := tb.TempDir()
	indexPath := filepath.Join(root, ".pituitary", "pituitary.db")
	configPath := filepath.Join(root, "pituitary.toml")

	mustWriteFile(tb, filepath.Join(root, "specs", "audit-logging", "spec.toml"), `
id = "SPEC-AUDIT"
title = "Audit Logging for Stateful Mutations"
status = "accepted"
domain = "runtime"
body = "body.md"
applies_to = ["code://src/service/handler.go"]
`)
	mustWriteFile(tb, filepath.Join(root, "specs", "audit-logging", "body.md"), `
# Audit Logging for Stateful Mutations

## Requirements

Every stateful mutation must emit an audit log before writing state.
`)
	mustWriteFile(tb, filepath.Join(root, "src", "service", "handler.go"), `
package service

func HandleCreate() {
	writeState()
}
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
`, root, indexPath, filepath.Join(root, "specs")))

	cfg, err := config.Load(configPath)
	if err != nil {
		tb.Fatalf("config.Load() error = %v", err)
	}
	return cfg
}
