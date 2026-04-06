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

type Relation = sdk.Relation
type InferenceFieldConfidence = sdk.InferenceFieldConfidence
type InferenceConfidence = sdk.InferenceConfidence
type SpecRecord = sdk.SpecRecord
type DocRecord = sdk.DocRecord
