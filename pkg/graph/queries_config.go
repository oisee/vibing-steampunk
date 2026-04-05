package graph

import (
	"sort"
	"strings"
)

// ConfigReaderEntry represents an object that reads a TVARVC variable.
type ConfigReaderEntry struct {
	NodeID     string `json:"node_id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Package    string `json:"package,omitempty"`
	Confidence string `json:"confidence"` // Best confidence across evidence: HIGH or MEDIUM
	EdgeCount  int    `json:"edge_count"` // Number of READS_CONFIG edges (evidence pieces)
}

// ConfigUsageResult is the result of WhereUsedConfig.
type ConfigUsageResult struct {
	Variable     string              `json:"variable"`      // Canonical node ID (TVARVC:ZVAR)
	VariableName string              `json:"variable_name"` // Plain name (ZVAR)
	Found        bool                `json:"found"`         // Whether the TVARVC node exists in graph
	Readers      []ConfigReaderEntry `json:"readers"`
}

// WhereUsedConfig finds objects that read a specific TVARVC variable
// by traversing incoming EdgeReadsConfig edges.
//
// Input accepts either canonical node ID ("TVARVC:ZKEKEKE") or plain
// variable name ("ZKEKEKE") — both are normalized to the canonical form.
//
// When multiple READS_CONFIG edges exist from the same object (duplicate
// evidence from builder), they are collapsed into one entry with the best
// confidence (HIGH > MEDIUM) and total edge count.
func WhereUsedConfig(g *Graph, variable string) *ConfigUsageResult {
	// Normalize input: accept both "TVARVC:ZVAR" and "ZVAR"
	variable = strings.TrimSpace(variable)
	varName := variable
	varNodeID := ""

	if strings.HasPrefix(strings.ToUpper(variable), NodeTVARVC+":") {
		// Already canonical: TVARVC:ZKEKEKE
		varNodeID = strings.ToUpper(variable)
		varName = varNodeID[len(NodeTVARVC)+1:]
	} else {
		// Plain name
		varName = strings.ToUpper(variable)
		varNodeID = NodeID(NodeTVARVC, varName)
	}

	result := &ConfigUsageResult{
		Variable:     varNodeID,
		VariableName: varName,
	}

	// Check if variable node exists
	if g.GetNode(varNodeID) != nil {
		result.Found = true
	}

	// Traverse incoming READS_CONFIG edges
	// Collapse by source object: best confidence wins, count all evidence
	type readerAcc struct {
		confidence string
		count      int
	}
	readers := make(map[string]*readerAcc)

	for _, e := range g.InEdges(varNodeID) {
		if e.Kind != EdgeReadsConfig {
			continue
		}

		acc, ok := readers[e.From]
		if !ok {
			acc = &readerAcc{}
			readers[e.From] = acc
		}
		acc.count++

		// Promote confidence: HIGH > MEDIUM > empty
		conf, _ := e.GetMeta(MetaConfidence)
		confStr, _ := conf.(string)
		if confStr == "HIGH" || (confStr == "MEDIUM" && acc.confidence == "") {
			acc.confidence = confStr
		}
	}

	// Build result entries
	entries := make([]ConfigReaderEntry, 0, len(readers))
	for nodeID, acc := range readers {
		entry := ConfigReaderEntry{
			NodeID:     nodeID,
			Confidence: acc.confidence,
			EdgeCount:  acc.count,
		}
		if n := g.GetNode(nodeID); n != nil {
			entry.Name = n.Name
			entry.Type = n.Type
			entry.Package = n.Package
		}
		entries = append(entries, entry)
	}

	// Sort by confidence (HIGH first), then by node ID for stability
	sort.Slice(entries, func(i, j int) bool {
		ci, cj := confidenceRank(entries[i].Confidence), confidenceRank(entries[j].Confidence)
		if ci != cj {
			return ci < cj
		}
		return entries[i].NodeID < entries[j].NodeID
	})

	result.Readers = entries
	return result
}

// confidenceRank returns sort rank (lower = better).
func confidenceRank(c string) int {
	switch c {
	case "HIGH":
		return 0
	case "MEDIUM":
		return 1
	default:
		return 2
	}
}
