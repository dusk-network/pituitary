package index

import (
	"testing"
)

func TestComputeGovernanceDelta_NoChanges(t *testing.T) {
	t.Parallel()
	arts := []snapshotArtifact{{ref: "SPEC-001", title: "Auth", status: "accepted", domain: "auth"}}
	edges := []snapshotEdge{{fromRef: "SPEC-001", toRef: "code://a.go", edgeType: "applies_to", edgeSource: "manual", confidence: "extracted"}}

	delta := computeGovernanceDelta(arts, arts, edges, edges)
	if len(delta.AddedSpecs) != 0 || len(delta.RemovedSpecs) != 0 || len(delta.UpdatedSpecs) != 0 {
		t.Fatalf("expected no spec changes, got added=%d removed=%d updated=%d", len(delta.AddedSpecs), len(delta.RemovedSpecs), len(delta.UpdatedSpecs))
	}
	if len(delta.AddedEdges) != 0 || len(delta.RemovedEdges) != 0 {
		t.Fatalf("expected no edge changes, got added=%d removed=%d", len(delta.AddedEdges), len(delta.RemovedEdges))
	}
	if delta.Summary != "no governance changes" {
		t.Fatalf("unexpected summary: %s", delta.Summary)
	}
}

func TestComputeGovernanceDelta_AddedSpec(t *testing.T) {
	t.Parallel()
	oldArts := []snapshotArtifact{{ref: "SPEC-001", title: "Auth", status: "accepted", domain: "auth"}}
	newArts := []snapshotArtifact{
		{ref: "SPEC-001", title: "Auth", status: "accepted", domain: "auth"},
		{ref: "SPEC-002", title: "Rate Limiting", status: "draft", domain: "api"},
	}
	oldEdges := []snapshotEdge{{fromRef: "SPEC-001", toRef: "code://a.go", edgeType: "applies_to", edgeSource: "manual", confidence: "extracted"}}
	newEdges := []snapshotEdge{
		{fromRef: "SPEC-001", toRef: "code://a.go", edgeType: "applies_to", edgeSource: "manual", confidence: "extracted"},
		{fromRef: "SPEC-002", toRef: "code://b.go", edgeType: "applies_to", edgeSource: "inferred", confidence: "inferred"},
	}

	delta := computeGovernanceDelta(oldArts, newArts, oldEdges, newEdges)
	if len(delta.AddedSpecs) != 1 {
		t.Fatalf("expected 1 added spec, got %d", len(delta.AddedSpecs))
	}
	if delta.AddedSpecs[0].Ref != "SPEC-002" {
		t.Fatalf("expected added spec SPEC-002, got %s", delta.AddedSpecs[0].Ref)
	}
	if len(delta.AddedEdges) != 1 {
		t.Fatalf("expected 1 added edge, got %d", len(delta.AddedEdges))
	}
	if delta.AddedEdges[0].FromRef != "SPEC-002" || delta.AddedEdges[0].ToRef != "code://b.go" {
		t.Fatalf("unexpected added edge: %+v", delta.AddedEdges[0])
	}
}

func TestComputeGovernanceDelta_RemovedSpec(t *testing.T) {
	t.Parallel()
	oldArts := []snapshotArtifact{
		{ref: "SPEC-001", title: "Auth", status: "accepted", domain: "auth"},
		{ref: "SPEC-002", title: "Rate Limiting", status: "accepted", domain: "api"},
	}
	newArts := []snapshotArtifact{
		{ref: "SPEC-001", title: "Auth", status: "accepted", domain: "auth"},
	}
	oldEdges := []snapshotEdge{
		{fromRef: "SPEC-001", toRef: "code://a.go", edgeType: "applies_to", edgeSource: "manual", confidence: "extracted"},
		{fromRef: "SPEC-002", toRef: "code://b.go", edgeType: "applies_to", edgeSource: "manual", confidence: "extracted"},
	}
	newEdges := []snapshotEdge{
		{fromRef: "SPEC-001", toRef: "code://a.go", edgeType: "applies_to", edgeSource: "manual", confidence: "extracted"},
	}

	delta := computeGovernanceDelta(oldArts, newArts, oldEdges, newEdges)
	if len(delta.RemovedSpecs) != 1 || delta.RemovedSpecs[0].Ref != "SPEC-002" {
		t.Fatalf("expected 1 removed spec SPEC-002, got %+v", delta.RemovedSpecs)
	}
	if len(delta.RemovedEdges) != 1 || delta.RemovedEdges[0].FromRef != "SPEC-002" {
		t.Fatalf("expected 1 removed edge from SPEC-002, got %+v", delta.RemovedEdges)
	}
}

func TestComputeGovernanceDelta_UpdatedSpec(t *testing.T) {
	t.Parallel()
	oldArts := []snapshotArtifact{{ref: "SPEC-001", title: "Auth", status: "draft", domain: "auth"}}
	newArts := []snapshotArtifact{{ref: "SPEC-001", title: "Auth", status: "accepted", domain: "auth"}}

	delta := computeGovernanceDelta(oldArts, newArts, nil, nil)
	if len(delta.UpdatedSpecs) != 1 || delta.UpdatedSpecs[0].Status != "accepted" {
		t.Fatalf("expected 1 updated spec with status accepted, got %+v", delta.UpdatedSpecs)
	}
}

func TestComputeGovernanceDelta_SummaryFormat(t *testing.T) {
	t.Parallel()
	oldArts := []snapshotArtifact{}
	newArts := []snapshotArtifact{
		{ref: "SPEC-001", title: "A", status: "draft"},
		{ref: "SPEC-002", title: "B", status: "draft"},
	}
	oldEdges := []snapshotEdge{}
	newEdges := []snapshotEdge{
		{fromRef: "SPEC-001", toRef: "code://a.go", edgeType: "applies_to", edgeSource: "manual", confidence: "extracted"},
	}

	delta := computeGovernanceDelta(oldArts, newArts, oldEdges, newEdges)
	if delta.Summary != "2 spec(s) added, 1 edge(s) added" {
		t.Fatalf("unexpected summary: %q", delta.Summary)
	}
}

func TestComputeGovernanceDelta_SortOrder(t *testing.T) {
	t.Parallel()
	oldArts := []snapshotArtifact{}
	newArts := []snapshotArtifact{
		{ref: "SPEC-003"},
		{ref: "SPEC-001"},
		{ref: "SPEC-002"},
	}

	delta := computeGovernanceDelta(oldArts, newArts, nil, nil)
	for i := 1; i < len(delta.AddedSpecs); i++ {
		if delta.AddedSpecs[i].Ref < delta.AddedSpecs[i-1].Ref {
			t.Fatalf("added specs not sorted: %v", delta.AddedSpecs)
		}
	}
}
