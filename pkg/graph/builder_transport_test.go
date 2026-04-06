package graph

import (
	"testing"
)

func TestBuildTransportGraph_Basic(t *testing.T) {
	headers := []TransportHeader{
		{TRKORR: "A4HK900001", TRFUNCTION: "K", TRSTATUS: "R", AS4USER: "DEVELOPER", AS4DATE: "20260401", AS4TEXT: "Feature A"},
		{TRKORR: "A4HK900002", TRFUNCTION: "W", TRSTATUS: "D", AS4USER: "ADMIN", AS4DATE: "20260405"},
	}
	objects := []TransportObject{
		{TRKORR: "A4HK900001", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_FOO"},
		{TRKORR: "A4HK900001", PGMID: "R3TR", Object: "PROG", ObjName: "ZREPORT"},
		{TRKORR: "A4HK900002", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_FOO"}, // same object, different TR
		{TRKORR: "A4HK900002", PGMID: "R3TR", Object: "TABL", ObjName: "ZTABLE"},
	}

	g := BuildTransportGraph(headers, objects)

	// 2 TR nodes + 3 object nodes (ZCL_FOO, ZREPORT, ZTABLE)
	if g.NodeCount() != 5 {
		t.Errorf("NodeCount: got %d, want 5", g.NodeCount())
	}

	// 4 IN_TRANSPORT edges (ZCL_FOO appears in both TRs)
	if g.EdgeCount() != 4 {
		t.Errorf("EdgeCount: got %d, want 4", g.EdgeCount())
	}

	// Check TR node metadata
	tr1 := g.GetNode("TR:A4HK900001")
	if tr1 == nil {
		t.Fatal("TR:A4HK900001 not found")
	}
	if tr1.Type != NodeTR {
		t.Errorf("TR node type: got %q, want %q", tr1.Type, NodeTR)
	}
	if v, _ := tr1.GetMeta("trfunction"); v != "K" {
		t.Errorf("TR trfunction: got %v, want K", v)
	}
	if v, _ := tr1.GetMeta("as4text"); v != "Feature A" {
		t.Errorf("TR as4text: got %v, want 'Feature A'", v)
	}

	// Check object → TR edge direction
	fooEdges := g.OutEdges("CLAS:ZCL_FOO")
	if len(fooEdges) != 2 {
		t.Errorf("ZCL_FOO should have 2 outgoing IN_TRANSPORT edges, got %d", len(fooEdges))
	}
	for _, e := range fooEdges {
		if e.Kind != EdgeInTransport {
			t.Errorf("Expected IN_TRANSPORT, got %s", e.Kind)
		}
		if e.Source != SourceE071 {
			t.Errorf("Expected E071 source, got %s", e.Source)
		}
	}

	// Check InEdges on TR node
	tr1In := g.InEdges("TR:A4HK900001")
	if len(tr1In) != 2 {
		t.Errorf("TR:A4HK900001 should have 2 incoming edges, got %d", len(tr1In))
	}
}

func TestBuildTransportGraph_TaskCollapse(t *testing.T) {
	headers := []TransportHeader{
		// Parent request
		{TRKORR: "A4HK900010", TRFUNCTION: "K", TRSTATUS: "D", AS4USER: "LEAD", AS4DATE: "20260401"},
		// Task under that request
		{TRKORR: "A4HK900011", STRKORR: "A4HK900010", TRFUNCTION: "K", TRSTATUS: "D", AS4USER: "DEV1", AS4DATE: "20260402"},
		// Another task
		{TRKORR: "A4HK900012", STRKORR: "A4HK900010", TRFUNCTION: "K", TRSTATUS: "D", AS4USER: "DEV2", AS4DATE: "20260403"},
	}
	objects := []TransportObject{
		// Object in task 1
		{TRKORR: "A4HK900011", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_FROM_TASK1"},
		// Object in task 2
		{TRKORR: "A4HK900012", PGMID: "R3TR", Object: "PROG", ObjName: "ZPROG_FROM_TASK2"},
		// Object directly in request
		{TRKORR: "A4HK900010", PGMID: "R3TR", Object: "TABL", ObjName: "ZTABLE_DIRECT"},
	}

	g := BuildTransportGraph(headers, objects)

	// Only 1 TR node (request), no task nodes
	trCount := 0
	for _, n := range g.Nodes() {
		if n.Type == NodeTR {
			trCount++
		}
	}
	if trCount != 1 {
		t.Errorf("Expected 1 TR node (request only), got %d", trCount)
	}

	// All 3 objects should link to the parent request
	requestID := "TR:A4HK900010"
	inEdges := g.InEdges(requestID)
	if len(inEdges) != 3 {
		t.Errorf("Parent request should have 3 incoming edges (collapsed from tasks), got %d", len(inEdges))
	}

	// Verify task objects are resolved to parent
	task1Obj := g.OutEdges("CLAS:ZCL_FROM_TASK1")
	if len(task1Obj) != 1 {
		t.Fatal("ZCL_FROM_TASK1 should have 1 edge")
	}
	if task1Obj[0].To != requestID {
		t.Errorf("Task object should link to parent request %s, got %s", requestID, task1Obj[0].To)
	}

	// Edge should carry parent request metadata, not task metadata
	if v, _ := task1Obj[0].GetMeta("as4user"); v != "LEAD" {
		t.Errorf("Edge as4user should be request owner LEAD, got %v", v)
	}
}

func TestBuildTransportGraph_SkipLIMU(t *testing.T) {
	headers := []TransportHeader{
		{TRKORR: "A4HK900020", TRFUNCTION: "K", TRSTATUS: "R", AS4USER: "DEV", AS4DATE: "20260401"},
	}
	objects := []TransportObject{
		{TRKORR: "A4HK900020", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_MAIN"},
		{TRKORR: "A4HK900020", PGMID: "LIMU", Object: "METH", ObjName: "ZCL_MAIN METHOD1"},
		{TRKORR: "A4HK900020", PGMID: "LIMU", Object: "CLSD", ObjName: "ZCL_MAIN"},
	}

	g := BuildTransportGraph(headers, objects)

	// Only 1 object node (LIMU entries skipped) + 1 TR node
	if g.NodeCount() != 2 {
		t.Errorf("NodeCount: got %d, want 2 (1 object + 1 TR, LIMU skipped)", g.NodeCount())
	}
	if g.EdgeCount() != 1 {
		t.Errorf("EdgeCount: got %d, want 1 (LIMU edges skipped)", g.EdgeCount())
	}
}

func TestBuildTransportGraph_Empty(t *testing.T) {
	g := BuildTransportGraph(nil, nil)

	if g.NodeCount() != 0 {
		t.Errorf("Empty graph NodeCount: got %d, want 0", g.NodeCount())
	}
	if g.EdgeCount() != 0 {
		t.Errorf("Empty graph EdgeCount: got %d, want 0", g.EdgeCount())
	}
}

func TestBuildTransportGraph_OrphanObjects(t *testing.T) {
	// Objects referencing a transport that has no header
	headers := []TransportHeader{
		{TRKORR: "A4HK900030", TRFUNCTION: "K", TRSTATUS: "R", AS4USER: "DEV", AS4DATE: "20260401"},
	}
	objects := []TransportObject{
		{TRKORR: "A4HK900030", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_GOOD"},
		{TRKORR: "A4HK999999", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_ORPHAN"}, // no header for this TR
	}

	g := BuildTransportGraph(headers, objects)

	// Orphan object should be silently skipped
	if g.NodeCount() != 2 { // ZCL_GOOD + TR:A4HK900030
		t.Errorf("NodeCount: got %d, want 2 (orphan skipped)", g.NodeCount())
	}
	if g.GetNode("CLAS:ZCL_ORPHAN") != nil {
		t.Error("Orphan object should not be in graph")
	}
}

func TestBuildTransportGraph_Stats(t *testing.T) {
	headers := []TransportHeader{
		{TRKORR: "A4HK900040", TRFUNCTION: "K", TRSTATUS: "R", AS4USER: "DEV", AS4DATE: "20260401"},
	}
	objects := []TransportObject{
		{TRKORR: "A4HK900040", PGMID: "R3TR", Object: "CLAS", ObjName: "ZCL_A"},
		{TRKORR: "A4HK900040", PGMID: "R3TR", Object: "PROG", ObjName: "ZPROG_B"},
	}

	g := BuildTransportGraph(headers, objects)
	s := g.Stats()

	if s.ByNodeType[NodeTR] != 1 {
		t.Errorf("Stats TR nodes: got %d, want 1", s.ByNodeType[NodeTR])
	}
	if s.ByNodeType[NodeCLAS] != 1 {
		t.Errorf("Stats CLAS nodes: got %d, want 1", s.ByNodeType[NodeCLAS])
	}
	if s.ByEdgeKind[EdgeInTransport] != 2 {
		t.Errorf("Stats IN_TRANSPORT edges: got %d, want 2", s.ByEdgeKind[EdgeInTransport])
	}
	if s.BySource[SourceE071] != 2 {
		t.Errorf("Stats E071 source: got %d, want 2", s.BySource[SourceE071])
	}
}

func TestTransportHeader_IsRequest(t *testing.T) {
	request := TransportHeader{TRKORR: "A4HK900001", STRKORR: ""}
	task := TransportHeader{TRKORR: "A4HK900002", STRKORR: "A4HK900001"}

	if !request.IsRequest() {
		t.Error("Request should be identified as request")
	}
	if task.IsRequest() {
		t.Error("Task should not be identified as request")
	}
}
