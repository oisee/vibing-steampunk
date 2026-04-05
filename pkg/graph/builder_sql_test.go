package graph

import (
	"testing"
)

func TestBuildWBCROSSGTGraph_Basic(t *testing.T) {
	rows := []WBCROSSGTRow{
		{Include: "ZCL_ORDER========CP", OType: "TY", Name: "ZIF_ORDER_API"},
		{Include: "ZCL_ORDER========CP", OType: "ME", Name: "ZCL_HELPER"},
		{Include: "ZCL_ORDER========CP", OType: "DA", Name: "ZORDER_TAB"},
	}

	g := BuildWBCROSSGTGraph(rows)

	// 1 source (ZCL_ORDER) + 3 targets
	if g.NodeCount() != 4 {
		t.Errorf("NodeCount: got %d, want 4", g.NodeCount())
	}
	if g.EdgeCount() != 3 {
		t.Errorf("EdgeCount: got %d, want 3", g.EdgeCount())
	}

	// Source should be normalized from include to class
	src := g.GetNode("CLAS:ZCL_ORDER")
	if src == nil {
		t.Fatal("CLAS:ZCL_ORDER not found — include normalization failed")
	}

	// ZIF_ → INTF
	intf := g.GetNode("INTF:ZIF_ORDER_API")
	if intf == nil {
		t.Error("INTF:ZIF_ORDER_API not found — name-based type inference failed")
	}

	// ZCL_ → CLAS
	helper := g.GetNode("CLAS:ZCL_HELPER")
	if helper == nil {
		t.Error("CLAS:ZCL_HELPER not found")
	}

	// DA → TABL
	tab := g.GetNode("TABL:ZORDER_TAB")
	if tab == nil {
		t.Error("TABL:ZORDER_TAB not found — DA type mapping failed")
	}

	// All edges should be REFERENCES from WBCROSSGT
	for _, e := range g.Edges() {
		if e.Kind != EdgeReferences {
			t.Errorf("Expected REFERENCES, got %s for edge %s→%s", e.Kind, e.From, e.To)
		}
		if e.Source != SourceWBCROSSGT {
			t.Errorf("Expected WBCROSSGT source, got %s", e.Source)
		}
	}
}

func TestBuildWBCROSSGTGraph_IncludeNormalization(t *testing.T) {
	rows := []WBCROSSGTRow{
		// Class pool include → CLAS
		{Include: "ZCL_FOO========CP", OType: "TY", Name: "ZCL_BAR"},
		// Interface include → INTF
		{Include: "ZIF_API========IP", OType: "TY", Name: "ZCL_IMPL"},
		// Function group include → FUGR
		{Include: "LZFUGRU01", OType: "FU", Name: "ZCL_UTIL"},
		// Plain program → PROG
		{Include: "ZREPORT", OType: "DA", Name: "ZTABLE"},
	}

	g := BuildWBCROSSGTGraph(rows)

	checks := []struct {
		id   string
		want bool
	}{
		{"CLAS:ZCL_FOO", true},
		{"INTF:ZIF_API", true},
		{"FUGR:ZFUGR", true},
		{"PROG:ZREPORT", true},
	}
	for _, c := range checks {
		if (g.GetNode(c.id) != nil) != c.want {
			t.Errorf("Node %s: exist=%v, want=%v", c.id, g.GetNode(c.id) != nil, c.want)
		}
	}
}

func TestBuildWBCROSSGTGraph_SkipSelfAndNoise(t *testing.T) {
	rows := []WBCROSSGTRow{
		// Self-reference (after normalization ZCL_FOO → ZCL_FOO)
		{Include: "ZCL_FOO========CP", OType: "TY", Name: "ZCL_FOO"},
		// Backslash component ref — noise
		{Include: "ZCL_FOO========CP", OType: "ME", Name: "ZCL_FOO\\METHOD1"},
		// Empty name
		{Include: "ZCL_FOO========CP", OType: "TY", Name: ""},
		// Empty include
		{Include: "", OType: "TY", Name: "ZCL_BAR"},
		// Valid reference
		{Include: "ZCL_FOO========CP", OType: "TY", Name: "ZCL_VALID"},
	}

	g := BuildWBCROSSGTGraph(rows)

	// Only 1 valid edge: ZCL_FOO → ZCL_VALID
	if g.EdgeCount() != 1 {
		t.Errorf("EdgeCount: got %d, want 1 (noise filtered)", g.EdgeCount())
	}
	if g.NodeCount() != 2 {
		t.Errorf("NodeCount: got %d, want 2", g.NodeCount())
	}
}

func TestBuildWBCROSSGTGraph_DuplicateRows(t *testing.T) {
	rows := []WBCROSSGTRow{
		{Include: "ZCL_A========CP", OType: "TY", Name: "ZCL_B"},
		{Include: "ZCL_A========CP", OType: "TY", Name: "ZCL_B"}, // duplicate
		{Include: "ZCL_A========CU", OType: "ME", Name: "ZCL_B"}, // same object, different include
	}

	g := BuildWBCROSSGTGraph(rows)

	// Node merge: only 2 unique nodes
	if g.NodeCount() != 2 {
		t.Errorf("NodeCount: got %d, want 2 (nodes merged)", g.NodeCount())
	}
	// Duplicate edges allowed (different evidence: different include, different otype)
	if g.EdgeCount() != 3 {
		t.Errorf("EdgeCount: got %d, want 3 (duplicates are different evidence)", g.EdgeCount())
	}
}

func TestBuildCROSSGraph_FunctionCall(t *testing.T) {
	rows := []CROSSRow{
		{Include: "ZREPORT", Type: "FU", Name: "Z_MY_FUNC"},
	}

	g := BuildCROSSGraph(rows)

	if g.EdgeCount() != 1 {
		t.Fatalf("EdgeCount: got %d, want 1", g.EdgeCount())
	}

	e := g.Edges()[0]
	if e.From != "PROG:ZREPORT" {
		t.Errorf("From: got %q, want PROG:ZREPORT", e.From)
	}
	// FM name stored as TYPE (neutral) — not FUGR, because we don't know the function group.
	// MCP/CLI layer resolves FM→FUGR via TADIR.
	if e.To != "TYPE:Z_MY_FUNC" {
		t.Errorf("To: got %q, want TYPE:Z_MY_FUNC (FM as neutral TYPE, not FUGR)", e.To)
	}
	if e.Kind != EdgeCalls {
		t.Errorf("Kind: got %s, want CALLS", e.Kind)
	}
	if e.Source != SourceCROSS {
		t.Errorf("Source: got %s, want CROSS", e.Source)
	}
}

func TestBuildCROSSGraph_ProgramSubmit(t *testing.T) {
	rows := []CROSSRow{
		{Include: "ZREPORT", Type: "PR", Name: "ZOTHER_REPORT"},
	}

	g := BuildCROSSGraph(rows)
	e := g.Edges()[0]

	if e.Kind != EdgeCalls {
		t.Errorf("PR should map to CALLS, got %s", e.Kind)
	}
	if e.To != "PROG:ZOTHER_REPORT" {
		t.Errorf("To: got %q, want PROG:ZOTHER_REPORT", e.To)
	}
}

func TestBuildCROSSGraph_DataReference(t *testing.T) {
	rows := []CROSSRow{
		{Include: "ZCL_FOO========CP", Type: "DA", Name: "ZTABLE"},
	}

	g := BuildCROSSGraph(rows)
	e := g.Edges()[0]

	if e.Kind != EdgeReferences {
		t.Errorf("DA should map to REFERENCES, got %s", e.Kind)
	}
	if e.To != "TABL:ZTABLE" {
		t.Errorf("To: got %q, want TABL:ZTABLE", e.To)
	}
	// Source include should be normalized to class
	if e.From != "CLAS:ZCL_FOO" {
		t.Errorf("From: got %q, want CLAS:ZCL_FOO", e.From)
	}
}

func TestBuildCROSSGraph_UnknownType(t *testing.T) {
	rows := []CROSSRow{
		// SU (subroutine) — we conservatively map to REFERENCES
		{Include: "ZREPORT", Type: "SU", Name: "ZSUBFORM"},
	}

	g := BuildCROSSGraph(rows)
	e := g.Edges()[0]

	if e.Kind != EdgeReferences {
		t.Errorf("SU should map to REFERENCES (conservative), got %s", e.Kind)
	}
	if e.RefDetail != "TYPE:SU" {
		t.Errorf("RefDetail should preserve TYPE code: got %q", e.RefDetail)
	}
}

func TestBuildCROSSGraph_SkipSelfReference(t *testing.T) {
	rows := []CROSSRow{
		{Include: "ZREPORT", Type: "FU", Name: "ZREPORT"}, // self
		{Include: "ZREPORT", Type: "FU", Name: "Z_VALID"},  // valid
	}

	g := BuildCROSSGraph(rows)

	if g.EdgeCount() != 1 {
		t.Errorf("EdgeCount: got %d, want 1 (self skipped)", g.EdgeCount())
	}
}

func TestBuildCROSSGraph_Empty(t *testing.T) {
	g := BuildCROSSGraph(nil)
	if g.NodeCount() != 0 || g.EdgeCount() != 0 {
		t.Error("Empty input should produce empty graph")
	}
}

func TestBuildCROSSGraph_Provenance(t *testing.T) {
	rows := []CROSSRow{
		{Include: "ZREPORT", Type: "FU", Name: "Z_FM"},
	}

	g := BuildCROSSGraph(rows)
	e := g.Edges()[0]

	// RawInclude preserved for traceability
	if e.RawInclude != "ZREPORT" {
		t.Errorf("RawInclude: got %q, want ZREPORT", e.RawInclude)
	}
	// RefDetail preserves TYPE code
	if e.RefDetail != "TYPE:FU" {
		t.Errorf("RefDetail: got %q, want TYPE:FU", e.RefDetail)
	}
}
