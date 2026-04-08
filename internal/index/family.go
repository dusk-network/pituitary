package index

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

// SpecFamily represents a discovered governance community.
type SpecFamily struct {
	ID       int      `json:"id"`
	Members  []string `json:"members"`
	Size     int      `json:"size"`
	Cohesion float64  `json:"cohesion"`
	Label    string   `json:"label,omitempty"`
}

// FamilyAssignment stores the community assignment for one artifact.
type FamilyAssignment struct {
	Ref      string `json:"ref"`
	FamilyID int    `json:"family_id"`
}

// FamilyResult holds the full output of spec family discovery.
type FamilyResult struct {
	Families    []SpecFamily       `json:"families"`
	Assignments []FamilyAssignment `json:"assignments,omitempty"`
	Ungoverned  []string           `json:"ungoverned,omitempty"`
}

// DiscoverFamiliesContext runs community detection on the spec dependency graph.
func DiscoverFamiliesContext(ctx context.Context, dbPath string) (*FamilyResult, error) {
	db, err := OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return nil, fmt.Errorf("open index for family discovery: %w", err)
	}
	defer db.Close()

	return discoverFamiliesDBContext(ctx, db)
}

func discoverFamiliesDBContext(ctx context.Context, db *sql.DB) (*FamilyResult, error) {
	// Load all spec refs.
	specRefs, err := loadAllSpecRefsContext(ctx, db)
	if err != nil {
		return nil, err
	}
	if len(specRefs) == 0 {
		return &FamilyResult{}, nil
	}

	// Build undirected adjacency from edges.
	adj, err := buildSpecAdjacencyContext(ctx, db, specRefs)
	if err != nil {
		return nil, err
	}

	// Run Louvain community detection.
	communities := louvain(specRefs, adj)

	// Build result.
	result := buildFamilyResult(specRefs, communities, adj)

	// Find ungoverned code files: files in ast_cache not covered by any applies_to edge.
	result.Ungoverned, err = findUngovernedFilesContext(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("find ungoverned files: %w", err)
	}

	return result, nil
}

func loadAllSpecRefsContext(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT ref FROM artifacts WHERE kind = 'spec' ORDER BY ref`)
	if err != nil {
		return nil, fmt.Errorf("query spec refs: %w", err)
	}
	defer rows.Close()

	var refs []string
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

// buildSpecAdjacencyContext builds an undirected adjacency list from spec edges.
// Connections come from: depends_on, supersedes, relates_to (direct), and
// shared applies_to targets (specs that govern the same code file).
func buildSpecAdjacencyContext(ctx context.Context, db *sql.DB, specRefs []string) (map[string]map[string]float64, error) {
	adj := make(map[string]map[string]float64)
	for _, ref := range specRefs {
		adj[ref] = make(map[string]float64)
	}

	// Direct edges: depends_on, supersedes, relates_to between specs.
	rows, err := db.QueryContext(ctx, `
		SELECT e.from_ref, e.to_ref
		FROM edges e
		JOIN artifacts a1 ON a1.ref = e.from_ref AND a1.kind = 'spec'
		JOIN artifacts a2 ON a2.ref = e.to_ref AND a2.kind = 'spec'
		WHERE e.edge_type IN ('depends_on', 'supersedes', 'relates_to')
	`)
	if err != nil {
		return nil, fmt.Errorf("query spec-spec edges: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var from, to string
		if err := rows.Scan(&from, &to); err != nil {
			return nil, err
		}
		if _, ok := adj[from]; ok {
			adj[from][to] += 1.0
		}
		if _, ok := adj[to]; ok {
			adj[to][from] += 1.0
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Shared applies_to: specs governing the same code/config target.
	targetRows, err := db.QueryContext(ctx, `
		SELECT e1.from_ref, e2.from_ref
		FROM edges e1
		JOIN edges e2 ON e1.to_ref = e2.to_ref AND e1.from_ref < e2.from_ref
		JOIN artifacts a1 ON a1.ref = e1.from_ref AND a1.kind = 'spec'
		JOIN artifacts a2 ON a2.ref = e2.from_ref AND a2.kind = 'spec'
		WHERE e1.edge_type = 'applies_to' AND e2.edge_type = 'applies_to'
	`)
	if err != nil {
		return nil, fmt.Errorf("query shared applies_to: %w", err)
	}
	defer targetRows.Close()

	for targetRows.Next() {
		var from, to string
		if err := targetRows.Scan(&from, &to); err != nil {
			return nil, err
		}
		if _, ok := adj[from]; ok {
			adj[from][to] += 0.5 // shared target is a weaker signal
		}
		if _, ok := adj[to]; ok {
			adj[to][from] += 0.5
		}
	}
	if err := targetRows.Err(); err != nil {
		return nil, err
	}

	return adj, nil
}

// louvain runs a simplified Louvain community detection algorithm.
// Returns a map from node → community ID.
func louvain(nodes []string, adj map[string]map[string]float64) map[string]int {
	// Initialize each node in its own community.
	community := make(map[string]int, len(nodes))
	for i, node := range nodes {
		community[node] = i
	}

	// Compute total edge weight (2m in modularity formula).
	totalWeight := 0.0
	for _, neighbors := range adj {
		for _, w := range neighbors {
			totalWeight += w
		}
	}
	if totalWeight == 0 {
		return community
	}

	// Node degree (sum of weights).
	degree := make(map[string]float64, len(nodes))
	for node, neighbors := range adj {
		for _, w := range neighbors {
			degree[node] += w
		}
	}

	// Iterative optimization with a max iteration limit.
	const maxIterations = 100
	improved := true
	for iter := 0; improved && iter < maxIterations; iter++ {
		improved = false
		for _, node := range nodes {
			currentComm := community[node]
			bestComm := currentComm
			bestGain := 0.0

			// Compute community-level aggregates for neighbors.
			neighborComms := make(map[int]float64)
			for neighbor, w := range adj[node] {
				neighborComms[community[neighbor]] += w
			}

			// Try moving to each neighboring community.
			// Gain threshold to avoid oscillation on symmetric graphs.
			const minGain = 1e-6
			ki := degree[node]
			edgesToCurrent := neighborComms[currentComm]
			for comm, edgesIn := range neighborComms {
				if comm == currentComm {
					continue
				}
				sumTotNew := communityDegreeSum(community, degree, comm, node)
				sumTotOld := communityDegreeSum(community, degree, currentComm, node)
				// Net gain: benefit of joining new - cost of leaving current.
				gain := (edgesIn-edgesToCurrent)/totalWeight - ki*(sumTotNew-sumTotOld)/(totalWeight*totalWeight)
				if gain > bestGain+minGain {
					bestGain = gain
					bestComm = comm
				}
			}

			if bestComm != currentComm {
				community[node] = bestComm
				improved = true
			}
		}
	}

	// Renumber communities sequentially.
	return renumberCommunities(community)
}

func communityDegreeSum(community map[string]int, degree map[string]float64, commID int, excludeNode string) float64 {
	sum := 0.0
	for node, c := range community {
		if c == commID && node != excludeNode {
			sum += degree[node]
		}
	}
	return sum
}

func renumberCommunities(community map[string]int) map[string]int {
	unique := make(map[int]struct{})
	for _, c := range community {
		unique[c] = struct{}{}
	}

	originalIDs := make([]int, 0, len(unique))
	for c := range unique {
		originalIDs = append(originalIDs, c)
	}
	sort.Ints(originalIDs)

	remap := make(map[int]int, len(originalIDs))
	for next, c := range originalIDs {
		remap[c] = next
	}

	result := make(map[string]int, len(community))
	for node, c := range community {
		result[node] = remap[c]
	}
	return result
}

func buildFamilyResult(specRefs []string, community map[string]int, adj map[string]map[string]float64) *FamilyResult {
	// Group specs by community.
	familyMembers := make(map[int][]string)
	for _, ref := range specRefs {
		cid := community[ref]
		familyMembers[cid] = append(familyMembers[cid], ref)
	}

	// Build families with cohesion scores.
	var families []SpecFamily
	for cid, members := range familyMembers {
		sort.Strings(members)
		cohesion := computeCohesion(members, adj)
		families = append(families, SpecFamily{
			ID:       cid,
			Members:  members,
			Size:     len(members),
			Cohesion: cohesion,
		})
	}
	sort.Slice(families, func(i, j int) bool { return families[i].ID < families[j].ID })

	// Build assignments.
	assignments := make([]FamilyAssignment, 0, len(specRefs))
	for _, ref := range specRefs {
		assignments = append(assignments, FamilyAssignment{
			Ref:      ref,
			FamilyID: community[ref],
		})
	}

	return &FamilyResult{
		Families:    families,
		Assignments: assignments,
	}
}

// computeCohesion calculates intra-community edge density (unweighted).
// cohesion = actual_connected_pairs / possible_pairs, always in [0, 1].
func computeCohesion(members []string, adj map[string]map[string]float64) float64 {
	if len(members) < 2 {
		return 1.0
	}

	intraEdges := 0.0
	for i := 0; i < len(members); i++ {
		for j := i + 1; j < len(members); j++ {
			if neighbors, ok := adj[members[i]]; ok {
				if _, connected := neighbors[members[j]]; connected {
					intraEdges++
					continue
				}
			}
			if neighbors, ok := adj[members[j]]; ok {
				if _, connected := neighbors[members[i]]; connected {
					intraEdges++
				}
			}
		}
	}

	possibleEdges := float64(len(members)) * float64(len(members)-1) / 2
	return intraEdges / possibleEdges
}

func findUngovernedFilesContext(ctx context.Context, db *sql.DB) ([]string, error) {
	// Check if ast_cache table exists.
	var tableCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='ast_cache'`).Scan(&tableCount); err != nil {
		return nil, fmt.Errorf("check ast_cache table existence: %w", err)
	}
	if tableCount == 0 {
		return nil, nil
	}

	rows, err := db.QueryContext(ctx, `
		SELECT ac.path FROM ast_cache ac
		WHERE NOT EXISTS (
			SELECT 1 FROM edges e
			WHERE e.edge_type = 'applies_to'
			AND (e.to_ref = 'code://' || ac.path OR e.to_ref = 'config://' || ac.path)
		)
		ORDER BY ac.path
	`)
	if err != nil {
		return nil, fmt.Errorf("query ungoverned files: %w", err)
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}
