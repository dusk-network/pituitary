package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dusk-network/pituitary/internal/app"
	"github.com/dusk-network/pituitary/internal/config"
)

func configErrorIssue(message string) cliIssue {
	return cliIssue{
		Code:    app.CodeConfigError,
		Message: message,
	}
}

func configLoadIssue(message string, resolution *configResolution) cliIssue {
	return enrichSelectedConfigLoadIssue(cliIssue{
		Code:    app.CodeConfigError,
		Message: message,
		Details: map[string]any{
			app.IssueDetailPhase: app.IssuePhaseConfigLoad,
		},
	}, resolution)
}

func enrichAppConfigLoadIssue(issue cliIssue, resolution *configResolution) cliIssue {
	if !isConfigLoadIssue(issue) {
		return issue
	}
	return enrichSelectedConfigLoadIssue(issue, resolution)
}

func isConfigLoadIssue(issue cliIssue) bool {
	if issue.Code != app.CodeConfigError || issue.Details == nil {
		return false
	}
	phase, _ := issue.Details[app.IssueDetailPhase].(string)
	return phase == app.IssuePhaseConfigLoad
}

func enrichSelectedConfigLoadIssue(issue cliIssue, resolution *configResolution) cliIssue {
	if resolution == nil {
		return issue
	}
	selected := selectedConfigCandidate(resolution)
	alternative, hasAlternative := firstValidShadowedConfigCandidate(resolution)
	if selected.Path == "" && !hasAlternative {
		return issue
	}

	lines := make([]string, 0, 2)
	if selected.Path != "" {
		lines = append(lines, selectedConfigLoadLine(selected))
	}
	if hasAlternative {
		lines = append(lines, validShadowedConfigLine(selected, alternative))
	}
	if len(lines) == 0 {
		return issue
	}

	message := strings.TrimRight(issue.Message, "\n")
	if message != "" {
		message += "\n"
	}
	issue.Message = message + strings.Join(lines, "\n")

	if issue.Details == nil {
		issue.Details = make(map[string]any)
	}
	issue.Details["config_resolution"] = configResolutionDetails(selected, alternative, hasAlternative)
	return issue
}

func selectedConfigCandidate(resolution *configResolution) configResolutionCandidate {
	if resolution == nil {
		return configResolutionCandidate{}
	}
	for _, candidate := range resolution.Candidates {
		if candidate.Status == "selected" {
			return candidate
		}
	}
	return configResolutionCandidate{}
}

func firstValidShadowedConfigCandidate(resolution *configResolution) (configResolutionCandidate, bool) {
	if resolution == nil {
		return configResolutionCandidate{}, false
	}
	selected := selectedConfigCandidate(resolution)
	for _, candidate := range resolution.Candidates {
		if candidate.Status != "shadowed" || strings.TrimSpace(candidate.Path) == "" {
			continue
		}
		if sameConfigPath(candidate.Path, selected.Path) {
			continue
		}
		if _, err := config.Load(candidate.Path); err == nil {
			return candidate, true
		}
	}
	return configResolutionCandidate{}, false
}

func selectedConfigLoadLine(selected configResolutionCandidate) string {
	if selected.Source == "" {
		return fmt.Sprintf("selected config: %s", displayConfigPath(selected.Path))
	}
	return fmt.Sprintf("selected config from %s: %s", configSourceLabel(selected.Source), displayConfigPath(selected.Path))
}

func validShadowedConfigLine(selected, alternative configResolutionCandidate) string {
	if selected.Source == configSourceDiscovery && alternative.Source == configSourceDiscovery {
		return fmt.Sprintf("shadowed config also loads; retry with `--config %s`", displayConfigPath(alternative.Path))
	}
	return fmt.Sprintf("another config candidate also loads; inspect it or retry with `--config %s`", displayConfigPath(alternative.Path))
}

func configResolutionDetails(selected, alternative configResolutionCandidate, hasAlternative bool) map[string]any {
	details := make(map[string]any)
	if strings.TrimSpace(selected.Path) != "" {
		details["selected"] = configResolutionCandidateDetail(selected)
	}
	if hasAlternative {
		details["valid_shadowed"] = configResolutionCandidateDetail(alternative)
	}
	return details
}

func configResolutionCandidateDetail(candidate configResolutionCandidate) map[string]any {
	detail := map[string]any{
		"path":   filepath.ToSlash(candidate.Path),
		"source": candidate.Source,
		"status": candidate.Status,
	}
	if candidate.Precedence != 0 {
		detail["precedence"] = candidate.Precedence
	}
	return detail
}

func displayConfigPath(path string) string {
	return filepath.ToSlash(path)
}

func sameConfigPath(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if absA, err := filepath.Abs(a); err == nil {
		a = absA
	}
	if absB, err := filepath.Abs(b); err == nil {
		b = absB
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
