package analysis

import (
	"context"
	"strings"

	"github.com/dusk-network/pituitary/internal/config"
)

// metadataInferrer is an optional capability for analysis providers that
// support inferring spec metadata from unstructured body text. Used by
// canonicalize when runtime.analysis is configured.
type metadataInferrer interface {
	InferMetadata(ctx context.Context, request metadataInferenceRequest) (*MetadataInferenceResult, error)
}

// metadataInferenceRequest groups the body text and existing spec context sent
// to the analysis model for metadata inference.
type metadataInferenceRequest struct {
	Body          string               `json:"body"`
	Title         string               `json:"title"`
	ExistingSpecs []analysisSpecPrompt `json:"existing_specs,omitempty"`
}

// MetadataInferenceResult contains the model-inferred metadata fields, each
// with a confidence score.
type MetadataInferenceResult struct {
	Domain     *InferredStringField    `json:"domain,omitempty"`
	Tags       *InferredStringsField   `json:"tags,omitempty"`
	DependsOn  []InferredRelationField `json:"depends_on,omitempty"`
	AppliesTo  *InferredStringsField   `json:"applies_to,omitempty"`
	Status     *InferredStringField    `json:"status,omitempty"`
	Supersedes []InferredRelationField `json:"supersedes,omitempty"`
}

// InferredStringField is a single inferred string value with confidence.
type InferredStringField struct {
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// InferredStringsField is an inferred string list with confidence.
type InferredStringsField struct {
	Values     []string `json:"values"`
	Confidence float64  `json:"confidence"`
	Reason     string   `json:"reason"`
}

// InferredRelationField is an inferred spec relation with confidence.
type InferredRelationField struct {
	Ref        string  `json:"ref"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

const (
	openAICompatibleInferMetadataSystemPrompt = "You are Pituitary's metadata inference engine. Given a markdown spec body and optional existing specs for context, infer structured metadata fields. Return only one JSON object with optional keys: domain (string, the knowledge domain), tags (string array, descriptive tags), depends_on (array of {ref, confidence, reason} for specs this depends on), applies_to (string array, code paths or modules this governs), status (string: draft, review, accepted, superseded, or deprecated), supersedes (array of {ref, confidence, reason} for specs this replaces). Each top-level field should include confidence (0.0 to 1.0) and reason (brief justification). Only include fields you can infer with reasonable confidence. Do not fabricate spec refs — only reference refs from the existing_specs list."
	inferMetadataBodyLimit                    = 2000
)

// InferMetadata implements metadataInferrer for the OpenAI-compatible provider.
func (p *openAICompatibleAnalysisProvider) InferMetadata(ctx context.Context, request metadataInferenceRequest) (*MetadataInferenceResult, error) {
	payload := inferMetadataPrompt{
		Command:       "canonicalize-infer-metadata",
		Title:         request.Title,
		Body:          truncateForAnalysisPrompt(request.Body, inferMetadataBodyLimit),
		ExistingSpecs: truncateExistingSpecs(request.ExistingSpecs),
	}

	var response MetadataInferenceResult
	if err := p.completeJSON(ctx, payload.Command, openAICompatibleInferMetadataSystemPrompt, payload, &response); err != nil {
		return nil, err
	}
	normalizeMetadataInference(&response)
	return &response, nil
}

type inferMetadataPrompt struct {
	Command       string               `json:"command"`
	Title         string               `json:"title"`
	Body          string               `json:"body"`
	ExistingSpecs []analysisSpecPrompt `json:"existing_specs,omitempty"`
}

func truncateExistingSpecs(specs []analysisSpecPrompt) []analysisSpecPrompt {
	limit := minInt(len(specs), 12)
	result := make([]analysisSpecPrompt, 0, limit)
	for i := 0; i < limit; i++ {
		spec := specs[i]
		// Strip sections to reduce prompt size — only ref/title/status/domain needed.
		spec.Sections = nil
		result = append(result, spec)
	}
	return result
}

func normalizeMetadataInference(result *MetadataInferenceResult) {
	if result.Domain != nil {
		result.Domain.Value = strings.ToLower(strings.TrimSpace(result.Domain.Value))
		result.Domain.Confidence = clampConfidence(result.Domain.Confidence)
		if result.Domain.Value == "" {
			result.Domain = nil
		}
	}
	if result.Tags != nil {
		result.Tags.Values = normalizeStringList(result.Tags.Values, 8)
		result.Tags.Confidence = clampConfidence(result.Tags.Confidence)
		if len(result.Tags.Values) == 0 {
			result.Tags = nil
		}
	}
	if result.AppliesTo != nil {
		result.AppliesTo.Values = normalizeStringList(result.AppliesTo.Values, 8)
		result.AppliesTo.Confidence = clampConfidence(result.AppliesTo.Confidence)
		if len(result.AppliesTo.Values) == 0 {
			result.AppliesTo = nil
		}
	}
	if result.Status != nil {
		result.Status.Value = strings.ToLower(strings.TrimSpace(result.Status.Value))
		result.Status.Confidence = clampConfidence(result.Status.Confidence)
		switch result.Status.Value {
		case "draft", "review", "accepted", "superseded", "deprecated":
		default:
			result.Status = nil
		}
	}
	result.DependsOn = normalizeInferredRelations(result.DependsOn)
	result.Supersedes = normalizeInferredRelations(result.Supersedes)
}

func normalizeInferredRelations(relations []InferredRelationField) []InferredRelationField {
	result := make([]InferredRelationField, 0, len(relations))
	for _, rel := range relations {
		ref := strings.TrimSpace(rel.Ref)
		if ref == "" {
			continue
		}
		rel.Ref = ref
		rel.Confidence = clampConfidence(rel.Confidence)
		result = append(result, rel)
	}
	if len(result) > 6 {
		result = result[:6]
	}
	return result
}

func clampConfidence(c float64) float64 {
	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return c
}

// InferSpecMetadata is the public entry point for metadata inference. It
// resolves the analysis provider and runs inference if available.
func InferSpecMetadata(ctx context.Context, cfg *config.Config, title, body string) (*MetadataInferenceResult, error) {
	return InferSpecMetadataWithSpecs(ctx, cfg, title, body, nil)
}

// InferSpecMetadataWithSpecs runs metadata inference with existing spec context.
func InferSpecMetadataWithSpecs(ctx context.Context, cfg *config.Config, title, body string, existingSpecs []analysisSpecPrompt) (*MetadataInferenceResult, error) {
	if cfg == nil {
		return nil, nil
	}

	analyzer, err := newQualitativeAnalyzer(cfg.Runtime.Analysis)
	if err != nil {
		return nil, err
	}
	inferrer, ok := analyzer.(metadataInferrer)
	if !ok {
		return nil, nil
	}

	// Load existing specs for context if not provided.
	if existingSpecs == nil {
		repo, err := openAnalysisRepositoryContext(ctx, cfg)
		if err != nil {
			return nil, nil // Non-fatal: inference without context.
		}
		defer repo.Close()

		allSpecs, err := repo.loadAllSpecs()
		if err == nil {
			for _, ref := range sortedSpecRefs(allSpecs) {
				existingSpecs = append(existingSpecs, analysisSpecFromDocument(allSpecs[ref]))
			}
		}
	}

	return inferrer.InferMetadata(ctx, metadataInferenceRequest{
		Title:         title,
		Body:          body,
		ExistingSpecs: existingSpecs,
	})
}
