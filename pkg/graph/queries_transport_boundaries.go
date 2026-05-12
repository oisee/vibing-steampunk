package graph

import (
	"sort"
	"strings"
)

// TransportScope represents the set of objects contained in one or more transports.
// Used by AnalyzeTransportBoundaries to determine what's "inside" vs "outside".
type TransportScope struct {
	Label      string          // Display label (TR number, CR ID, etc.)
	Transports map[string]bool // TR numbers in scope
	Objects    map[string]bool // NodeIDs (TYPE:NAME) of objects in scope
	names      map[string]bool // Object names only (for fuzzy type matching)
}

// InScope returns true if the given node ID is part of the transport scope.
// Checks both full node ID (TYPE:NAME) and name-only, because the parser
// may create nodes with guessed types (TYPE:ZCL_FOO) that differ from the
// canonical TADIR type (CLAS:ZCL_FOO) used in the scope.
func (s *TransportScope) InScope(nodeID string) bool {
	if s.Objects[nodeID] {
		return true
	}
	// Lazy-build name index
	if s.names == nil {
		s.names = make(map[string]bool, len(s.Objects))
		for id := range s.Objects {
			if parts := strings.SplitN(id, ":", 2); len(parts) == 2 {
				s.names[parts[1]] = true
			}
		}
	}
	if parts := strings.SplitN(nodeID, ":", 2); len(parts) == 2 {
		return s.names[parts[1]]
	}
	return false
}

// TransportBoundaryEntry represents a dependency that crosses the transport boundary.
type TransportBoundaryEntry struct {
	SourceNodeID  string `json:"source_node_id"`
	SourceName    string `json:"source_name"`
	SourceType    string `json:"source_type"`
	SourcePackage string `json:"source_package,omitempty"`
	TargetNodeID  string `json:"target_node_id"`
	TargetName    string `json:"target_name"`
	TargetType    string `json:"target_type"`
	TargetPackage string `json:"target_package,omitempty"`
	EdgeKind      string `json:"edge_kind"`
	RefDetail     string `json:"ref_detail,omitempty"`
	Status        string `json:"status"` // MISSING, STANDARD, DYNAMIC
}

// TransportBoundaryReport is the result of a transport boundary analysis.
type TransportBoundaryReport struct {
	Scope         string                    `json:"scope"`
	ObjectCount   int                       `json:"object_count"`
	Missing       []TransportBoundaryEntry  `json:"missing"`        // Custom objects not in transport
	Standard      []TransportBoundaryEntry  `json:"standard"`       // SAP standard refs (informational)
	Dynamic       []TransportBoundaryEntry  `json:"dynamic"`        // Unresolved dynamic calls
	CrossPackage  []TransportBoundaryEntry  `json:"cross_package"`  // In-scope but different package
	Summary       TransportBoundarySummary  `json:"summary"`
}

// TransportBoundarySummary provides counts for the boundary report.
type TransportBoundarySummary struct {
	TotalDeps       int  `json:"total_deps"`
	InScope         int  `json:"in_scope"`
	InScopeSamePkg  int  `json:"in_scope_same_pkg"`
	InScopeCrossPkg int  `json:"in_scope_cross_pkg"`
	Missing         int  `json:"missing"`
	Standard        int  `json:"standard"`
	Dynamic         int  `json:"dynamic"`
	SelfConsistent  bool `json:"self_consistent"` // true if Missing == 0
}

// AnalyzeTransportBoundaries checks whether a transport set is self-consistent:
// do the objects it contains depend on anything NOT in the set?
//
// The graph must contain both structural edges (CALLS, REFERENCES, etc.)
// and the transport objects as nodes. Only forward (outgoing) structural
// edges from in-scope objects are analyzed.
//
// Returns a report classifying each external dependency as MISSING (custom,
// not in transport), STANDARD (SAP standard), or DYNAMIC (unresolved).
func AnalyzeTransportBoundaries(g *Graph, scope *TransportScope) *TransportBoundaryReport {
	report := &TransportBoundaryReport{
		Scope:       scope.Label,
		ObjectCount: len(scope.Objects),
	}

	// Track unique source→target pairs to avoid duplicates
	type depKey struct{ from, to string }
	seen := make(map[depKey]bool)

	for nodeID := range scope.Objects {
		for _, e := range g.OutEdges(nodeID) {
			// Skip transport/co-transport edges — we want structural deps only
			if e.Kind == EdgeInTransport || e.Kind == EdgeCoTransported || e.Kind == EdgeReadsConfig {
				continue
			}

			// Skip self-refs
			if e.To == nodeID {
				continue
			}

			key := depKey{nodeID, e.To}
			if seen[key] {
				continue
			}
			seen[key] = true

			report.Summary.TotalDeps++

			// In scope — classify as same-package or cross-package
			if scope.InScope(e.To) {
				report.Summary.InScope++

				// Check if it crosses a package boundary within the scope
				fromNode := g.GetNode(nodeID)
				toNode := g.GetNode(e.To)
				if fromNode != nil && toNode != nil && fromNode.Package != "" && toNode.Package != "" && fromNode.Package != toNode.Package {
					report.Summary.InScopeCrossPkg++
					entry := TransportBoundaryEntry{
						SourceNodeID:  nodeID,
						TargetNodeID:  e.To,
						SourceName:    fromNode.Name,
						SourceType:    fromNode.Type,
						SourcePackage: fromNode.Package,
						TargetName:    toNode.Name,
						TargetType:    toNode.Type,
						TargetPackage: toNode.Package,
						EdgeKind:      string(e.Kind),
						RefDetail:     e.RefDetail,
						Status:        "CROSS_PACKAGE",
					}
					report.CrossPackage = append(report.CrossPackage, entry)
				} else {
					report.Summary.InScopeSamePkg++
				}
				continue
			}

			// Build entry
			entry := TransportBoundaryEntry{
				SourceNodeID: nodeID,
				TargetNodeID: e.To,
				EdgeKind:     string(e.Kind),
				RefDetail:    e.RefDetail,
			}

			// Resolve node details
			if n := g.GetNode(nodeID); n != nil {
				entry.SourceName = n.Name
				entry.SourceType = n.Type
				entry.SourcePackage = n.Package
			}
			if n := g.GetNode(e.To); n != nil {
				entry.TargetName = n.Name
				entry.TargetType = n.Type
				entry.TargetPackage = n.Package
			}

			// Classify
			if e.Kind == EdgeDynamic {
				entry.Status = "DYNAMIC"
				report.Dynamic = append(report.Dynamic, entry)
				report.Summary.Dynamic++
			} else if IsStandardObject(entry.TargetName) {
				entry.Status = "STANDARD"
				report.Standard = append(report.Standard, entry)
				report.Summary.Standard++
			} else {
				entry.Status = "MISSING"
				report.Missing = append(report.Missing, entry)
				report.Summary.Missing++
			}
		}
	}

	report.Summary.SelfConsistent = len(report.Missing) == 0

	// Sort for stable output
	sortEntries := func(entries []TransportBoundaryEntry) {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].SourceNodeID != entries[j].SourceNodeID {
				return entries[i].SourceNodeID < entries[j].SourceNodeID
			}
			return entries[i].TargetNodeID < entries[j].TargetNodeID
		})
	}
	sortEntries(report.Missing)
	sortEntries(report.Standard)
	sortEntries(report.Dynamic)
	// Sort cross-package by target package for clean grouping
	sort.Slice(report.CrossPackage, func(i, j int) bool {
		if report.CrossPackage[i].TargetPackage != report.CrossPackage[j].TargetPackage {
			return report.CrossPackage[i].TargetPackage < report.CrossPackage[j].TargetPackage
		}
		if report.CrossPackage[i].SourcePackage != report.CrossPackage[j].SourcePackage {
			return report.CrossPackage[i].SourcePackage < report.CrossPackage[j].SourcePackage
		}
		return report.CrossPackage[i].SourceNodeID < report.CrossPackage[j].SourceNodeID
	})

	return report
}
