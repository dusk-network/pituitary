package runtimeprobe

import (
	"context"
	"fmt"
	"strings"

	"github.com/dusk-network/pituitary/internal/analysis"
	"github.com/dusk-network/pituitary/internal/config"
	"github.com/dusk-network/pituitary/internal/index"
)

type Scope string

const (
	ScopeNone     Scope = "none"
	ScopeEmbedder Scope = "embedder"
	ScopeAnalysis Scope = "analysis"
	ScopeAll      Scope = "all"

	StatusReady    = "ready"
	StatusDisabled = "disabled"
)

type Result struct {
	Scope  string  `json:"scope"`
	Checks []Check `json:"checks"`
}

type Check struct {
	Name     string `json:"name"`
	Profile  string `json:"profile,omitempty"`
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Timeout  int    `json:"timeout_ms,omitempty"`
	Retries  int    `json:"max_retries,omitempty"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}

func ParseScope(raw string) (Scope, error) {
	switch scope := Scope(strings.TrimSpace(raw)); scope {
	case "", ScopeNone:
		return ScopeNone, nil
	case ScopeEmbedder, ScopeAnalysis, ScopeAll:
		return scope, nil
	default:
		return ScopeNone, fmt.Errorf("unsupported runtime probe scope %q (supported: %q, %q, %q, %q)", raw, ScopeNone, ScopeEmbedder, ScopeAnalysis, ScopeAll)
	}
}

func Run(ctx context.Context, cfg *config.Config, scope Scope) (*Result, error) {
	if cfg == nil || scope == ScopeNone {
		return nil, nil
	}

	checks := make([]Check, 0, 2)
	if scope == ScopeEmbedder || scope == ScopeAll {
		check, err := probeEmbedder(ctx, cfg.Runtime.Embedder)
		if err != nil {
			return nil, err
		}
		checks = append(checks, check)
	}
	if scope == ScopeAnalysis || scope == ScopeAll {
		check, err := probeAnalysis(ctx, cfg.Runtime.Analysis)
		if err != nil {
			return nil, err
		}
		checks = append(checks, check)
	}

	return &Result{
		Scope:  string(scope),
		Checks: checks,
	}, nil
}

func probeEmbedder(ctx context.Context, provider config.RuntimeProvider) (Check, error) {
	check := configuredCheck("runtime.embedder", provider)

	embedder, err := index.NewEmbedder(provider)
	if err != nil {
		return Check{}, err
	}
	if _, err := embedder.Dimension(ctx); err != nil {
		return Check{}, err
	}

	check.Status = StatusReady
	return check, nil
}

func probeAnalysis(ctx context.Context, provider config.RuntimeProvider) (Check, error) {
	check := configuredCheck("runtime.analysis", provider)

	if strings.TrimSpace(provider.Provider) == "" || strings.TrimSpace(provider.Provider) == config.RuntimeProviderDisabled {
		check.Status = StatusDisabled
		check.Message = "runtime.analysis is disabled in config"
		return check, nil
	}

	if err := analysis.ProbeProviderContext(ctx, provider); err != nil {
		return Check{}, err
	}

	check.Status = StatusReady
	return check, nil
}

func configuredCheck(name string, provider config.RuntimeProvider) Check {
	return Check{
		Name:     name,
		Profile:  strings.TrimSpace(provider.Profile),
		Provider: strings.TrimSpace(provider.Provider),
		Model:    strings.TrimSpace(provider.Model),
		Endpoint: strings.TrimSpace(provider.Endpoint),
		Timeout:  provider.TimeoutMS,
		Retries:  provider.MaxRetries,
	}
}
