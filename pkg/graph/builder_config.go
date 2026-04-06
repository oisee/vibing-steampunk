package graph

import "strings"

// TVARVCVariable represents a known TVARVC/STVARV variable entry.
type TVARVCVariable struct {
	Name string // Variable name (e.g., ZKEKEKE)
	Type string // TVARVC TYPE field: P=parameter, S=select-option range (informational only)
}

// TVARVCReference represents a confirmed or candidate reference from a program/object
// to a TVARVC variable. Upstream code (MCP handler) is responsible for discovery
// (CROSS table lookup + optional source grep). This builder only consumes results.
type TVARVCReference struct {
	VariableName string // TVARVC variable name being read
	ObjectType   string // Referencing object type: CLAS, PROG, FUGR, etc.
	ObjectName   string // Referencing object name: ZCL_FOO, ZREPORT, etc.
	Confirmed    bool   // true = grep-confirmed (HIGH confidence), false = CROSS-only (MEDIUM)
}

// BuildConfigGraph builds a graph from TVARVC variables and their references.
//
// For each variable, creates a TVARVC:<name> node.
// For each reference, creates an object → TVARVC edge (EdgeReadsConfig, SourceTVARVC_CROSS).
// Confidence is tracked in Edge.Meta based on TVARVCReference.Confirmed.
//
// This is a heuristic source: references come from CROSS table cross-refs
// filtered by optional source grep. False positives (variable name in comments)
// and false negatives (dynamic name construction) are possible.
func BuildConfigGraph(variables []TVARVCVariable, references []TVARVCReference) *Graph {
	g := New()

	// Create variable nodes
	for _, v := range variables {
		name := strings.ToUpper(strings.TrimSpace(v.Name))
		if name == "" {
			continue
		}
		n := &Node{
			ID:   NodeID(NodeTVARVC, name),
			Name: name,
			Type: NodeTVARVC,
		}
		if v.Type != "" {
			n.SetMeta("tvarvc_type", strings.ToUpper(strings.TrimSpace(v.Type)))
		}
		g.AddNode(n)
	}

	// Create reference edges
	for _, ref := range references {
		varName := strings.ToUpper(strings.TrimSpace(ref.VariableName))
		objType := strings.ToUpper(strings.TrimSpace(ref.ObjectType))
		objName := strings.ToUpper(strings.TrimSpace(ref.ObjectName))

		if varName == "" || objType == "" || objName == "" {
			continue
		}

		// Ensure variable node exists (may not be in variables list if discovered dynamically)
		varNodeID := NodeID(NodeTVARVC, varName)
		g.AddNode(&Node{
			ID:   varNodeID,
			Name: varName,
			Type: NodeTVARVC,
		})

		// Ensure object node exists
		objNodeID := NodeID(objType, objName)
		g.AddNode(&Node{
			ID:   objNodeID,
			Name: objName,
			Type: objType,
		})

		// Object → TVARVC edge (dependent → dependency)
		confidence := "MEDIUM"
		if ref.Confirmed {
			confidence = "HIGH"
		}

		e := &Edge{
			From:   objNodeID,
			To:     varNodeID,
			Kind:   EdgeReadsConfig,
			Source: SourceTVARVC_CROSS,
		}
		e.SetMeta(MetaConfidence, confidence)
		g.AddEdge(e)
	}

	return g
}
