package graph

import "sort"

// CoChangeEntry represents an object that frequently co-changes with the target.
type CoChangeEntry struct {
	NodeID     string   `json:"node_id"`
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Package    string   `json:"package,omitempty"`
	Count      int      `json:"count"`      // Number of shared transports
	Transports []string `json:"transports"` // Which TR IDs they share
}

// CoChangeResult is the result of WhatChangesWith.
type CoChangeResult struct {
	Target          string          `json:"target"`
	TotalTransports int             `json:"total_transports"`
	CoChanges       []CoChangeEntry `json:"co_changes"`
}

// WhatChangesWith finds objects that frequently appear in the same transports
// as the target object. Co-change is computed at query time from IN_TRANSPORT
// edges — no derived edges are stored.
//
// Algorithm:
//  1. From target, follow OutEdges(IN_TRANSPORT) → find all TR nodes
//  2. For each TR, follow InEdges(IN_TRANSPORT) → find all co-occurring objects
//  3. Count how many transports each object shares with target
//  4. Sort by frequency descending, limit to topN (0 = unlimited)
//
// The target object is excluded from results.
func WhatChangesWith(g *Graph, targetNodeID string, topN int) *CoChangeResult {
	result := &CoChangeResult{
		Target: targetNodeID,
	}

	// Step 1: find all transports containing the target
	var transportIDs []string
	for _, e := range g.OutEdges(targetNodeID) {
		if e.Kind == EdgeInTransport {
			transportIDs = append(transportIDs, e.To)
		}
	}
	result.TotalTransports = len(transportIDs)

	if len(transportIDs) == 0 {
		return result
	}

	// Step 2+3: for each transport, collect co-occurring objects and count
	type coEntry struct {
		transports map[string]bool
	}
	coMap := make(map[string]*coEntry)

	for _, trID := range transportIDs {
		for _, e := range g.InEdges(trID) {
			if e.Kind != EdgeInTransport {
				continue
			}
			// Skip the target itself
			if e.From == targetNodeID {
				continue
			}

			ce, ok := coMap[e.From]
			if !ok {
				ce = &coEntry{transports: make(map[string]bool)}
				coMap[e.From] = ce
			}
			ce.transports[trID] = true
		}
	}

	// Step 4: build result entries
	entries := make([]CoChangeEntry, 0, len(coMap))
	for nodeID, ce := range coMap {
		entry := CoChangeEntry{
			NodeID: nodeID,
			Count:  len(ce.transports),
		}

		// Resolve node details
		if n := g.GetNode(nodeID); n != nil {
			entry.Name = n.Name
			entry.Type = n.Type
			entry.Package = n.Package
		}

		// Collect transport IDs
		trIDs := make([]string, 0, len(ce.transports))
		for trID := range ce.transports {
			trIDs = append(trIDs, trID)
		}
		sort.Strings(trIDs)
		entry.Transports = trIDs

		entries = append(entries, entry)
	}

	// Sort by count descending, then by node ID for stability
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		return entries[i].NodeID < entries[j].NodeID
	})

	// Limit
	if topN > 0 && len(entries) > topN {
		entries = entries[:topN]
	}

	result.CoChanges = entries
	return result
}
