package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	metadataInferenceKindKey    = "inference_kind"
	metadataInferenceLevelKey   = "inference_level"
	metadataInferenceScoreKey   = "inference_score"
	metadataInferenceReasonsKey = "inference_reasons_json"
	metadataInferenceFieldsKey  = "inference_fields_json"
)

// InferenceFieldConfidence captures one inferred field's source and strength.
type InferenceFieldConfidence struct {
	Name   string  `json:"name"`
	Level  string  `json:"level"`
	Score  float64 `json:"score"`
	Source string  `json:"source"`
}

// InferenceConfidence summarizes how strongly Pituitary trusts inferred metadata.
type InferenceConfidence struct {
	Kind    string                     `json:"kind,omitempty"`
	Level   string                     `json:"level"`
	Score   float64                    `json:"score"`
	Reasons []string                   `json:"reasons,omitempty"`
	Fields  []InferenceFieldConfidence `json:"fields,omitempty"`
}

// ConfidenceLevelFromScore maps a normalized score into a stable level.
func ConfidenceLevelFromScore(score float64) string {
	switch {
	case score >= 0.8:
		return "high"
	case score >= 0.5:
		return "medium"
	default:
		return "low"
	}
}

// Field returns the named field confidence when present.
func (c *InferenceConfidence) Field(name string) (InferenceFieldConfidence, bool) {
	if c == nil {
		return InferenceFieldConfidence{}, false
	}
	name = strings.TrimSpace(name)
	for _, field := range c.Fields {
		if field.Name == name {
			return field, true
		}
	}
	return InferenceFieldConfidence{}, false
}

// EncodeInferenceConfidence stores structured inference details in artifact metadata.
func EncodeInferenceConfidence(metadata map[string]string, confidence *InferenceConfidence) (map[string]string, error) {
	if confidence == nil {
		return metadata, nil
	}
	if metadata == nil {
		metadata = map[string]string{}
	}
	reasonsJSON, err := json.Marshal(confidence.Reasons)
	if err != nil {
		return nil, fmt.Errorf("marshal inference reasons: %w", err)
	}
	fieldsJSON, err := json.Marshal(confidence.Fields)
	if err != nil {
		return nil, fmt.Errorf("marshal inference fields: %w", err)
	}
	metadata[metadataInferenceKindKey] = strings.TrimSpace(confidence.Kind)
	metadata[metadataInferenceLevelKey] = strings.TrimSpace(confidence.Level)
	metadata[metadataInferenceScoreKey] = strconv.FormatFloat(confidence.Score, 'f', 2, 64)
	metadata[metadataInferenceReasonsKey] = string(reasonsJSON)
	metadata[metadataInferenceFieldsKey] = string(fieldsJSON)
	return metadata, nil
}

// DecodeInferenceConfidence loads structured inference details from artifact metadata.
func DecodeInferenceConfidence(metadata map[string]string) (*InferenceConfidence, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	rawScore := strings.TrimSpace(metadata[metadataInferenceScoreKey])
	rawLevel := strings.TrimSpace(metadata[metadataInferenceLevelKey])
	rawKind := strings.TrimSpace(metadata[metadataInferenceKindKey])
	if rawScore == "" && rawLevel == "" && rawKind == "" {
		return nil, nil
	}

	score, err := strconv.ParseFloat(defaultInferenceValue(rawScore, "0"), 64)
	if err != nil {
		return nil, fmt.Errorf("parse inference score %q: %w", rawScore, err)
	}

	var reasons []string
	if rawReasons := strings.TrimSpace(metadata[metadataInferenceReasonsKey]); rawReasons != "" {
		if err := json.Unmarshal([]byte(rawReasons), &reasons); err != nil {
			return nil, fmt.Errorf("parse inference reasons: %w", err)
		}
	}

	var fields []InferenceFieldConfidence
	if rawFields := strings.TrimSpace(metadata[metadataInferenceFieldsKey]); rawFields != "" {
		if err := json.Unmarshal([]byte(rawFields), &fields); err != nil {
			return nil, fmt.Errorf("parse inference fields: %w", err)
		}
	}

	level := rawLevel
	if level == "" {
		level = ConfidenceLevelFromScore(score)
	}
	return &InferenceConfidence{
		Kind:    rawKind,
		Level:   level,
		Score:   score,
		Reasons: reasons,
		Fields:  fields,
	}, nil
}

func defaultInferenceValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
