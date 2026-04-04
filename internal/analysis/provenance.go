package analysis

// Provenance labels identify the source of a finding, enabling downstream
// consumers to filter or weight findings by their origin.
const (
	// ProvenanceLiteral indicates a finding produced by deterministic string
	// or pattern matching.
	ProvenanceLiteral = "literal"

	// ProvenanceEmbeddingSimilarity indicates a finding produced by
	// embedding-based similarity search.
	ProvenanceEmbeddingSimilarity = "embedding_similarity"

	// ProvenanceModelAdjudication indicates a finding produced by a
	// bounded analysis model invocation.
	ProvenanceModelAdjudication = "model_adjudication"

	// ProvenanceModelInference indicates a metadata field inferred by
	// the analysis model from body text.
	ProvenanceModelInference = "model_inference"
)
