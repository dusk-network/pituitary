package index

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dusk-network/pituitary/internal/model"
)

// RelationGraphFinding reports one invalid graph condition.
type RelationGraphFinding struct {
	Code         string   `json:"code"`
	RelationType string   `json:"relation_type,omitempty"`
	Refs         []string `json:"refs,omitempty"`
	Message      string   `json:"message"`
}

// RelationGraphStatus reports whether the current spec graph is structurally valid.
type RelationGraphStatus struct {
	State    string                 `json:"state"`
	Findings []RelationGraphFinding `json:"findings,omitempty"`
}

// GraphValidationError reports invalid relation-graph findings.
type GraphValidationError struct {
	Findings []RelationGraphFinding
}

func (e *GraphValidationError) Error() string {
	if e == nil || len(e.Findings) == 0 {
		return "relation graph is invalid"
	}

	lines := make([]string, 0, len(e.Findings)+1)
	lines = append(lines, "relation graph is invalid:")
	for _, finding := range e.Findings {
		lines = append(lines, "- "+finding.Message)
	}
	return strings.Join(lines, "\n")
}

// IsGraphValidationError reports whether err is a graph-validation error.
func IsGraphValidationError(err error) bool {
	_, ok := err.(*GraphValidationError)
	return ok
}

// InspectRelationGraph validates spec relations and returns a structured status.
func InspectRelationGraph(specs []model.SpecRecord) *RelationGraphStatus {
	findings := relationGraphFindings(specs)
	status := &RelationGraphStatus{State: "valid"}
	if len(findings) > 0 {
		status.State = "invalid"
		status.Findings = findings
	}
	return status
}

// ValidateRelationGraph returns an error when the spec graph is invalid.
func ValidateRelationGraph(specs []model.SpecRecord) error {
	status := InspectRelationGraph(specs)
	if status.State == "valid" {
		return nil
	}
	return &GraphValidationError{Findings: status.Findings}
}

func relationGraphFindings(specs []model.SpecRecord) []RelationGraphFinding {
	specRefs := make(map[string]struct{}, len(specs))
	edgeKinds := make(map[string]map[string]map[model.RelationType]struct{}, len(specs))
	dependsOn := make(map[string][]string, len(specs))
	supersedes := make(map[string][]string, len(specs))
	findings := make([]RelationGraphFinding, 0, 8)
	seenFinding := map[string]struct{}{}

	for _, spec := range specs {
		specRefs[spec.Ref] = struct{}{}
	}
	for _, spec := range specs {
		targetKinds := edgeKinds[spec.Ref]
		if targetKinds == nil {
			targetKinds = make(map[string]map[model.RelationType]struct{})
			edgeKinds[spec.Ref] = targetKinds
		}

		for _, relation := range spec.Relations {
			if relation.Ref == "" {
				continue
			}
			if relation.Ref == spec.Ref {
				appendRelationFinding(&findings, seenFinding, RelationGraphFinding{
					Code:         "self_reference",
					RelationType: string(relation.Type),
					Refs:         []string{spec.Ref},
					Message:      fmt.Sprintf("%s declares %s on itself", spec.Ref, relation.Type),
				})
			}

			types := targetKinds[relation.Ref]
			if types == nil {
				types = make(map[model.RelationType]struct{})
				targetKinds[relation.Ref] = types
			}
			types[relation.Type] = struct{}{}

			if relation.Type == model.RelationDependsOn {
				appendUniqueEdge(dependsOn, spec.Ref, relation.Ref, specRefs)
			}
			if relation.Type == model.RelationSupersedes {
				appendUniqueEdge(supersedes, spec.Ref, relation.Ref, specRefs)
			}
		}
	}

	for fromRef, targets := range edgeKinds {
		for toRef, types := range targets {
			if hasRelationType(types, model.RelationDependsOn) && hasRelationType(types, model.RelationSupersedes) {
				appendRelationFinding(&findings, seenFinding, RelationGraphFinding{
					Code:         "contradictory_relation_pair",
					RelationType: "depends_on+supersedes",
					Refs:         []string{fromRef, toRef},
					Message:      fmt.Sprintf("%s declares both depends_on and supersedes for %s", fromRef, toRef),
				})
			}
			if hasRelationType(types, model.RelationSupersedes) && hasEdge(dependsOn, toRef, fromRef) {
				appendRelationFinding(&findings, seenFinding, RelationGraphFinding{
					Code:         "supersedes_depends_on_conflict",
					RelationType: "supersedes+depends_on",
					Refs:         []string{fromRef, toRef},
					Message:      fmt.Sprintf("%s supersedes %s while %s depends_on %s", fromRef, toRef, toRef, fromRef),
				})
			}
		}
	}

	findings = append(findings, detectRelationCycles(model.RelationDependsOn, dependsOn)...)
	findings = append(findings, detectRelationCycles(model.RelationSupersedes, supersedes)...)
	sort.Slice(findings, func(i, j int) bool {
		switch {
		case findings[i].Code != findings[j].Code:
			return findings[i].Code < findings[j].Code
		case findings[i].RelationType != findings[j].RelationType:
			return findings[i].RelationType < findings[j].RelationType
		default:
			return strings.Join(findings[i].Refs, ",") < strings.Join(findings[j].Refs, ",")
		}
	})
	return findings
}

func appendUniqueEdge(edges map[string][]string, fromRef, toRef string, specRefs map[string]struct{}) {
	if _, ok := specRefs[toRef]; !ok {
		return
	}
	if hasEdge(edges, fromRef, toRef) {
		return
	}
	edges[fromRef] = append(edges[fromRef], toRef)
	sort.Strings(edges[fromRef])
}

func hasEdge(edges map[string][]string, fromRef, toRef string) bool {
	for _, candidate := range edges[fromRef] {
		if candidate == toRef {
			return true
		}
	}
	return false
}

func hasRelationType(types map[model.RelationType]struct{}, relationType model.RelationType) bool {
	_, ok := types[relationType]
	return ok
}

func appendRelationFinding(findings *[]RelationGraphFinding, seen map[string]struct{}, finding RelationGraphFinding) {
	key := finding.Code + "|" + finding.RelationType + "|" + strings.Join(finding.Refs, ",")
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*findings = append(*findings, finding)
}

func detectRelationCycles(relationType model.RelationType, adjacency map[string][]string) []RelationGraphFinding {
	visited := make(map[string]int, len(adjacency))
	stack := make([]string, 0, len(adjacency))
	stackIndex := make(map[string]int, len(adjacency))
	seenCycles := map[string]struct{}{}
	findings := make([]RelationGraphFinding, 0, 2)

	var visit func(string)
	visit = func(ref string) {
		visited[ref] = 1
		stackIndex[ref] = len(stack)
		stack = append(stack, ref)

		for _, next := range adjacency[ref] {
			switch visited[next] {
			case 0:
				visit(next)
			case 1:
				refs := append([]string{}, stack[stackIndex[next]:]...)
				refs = append(refs, next)
				key := relationTypeCycleKey(relationType, refs)
				if _, ok := seenCycles[key]; ok {
					continue
				}
				seenCycles[key] = struct{}{}
				findings = append(findings, RelationGraphFinding{
					Code:         "cycle_detected",
					RelationType: string(relationType),
					Refs:         refs,
					Message:      fmt.Sprintf("%s cycle detected: %s", relationType, strings.Join(refs, " -> ")),
				})
			}
		}

		stack = stack[:len(stack)-1]
		delete(stackIndex, ref)
		visited[ref] = 2
	}

	nodes := make([]string, 0, len(adjacency))
	for ref := range adjacency {
		nodes = append(nodes, ref)
	}
	sort.Strings(nodes)
	for _, ref := range nodes {
		if visited[ref] == 0 {
			visit(ref)
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		return strings.Join(findings[i].Refs, ",") < strings.Join(findings[j].Refs, ",")
	})
	return findings
}

func relationTypeCycleKey(relationType model.RelationType, refs []string) string {
	if len(refs) == 0 {
		return string(relationType)
	}
	cycle := append([]string{}, refs...)
	if len(cycle) > 1 && cycle[0] == cycle[len(cycle)-1] {
		cycle = cycle[:len(cycle)-1]
	}
	if len(cycle) == 0 {
		return string(relationType)
	}

	best := strings.Join(cycle, ",")
	for i := 1; i < len(cycle); i++ {
		rotated := append(append([]string{}, cycle[i:]...), cycle[:i]...)
		candidate := strings.Join(rotated, ",")
		if candidate < best {
			best = candidate
		}
	}
	return string(relationType) + "|" + best
}
