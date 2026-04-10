package analysis

import (
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
)

// CommandRuntime captures runtime provenance for one analysis result.
type CommandRuntime struct {
	Analysis *RuntimeUsage `json:"analysis,omitempty"`
}

// RuntimeUsage records the configured runtime surface and whether the command
// consulted it during this run.
type RuntimeUsage struct {
	Profile  string `json:"profile,omitempty"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Used     bool   `json:"used"`
}

func newAnalysisRuntimeUsage(provider config.RuntimeProvider) *RuntimeUsage {
	resolvedProvider := strings.TrimSpace(provider.Provider)
	if resolvedProvider == "" || resolvedProvider == config.RuntimeProviderDisabled {
		return nil
	}

	return &RuntimeUsage{
		Profile:  strings.TrimSpace(provider.Profile),
		Provider: resolvedProvider,
		Model:    strings.TrimSpace(provider.Model),
		Endpoint: strings.TrimSpace(provider.Endpoint),
	}
}
