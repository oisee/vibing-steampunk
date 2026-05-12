package graph

import "testing"

func buildTransportBoundaryTestGraph() (*Graph, *TransportScope) {
	// Transport A4HK900001 contains: ZCL_ORDER, ZCL_ITEM, ZPROG_MAIN
	// ZCL_ORDER calls ZCL_ITEM (in scope) and ZIF_LOGGER (missing custom)
	// ZCL_ORDER references CL_GUI_ALV_GRID (standard SAP)
	// ZPROG_MAIN calls ZCL_ORDER (in scope)

	headers := []TransportHeader{
		{TRKORR: "A4HK900001", TRFUNCTION: "K", TRSTATUS: "D", AS4USER: "DEV", AS4DATE: "20260401"},
	}
	objects := []TransportObject{
		{TRKORR: "A4HK900001", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_ORDER"},
		{TRKORR: "A4HK900001", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_ITEM"},
		{TRKORR: "A4HK900001", PGMID: "R3TR", Object: "PROG", ObjName: "ZPROG_MAIN"},
	}

	g := BuildTransportGraph(headers, objects)

	// Add structural edges
	g.AddEdge(&Edge{From: "CLAS:ZCL_ORDER", To: "CLAS:ZCL_ITEM", Kind: EdgeCalls, Source: SourceCROSS})
	g.AddEdge(&Edge{From: "CLAS:ZCL_ORDER", To: "INTF:ZIF_LOGGER", Kind: EdgeReferences, Source: SourceWBCROSSGT})
	g.AddEdge(&Edge{From: "CLAS:ZCL_ORDER", To: "CLAS:CL_GUI_ALV_GRID", Kind: EdgeReferences, Source: SourceWBCROSSGT})
	g.AddEdge(&Edge{From: "PROG:ZPROG_MAIN", To: "CLAS:ZCL_ORDER", Kind: EdgeCalls, Source: SourceCROSS})

	// Ensure target nodes exist
	g.AddNode(&Node{ID: "INTF:ZIF_LOGGER", Name: "ZIF_LOGGER", Type: "INTF"})
	g.AddNode(&Node{ID: "CLAS:CL_GUI_ALV_GRID", Name: "CL_GUI_ALV_GRID", Type: "CLAS"})

	scope := &TransportScope{
		Label:      "A4HK900001",
		Transports: map[string]bool{"A4HK900001": true},
		Objects: map[string]bool{
			"CLAS:ZCL_ORDER": true,
			"CLAS:ZCL_ITEM":  true,
			"PROG:ZPROG_MAIN": true,
		},
	}

	return g, scope
}

func TestAnalyzeTransportBoundaries_Basic(t *testing.T) {
	g, scope := buildTransportBoundaryTestGraph()
	report := AnalyzeTransportBoundaries(g, scope)

	if report.ObjectCount != 3 {
		t.Errorf("ObjectCount = %d, want 3", report.ObjectCount)
	}

	// ZCL_ORDER→ZCL_ITEM is in scope, ZCL_ORDER→ZIF_LOGGER is missing,
	// ZCL_ORDER→CL_GUI_ALV_GRID is standard, ZPROG_MAIN→ZCL_ORDER is in scope
	if len(report.Missing) != 1 {
		t.Fatalf("Expected 1 missing dep, got %d", len(report.Missing))
	}
	if report.Missing[0].TargetName != "ZIF_LOGGER" {
		t.Errorf("Missing target = %q, want ZIF_LOGGER", report.Missing[0].TargetName)
	}

	if len(report.Standard) != 1 {
		t.Fatalf("Expected 1 standard dep, got %d", len(report.Standard))
	}
	if report.Standard[0].TargetName != "CL_GUI_ALV_GRID" {
		t.Errorf("Standard target = %q, want CL_GUI_ALV_GRID", report.Standard[0].TargetName)
	}

	if report.Summary.InScope != 2 {
		t.Errorf("InScope = %d, want 2", report.Summary.InScope)
	}

	if report.Summary.SelfConsistent {
		t.Error("Should NOT be self-consistent (ZIF_LOGGER missing)")
	}
}

func TestAnalyzeTransportBoundaries_SelfConsistent(t *testing.T) {
	// Transport with all deps satisfied
	g := New()
	g.AddNode(&Node{ID: "CLAS:ZCL_A", Name: "ZCL_A", Type: "CLAS"})
	g.AddNode(&Node{ID: "CLAS:ZCL_B", Name: "ZCL_B", Type: "CLAS"})
	g.AddEdge(&Edge{From: "CLAS:ZCL_A", To: "CLAS:ZCL_B", Kind: EdgeCalls, Source: SourceCROSS})

	scope := &TransportScope{
		Label:   "A4HK000001",
		Objects: map[string]bool{"CLAS:ZCL_A": true, "CLAS:ZCL_B": true},
	}

	report := AnalyzeTransportBoundaries(g, scope)
	if !report.Summary.SelfConsistent {
		t.Error("Should be self-consistent")
	}
	if report.Summary.Missing != 0 {
		t.Errorf("Missing = %d, want 0", report.Summary.Missing)
	}
}

func TestAnalyzeTransportBoundaries_SkipsTransportEdges(t *testing.T) {
	// IN_TRANSPORT edges should not be analyzed as structural deps
	g, scope := buildTransportBoundaryTestGraph()
	report := AnalyzeTransportBoundaries(g, scope)

	// Transport edges (object→TR node) should NOT appear in any category
	for _, e := range report.Missing {
		if e.EdgeKind == string(EdgeInTransport) {
			t.Error("IN_TRANSPORT edge should not appear in Missing")
		}
	}
	for _, e := range report.Standard {
		if e.EdgeKind == string(EdgeInTransport) {
			t.Error("IN_TRANSPORT edge should not appear in Standard")
		}
	}
}

func TestAnalyzeTransportBoundaries_Dynamic(t *testing.T) {
	g := New()
	g.AddNode(&Node{ID: "PROG:ZPROG", Name: "ZPROG", Type: "PROG"})
	g.AddEdge(&Edge{From: "PROG:ZPROG", To: "DYNAMIC:lv_fm", Kind: EdgeDynamic, Source: SourceParser, RefDetail: "DYNAMIC_FM:lv_fm"})

	scope := &TransportScope{
		Label:   "A4HK000001",
		Objects: map[string]bool{"PROG:ZPROG": true},
	}

	report := AnalyzeTransportBoundaries(g, scope)
	if len(report.Dynamic) != 1 {
		t.Fatalf("Expected 1 dynamic dep, got %d", len(report.Dynamic))
	}
	if report.Summary.Dynamic != 1 {
		t.Errorf("Dynamic count = %d, want 1", report.Summary.Dynamic)
	}
}
