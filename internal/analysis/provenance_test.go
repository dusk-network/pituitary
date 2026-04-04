package analysis

import "testing"

func TestProvenanceConstants(t *testing.T) {
	t.Parallel()

	// Verify provenance constants are distinct and non-empty.
	values := []string{
		ProvenanceLiteral,
		ProvenanceEmbeddingSimilarity,
		ProvenanceModelAdjudication,
		ProvenanceModelInference,
	}
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		if v == "" {
			t.Error("provenance constant is empty")
		}
		if _, ok := seen[v]; ok {
			t.Errorf("duplicate provenance constant: %q", v)
		}
		seen[v] = struct{}{}
	}
}
