package analysis

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
)

// driftImplicatedDocMinScore is the minimum doc/spec similarity required to
// keep a doc in the diff-driven shortlist.
const driftImplicatedDocMinScore = 0.35

// DriftChangedFile summarizes one changed path from a diff-driven doc-drift run.
type DriftChangedFile struct {
	Path             string `json:"path"`
	AddedLineCount   int    `json:"added_line_count,omitempty"`
	RemovedLineCount int    `json:"removed_line_count,omitempty"`
}

// DriftImplicatedSpec reports one spec implicated by a diff-driven doc-drift run.
type DriftImplicatedSpec struct {
	Ref     string   `json:"ref"`
	Title   string   `json:"title,omitempty"`
	Repo    string   `json:"repo,omitempty"`
	Files   []string `json:"files,omitempty"`
	Reasons []string `json:"reasons,omitempty"`
	Score   float64  `json:"score,omitempty"`
}

// DriftImplicatedDoc reports one doc implicated by a diff-driven doc-drift run.
type DriftImplicatedDoc struct {
	DocRef    string   `json:"doc_ref"`
	Title     string   `json:"title,omitempty"`
	Repo      string   `json:"repo,omitempty"`
	SourceRef string   `json:"source_ref"`
	SpecRefs  []string `json:"spec_refs,omitempty"`
	Files     []string `json:"files,omitempty"`
	Reasons   []string `json:"reasons,omitempty"`
	Score     float64  `json:"score,omitempty"`
}

type driftImplicatedSpecState struct {
	ref     string
	title   string
	repo    string
	files   map[string]struct{}
	reasons map[string]struct{}
	score   float64
}

type driftImplicatedDocState struct {
	doc      docDocument
	specRefs map[string]struct{}
	files    map[string]struct{}
	reasons  map[string]struct{}
	score    float64
}

func resolveDiffDocDriftContext(ctx context.Context, repo *analysisRepository, cfg *config.Config, diffText string) (map[string]docDocument, map[string]specDocument, []DriftChangedFile, []DriftImplicatedSpec, []DriftImplicatedDoc, error) {
	parsed, err := parseDiffTargets(diffText)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	targets, err := loadParsedDiffComplianceTargetsContext(ctx, parsed)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	prepared, err := prepareComplianceEvaluationTargetsContext(ctx, repo, cfg, targets)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	specStates := make(map[string]*driftImplicatedSpecState)
	for _, target := range prepared {
		if len(target.ExplicitRefs) > 0 {
			for _, ref := range target.ExplicitRefs {
				addDiffDriftSpecState(specStates, ref, target.Target.Path, fmt.Sprintf("applies_to matched changed path %s", target.Target.Path), 1.0)
			}
			continue
		}
		if len(target.Target.Embedding) == 0 {
			continue
		}
		suggestions, err := repo.complianceSemanticSuggestions(target.Target.Embedding)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		for _, suggestion := range suggestions {
			if suggestion.Score < complianceWeakSuggestionThreshold {
				continue
			}
			addDiffDriftSpecState(
				specStates,
				suggestion.Ref,
				target.Target.Path,
				fmt.Sprintf("semantic diff match from %s (score %.3f)", target.Target.Path, roundScore(suggestion.Score)),
				suggestion.Score,
			)
		}
	}

	specRefs := make([]string, 0, len(specStates))
	for ref := range specStates {
		specRefs = append(specRefs, ref)
	}
	sort.Strings(specRefs)

	loadedSpecs, err := repo.loadSelectedSpecs(specRefs)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	for ref, state := range specStates {
		if spec, ok := loadedSpecs[ref]; ok {
			state.title = spec.Record.Title
			state.repo = artifactRepoID(spec.Record.Metadata)
		}
	}

	docStates := make(map[string]*driftImplicatedDocState)
	for _, ref := range specRefs {
		spec, ok := loadedSpecs[ref]
		if !ok {
			continue
		}
		docRefs, err := repo.impactedDocRefs(spec)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		docs, err := repo.loadSelectedDocs(docRefs)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		for docRef, doc := range docs {
			score := documentSimilarity(spec.Sections, doc.Sections)
			if score < driftImplicatedDocMinScore {
				continue
			}
			state, ok := docStates[docRef]
			if !ok {
				state = &driftImplicatedDocState{
					doc:      doc,
					specRefs: map[string]struct{}{},
					files:    map[string]struct{}{},
					reasons:  map[string]struct{}{},
				}
				docStates[docRef] = state
			}
			state.specRefs[ref] = struct{}{}
			for file := range specStates[ref].files {
				state.files[file] = struct{}{}
			}
			for reason := range specStates[ref].reasons {
				state.reasons[fmt.Sprintf("%s: %s", ref, reason)] = struct{}{}
			}
			if score > state.score {
				state.score = score
			}
		}
	}

	selectedDocs := make(map[string]docDocument, len(docStates))
	for ref, state := range docStates {
		selectedDocs[ref] = state.doc
	}

	relevantSpecRefs := append([]string(nil), specRefs...)
	docRelevantSpecRefs, err := repo.relevantDocDriftSpecRefs(selectedDocs)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	relevantSpecRefs = uniqueStrings(append(relevantSpecRefs, docRelevantSpecRefs...))
	selectedSpecs, err := repo.loadSelectedSpecs(relevantSpecRefs)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	return selectedDocs, selectedSpecs, buildDriftChangedFiles(parsed), buildDriftImplicatedSpecs(specRefs, specStates), buildDriftImplicatedDocs(docStates), nil
}

func addDiffDriftSpecState(states map[string]*driftImplicatedSpecState, ref, path, reason string, score float64) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return
	}
	state, ok := states[ref]
	if !ok {
		state = &driftImplicatedSpecState{
			ref:     ref,
			files:   map[string]struct{}{},
			reasons: map[string]struct{}{},
		}
		states[ref] = state
	}
	if path = strings.TrimSpace(path); path != "" {
		state.files[path] = struct{}{}
	}
	if reason = strings.TrimSpace(reason); reason != "" {
		state.reasons[reason] = struct{}{}
	}
	if score > state.score {
		state.score = score
	}
}

func buildDriftChangedFiles(items []parsedDiffTarget) []DriftChangedFile {
	if len(items) == 0 {
		return nil
	}
	result := make([]DriftChangedFile, 0, len(items))
	for _, item := range items {
		result = append(result, DriftChangedFile{
			Path:             item.Path,
			AddedLineCount:   len(item.Added),
			RemovedLineCount: len(item.Removed),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})
	return result
}

func buildDriftImplicatedSpecs(refs []string, states map[string]*driftImplicatedSpecState) []DriftImplicatedSpec {
	if len(refs) == 0 {
		return nil
	}
	result := make([]DriftImplicatedSpec, 0, len(refs))
	for _, ref := range refs {
		state := states[ref]
		if state == nil {
			continue
		}
		result = append(result, DriftImplicatedSpec{
			Ref:     ref,
			Title:   state.title,
			Repo:    state.repo,
			Files:   sortedStringSet(state.files),
			Reasons: sortedStringSet(state.reasons),
			Score:   roundScore(state.score),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		switch {
		case result[i].Score != result[j].Score:
			return result[i].Score > result[j].Score
		default:
			return result[i].Ref < result[j].Ref
		}
	})
	return result
}

func buildDriftImplicatedDocs(states map[string]*driftImplicatedDocState) []DriftImplicatedDoc {
	if len(states) == 0 {
		return nil
	}
	result := make([]DriftImplicatedDoc, 0, len(states))
	for ref, state := range states {
		result = append(result, DriftImplicatedDoc{
			DocRef:    ref,
			Title:     state.doc.Record.Title,
			Repo:      artifactRepoID(state.doc.Record.Metadata),
			SourceRef: state.doc.Record.SourceRef,
			SpecRefs:  sortedStringSet(state.specRefs),
			Files:     sortedStringSet(state.files),
			Reasons:   sortedStringSet(state.reasons),
			Score:     roundScore(state.score),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		switch {
		case result[i].Score != result[j].Score:
			return result[i].Score > result[j].Score
		default:
			return result[i].DocRef < result[j].DocRef
		}
	})
	return result
}

func sortedStringSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
