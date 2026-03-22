package analysis

import (
	"fmt"
	"strings"

	"github.com/dusk-network/pituitary/internal/model"
)

// Warning reports a non-fatal quality caveat attached to an analysis result.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Ref     string `json:"ref,omitempty"`
}

// SpecInference surfaces confidence for one spec ref included in a result.
type SpecInference struct {
	Ref        string                     `json:"ref"`
	Confidence *model.InferenceConfidence `json:"confidence,omitempty"`
}

func buildSpecInferences(specs map[string]specDocument, refs []string) []SpecInference {
	result := make([]SpecInference, 0, len(refs))
	for _, ref := range uniqueStrings(refs) {
		spec, ok := specs[ref]
		if !ok || spec.Record.Inference == nil {
			continue
		}
		result = append(result, SpecInference{
			Ref:        ref,
			Confidence: spec.Record.Inference,
		})
	}
	return result
}

func buildSpecInferenceWarnings(subject string, specs ...specDocument) []Warning {
	seen := map[string]struct{}{}
	warnings := make([]Warning, 0, len(specs))
	for _, spec := range specs {
		warning, ok := highStakesInferenceWarning(subject, spec.Record)
		if !ok {
			continue
		}
		key := warning.Ref + "\x00" + warning.Code
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		warnings = append(warnings, warning)
	}
	return warnings
}

func highStakesInferenceWarning(subject string, record model.SpecRecord) (Warning, bool) {
	confidence := record.Inference
	if confidence == nil {
		return Warning{}, false
	}
	if confidence.Level == "high" && !fieldIsLow(confidence, "status") && !fieldIsLow(confidence, "applies_to") {
		return Warning{}, false
	}

	reasons := confidence.Reasons
	message := fmt.Sprintf("spec %s uses %s-confidence inferred metadata", record.Ref, confidence.Level)
	if len(reasons) > 0 {
		message += fmt.Sprintf(" (%s)", strings.Join(reasons, "; "))
	}
	if strings.TrimSpace(subject) != "" {
		message += fmt.Sprintf("; %s results may be incomplete", subject)
	}
	return Warning{
		Code:    "low_confidence_inference",
		Message: message,
		Ref:     record.Ref,
	}, true
}

func fieldIsLow(confidence *model.InferenceConfidence, name string) bool {
	field, ok := confidence.Field(name)
	return ok && field.Level == "low"
}
