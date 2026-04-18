package config

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

func parse(file io.Reader) (rawConfig, error) {
	var cfg rawConfig

	metadata, err := toml.NewDecoder(file).Decode(&cfg)
	if err != nil {
		return rawConfig{}, err
	}

	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		if err := formatUndecodedKeys(undecoded); err != nil {
			return rawConfig{}, err
		}
	}

	return cfg, nil
}

func formatUndecodedKeys(keys []toml.Key) error {
	messages := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if isOpaqueSourceOptionKey(key) {
			continue
		}
		message := undecodedKeyMessage(key)
		if _, exists := seen[message]; exists {
			continue
		}
		seen[message] = struct{}{}
		messages = append(messages, message)
	}
	sort.Strings(messages)
	if len(messages) == 0 {
		return nil
	}
	return errors.New(strings.Join(messages, "\n"))
}

func isOpaqueSourceOptionKey(key toml.Key) bool {
	return len(key) >= 3 && key[0] == "sources" && key[1] == "options"
}

func undecodedKeyMessage(key toml.Key) string {
	if len(key) == 0 {
		return "unsupported empty key"
	}
	if len(key) == 1 {
		return fmt.Sprintf("key %q is outside a supported section", key[0])
	}

	switch key[0] {
	case "workspace":
		switch {
		case len(key) == 2:
			return unsupportedFieldMessage("workspace", key[1], []string{
				"root",
				"repo_id",
				"index_path",
				"infer_applies_to",
				"repos",
			})
		case key[1] == "repos" && len(key) == 3:
			return unsupportedFieldMessage("workspace.repos", key[2], []string{"id", "root"})
		default:
			return fmt.Sprintf("unsupported workspace field %q", strings.Join(key[1:], "."))
		}
	case "runtime":
		switch len(key) {
		case 2:
			return unsupportedFieldMessage("runtime", key[1], []string{"profiles", "embedder", "analysis", "chunking", "search"})
		default:
			switch key[1] {
			case "embedder", "analysis":
				return unsupportedFieldMessage(
					"runtime."+key[1],
					strings.Join(key[2:], "."),
					runtimeProviderFields(),
				)
			case "profiles":
				if len(key) >= 4 {
					return unsupportedFieldMessage(
						"runtime.profiles."+key[2],
						strings.Join(key[3:], "."),
						runtimeProviderFields(),
					)
				}
				return fmt.Sprintf("unsupported runtime.profiles field %q", strings.Join(key[2:], "."))
			case "chunking":
				if len(key) < 3 {
					return fmt.Sprintf("unsupported runtime.chunking field %q", strings.Join(key[2:], "."))
				}
				switch key[2] {
				case "spec", "doc":
					if len(key) == 3 {
						return fmt.Sprintf("unsupported runtime.chunking.%s field %q", key[2], strings.Join(key[3:], "."))
					}
					scope := "runtime.chunking." + key[2]
					return unsupportedFieldMessage(scope, strings.Join(key[3:], "."), chunkingKindFields())
				case "contextualizer":
					if len(key) == 3 {
						return fmt.Sprintf("unsupported runtime.chunking.contextualizer field %q", strings.Join(key[3:], "."))
					}
					return unsupportedFieldMessage("runtime.chunking.contextualizer", strings.Join(key[3:], "."), chunkingContextualizerFields())
				default:
					return unsupportedFieldMessage("runtime.chunking", key[2], []string{"spec", "doc", "contextualizer"})
				}
			case "search":
				if len(key) < 3 {
					return fmt.Sprintf("unsupported runtime.search field %q", strings.Join(key[2:], "."))
				}
				switch key[2] {
				case "fusion":
					if len(key) == 3 {
						return fmt.Sprintf("unsupported runtime.search.fusion field %q", strings.Join(key[3:], "."))
					}
					return unsupportedFieldMessage("runtime.search.fusion", strings.Join(key[3:], "."), searchFusionFields())
				default:
					return unsupportedFieldMessage("runtime.search", key[2], []string{"fusion", "reranker"})
				}
			default:
				return fmt.Sprintf("unsupported runtime.%s field %q", key[1], strings.Join(key[2:], "."))
			}
		}
	case "terminology":
		switch len(key) {
		case 2:
			return unsupportedFieldMessage("terminology", key[1], []string{"exclude_paths", "policies"})
		default:
			if key[1] == "policies" && len(key) == 3 {
				return unsupportedFieldMessage("terminology.policies", key[2], []string{
					"preferred",
					"historical_aliases",
					"deprecated_terms",
					"forbidden_current",
					"docs_severity",
					"specs_severity",
				})
			}
			return fmt.Sprintf("unsupported terminology.%s field %q", key[1], strings.Join(key[2:], "."))
		}
	case "sources":
		return unsupportedFieldMessage("sources", strings.Join(key[1:], "."), []string{
			"name",
			"adapter",
			"kind",
			"role",
			"repo",
			"path",
			"files",
			"include",
			"exclude",
			"options",
		})
	default:
		return fmt.Sprintf("unsupported section %q", key[0])
	}
}

func runtimeProviderFields() []string {
	return []string{
		"profile",
		"provider",
		"model",
		"endpoint",
		"api_key_env",
		"timeout_ms",
		"max_retries",
		"max_response_tokens",
	}
}

func chunkingKindFields() []string {
	return []string{
		"policy",
		"max_tokens",
		"overlap_tokens",
		"max_sections",
		"child_max_tokens",
		"child_overlap_tokens",
	}
}

func chunkingContextualizerFields() []string {
	return []string{"format"}
}

func searchFusionFields() []string {
	return []string{"strategy", "k"}
}

func unsupportedFieldMessage(scope, field string, valid []string) string {
	message := fmt.Sprintf("unsupported %s field %q", scope, field)
	if strings.Contains(field, ".") || len(valid) == 0 {
		return message
	}
	suggestions := rankedFieldSuggestions(field, valid)
	if len(suggestions) == 0 {
		return message
	}
	return fmt.Sprintf("%s; did you mean one of: %s", message, quotedList(suggestions))
}

func rankedFieldSuggestions(field string, valid []string) []string {
	if len(valid) == 0 {
		return nil
	}
	type suggestion struct {
		field    string
		distance int
	}
	ranked := make([]suggestion, 0, len(valid))
	for _, candidate := range valid {
		ranked = append(ranked, suggestion{
			field:    candidate,
			distance: levenshteinDistance(strings.ToLower(field), strings.ToLower(candidate)),
		})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].distance != ranked[j].distance {
			return ranked[i].distance < ranked[j].distance
		}
		if len(ranked[i].field) != len(ranked[j].field) {
			return len(ranked[i].field) < len(ranked[j].field)
		}
		return ranked[i].field < ranked[j].field
	})

	limit := len(ranked)
	if limit > 6 {
		limit = 6
	}
	result := make([]string, 0, limit)
	for _, item := range ranked[:limit] {
		result = append(result, item.field)
	}
	return result
}

func quotedList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, strconv.Quote(value))
	}
	return strings.Join(quoted, ", ")
}

func levenshteinDistance(left, right string) int {
	if left == right {
		return 0
	}
	if left == "" {
		return len([]rune(right))
	}
	if right == "" {
		return len([]rune(left))
	}

	leftRunes := []rune(left)
	rightRunes := []rune(right)
	prev := make([]int, len(rightRunes)+1)
	curr := make([]int, len(rightRunes)+1)
	for j := range prev {
		prev[j] = j
	}
	for i, leftRune := range leftRunes {
		curr[0] = i + 1
		for j, rightRune := range rightRunes {
			cost := 0
			if leftRune != rightRune {
				cost = 1
			}
			insertion := curr[j] + 1
			deletion := prev[j+1] + 1
			substitution := prev[j] + cost
			curr[j+1] = minInt(insertion, deletion, substitution)
		}
		prev, curr = curr, prev
	}
	return prev[len(rightRunes)]
}

func minInt(values ...int) int {
	best := values[0]
	for _, value := range values[1:] {
		if value < best {
			best = value
		}
	}
	return best
}
