package graph

import (
	"testing"
)

func TestImpact_SingleHop(t *testing.T) {
	g := New()
	g.AddNode(&Node{ID: "CLAS:ZCL_TARGET", Name: "ZCL_TARGET", Type: NodeCLAS, Package: "$ZDEV"})
	g.AddNode(&Node{ID: "PROG:ZCALLER", Name: "ZCALLER", Type: NodePROG, Package: "$ZDEV"})
	g.AddEdge(&Edge{From: "PROG:ZCALLER", To: "CLAS:ZCL_TARGET", Kind: EdgeCalls, Source: SourceParser})

	result := Impact(g, "CLAS:ZCL_TARGET", nil)

	if result.Root != "CLAS:ZCL_TARGET" {
		t.Errorf("Root: got %q", result.Root)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("Entries: got %d, want 1", len(result.Entries))
	}

	e := result.Entries[0]
	if e.NodeID != "PROG:ZCALLER" {
		t.Errorf("NodeID: got %q, want PROG:ZCALLER", e.NodeID)
	}
	if e.Depth != 1 {
		t.Errorf("Depth: got %d, want 1", e.Depth)
	}
	if e.ViaEdge != EdgeCalls {
		t.Errorf("ViaEdge: got %s, want CALLS", e.ViaEdge)
	}
	if e.ViaFrom != "CLAS:ZCL_TARGET" {
		t.Errorf("ViaFrom: got %q, want CLAS:ZCL_TARGET", e.ViaFrom)
	}
	if e.Name != "ZCALLER" {
		t.Errorf("Name: got %q, want ZCALLER", e.Name)
	}
}

func TestImpact_MultiHop(t *testing.T) {
	// Chain: D → C → B → A (target)
	g := New()
	g.AddNode(&Node{ID: "CLAS:A", Name: "A", Type: NodeCLAS})
	g.AddNode(&Node{ID: "CLAS:B", Name: "B", Type: NodeCLAS})
	g.AddNode(&Node{ID: "CLAS:C", Name: "C", Type: NodeCLAS})
	g.AddNode(&Node{ID: "CLAS:D", Name: "D", Type: NodeCLAS})
	g.AddEdge(&Edge{From: "CLAS:B", To: "CLAS:A", Kind: EdgeCalls, Source: SourceParser})
	g.AddEdge(&Edge{From: "CLAS:C", To: "CLAS:B", Kind: EdgeCalls, Source: SourceParser})
	g.AddEdge(&Edge{From: "CLAS:D", To: "CLAS:C", Kind: EdgeReferences, Source: SourceWBCROSSGT})

	result := Impact(g, "CLAS:A", &ImpactOptions{MaxDepth: 5})

	if len(result.Entries) != 3 {
		t.Fatalf("Entries: got %d, want 3", len(result.Entries))
	}

	// BFS order: B(depth=1), C(depth=2), D(depth=3)
	depths := make(map[string]int)
	for _, e := range result.Entries {
		depths[e.NodeID] = e.Depth
	}
	if depths["CLAS:B"] != 1 {
		t.Errorf("B depth: got %d, want 1", depths["CLAS:B"])
	}
	if depths["CLAS:C"] != 2 {
		t.Errorf("C depth: got %d, want 2", depths["CLAS:C"])
	}
	if depths["CLAS:D"] != 3 {
		t.Errorf("D depth: got %d, want 3", depths["CLAS:D"])
	}

	// Check path info: D should have ViaFrom=C
	for _, e := range result.Entries {
		if e.NodeID == "CLAS:D" {
			if e.ViaFrom != "CLAS:C" {
				t.Errorf("D.ViaFrom: got %q, want CLAS:C", e.ViaFrom)
			}
			if e.ViaEdge != EdgeReferences {
				t.Errorf("D.ViaEdge: got %s, want REFERENCES", e.ViaEdge)
			}
		}
	}
}

func TestImpact_MaxDepthCutoff(t *testing.T) {
	// Chain: D → C → B → A, but max depth = 2
	g := New()
	g.AddNode(&Node{ID: "CLAS:A", Name: "A", Type: NodeCLAS})
	g.AddNode(&Node{ID: "CLAS:B", Name: "B", Type: NodeCLAS})
	g.AddNode(&Node{ID: "CLAS:C", Name: "C", Type: NodeCLAS})
	g.AddNode(&Node{ID: "CLAS:D", Name: "D", Type: NodeCLAS})
	g.AddEdge(&Edge{From: "CLAS:B", To: "CLAS:A", Kind: EdgeCalls, Source: SourceParser})
	g.AddEdge(&Edge{From: "CLAS:C", To: "CLAS:B", Kind: EdgeCalls, Source: SourceParser})
	g.AddEdge(&Edge{From: "CLAS:D", To: "CLAS:C", Kind: EdgeCalls, Source: SourceParser})

	result := Impact(g, "CLAS:A", &ImpactOptions{MaxDepth: 2})

	if len(result.Entries) != 2 {
		t.Errorf("MaxDepth=2: got %d entries, want 2 (B and C, not D)", len(result.Entries))
	}
	for _, e := range result.Entries {
		if e.NodeID == "CLAS:D" {
			t.Error("D should be beyond max depth")
		}
	}
}

func TestImpact_CycleSafety(t *testing.T) {
	// Cycle: A → B → C → A
	g := New()
	g.AddNode(&Node{ID: "CLAS:A", Name: "A", Type: NodeCLAS})
	g.AddNode(&Node{ID: "CLAS:B", Name: "B", Type: NodeCLAS})
	g.AddNode(&Node{ID: "CLAS:C", Name: "C", Type: NodeCLAS})
	g.AddEdge(&Edge{From: "CLAS:B", To: "CLAS:A", Kind: EdgeCalls, Source: SourceParser})
	g.AddEdge(&Edge{From: "CLAS:C", To: "CLAS:B", Kind: EdgeCalls, Source: SourceParser})
	g.AddEdge(&Edge{From: "CLAS:A", To: "CLAS:C", Kind: EdgeCalls, Source: SourceParser}) // cycle back

	result := Impact(g, "CLAS:A", &ImpactOptions{MaxDepth: 10})

	// Should find B and C, not loop forever, and not include A itself
	if len(result.Entries) != 2 {
		t.Errorf("Cycle: got %d entries, want 2 (B and C)", len(result.Entries))
	}
	for _, e := range result.Entries {
		if e.NodeID == "CLAS:A" {
			t.Error("Root should not appear in results")
		}
	}
}

func TestImpact_EdgeKindFilter(t *testing.T) {
	g := New()
	g.AddNode(&Node{ID: "CLAS:TARGET", Name: "TARGET", Type: NodeCLAS})
	g.AddNode(&Node{ID: "PROG:CALLER", Name: "CALLER", Type: NodePROG})
	g.AddNode(&Node{ID: "PROG:REFERER", Name: "REFERER", Type: NodePROG})
	g.AddEdge(&Edge{From: "PROG:CALLER", To: "CLAS:TARGET", Kind: EdgeCalls, Source: SourceParser})
	g.AddEdge(&Edge{From: "PROG:REFERER", To: "CLAS:TARGET", Kind: EdgeReferences, Source: SourceWBCROSSGT})

	// Filter to CALLS only
	result := Impact(g, "CLAS:TARGET", &ImpactOptions{
		EdgeKinds: []EdgeKind{EdgeCalls},
	})

	if len(result.Entries) != 1 {
		t.Fatalf("Filter CALLS: got %d entries, want 1", len(result.Entries))
	}
	if result.Entries[0].NodeID != "PROG:CALLER" {
		t.Errorf("Expected CALLER, got %q", result.Entries[0].NodeID)
	}
}

func TestImpact_SeedNotInResults(t *testing.T) {
	g := New()
	g.AddNode(&Node{ID: "CLAS:ROOT", Name: "ROOT", Type: NodeCLAS})

	result := Impact(g, "CLAS:ROOT", nil)

	if len(result.Entries) != 0 {
		t.Errorf("Isolated node: got %d entries, want 0", len(result.Entries))
	}
}

func TestImpact_NonexistentRoot(t *testing.T) {
	g := New()

	result := Impact(g, "CLAS:GHOST", nil)

	if len(result.Entries) != 0 {
		t.Errorf("Nonexistent root: got %d entries, want 0", len(result.Entries))
	}
	if result.Root != "CLAS:GHOST" {
		t.Errorf("Root: got %q", result.Root)
	}
}

func TestImpact_MixedGraphWithTransport(t *testing.T) {
	// Mixed graph: code deps + transport edges
	// PROG:ZCALLER --CALLS--> CLAS:ZCL_TARGET
	// CLAS:ZCL_TARGET --IN_TRANSPORT--> TR:A4HK900001
	// CLAS:ZCL_OTHER --IN_TRANSPORT--> TR:A4HK900001
	//
	// Impact from ZCL_TARGET should find ZCALLER (via CALLS).
	// Impact from TR:A4HK900001 should find ZCL_TARGET and ZCL_OTHER (via IN_TRANSPORT).
	g := New()
	g.AddNode(&Node{ID: "CLAS:ZCL_TARGET", Name: "ZCL_TARGET", Type: NodeCLAS})
	g.AddNode(&Node{ID: "PROG:ZCALLER", Name: "ZCALLER", Type: NodePROG})
	g.AddNode(&Node{ID: "TR:A4HK900001", Name: "A4HK900001", Type: NodeTR})
	g.AddNode(&Node{ID: "CLAS:ZCL_OTHER", Name: "ZCL_OTHER", Type: NodeCLAS})
	g.AddEdge(&Edge{From: "PROG:ZCALLER", To: "CLAS:ZCL_TARGET", Kind: EdgeCalls, Source: SourceParser})
	g.AddEdge(&Edge{From: "CLAS:ZCL_TARGET", To: "TR:A4HK900001", Kind: EdgeInTransport, Source: SourceE071})
	g.AddEdge(&Edge{From: "CLAS:ZCL_OTHER", To: "TR:A4HK900001", Kind: EdgeInTransport, Source: SourceE071})

	// Impact from code target: only code callers
	r1 := Impact(g, "CLAS:ZCL_TARGET", nil)
	if len(r1.Entries) != 1 {
		t.Errorf("Impact from code target: got %d entries, want 1 (ZCALLER)", len(r1.Entries))
	}
	if len(r1.Entries) == 1 && r1.Entries[0].NodeID != "PROG:ZCALLER" {
		t.Errorf("Expected ZCALLER, got %q", r1.Entries[0].NodeID)
	}

	// Impact from transport at depth=1: only direct objects in that transport
	r2 := Impact(g, "TR:A4HK900001", &ImpactOptions{MaxDepth: 1})
	if len(r2.Entries) != 2 {
		t.Fatalf("Impact from TR depth=1: got %d entries, want 2", len(r2.Entries))
	}
	ids := map[string]bool{}
	for _, e := range r2.Entries {
		ids[e.NodeID] = true
	}
	if !ids["CLAS:ZCL_TARGET"] {
		t.Error("Missing ZCL_TARGET in transport impact")
	}
	if !ids["CLAS:ZCL_OTHER"] {
		t.Error("Missing ZCL_OTHER in transport impact")
	}

	// Mixed: impact from TR with depth=2 should also find ZCALLER (code caller of ZCL_TARGET)
	r3 := Impact(g, "TR:A4HK900001", &ImpactOptions{MaxDepth: 2})
	ids3 := map[string]bool{}
	for _, e := range r3.Entries {
		ids3[e.NodeID] = true
	}
	if !ids3["PROG:ZCALLER"] {
		t.Error("Depth=2 from TR should reach ZCALLER via ZCL_TARGET")
	}

	// Filter to IN_TRANSPORT only from TR: should not reach ZCALLER
	r4 := Impact(g, "TR:A4HK900001", &ImpactOptions{
		MaxDepth:  2,
		EdgeKinds: []EdgeKind{EdgeInTransport},
	})
	for _, e := range r4.Entries {
		if e.NodeID == "PROG:ZCALLER" {
			t.Error("IN_TRANSPORT filter should not reach ZCALLER (requires CALLS edge)")
		}
	}
}

func TestImpact_FanOut(t *testing.T) {
	// Target with many direct dependents
	g := New()
	g.AddNode(&Node{ID: "CLAS:LIB", Name: "LIB", Type: NodeCLAS})
	for i := 0; i < 50; i++ {
		id := NodeID(NodePROG, "ZPROG_"+string(rune('A'+i%26))+string(rune('0'+i/26)))
		g.AddNode(&Node{ID: id, Name: id[5:], Type: NodePROG})
		g.AddEdge(&Edge{From: id, To: "CLAS:LIB", Kind: EdgeReferences, Source: SourceWBCROSSGT})
	}

	result := Impact(g, "CLAS:LIB", &ImpactOptions{MaxDepth: 1})

	if len(result.Entries) != 50 {
		t.Errorf("Fan-out: got %d entries, want 50", len(result.Entries))
	}
}
