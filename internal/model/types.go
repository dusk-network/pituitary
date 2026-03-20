package model

const (
	ArtifactKindSpec = "spec"
	ArtifactKindDoc  = "doc"

	BodyFormatMarkdown = "markdown"

	StatusDraft      = "draft"
	StatusReview     = "review"
	StatusAccepted   = "accepted"
	StatusSuperseded = "superseded"
	StatusDeprecated = "deprecated"
)

// RelationType identifies a graph edge between canonical artifacts.
type RelationType string

const (
	RelationDependsOn  RelationType = "depends_on"
	RelationSupersedes RelationType = "supersedes"
)

// Relation represents one explicit edge emitted by a normalized spec record.
type Relation struct {
	Type RelationType `json:"type"`
	Ref  string       `json:"ref"`
}

// SpecRecord is the canonical v1 representation of a spec bundle.
type SpecRecord struct {
	Ref         string            `json:"ref"`
	Kind        string            `json:"kind"`
	Title       string            `json:"title"`
	Status      string            `json:"status"`
	Domain      string            `json:"domain"`
	Authors     []string          `json:"authors,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Relations   []Relation        `json:"relations,omitempty"`
	AppliesTo   []string          `json:"applies_to,omitempty"`
	SourceRef   string            `json:"source_ref"`
	BodyFormat  string            `json:"body_format"`
	BodyText    string            `json:"body_text"`
	ContentHash string            `json:"content_hash,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// DocRecord is the canonical v1 representation of one markdown document.
type DocRecord struct {
	Ref         string            `json:"ref"`
	Kind        string            `json:"kind"`
	Title       string            `json:"title"`
	SourceRef   string            `json:"source_ref"`
	BodyFormat  string            `json:"body_format"`
	BodyText    string            `json:"body_text"`
	ContentHash string            `json:"content_hash,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}
