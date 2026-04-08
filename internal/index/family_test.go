package index

import (
	"testing"
)

func TestLouvain_SingleNode(t *testing.T) {
	t.Parallel()
	nodes := []string{"SPEC-001"}
	adj := map[string]map[string]float64{
		"SPEC-001": {},
	}

	community := louvain(nodes, adj)
	if len(community) != 1 {
		t.Fatalf("expected 1 community assignment, got %d", len(community))
	}
}

func TestLouvain_TwoClusters(t *testing.T) {
	t.Parallel()
	nodes := []string{"A", "B", "C", "D"}
	adj := map[string]map[string]float64{
		"A": {"B": 1.0},
		"B": {"A": 1.0},
		"C": {"D": 1.0},
		"D": {"C": 1.0},
	}

	community := louvain(nodes, adj)

	// A and B should be in same community, C and D should be in same community.
	if community["A"] != community["B"] {
		t.Errorf("A and B should be in same community: A=%d B=%d", community["A"], community["B"])
	}
	if community["C"] != community["D"] {
		t.Errorf("C and D should be in same community: C=%d D=%d", community["C"], community["D"])
	}
	if community["A"] == community["C"] {
		t.Error("A and C should be in different communities")
	}
}

func TestLouvain_FullyConnected(t *testing.T) {
	t.Parallel()
	nodes := []string{"A", "B", "C"}
	adj := map[string]map[string]float64{
		"A": {"B": 1.0, "C": 1.0},
		"B": {"A": 1.0, "C": 1.0},
		"C": {"A": 1.0, "B": 1.0},
	}

	community := louvain(nodes, adj)

	// All should be in the same community.
	if community["A"] != community["B"] || community["B"] != community["C"] {
		t.Errorf("fully connected graph should have one community: A=%d B=%d C=%d",
			community["A"], community["B"], community["C"])
	}
}

func TestLouvain_Disconnected(t *testing.T) {
	t.Parallel()
	nodes := []string{"A", "B", "C"}
	adj := map[string]map[string]float64{
		"A": {},
		"B": {},
		"C": {},
	}

	community := louvain(nodes, adj)
	// Disconnected nodes stay in their own communities.
	if community["A"] == community["B"] || community["B"] == community["C"] || community["A"] == community["C"] {
		t.Errorf("disconnected nodes should be in different communities: A=%d B=%d C=%d",
			community["A"], community["B"], community["C"])
	}
}

func TestComputeCohesion_FullyConnected(t *testing.T) {
	t.Parallel()
	adj := map[string]map[string]float64{
		"A": {"B": 1.0, "C": 1.0},
		"B": {"A": 1.0, "C": 1.0},
		"C": {"A": 1.0, "B": 1.0},
	}

	cohesion := computeCohesion([]string{"A", "B", "C"}, adj)
	if cohesion != 1.0 {
		t.Errorf("cohesion = %f, want 1.0", cohesion)
	}
}

func TestComputeCohesion_HalfConnected(t *testing.T) {
	t.Parallel()
	adj := map[string]map[string]float64{
		"A": {"B": 1.0},
		"B": {"A": 1.0},
		"C": {},
	}

	// 1 edge out of 3 possible = 0.333...
	cohesion := computeCohesion([]string{"A", "B", "C"}, adj)
	expected := 1.0 / 3.0
	if cohesion < expected-0.01 || cohesion > expected+0.01 {
		t.Errorf("cohesion = %f, want ~%f", cohesion, expected)
	}
}

func TestComputeCohesion_SingleMember(t *testing.T) {
	t.Parallel()
	adj := map[string]map[string]float64{"A": {}}
	cohesion := computeCohesion([]string{"A"}, adj)
	if cohesion != 1.0 {
		t.Errorf("single member cohesion = %f, want 1.0", cohesion)
	}
}

func TestBuildFamilyResult(t *testing.T) {
	t.Parallel()
	specRefs := []string{"SPEC-001", "SPEC-002", "SPEC-003"}
	community := map[string]int{
		"SPEC-001": 0,
		"SPEC-002": 0,
		"SPEC-003": 1,
	}
	adj := map[string]map[string]float64{
		"SPEC-001": {"SPEC-002": 1.0},
		"SPEC-002": {"SPEC-001": 1.0},
		"SPEC-003": {},
	}

	result := buildFamilyResult(specRefs, community, adj)

	if len(result.Families) != 2 {
		t.Fatalf("expected 2 families, got %d", len(result.Families))
	}
	if len(result.Assignments) != 3 {
		t.Fatalf("expected 3 assignments, got %d", len(result.Assignments))
	}

	// Family 0 should have 2 members, family 1 should have 1.
	familySizes := map[int]int{}
	for _, f := range result.Families {
		familySizes[f.ID] = f.Size
	}
	if familySizes[0] != 2 {
		t.Errorf("family 0 size = %d, want 2", familySizes[0])
	}
	if familySizes[1] != 1 {
		t.Errorf("family 1 size = %d, want 1", familySizes[1])
	}
}

func TestRenumberCommunities(t *testing.T) {
	t.Parallel()
	input := map[string]int{"A": 5, "B": 5, "C": 12}
	result := renumberCommunities(input)

	if result["A"] != result["B"] {
		t.Error("A and B should be in the same community")
	}
	if result["A"] == result["C"] {
		t.Error("A and C should be in different communities")
	}
	// IDs should be 0 and 1 (sequential).
	ids := map[int]bool{}
	for _, c := range result {
		ids[c] = true
	}
	if !ids[0] || !ids[1] || len(ids) != 2 {
		t.Errorf("communities not sequential: %v", result)
	}
}
