package source

import (
	"testing"

	"github.com/dusk-network/pituitary/internal/model"
)

func TestEnrichCanonicalizeWithInferenceFillsDomain(t *testing.T) {
	t.Parallel()

	spec := model.SpecRecord{
		Ref:    "test-spec",
		Title:  "Test Spec",
		Domain: "",
		Inference: &model.InferenceConfidence{
			Fields: []model.InferenceFieldConfidence{
				{Name: "domain", Source: "absent", Score: 0.25, Level: "LOW"},
			},
		},
		Metadata: map[string]string{},
	}

	inference := &CanonicalizeInference{
		Domain: &InferredValue{Value: "authentication", Confidence: 0.75},
	}

	enrichCanonicalizeWithInference(&spec, inference)

	if spec.Domain != "authentication" {
		t.Errorf("domain = %q, want 'authentication'", spec.Domain)
	}

	// Check that inference field was updated.
	for _, f := range spec.Inference.Fields {
		if f.Name == "domain" {
			if f.Source != "model" {
				t.Errorf("domain source = %q, want 'model'", f.Source)
			}
			if f.Score != 0.75 {
				t.Errorf("domain score = %f, want 0.75", f.Score)
			}
			return
		}
	}
	t.Error("domain field not found in inference")
}

func TestEnrichCanonicalizeWithInferenceDoesNotOverrideExplicit(t *testing.T) {
	t.Parallel()

	spec := model.SpecRecord{
		Ref:    "test-spec",
		Title:  "Test Spec",
		Domain: "networking",
		Inference: &model.InferenceConfidence{
			Fields: []model.InferenceFieldConfidence{
				{Name: "domain", Source: "explicit", Score: 0.85, Level: "HIGH"},
			},
		},
		Metadata: map[string]string{},
	}

	inference := &CanonicalizeInference{
		Domain: &InferredValue{Value: "authentication", Confidence: 0.75},
	}

	enrichCanonicalizeWithInference(&spec, inference)

	// Explicit domain should NOT be overridden.
	if spec.Domain != "networking" {
		t.Errorf("domain = %q, want 'networking' (preserved explicit)", spec.Domain)
	}
}

func TestEnrichCanonicalizeWithInferenceAddsRelations(t *testing.T) {
	t.Parallel()

	spec := model.SpecRecord{
		Ref:       "test-spec",
		Title:     "Test Spec",
		Metadata:  map[string]string{},
		Inference: &model.InferenceConfidence{},
	}

	inference := &CanonicalizeInference{
		DependsOn: []InferredRef{
			{Ref: "base-spec", Confidence: 0.8},
			{Ref: "weak-spec", Confidence: 0.3}, // Below threshold.
		},
		Supersedes: []InferredRef{
			{Ref: "old-spec", Confidence: 0.9},
		},
	}

	enrichCanonicalizeWithInference(&spec, inference)

	if len(spec.Relations) != 2 {
		t.Fatalf("got %d relations, want 2", len(spec.Relations))
	}
	if spec.Relations[0].Ref != "base-spec" || spec.Relations[0].Type != model.RelationDependsOn {
		t.Errorf("relations[0] = %+v, want depends_on base-spec", spec.Relations[0])
	}
	if spec.Relations[1].Ref != "old-spec" || spec.Relations[1].Type != model.RelationSupersedes {
		t.Errorf("relations[1] = %+v, want supersedes old-spec", spec.Relations[1])
	}
}

func TestEnrichCanonicalizeWithInferenceOverridesDefaultStatus(t *testing.T) {
	t.Parallel()

	spec := model.SpecRecord{
		Ref:    "test-spec",
		Title:  "Test Spec",
		Status: "draft",
		Metadata: map[string]string{
			"status_source": "default",
		},
		Inference: &model.InferenceConfidence{
			Fields: []model.InferenceFieldConfidence{
				{Name: "status", Source: "default", Score: 0.35, Level: "LOW"},
			},
		},
	}

	inference := &CanonicalizeInference{
		Status: &InferredValue{Value: "accepted", Confidence: 0.7},
	}

	enrichCanonicalizeWithInference(&spec, inference)

	if spec.Status != "accepted" {
		t.Errorf("status = %q, want 'accepted'", spec.Status)
	}
	if spec.Metadata["status_source"] != "model" {
		t.Errorf("status_source = %q, want 'model'", spec.Metadata["status_source"])
	}
}

func TestEnrichCanonicalizeWithInferenceNilInference(t *testing.T) {
	t.Parallel()

	spec := model.SpecRecord{
		Ref:       "test-spec",
		Title:     "Test Spec",
		Metadata:  map[string]string{},
		Inference: &model.InferenceConfidence{},
	}

	// Should not panic.
	enrichCanonicalizeWithInference(&spec, nil)
}
