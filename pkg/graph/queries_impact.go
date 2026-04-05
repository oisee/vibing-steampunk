package graph

// ImpactEntry represents a node reached during reverse-dependency traversal.
type ImpactEntry struct {
	NodeID  string   `json:"node_id"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Package string   `json:"package,omitempty"`
	Depth   int      `json:"depth"`           // Hops from root (1 = direct dependent)
	ViaEdge EdgeKind `json:"via_edge"`        // Edge kind that led here
	ViaFrom string   `json:"via_from"`        // Predecessor node ID in the path
}

// ImpactResult is the result of reverse-dependency impact analysis.
type ImpactResult struct {
	Root    string        `json:"root"`
	MaxDepth int          `json:"max_depth"`
	Entries []ImpactEntry `json:"entries"`
}

// ImpactOptions configures the impact query.
type ImpactOptions struct {
	MaxDepth  int        // Maximum traversal depth (default 3, min 1)
	EdgeKinds []EdgeKind // If non-empty, only traverse these edge kinds
}

// Impact performs reverse-dependency traversal from a root node.
// It follows InEdges (who depends on this node?) via BFS up to MaxDepth hops.
//
// The traversal crosses whatever edge kinds exist in the graph — if code edges
// and transport edges are both present, BFS will follow both. Use EdgeKinds
// filter to restrict to specific layers.
//
// The root node itself is NOT included in results.
// Cycles are handled via visited set — each node appears at most once,
// at the shallowest depth it was first reached.
func Impact(g *Graph, rootNodeID string, opts *ImpactOptions) *ImpactResult {
	maxDepth := 3
	if opts != nil && opts.MaxDepth > 0 {
		maxDepth = opts.MaxDepth
	}

	result := &ImpactResult{
		Root:     rootNodeID,
		MaxDepth: maxDepth,
	}

	// Build edge kind filter set
	var kindFilter map[EdgeKind]bool
	if opts != nil && len(opts.EdgeKinds) > 0 {
		kindFilter = make(map[EdgeKind]bool, len(opts.EdgeKinds))
		for _, k := range opts.EdgeKinds {
			kindFilter[k] = true
		}
	}

	// BFS state
	type bfsItem struct {
		nodeID  string
		depth   int
		viaEdge EdgeKind
		viaFrom string
	}

	visited := make(map[string]bool)
	visited[rootNodeID] = true

	queue := make([]bfsItem, 0, 32)

	// Seed: all InEdges of root
	for _, e := range g.InEdges(rootNodeID) {
		if kindFilter != nil && !kindFilter[e.Kind] {
			continue
		}
		if !visited[e.From] {
			visited[e.From] = true
			queue = append(queue, bfsItem{
				nodeID:  e.From,
				depth:   1,
				viaEdge: e.Kind,
				viaFrom: rootNodeID,
			})
		}
	}

	// BFS loop
	for i := 0; i < len(queue); i++ {
		item := queue[i]

		// Build entry
		entry := ImpactEntry{
			NodeID:  item.nodeID,
			Depth:   item.depth,
			ViaEdge: item.viaEdge,
			ViaFrom: item.viaFrom,
		}
		if n := g.GetNode(item.nodeID); n != nil {
			entry.Name = n.Name
			entry.Type = n.Type
			entry.Package = n.Package
		}
		result.Entries = append(result.Entries, entry)

		// Expand further if not at max depth
		if item.depth < maxDepth {
			for _, e := range g.InEdges(item.nodeID) {
				if kindFilter != nil && !kindFilter[e.Kind] {
					continue
				}
				if !visited[e.From] {
					visited[e.From] = true
					queue = append(queue, bfsItem{
						nodeID:  e.From,
						depth:   item.depth + 1,
						viaEdge: e.Kind,
						viaFrom: item.nodeID,
					})
				}
			}
		}
	}

	return result
}
