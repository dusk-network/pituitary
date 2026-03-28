package model

import "github.com/dusk-network/pituitary/sdk"

func ConfidenceLevelFromScore(score float64) string {
	return sdk.ConfidenceLevelFromScore(score)
}

func EncodeInferenceConfidence(metadata map[string]string, confidence *InferenceConfidence) (map[string]string, error) {
	return sdk.EncodeInferenceConfidence(metadata, confidence)
}

func DecodeInferenceConfidence(metadata map[string]string) (*InferenceConfidence, error) {
	return sdk.DecodeInferenceConfidence(metadata)
}
