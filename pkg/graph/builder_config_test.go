package graph

import (
	"testing"
)

func TestBuildConfigGraph_Basic(t *testing.T) {
	vars := []TVARVCVariable{
		{Name: "ZKEKEKE", Type: "P"},
	}
	refs := []TVARVCReference{
		{VariableName: "ZKEKEKE", ObjectType: "PROG", ObjectName: "ZREPORT", Confirmed: true},
		{VariableName: "ZKEKEKE", ObjectType: "CLAS", ObjectName: "ZCL_ORDER", Confirmed: false},
	}

	g := BuildConfigGraph(vars, refs)

	// 1 TVARVC node + 2 object nodes
	if g.NodeCount() != 3 {
		t.Errorf("NodeCount: got %d, want 3", g.NodeCount())
	}
	if g.EdgeCount() != 2 {
		t.Errorf("EdgeCount: got %d, want 2", g.EdgeCount())
	}

	// Variable node
	varNode := g.GetNode("TVARVC:ZKEKEKE")
	if varNode == nil {
		t.Fatal("TVARVC:ZKEKEKE not found")
	}
	if varNode.Type != NodeTVARVC {
		t.Errorf("Variable node type: got %q, want %q", varNode.Type, NodeTVARVC)
	}
	if v, _ := varNode.GetMeta("tvarvc_type"); v != "P" {
		t.Errorf("tvarvc_type: got %v, want P", v)
	}
}

func TestBuildConfigGraph_EdgeDirection(t *testing.T) {
	vars := []TVARVCVariable{{Name: "ZVAR1"}}
	refs := []TVARVCReference{
		{VariableName: "ZVAR1", ObjectType: "PROG", ObjectName: "ZREADER", Confirmed: true},
	}

	g := BuildConfigGraph(vars, refs)

	// Edge: PROG:ZREADER → TVARVC:ZVAR1 (dependent → dependency)
	out := g.OutEdges("PROG:ZREADER")
	if len(out) != 1 {
		t.Fatalf("ZREADER OutEdges: got %d, want 1", len(out))
	}
	if out[0].To != "TVARVC:ZVAR1" {
		t.Errorf("Edge target: got %q, want TVARVC:ZVAR1", out[0].To)
	}
	if out[0].Kind != EdgeReadsConfig {
		t.Errorf("Edge kind: got %s, want READS_CONFIG", out[0].Kind)
	}

	// InEdges on variable should find the reader
	in := g.InEdges("TVARVC:ZVAR1")
	if len(in) != 1 {
		t.Fatalf("ZVAR1 InEdges: got %d, want 1", len(in))
	}
	if in[0].From != "PROG:ZREADER" {
		t.Errorf("InEdge from: got %q, want PROG:ZREADER", in[0].From)
	}
}

func TestBuildConfigGraph_HeuristicSource(t *testing.T) {
	refs := []TVARVCReference{
		{VariableName: "ZVAR1", ObjectType: "PROG", ObjectName: "ZPROG", Confirmed: true},
	}

	g := BuildConfigGraph(nil, refs)

	edges := g.Edges()
	if len(edges) != 1 {
		t.Fatalf("Edges: got %d, want 1", len(edges))
	}

	e := edges[0]
	// Source must be TVARVC_CROSS (heuristic)
	if e.Source != SourceTVARVC_CROSS {
		t.Errorf("Source: got %s, want TVARVC_CROSS", e.Source)
	}
}

func TestBuildConfigGraph_Confidence(t *testing.T) {
	refs := []TVARVCReference{
		{VariableName: "ZVAR1", ObjectType: "PROG", ObjectName: "ZCONFIRMED", Confirmed: true},
		{VariableName: "ZVAR1", ObjectType: "PROG", ObjectName: "ZUNCONFIRMED", Confirmed: false},
	}

	g := BuildConfigGraph(nil, refs)

	for _, e := range g.Edges() {
		conf, ok := e.GetMeta(MetaConfidence)
		if !ok {
			t.Errorf("Edge %s→%s missing confidence meta", e.From, e.To)
			continue
		}
		if e.From == "PROG:ZCONFIRMED" && conf != "HIGH" {
			t.Errorf("Confirmed ref should be HIGH, got %v", conf)
		}
		if e.From == "PROG:ZUNCONFIRMED" && conf != "MEDIUM" {
			t.Errorf("Unconfirmed ref should be MEDIUM, got %v", conf)
		}
	}
}

func TestBuildConfigGraph_DuplicateEvidence(t *testing.T) {
	// Same program references same variable twice — both edges kept (different evidence)
	refs := []TVARVCReference{
		{VariableName: "ZVAR1", ObjectType: "PROG", ObjectName: "ZPROG", Confirmed: false},
		{VariableName: "ZVAR1", ObjectType: "PROG", ObjectName: "ZPROG", Confirmed: true}, // upgraded confirmation
	}

	g := BuildConfigGraph(nil, refs)

	// Both edges are kept — they represent different evidence (MEDIUM vs HIGH).
	// Deduplication is a query-layer concern, not builder.
	if g.EdgeCount() != 2 {
		t.Errorf("EdgeCount: got %d, want 2 (duplicate evidence preserved)", g.EdgeCount())
	}
	// But only 2 nodes (ZPROG + ZVAR1, merged)
	if g.NodeCount() != 2 {
		t.Errorf("NodeCount: got %d, want 2 (nodes merged)", g.NodeCount())
	}
}

func TestBuildConfigGraph_MixedObjectTypes(t *testing.T) {
	refs := []TVARVCReference{
		{VariableName: "ZVAR1", ObjectType: "PROG", ObjectName: "ZREPORT", Confirmed: true},
		{VariableName: "ZVAR1", ObjectType: "CLAS", ObjectName: "ZCL_READER", Confirmed: true},
		{VariableName: "ZVAR1", ObjectType: "FUGR", ObjectName: "ZFUGR", Confirmed: false},
	}

	g := BuildConfigGraph(nil, refs)

	if g.NodeCount() != 4 { // 3 objects + 1 TVARVC
		t.Errorf("NodeCount: got %d, want 4", g.NodeCount())
	}
	if g.EdgeCount() != 3 {
		t.Errorf("EdgeCount: got %d, want 3", g.EdgeCount())
	}

	// All types should be preserved
	if n := g.GetNode("PROG:ZREPORT"); n == nil || n.Type != NodePROG {
		t.Error("PROG node missing or wrong type")
	}
	if n := g.GetNode("CLAS:ZCL_READER"); n == nil || n.Type != NodeCLAS {
		t.Error("CLAS node missing or wrong type")
	}
	if n := g.GetNode("FUGR:ZFUGR"); n == nil || n.Type != NodeFUGR {
		t.Error("FUGR node missing or wrong type")
	}
}

func TestBuildConfigGraph_EmptyAndNoise(t *testing.T) {
	vars := []TVARVCVariable{
		{Name: ""},        // empty name
		{Name: "  "},      // whitespace
		{Name: "ZVALID"},  // valid
	}
	refs := []TVARVCReference{
		{VariableName: "", ObjectType: "PROG", ObjectName: "ZPROG"},         // empty var
		{VariableName: "ZVAR", ObjectType: "", ObjectName: "ZPROG"},         // empty type
		{VariableName: "ZVAR", ObjectType: "PROG", ObjectName: ""},          // empty name
		{VariableName: "ZVAR", ObjectType: "PROG", ObjectName: "ZVALID_REF", Confirmed: true}, // valid
	}

	g := BuildConfigGraph(vars, refs)

	// 1 valid variable (ZVALID) + 1 auto-created variable (ZVAR) + 1 object (ZVALID_REF)
	if g.NodeCount() != 3 {
		t.Errorf("NodeCount: got %d, want 3 (noise filtered)", g.NodeCount())
	}
	// 1 valid edge
	if g.EdgeCount() != 1 {
		t.Errorf("EdgeCount: got %d, want 1 (noise filtered)", g.EdgeCount())
	}
}

func TestBuildConfigGraph_VariableAutoCreated(t *testing.T) {
	// Reference to a variable not in the variables list → auto-created
	refs := []TVARVCReference{
		{VariableName: "ZDYNAMIC_VAR", ObjectType: "PROG", ObjectName: "ZPROG", Confirmed: true},
	}

	g := BuildConfigGraph(nil, refs) // no variables list

	varNode := g.GetNode("TVARVC:ZDYNAMIC_VAR")
	if varNode == nil {
		t.Fatal("Auto-created TVARVC node missing")
	}
	if varNode.Type != NodeTVARVC {
		t.Errorf("Type: got %q, want %q", varNode.Type, NodeTVARVC)
	}
}

func TestBuildConfigGraph_MultipleVariables(t *testing.T) {
	vars := []TVARVCVariable{
		{Name: "ZVAR_A", Type: "P"},
		{Name: "ZVAR_B", Type: "S"},
	}
	refs := []TVARVCReference{
		{VariableName: "ZVAR_A", ObjectType: "PROG", ObjectName: "ZPROG1", Confirmed: true},
		{VariableName: "ZVAR_B", ObjectType: "PROG", ObjectName: "ZPROG1", Confirmed: true},  // same prog, different var
		{VariableName: "ZVAR_A", ObjectType: "CLAS", ObjectName: "ZCL_OTHER", Confirmed: false},
	}

	g := BuildConfigGraph(vars, refs)

	// 2 TVARVC nodes + 2 object nodes
	if g.NodeCount() != 4 {
		t.Errorf("NodeCount: got %d, want 4", g.NodeCount())
	}
	if g.EdgeCount() != 3 {
		t.Errorf("EdgeCount: got %d, want 3", g.EdgeCount())
	}

	// ZPROG1 should have 2 outgoing READS_CONFIG edges (to ZVAR_A and ZVAR_B)
	out := g.OutEdges("PROG:ZPROG1")
	if len(out) != 2 {
		t.Errorf("ZPROG1 OutEdges: got %d, want 2", len(out))
	}
}
