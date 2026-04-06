package index

import (
	"strings"
	"testing"

	"github.com/dusk-network/pituitary/internal/model"
)

func TestInspectRelationGraphDetectsDependsOnCycle(t *testing.T) {
	t.Parallel()

	status := InspectRelationGraph([]model.SpecRecord{
		specWithRelations("SPEC-100", model.Relation{Type: model.RelationDependsOn, Ref: "SPEC-101"}),
		specWithRelations("SPEC-101", model.Relation{Type: model.RelationDependsOn, Ref: "SPEC-100"}),
	})

	if got, want := status.State, "invalid"; got != want {
		t.Fatalf("state = %q, want %q", got, want)
	}
	if len(status.Findings) != 1 {
		t.Fatalf("findings = %+v, want one cycle finding", status.Findings)
	}
	if status.Findings[0].Code != "cycle_detected" || status.Findings[0].RelationType != string(model.RelationDependsOn) {
		t.Fatalf("finding = %+v, want depends_on cycle", status.Findings[0])
	}
	if !strings.Contains(status.Findings[0].Message, "SPEC-100 -> SPEC-101 -> SPEC-100") &&
		!strings.Contains(status.Findings[0].Message, "SPEC-101 -> SPEC-100 -> SPEC-101") {
		t.Fatalf("finding message = %q, want explicit cycle path", status.Findings[0].Message)
	}
}

func TestInspectRelationGraphDetectsSupersedesCycleAndContradictions(t *testing.T) {
	t.Parallel()

	status := InspectRelationGraph([]model.SpecRecord{
		specWithRelations("SPEC-200",
			model.Relation{Type: model.RelationSupersedes, Ref: "SPEC-201"},
			model.Relation{Type: model.RelationDependsOn, Ref: "SPEC-201"},
		),
		specWithRelations("SPEC-201",
			model.Relation{Type: model.RelationSupersedes, Ref: "SPEC-200"},
			model.Relation{Type: model.RelationDependsOn, Ref: "SPEC-200"},
		),
	})

	if got, want := status.State, "invalid"; got != want {
		t.Fatalf("state = %q, want %q", got, want)
	}
	codes := make(map[string]struct{}, len(status.Findings))
	for _, finding := range status.Findings {
		codes[finding.Code] = struct{}{}
	}
	for _, want := range []string{"cycle_detected", "contradictory_relation_pair", "supersedes_depends_on_conflict"} {
		if _, ok := codes[want]; !ok {
			t.Fatalf("findings = %+v, want %s", status.Findings, want)
		}
	}
}

func TestValidateRelationGraphReturnsStructuredError(t *testing.T) {
	t.Parallel()

	err := ValidateRelationGraph([]model.SpecRecord{
		specWithRelations("SPEC-300", model.Relation{Type: model.RelationDependsOn, Ref: "SPEC-300"}),
	})
	if err == nil {
		t.Fatal("ValidateRelationGraph() error = nil, want validation failure")
	}
	if !IsGraphValidationError(err) {
		t.Fatalf("ValidateRelationGraph() error = %T, want GraphValidationError", err)
	}
	if !strings.Contains(err.Error(), "SPEC-300 declares depends_on on itself") {
		t.Fatalf("ValidateRelationGraph() error = %q, want self-reference detail", err)
	}
}

func TestInspectRelationGraphAllowsMutualRelatesToWithoutCycle(t *testing.T) {
	t.Parallel()

	status := InspectRelationGraph([]model.SpecRecord{
		specWithRelations("SPEC-400",
			model.Relation{Type: model.RelationRelatesTo, Ref: "SPEC-401"},
		),
		specWithRelations("SPEC-401",
			model.Relation{Type: model.RelationRelatesTo, Ref: "SPEC-400"},
		),
	})

	if got, want := status.State, "valid"; got != want {
		t.Fatalf("state = %q, want %q; mutual relates_to should not trigger cycle detection", got, want)
	}
}

func TestInspectRelationGraphAllowsRelatesToAlongsideDependsOn(t *testing.T) {
	t.Parallel()

	status := InspectRelationGraph([]model.SpecRecord{
		specWithRelations("SPEC-500",
			model.Relation{Type: model.RelationDependsOn, Ref: "SPEC-501"},
			model.Relation{Type: model.RelationRelatesTo, Ref: "SPEC-502"},
		),
		specWithRelations("SPEC-501"),
		specWithRelations("SPEC-502"),
	})

	if got, want := status.State, "valid"; got != want {
		t.Fatalf("state = %q, want %q; relates_to should not conflict with depends_on", got, want)
	}
}

func TestInspectRelationGraphDetectsRelatesToSelfReference(t *testing.T) {
	t.Parallel()

	status := InspectRelationGraph([]model.SpecRecord{
		specWithRelations("SPEC-600",
			model.Relation{Type: model.RelationRelatesTo, Ref: "SPEC-600"},
		),
	})

	if got, want := status.State, "invalid"; got != want {
		t.Fatalf("state = %q, want %q; self-referencing relates_to should be invalid", got, want)
	}
	if len(status.Findings) != 1 || status.Findings[0].Code != "self_reference" {
		t.Fatalf("findings = %+v, want one self_reference finding", status.Findings)
	}
}

func specWithRelations(ref string, relations ...model.Relation) model.SpecRecord {
	return model.SpecRecord{
		Ref:       ref,
		Kind:      model.ArtifactKindSpec,
		Title:     ref,
		Status:    model.StatusAccepted,
		Relations: relations,
	}
}
