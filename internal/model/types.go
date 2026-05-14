package model

import "github.com/dusk-network/pituitary/sdk"

const (
	ArtifactKindSpec = sdk.ArtifactKindSpec
	ArtifactKindDoc  = sdk.ArtifactKindDoc

	BodyFormatMarkdown = sdk.BodyFormatMarkdown

	StatusDraft      = sdk.StatusDraft
	StatusReview     = sdk.StatusReview
	StatusAccepted   = sdk.StatusAccepted
	StatusSuperseded = sdk.StatusSuperseded
	StatusDeprecated = sdk.StatusDeprecated
)

type RelationType = sdk.RelationType

const (
	RelationDependsOn  = sdk.RelationDependsOn
	RelationSupersedes = sdk.RelationSupersedes
	RelationRelatesTo  = sdk.RelationRelatesTo
)

type (
	Relation                 = sdk.Relation
	InferenceFieldConfidence = sdk.InferenceFieldConfidence
	InferenceConfidence      = sdk.InferenceConfidence
	SpecRecord               = sdk.SpecRecord
	DocRecord                = sdk.DocRecord
)
