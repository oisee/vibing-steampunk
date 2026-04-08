package graph

import "strings"

// WBCROSSGTRow represents a row from the WBCROSSGT table (global type cross-references).
// WBCROSSGT tracks object-level references: which objects does INCLUDE reference?
// Columns: INCLUDE (referencing include), OTYPE (reference type code), NAME (referenced object).
type WBCROSSGTRow struct {
	Include string // Referencing include (e.g., ZCL_FOO========CP, ZREPORT, LZFUGRU01)
	OType   string // Reference type: DA=data, TY=type, ME=method, IN=interface, EV=event, ...
	Name    string // Referenced object name
}

// CROSSRow represents a row from the CROSS table (classical cross-references).
// CROSS tracks procedural references: function calls, subroutine calls, SUBMIT, etc.
// Columns: INCLUDE (referencing include), TYPE (reference type code), NAME (referenced object).
type CROSSRow struct {
	Include string // Referencing include (e.g., ZREPORT, LZFUGRF01)
	Type    string // Reference type: FU=function, SU=subroutine, PR=program (SUBMIT), ...
	Name    string // Referenced object name (function module name, program name, etc.)
}

// BuildWBCROSSGTGraph builds a graph from WBCROSSGT rows.
// Each row represents a type-level reference from an include to a named object.
//
// Edge semantics: all WBCROSSGT edges are EdgeReferences with SourceWBCROSSGT.
// The OTYPE code is preserved in RefDetail for downstream consumers, but we do
// NOT attempt to infer CALLS vs REFERENCES from OTYPE alone — WBCROSSGT tracks
// type references (DATA TYPE, TYPE REF TO, INTERFACES, INHERITING FROM, etc.),
// not call relationships. Call semantics come from CROSS or the parser.
func BuildWBCROSSGTGraph(rows []WBCROSSGTRow) *Graph {
	g := New()

	for _, row := range rows {
		name := strings.TrimSpace(row.Name)
		include := strings.TrimSpace(row.Include)
		otype := strings.TrimSpace(row.OType)

		if name == "" || include == "" {
			continue
		}

		// Skip self-references and component-internal refs (contain backslash)
		if strings.Contains(name, "\\") {
			continue
		}

		// Normalize the include to an object-level node
		fromID, fromType, fromName := NormalizeInclude(include)

		// Skip self-references after normalization
		if strings.EqualFold(fromName, name) {
			continue
		}

		// Create source node
		g.AddNode(&Node{
			ID:   fromID,
			Name: fromName,
			Type: fromType,
		})

		// Infer target node type from OTYPE where reliable
		toType := inferNodeTypeFromWBCROSSGT(otype, name)
		toID := NodeID(toType, name)

		g.AddNode(&Node{
			ID:   toID,
			Name: strings.ToUpper(name),
			Type: toType,
		})

		g.AddEdge(&Edge{
			From:       fromID,
			To:         toID,
			Kind:       EdgeReferences,
			Source:     SourceWBCROSSGT,
			RawInclude: include,
			RefDetail:  "OTYPE:" + otype,
		})
	}

	return g
}

// inferNodeTypeFromWBCROSSGT maps WBCROSSGT OTYPE codes to canonical node types
// where the mapping is reliable. Returns a best-guess type; callers should not
// treat this as definitive — TADIR lookup is the only authoritative source.
func inferNodeTypeFromWBCROSSGT(otype, name string) string {
	otype = strings.ToUpper(strings.TrimSpace(otype))
	switch otype {
	case "ME", "EV", "IN", "TY":
		// ME=method ref, EV=event, IN=interface ref, TY=type ref
		// These often reference classes or interfaces, but could be types.
		// Use name prefix heuristic: ZIF_* → INTF, ZCL_*/ZCX_* → CLAS
		return guessTypeFromName(name)
	case "DA":
		// DA=data reference — could be table, structure, or data element
		return NodeTABL
	default:
		return guessTypeFromName(name)
	}
}

// guessTypeFromName applies SAP naming conventions to guess node type.
// This is a heuristic, not authoritative.
func guessTypeFromName(name string) string {
	upper := strings.ToUpper(strings.TrimSpace(name))
	switch {
	case strings.HasPrefix(upper, "ZIF_") || strings.HasPrefix(upper, "YIF_") ||
		strings.HasPrefix(upper, "IF_"):
		return NodeINTF
	case strings.HasPrefix(upper, "ZCL_") || strings.HasPrefix(upper, "YCL_") ||
		strings.HasPrefix(upper, "ZCX_") || strings.HasPrefix(upper, "YCX_") ||
		strings.HasPrefix(upper, "CL_") || strings.HasPrefix(upper, "CX_"):
		return NodeCLAS
	default:
		// Can't tell — use generic TYPE to avoid misclassification
		return NodeTYPE
	}
}

// BuildCROSSGraph builds a graph from CROSS rows.
// Each row represents a procedural reference from an include to a named object.
//
// Edge semantics depend on TYPE code:
//   - FU (function call) → EdgeCalls to FUGR:<name>
//   - PR (program/SUBMIT) → EdgeCalls to PROG:<name>
//   - Other codes → EdgeReferences (conservative; we don't have enough
//     semantic info to reliably classify SU/macro/etc.)
//
// All edges have SourceCROSS with TYPE code in RefDetail.
func BuildCROSSGraph(rows []CROSSRow) *Graph {
	g := New()

	for _, row := range rows {
		name := strings.TrimSpace(row.Name)
		include := strings.TrimSpace(row.Include)
		refType := strings.ToUpper(strings.TrimSpace(row.Type))

		if name == "" || include == "" {
			continue
		}

		// Normalize include to object-level node
		fromID, fromType, fromName := NormalizeInclude(include)

		// Skip self-references
		if strings.EqualFold(fromName, name) {
			continue
		}

		g.AddNode(&Node{
			ID:   fromID,
			Name: fromName,
			Type: fromType,
		})

		// Determine edge kind and target type based on CROSS TYPE code
		edgeKind, toType := classifyCROSSRef(refType, name)

		toName := strings.ToUpper(name)
		toID := NodeID(toType, toName)

		g.AddNode(&Node{
			ID:   toID,
			Name: toName,
			Type: toType,
		})

		g.AddEdge(&Edge{
			From:       fromID,
			To:         toID,
			Kind:       edgeKind,
			Source:     SourceCROSS,
			RawInclude: include,
			RefDetail:  "TYPE:" + refType,
		})
	}

	return g
}

// classifyCROSSRef maps CROSS TYPE codes to edge kind and target node type.
// Only FU and PR have clear enough semantics for CALLS; everything else
// gets conservative EdgeReferences.
func classifyCROSSRef(refType, name string) (EdgeKind, string) {
	switch refType {
	case "FU":
		// Function module call. CROSS NAME contains the FM name (e.g., Z_MY_FUNC),
		// NOT the function group name. We cannot reliably derive FUGR from FM name
		// alone (convention varies: Z_FUGR_FM vs ZFUGR_FM vs arbitrary).
		// Using NodeFUGR would mislabel the node — FUGR IDs must be function groups.
		// We use NodeTYPE as a neutral placeholder; the FM name is preserved in
		// RefDetail ("TYPE:FU") and the node name, so MCP/CLI layers can resolve
		// FM→FUGR via TADIR lookup later.
		// NOTE: builder_parser.go uses FUGR:<fm_name> for the same case — that's
		// a known inconsistency to unify when TADIR resolution is added.
		return EdgeCalls, NodeTYPE
	case "PR":
		// SUBMIT / program call
		return EdgeCalls, NodePROG
	case "DA":
		// Data reference (table/structure)
		return EdgeReferences, NodeTABL
	case "TY":
		// Type reference
		return EdgeReferences, guessTypeFromName(name)
	default:
		// SU (subroutine), macro, etc. — not enough info for CALLS
		return EdgeReferences, guessTypeFromName(name)
	}
}
