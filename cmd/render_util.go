package cmd

import (
	"fmt"
	"strings"
)

func renderFreshnessLabel(p renderPresentation, state string) string {
	switch state {
	case "fresh":
		return p.green(state)
	case "stale", "incompatible":
		return p.red(state)
	default:
		return p.yellow(state)
	}
}

func statusEmbedderSummary(provider string) string {
	if strings.TrimSpace(provider) == "" {
		return "embedder unknown"
	}
	return provider + " embedder"
}

func preferredDocLabel(docRef, sourceRef string) string {
	if strings.TrimSpace(sourceRef) != "" {
		return displaySourcePath(sourceRef)
	}
	return docRef
}

func displaySourcePath(sourceRef string) string {
	sourceRef = strings.TrimSpace(sourceRef)
	if sourceRef == "" {
		return ""
	}
	return strings.TrimPrefix(sourceRef, "file://")
}

func displayDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func repoPathLabel(repo, label string) string {
	repo = strings.TrimSpace(repo)
	label = strings.TrimSpace(label)
	switch {
	case repo == "":
		return label
	case label == "":
		return repo
	default:
		return fmt.Sprintf("[%s] %s", repo, label)
	}
}

func isNonPrimaryRepoDoc(docRef, repo string) bool {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(docRef), "doc://"+repo+"/")
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func humanizeSymbol(value string) string {
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	return value
}

func truncateRenderLine(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
