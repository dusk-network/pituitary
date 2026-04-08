package index

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

// GovernanceDelta reports the governance posture changes between the old and
// new index state after an incremental update.
type GovernanceDelta struct {
	AddedSpecs   []DeltaSpec `json:"added_specs,omitempty"`
	RemovedSpecs []DeltaSpec `json:"removed_specs,omitempty"`
	UpdatedSpecs []DeltaSpec `json:"updated_specs,omitempty"`
	AddedEdges   []DeltaEdge `json:"added_edges,omitempty"`
	RemovedEdges []DeltaEdge `json:"removed_edges,omitempty"`
	Summary      string      `json:"summary"`
}

// DeltaSpec reports an artifact change in the governance delta.
type DeltaSpec struct {
	Ref    string `json:"ref"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"`
	Domain string `json:"domain,omitempty"`
}

// DeltaEdge reports a single governance edge change.
type DeltaEdge struct {
	FromRef    string `json:"from_ref"`
	ToRef      string `json:"to_ref"`
	EdgeType   string `json:"edge_type"`
	EdgeSource string `json:"edge_source"`
	Confidence string `json:"confidence,omitempty"`
}

// edgeKey is the identity of an edge for diff purposes.
type edgeKey struct {
	fromRef  string
	toRef    string
	edgeType string
}

// snapshotEdge holds one edge row from the database.
type snapshotEdge struct {
	fromRef    string
	toRef      string
	edgeType   string
	edgeSource string
	confidence string
}

// snapshotArtifact holds one artifact's governance-relevant metadata.
type snapshotArtifact struct {
	ref    string
	title  string
	status string
	domain string
}

// snapshotEdgesContext loads all edges from the current database state.
func snapshotEdgesContext(ctx context.Context, db *sql.DB) ([]snapshotEdge, error) {
	rows, err := db.QueryContext(ctx, `SELECT from_ref, to_ref, edge_type, edge_source, confidence FROM edges`)
	if err != nil {
		return nil, fmt.Errorf("snapshot edges: %w", err)
	}
	defer rows.Close()

	var edges []snapshotEdge
	for rows.Next() {
		var e snapshotEdge
		if err := rows.Scan(&e.fromRef, &e.toRef, &e.edgeType, &e.edgeSource, &e.confidence); err != nil {
			return nil, fmt.Errorf("scan snapshot edge: %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// snapshotSpecArtifactsContext loads spec artifact metadata from the current database state.
func snapshotSpecArtifactsContext(ctx context.Context, db *sql.DB) ([]snapshotArtifact, error) {
	rows, err := db.QueryContext(ctx, `SELECT ref, COALESCE(title,''), COALESCE(status,''), COALESCE(domain,'') FROM artifacts WHERE kind = 'spec'`)
	if err != nil {
		return nil, fmt.Errorf("snapshot spec artifacts: %w", err)
	}
	defer rows.Close()

	var artifacts []snapshotArtifact
	for rows.Next() {
		var a snapshotArtifact
		if err := rows.Scan(&a.ref, &a.title, &a.status, &a.domain); err != nil {
			return nil, fmt.Errorf("scan snapshot artifact: %w", err)
		}
		artifacts = append(artifacts, a)
	}
	return artifacts, rows.Err()
}

// snapshotEdgesTxContext loads all edges from within a transaction.
func snapshotEdgesTxContext(ctx context.Context, tx *sql.Tx) ([]snapshotEdge, error) {
	rows, err := tx.QueryContext(ctx, `SELECT from_ref, to_ref, edge_type, edge_source, confidence FROM edges`)
	if err != nil {
		return nil, fmt.Errorf("snapshot edges (tx): %w", err)
	}
	defer rows.Close()

	var edges []snapshotEdge
	for rows.Next() {
		var e snapshotEdge
		if err := rows.Scan(&e.fromRef, &e.toRef, &e.edgeType, &e.edgeSource, &e.confidence); err != nil {
			return nil, fmt.Errorf("scan snapshot edge (tx): %w", err)
		}
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

// snapshotSpecArtifactsTxContext loads spec artifact metadata from within a transaction.
func snapshotSpecArtifactsTxContext(ctx context.Context, tx *sql.Tx) ([]snapshotArtifact, error) {
	rows, err := tx.QueryContext(ctx, `SELECT ref, COALESCE(title,''), COALESCE(status,''), COALESCE(domain,'') FROM artifacts WHERE kind = 'spec'`)
	if err != nil {
		return nil, fmt.Errorf("snapshot spec artifacts (tx): %w", err)
	}
	defer rows.Close()

	var artifacts []snapshotArtifact
	for rows.Next() {
		var a snapshotArtifact
		if err := rows.Scan(&a.ref, &a.title, &a.status, &a.domain); err != nil {
			return nil, fmt.Errorf("scan snapshot artifact (tx): %w", err)
		}
		artifacts = append(artifacts, a)
	}
	return artifacts, rows.Err()
}

// computeGovernanceDelta computes the governance changes between old and new states.
func computeGovernanceDelta(
	oldArtifacts []snapshotArtifact, newArtifacts []snapshotArtifact,
	oldEdges []snapshotEdge, newEdges []snapshotEdge,
) *GovernanceDelta {
	delta := &GovernanceDelta{}

	// Diff artifacts.
	oldArtMap := make(map[string]snapshotArtifact, len(oldArtifacts))
	for _, a := range oldArtifacts {
		oldArtMap[a.ref] = a
	}
	newArtMap := make(map[string]snapshotArtifact, len(newArtifacts))
	for _, a := range newArtifacts {
		newArtMap[a.ref] = a
	}

	for _, a := range newArtifacts {
		old, existed := oldArtMap[a.ref]
		if !existed {
			delta.AddedSpecs = append(delta.AddedSpecs, DeltaSpec{
				Ref: a.ref, Title: a.title, Status: a.status, Domain: a.domain,
			})
		} else if old.status != a.status || old.domain != a.domain || old.title != a.title {
			delta.UpdatedSpecs = append(delta.UpdatedSpecs, DeltaSpec{
				Ref: a.ref, Title: a.title, Status: a.status, Domain: a.domain,
			})
		}
	}
	for _, a := range oldArtifacts {
		if _, exists := newArtMap[a.ref]; !exists {
			delta.RemovedSpecs = append(delta.RemovedSpecs, DeltaSpec{
				Ref: a.ref, Title: a.title, Status: a.status, Domain: a.domain,
			})
		}
	}

	// Diff edges.
	oldEdgeSet := make(map[edgeKey]snapshotEdge, len(oldEdges))
	for _, e := range oldEdges {
		oldEdgeSet[edgeKey{e.fromRef, e.toRef, e.edgeType}] = e
	}
	newEdgeSet := make(map[edgeKey]snapshotEdge, len(newEdges))
	for _, e := range newEdges {
		newEdgeSet[edgeKey{e.fromRef, e.toRef, e.edgeType}] = e
	}

	for k, e := range newEdgeSet {
		if _, existed := oldEdgeSet[k]; !existed {
			delta.AddedEdges = append(delta.AddedEdges, DeltaEdge{
				FromRef: e.fromRef, ToRef: e.toRef, EdgeType: e.edgeType,
				EdgeSource: e.edgeSource, Confidence: e.confidence,
			})
		}
	}
	for k, e := range oldEdgeSet {
		if _, exists := newEdgeSet[k]; !exists {
			delta.RemovedEdges = append(delta.RemovedEdges, DeltaEdge{
				FromRef: e.fromRef, ToRef: e.toRef, EdgeType: e.edgeType,
				EdgeSource: e.edgeSource, Confidence: e.confidence,
			})
		}
	}

	// Sort for deterministic output.
	sort.Slice(delta.AddedSpecs, func(i, j int) bool { return delta.AddedSpecs[i].Ref < delta.AddedSpecs[j].Ref })
	sort.Slice(delta.RemovedSpecs, func(i, j int) bool { return delta.RemovedSpecs[i].Ref < delta.RemovedSpecs[j].Ref })
	sort.Slice(delta.UpdatedSpecs, func(i, j int) bool { return delta.UpdatedSpecs[i].Ref < delta.UpdatedSpecs[j].Ref })
	sort.Slice(delta.AddedEdges, func(i, j int) bool { return deltaEdgeLess(delta.AddedEdges[i], delta.AddedEdges[j]) })
	sort.Slice(delta.RemovedEdges, func(i, j int) bool { return deltaEdgeLess(delta.RemovedEdges[i], delta.RemovedEdges[j]) })

	delta.Summary = formatDeltaSummary(delta)
	return delta
}

func deltaEdgeLess(a, b DeltaEdge) bool {
	if a.FromRef != b.FromRef {
		return a.FromRef < b.FromRef
	}
	if a.ToRef != b.ToRef {
		return a.ToRef < b.ToRef
	}
	return a.EdgeType < b.EdgeType
}

func formatDeltaSummary(delta *GovernanceDelta) string {
	parts := make([]string, 0, 6)
	if n := len(delta.AddedSpecs); n > 0 {
		parts = append(parts, fmt.Sprintf("%d spec(s) added", n))
	}
	if n := len(delta.RemovedSpecs); n > 0 {
		parts = append(parts, fmt.Sprintf("%d spec(s) removed", n))
	}
	if n := len(delta.UpdatedSpecs); n > 0 {
		parts = append(parts, fmt.Sprintf("%d spec(s) updated", n))
	}
	if n := len(delta.AddedEdges); n > 0 {
		parts = append(parts, fmt.Sprintf("%d edge(s) added", n))
	}
	if n := len(delta.RemovedEdges); n > 0 {
		parts = append(parts, fmt.Sprintf("%d edge(s) removed", n))
	}
	if len(parts) == 0 {
		return "no governance changes"
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += ", " + parts[i]
	}
	return result
}
