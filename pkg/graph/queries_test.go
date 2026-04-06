package graph

import (
	"testing"
)

// buildCoChangeTestGraph creates a graph with known transport relationships:
//
//	TR1: ZCL_A, ZCL_B, ZPROG_C
//	TR2: ZCL_A, ZCL_B, ZTABLE_D
//	TR3: ZCL_A, ZPROG_C
//
// So from ZCL_A's perspective:
//
//	ZCL_B:    2 shared transports (TR1, TR2)
//	ZPROG_C:  2 shared transports (TR1, TR3)
//	ZTABLE_D: 1 shared transport  (TR2)
func buildCoChangeTestGraph() *Graph {
	headers := []TransportHeader{
		{TRKORR: "A4HK000001", TRFUNCTION: "K", TRSTATUS: "R", AS4USER: "DEV", AS4DATE: "20260401"},
		{TRKORR: "A4HK000002", TRFUNCTION: "K", TRSTATUS: "R", AS4USER: "DEV", AS4DATE: "20260402"},
		{TRKORR: "A4HK000003", TRFUNCTION: "K", TRSTATUS: "R", AS4USER: "DEV", AS4DATE: "20260403"},
	}
	objects := []TransportObject{
		// TR1
		{TRKORR: "A4HK000001", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_A"},
		{TRKORR: "A4HK000001", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_B"},
		{TRKORR: "A4HK000001", PGMID: "R3TR", Object: "PROG", ObjName: "ZPROG_C"},
		// TR2
		{TRKORR: "A4HK000002", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_A"},
		{TRKORR: "A4HK000002", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_B"},
		{TRKORR: "A4HK000002", PGMID: "R3TR", Object: "TABL", ObjName: "ZTABLE_D"},
		// TR3
		{TRKORR: "A4HK000003", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_A"},
		{TRKORR: "A4HK000003", PGMID: "R3TR", Object: "PROG", ObjName: "ZPROG_C"},
	}
	return BuildTransportGraph(headers, objects)
}

func TestWhatChangesWith_Basic(t *testing.T) {
	g := buildCoChangeTestGraph()

	result := WhatChangesWith(g, "CLAS:ZCL_A", 0)

	if result.Target != "CLAS:ZCL_A" {
		t.Errorf("Target: got %q, want CLAS:ZCL_A", result.Target)
	}
	if result.TotalTransports != 3 {
		t.Errorf("TotalTransports: got %d, want 3", result.TotalTransports)
	}
	if len(result.CoChanges) != 3 {
		t.Fatalf("CoChanges count: got %d, want 3", len(result.CoChanges))
	}

	// First two should have count=2 (ZCL_B and ZPROG_C, sorted by ID)
	if result.CoChanges[0].Count != 2 {
		t.Errorf("Top entry count: got %d, want 2", result.CoChanges[0].Count)
	}
	if result.CoChanges[1].Count != 2 {
		t.Errorf("Second entry count: got %d, want 2", result.CoChanges[1].Count)
	}
	// Third should have count=1 (ZTABLE_D)
	if result.CoChanges[2].Count != 1 {
		t.Errorf("Third entry count: got %d, want 1", result.CoChanges[2].Count)
	}
	if result.CoChanges[2].NodeID != "TABL:ZTABLE_D" {
		t.Errorf("Third entry: got %q, want TABL:ZTABLE_D", result.CoChanges[2].NodeID)
	}
}

func TestWhatChangesWith_Ranking(t *testing.T) {
	g := buildCoChangeTestGraph()

	// ZCL_B and ZPROG_C both have count=2.
	// Sorted by ID: CLAS:ZCL_B < PROG:ZPROG_C
	result := WhatChangesWith(g, "CLAS:ZCL_A", 0)

	if result.CoChanges[0].NodeID != "CLAS:ZCL_B" {
		t.Errorf("Expected CLAS:ZCL_B first (alphabetical tiebreak), got %q", result.CoChanges[0].NodeID)
	}
	if result.CoChanges[1].NodeID != "PROG:ZPROG_C" {
		t.Errorf("Expected PROG:ZPROG_C second, got %q", result.CoChanges[1].NodeID)
	}
}

func TestWhatChangesWith_TopN(t *testing.T) {
	g := buildCoChangeTestGraph()

	result := WhatChangesWith(g, "CLAS:ZCL_A", 2)

	if len(result.CoChanges) != 2 {
		t.Errorf("TopN=2: got %d results, want 2", len(result.CoChanges))
	}
	// Should be the top 2 by count
	for _, e := range result.CoChanges {
		if e.Count < 2 {
			t.Errorf("TopN=2 should return only count>=2 entries, got count=%d for %s", e.Count, e.NodeID)
		}
	}
}

func TestWhatChangesWith_NoTransports(t *testing.T) {
	g := New()
	g.AddNode(&Node{ID: "CLAS:ZCL_LONELY", Name: "ZCL_LONELY", Type: NodeCLAS})

	result := WhatChangesWith(g, "CLAS:ZCL_LONELY", 0)

	if result.TotalTransports != 0 {
		t.Errorf("TotalTransports: got %d, want 0", result.TotalTransports)
	}
	if len(result.CoChanges) != 0 {
		t.Errorf("CoChanges: got %d, want 0", len(result.CoChanges))
	}
}

func TestWhatChangesWith_NonexistentNode(t *testing.T) {
	g := buildCoChangeTestGraph()

	result := WhatChangesWith(g, "CLAS:DOES_NOT_EXIST", 0)

	if result.TotalTransports != 0 {
		t.Errorf("TotalTransports for nonexistent: got %d, want 0", result.TotalTransports)
	}
	if len(result.CoChanges) != 0 {
		t.Errorf("CoChanges for nonexistent: got %d, want 0", len(result.CoChanges))
	}
}

func TestWhatChangesWith_SelfExcluded(t *testing.T) {
	g := buildCoChangeTestGraph()

	result := WhatChangesWith(g, "CLAS:ZCL_A", 0)

	for _, e := range result.CoChanges {
		if e.NodeID == "CLAS:ZCL_A" {
			t.Error("Target should not appear in its own co-change list")
		}
	}
}

func TestWhatChangesWith_TransportIDs(t *testing.T) {
	g := buildCoChangeTestGraph()

	result := WhatChangesWith(g, "CLAS:ZCL_A", 0)

	// Find ZCL_B entry — should share TR1 and TR2
	for _, e := range result.CoChanges {
		if e.NodeID == "CLAS:ZCL_B" {
			if len(e.Transports) != 2 {
				t.Errorf("ZCL_B shared transports: got %d, want 2", len(e.Transports))
			}
			// Transports should be sorted
			if len(e.Transports) == 2 && e.Transports[0] > e.Transports[1] {
				t.Errorf("Transports should be sorted: %v", e.Transports)
			}
			return
		}
	}
	t.Error("ZCL_B not found in co-changes")
}

func TestWhatChangesWith_NodeDetails(t *testing.T) {
	g := buildCoChangeTestGraph()

	result := WhatChangesWith(g, "CLAS:ZCL_A", 0)

	for _, e := range result.CoChanges {
		if e.NodeID == "CLAS:ZCL_B" {
			if e.Name != "ZCL_B" {
				t.Errorf("Name: got %q, want ZCL_B", e.Name)
			}
			if e.Type != NodeCLAS {
				t.Errorf("Type: got %q, want %q", e.Type, NodeCLAS)
			}
			return
		}
	}
	t.Error("ZCL_B not found")
}

func TestWhatChangesWith_DuplicateInSameTransport(t *testing.T) {
	// An object appearing twice in the same transport (e.g., from two tasks)
	// should only count as 1 co-change occurrence.
	headers := []TransportHeader{
		{TRKORR: "A4HK000010", TRFUNCTION: "K", TRSTATUS: "R", AS4USER: "DEV", AS4DATE: "20260401"},
	}
	objects := []TransportObject{
		{TRKORR: "A4HK000010", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_X"},
		{TRKORR: "A4HK000010", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_Y"},
		{TRKORR: "A4HK000010", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_Y"}, // duplicate
	}
	g := BuildTransportGraph(headers, objects)

	result := WhatChangesWith(g, "CLAS:ZCL_X", 0)

	if len(result.CoChanges) != 1 {
		t.Fatalf("Expected 1 co-change, got %d", len(result.CoChanges))
	}
	if result.CoChanges[0].Count != 1 {
		t.Errorf("Duplicate in same TR should count as 1, got %d", result.CoChanges[0].Count)
	}
}

func TestWhatChangesWith_NamespacedObject(t *testing.T) {
	headers := []TransportHeader{
		{TRKORR: "A4HK000020", TRFUNCTION: "K", TRSTATUS: "R", AS4USER: "DEV", AS4DATE: "20260401"},
	}
	objects := []TransportObject{
		{TRKORR: "A4HK000020", PGMID: "R3TR", Object: "CLAS", ObjName: "/UI5/CL_ROUTER"},
		{TRKORR: "A4HK000020", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_MY_APP"},
	}
	g := BuildTransportGraph(headers, objects)

	result := WhatChangesWith(g, "CLAS:ZCL_MY_APP", 0)

	if len(result.CoChanges) != 1 {
		t.Fatalf("Expected 1 co-change, got %d", len(result.CoChanges))
	}
	if result.CoChanges[0].NodeID != "CLAS:/UI5/CL_ROUTER" {
		t.Errorf("Expected namespaced object, got %q", result.CoChanges[0].NodeID)
	}
}
