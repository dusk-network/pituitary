package ast

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// SpecSummary is the minimal spec data needed for inference matching.
type SpecSummary struct {
	Ref             string
	Body            string
	ManualAppliesTo []string // existing manual applies_to refs (e.g. "code://path")
}

// InferredEdge is one inferred applies_to link from a spec to a code file.
type InferredEdge struct {
	SpecRef   string   `json:"spec_ref"`
	FilePath  string   `json:"file_path"`
	MatchedOn []string `json:"matched_on"`
}

// FileSymbols pairs a file path with its content hash and extracted symbols.
type FileSymbols struct {
	Path        string   `json:"path"`
	ContentHash string   `json:"content_hash"`
	Symbols     []Symbol `json:"symbols"`
}

// ContentHash computes SHA256(content + path) for cache keying.
func ContentHash(content []byte, path string) string {
	h := sha256.New()
	h.Write(content)
	h.Write([]byte(path))
	return hex.EncodeToString(h.Sum(nil))
}

// InferEdges matches file symbols against spec body text and returns inferred
// applies_to edges. Edges that duplicate existing manual applies_to are excluded.
func InferEdges(fileSymbols map[string][]Symbol, specs []SpecSummary) []InferredEdge {
	// Build reverse index: symbol name → list of file paths.
	symbolIndex := make(map[string][]string)
	for path, symbols := range fileSymbols {
		for _, sym := range symbols {
			if len(sym.Name) < MinIdentifierLength {
				continue
			}
			symbolIndex[sym.Name] = append(symbolIndex[sym.Name], path)
		}
	}

	var edges []InferredEdge
	for _, spec := range specs {
		manualSet := manualPathSet(spec.ManualAppliesTo)
		specIdentifiers := ScanSpecIdentifiers(spec.Body)

		// Track matches per file path.
		fileMatches := make(map[string][]string)
		for _, id := range specIdentifiers {
			for _, path := range symbolIndex[id] {
				if manualSet[path] {
					continue
				}
				fileMatches[path] = append(fileMatches[path], id)
			}
		}

		for path, matched := range fileMatches {
			edges = append(edges, InferredEdge{
				SpecRef:   spec.Ref,
				FilePath:  path,
				MatchedOn: unique(matched),
			})
		}
	}
	return edges
}

// manualPathSet normalizes manual applies_to refs into bare paths for dedup.
func manualPathSet(manual []string) map[string]bool {
	set := make(map[string]bool, len(manual))
	for _, ref := range manual {
		path := ref
		path = strings.TrimPrefix(path, "code://")
		path = strings.TrimPrefix(path, "config://")
		set[path] = true
	}
	return set
}

func unique(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
