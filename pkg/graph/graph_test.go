package graph

import (
	"testing"
)

func TestNormalizeInclude(t *testing.T) {
	tests := []struct {
		include  string
		wantID   string
		wantType string
		wantName string
	}{
		{"ZCL_FOO========CP", "CLAS:ZCL_FOO", "CLAS", "ZCL_FOO"},
		{"ZCL_FOO========CU", "CLAS:ZCL_FOO", "CLAS", "ZCL_FOO"},
		{"ZCL_FOO========CM001", "CLAS:ZCL_FOO", "CLAS", "ZCL_FOO"},
		{"ZCL_FOO========CT", "CLAS:ZCL_FOO", "CLAS", "ZCL_FOO"},
		{"ZIF_BAR========IP", "INTF:ZIF_BAR", "INTF", "ZIF_BAR"},
		{"ZIF_BAR========IU", "INTF:ZIF_BAR", "INTF", "ZIF_BAR"},
		{"ZREPORT", "PROG:ZREPORT", "PROG", "ZREPORT"},
	}

	for _, tt := range tests {
		t.Run(tt.include, func(t *testing.T) {
			id, typ, name := NormalizeInclude(tt.include)
			if id != tt.wantID {
				t.Errorf("ID: got %q, want %q", id, tt.wantID)
			}
			if typ != tt.wantType {
				t.Errorf("Type: got %q, want %q", typ, tt.wantType)
			}
			if name != tt.wantName {
				t.Errorf("Name: got %q, want %q", name, tt.wantName)
			}
		})
	}
}

func TestIsStandardObject(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"CL_GUI_ALV_GRID", true},
		{"ZCL_CUSTOM", false},
		{"YCL_CUSTOM", false},
		{"BAPI_USER_GET_DETAIL", true},
		{"", true},
	}
	for _, tt := range tests {
		if got := IsStandardObject(tt.name); got != tt.want {
			t.Errorf("IsStandardObject(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestGraphBasics(t *testing.T) {
	g := New()

	// Add nodes
	g.AddNode(&Node{ID: "CLAS:ZCL_A", Name: "ZCL_A", Type: "CLAS", Package: "$ZDEV"})
	g.AddNode(&Node{ID: "CLAS:ZCL_B", Name: "ZCL_B", Type: "CLAS", Package: "$ZDEV"})
	g.AddNode(&Node{ID: "CLAS:ZCL_EXT", Name: "ZCL_EXT", Type: "CLAS", Package: "$ZOTHER"})

	if g.NodeCount() != 3 {
		t.Errorf("NodeCount: got %d, want 3", g.NodeCount())
	}

	// Add edges
	g.AddEdge(&Edge{From: "CLAS:ZCL_A", To: "CLAS:ZCL_B", Kind: EdgeCalls, Source: SourceParser})
	g.AddEdge(&Edge{From: "CLAS:ZCL_A", To: "CLAS:ZCL_EXT", Kind: EdgeReferences, Source: SourceCROSS})

	if g.EdgeCount() != 2 {
		t.Errorf("EdgeCount: got %d, want 2", g.EdgeCount())
	}

	// Check adjacency
	out := g.OutEdges("CLAS:ZCL_A")
	if len(out) != 2 {
		t.Errorf("OutEdges(ZCL_A): got %d, want 2", len(out))
	}
	in := g.InEdges("CLAS:ZCL_B")
	if len(in) != 1 {
		t.Errorf("InEdges(ZCL_B): got %d, want 1", len(in))
	}

	// Node merge
	g.AddNode(&Node{ID: "CLAS:ZCL_A", Package: "$ZNEW"}) // package won't overwrite non-empty
	if n := g.GetNode("CLAS:ZCL_A"); n.Package != "$ZDEV" {
		t.Errorf("Package should not be overwritten: got %q", n.Package)
	}
	g.AddNode(&Node{ID: "CLAS:ZCL_B", Package: ""}) // empty won't overwrite
	if n := g.GetNode("CLAS:ZCL_B"); n.Package != "$ZDEV" {
		t.Errorf("Package should stay: got %q", n.Package)
	}
}

func TestGraphStats(t *testing.T) {
	g := New()
	g.AddNode(&Node{ID: "CLAS:ZCL_A", Type: "CLAS", Package: "$ZDEV"})
	g.AddNode(&Node{ID: "PROG:ZPROG", Type: "PROG", Package: "$ZDEV"})
	g.AddNode(&Node{ID: "CLAS:CL_STD", Type: "CLAS", Package: "SLIS"})
	g.AddEdge(&Edge{From: "CLAS:ZCL_A", To: "PROG:ZPROG", Kind: EdgeCalls, Source: SourceParser})
	g.AddEdge(&Edge{From: "CLAS:ZCL_A", To: "CLAS:CL_STD", Kind: EdgeReferences, Source: SourceCROSS})

	s := g.Stats()
	if s.NodeCount != 3 {
		t.Errorf("Stats.NodeCount: got %d, want 3", s.NodeCount)
	}
	if s.EdgeCount != 2 {
		t.Errorf("Stats.EdgeCount: got %d, want 2", s.EdgeCount)
	}
	if s.ByNodeType["CLAS"] != 2 {
		t.Errorf("Stats.ByNodeType[CLAS]: got %d, want 2", s.ByNodeType["CLAS"])
	}
	if s.ByEdgeKind[EdgeCalls] != 1 {
		t.Errorf("Stats.ByEdgeKind[CALLS]: got %d, want 1", s.ByEdgeKind[EdgeCalls])
	}
	if s.BySource[SourceParser] != 1 {
		t.Errorf("Stats.BySource[PARSER]: got %d, want 1", s.BySource[SourceParser])
	}
}

func TestNodeMeta(t *testing.T) {
	n := &Node{ID: "CLAS:ZCL_FOO", Name: "ZCL_FOO", Type: NodeCLAS}

	// Meta starts nil
	if _, ok := n.GetMeta(MetaConfidence); ok {
		t.Error("Expected no meta initially")
	}

	// SetMeta initializes map
	n.SetMeta(MetaConfidence, "HIGH")
	v, ok := n.GetMeta(MetaConfidence)
	if !ok || v != "HIGH" {
		t.Errorf("GetMeta(confidence): got %v/%v, want HIGH/true", v, ok)
	}

	// Additional meta
	n.SetMeta(MetaIsStandard, false)
	v2, ok := n.GetMeta(MetaIsStandard)
	if !ok || v2 != false {
		t.Errorf("GetMeta(is_standard): got %v/%v, want false/true", v2, ok)
	}
}

func TestEdgeMeta(t *testing.T) {
	e := &Edge{From: "PROG:ZTEST", To: "TVARVC:ZKEKEKE", Kind: EdgeReadsConfig, Source: SourceTVARVC_CROSS}

	e.SetMeta(MetaConfidence, "MEDIUM")
	v, ok := e.GetMeta(MetaConfidence)
	if !ok || v != "MEDIUM" {
		t.Errorf("Edge.GetMeta(confidence): got %v/%v, want MEDIUM/true", v, ok)
	}
}

func TestNodeMetaMerge(t *testing.T) {
	g := New()

	// First add with meta
	g.AddNode(&Node{
		ID: "CLAS:ZCL_A", Name: "ZCL_A", Type: NodeCLAS,
		Meta: map[string]any{MetaConfidence: "LOW", MetaIsStandard: false},
	})

	// Second add merges meta (new keys win)
	g.AddNode(&Node{
		ID: "CLAS:ZCL_A", Name: "ZCL_A", Type: NodeCLAS,
		Meta: map[string]any{MetaConfidence: "HIGH", MetaLastTransport: "20260405"},
	})

	n := g.GetNode("CLAS:ZCL_A")
	if v, _ := n.GetMeta(MetaConfidence); v != "HIGH" {
		t.Errorf("Meta merge should overwrite: got %v, want HIGH", v)
	}
	if v, _ := n.GetMeta(MetaIsStandard); v != false {
		t.Errorf("Meta merge should preserve existing: got %v, want false", v)
	}
	if v, _ := n.GetMeta(MetaLastTransport); v != "20260405" {
		t.Errorf("Meta merge should add new: got %v, want 20260405", v)
	}
}

func TestNewEdgeKindsAndSources(t *testing.T) {
	g := New()

	// Transport nodes and edges
	g.AddNode(&Node{ID: "CLAS:ZCL_FOO", Name: "ZCL_FOO", Type: NodeCLAS, Package: "$ZDEV"})
	g.AddNode(&Node{ID: "TR:A4HK900123", Name: "A4HK900123", Type: NodeTR})
	g.AddEdge(&Edge{From: "CLAS:ZCL_FOO", To: "TR:A4HK900123", Kind: EdgeInTransport, Source: SourceE071})

	// Config nodes and edges
	g.AddNode(&Node{ID: "TVARVC:ZKEKEKE", Name: "ZKEKEKE", Type: NodeTVARVC})
	g.AddNode(&Node{ID: "PROG:ZREPORT", Name: "ZREPORT", Type: NodePROG, Package: "$ZDEV"})
	g.AddEdge(&Edge{From: "PROG:ZREPORT", To: "TVARVC:ZKEKEKE", Kind: EdgeReadsConfig, Source: SourceTVARVC_CROSS})

	// Verify graph structure
	if g.NodeCount() != 4 {
		t.Errorf("NodeCount: got %d, want 4", g.NodeCount())
	}
	if g.EdgeCount() != 2 {
		t.Errorf("EdgeCount: got %d, want 2", g.EdgeCount())
	}

	// Transport: object → TR via OutEdges
	out := g.OutEdges("CLAS:ZCL_FOO")
	if len(out) != 1 || out[0].Kind != EdgeInTransport {
		t.Errorf("Expected IN_TRANSPORT edge from ZCL_FOO")
	}

	// Config: TVARVC ← PROG via InEdges (impact traversal direction)
	in := g.InEdges("TVARVC:ZKEKEKE")
	if len(in) != 1 || in[0].Kind != EdgeReadsConfig {
		t.Errorf("Expected READS_CONFIG edge to ZKEKEKE")
	}
	if in[0].Source != SourceTVARVC_CROSS {
		t.Errorf("Expected TVARVC_CROSS source, got %s", in[0].Source)
	}

	// Stats should reflect new types
	s := g.Stats()
	if s.ByNodeType[NodeTR] != 1 {
		t.Errorf("Stats.ByNodeType[TR]: got %d, want 1", s.ByNodeType[NodeTR])
	}
	if s.ByNodeType[NodeTVARVC] != 1 {
		t.Errorf("Stats.ByNodeType[TVARVC]: got %d, want 1", s.ByNodeType[NodeTVARVC])
	}
	if s.ByEdgeKind[EdgeInTransport] != 1 {
		t.Errorf("Stats.ByEdgeKind[IN_TRANSPORT]: got %d, want 1", s.ByEdgeKind[EdgeInTransport])
	}
	if s.ByEdgeKind[EdgeReadsConfig] != 1 {
		t.Errorf("Stats.ByEdgeKind[READS_CONFIG]: got %d, want 1", s.ByEdgeKind[EdgeReadsConfig])
	}
	if s.BySource[SourceE071] != 1 {
		t.Errorf("Stats.BySource[E071]: got %d, want 1", s.BySource[SourceE071])
	}
}

func TestExtractDepsFromSource(t *testing.T) {
	source := `REPORT ztest.
DATA lo TYPE REF TO zcl_helper.
DATA lt TYPE TABLE OF ztable.
CALL FUNCTION 'Z_MY_FUNC' EXPORTING iv = lv.
SUBMIT zother_report.
CREATE OBJECT lo TYPE zcl_factory.
SELECT * FROM ztable INTO TABLE lt.
INCLUDE ztest_incl.
lo->method( ).
zcl_utils=>static_method( ).
CALL TRANSACTION 'ZMY_TCODE'.
LEAVE TO TRANSACTION 'VA01'.
CALL TRANSFORMATION zmy_xslt SOURCE data = ls_data RESULT XML lv_xml.
`

	edges := ExtractDepsFromSource(source, "PROG:ZTEST")

	// Collect edge details
	found := make(map[string]bool)
	for _, e := range edges {
		key := string(e.Kind) + ":" + e.To
		found[key] = true
		t.Logf("Edge: %s → %s (%s, %s) [%s]", e.From, e.To, e.Kind, e.Source, e.RefDetail)
	}

	// Check expected edges
	expected := []struct {
		kind EdgeKind
		to   string
	}{
		{EdgeCalls, "FUGR:Z_MY_FUNC"},
		{EdgeCalls, "PROG:ZOTHER_REPORT"},
		{EdgeReferences, "CLAS:ZCL_HELPER"},
		{EdgeReferences, "CLAS:ZCL_FACTORY"},
		{EdgeContainsInclude, "PROG:ZTEST_INCL"},
		{EdgeCalls, "CLAS:ZCL_UTILS"},
		{EdgeCalls, "TRAN:ZMY_TCODE"},
		{EdgeCalls, "TRAN:VA01"},
		{EdgeCalls, "XSLT:ZMY_XSLT"},
	}

	for _, exp := range expected {
		key := string(exp.kind) + ":" + exp.to
		if !found[key] {
			t.Errorf("Missing edge: %s → %s", exp.kind, exp.to)
		}
	}

	// Check SELECT FROM
	hasTableRef := false
	for _, e := range edges {
		if e.Kind == EdgeReferences && e.To == "TABL:ZTABLE" {
			hasTableRef = true
		}
	}
	if !hasTableRef {
		t.Error("Missing SELECT FROM ztable reference")
	}
}

func TestExtractDynamicCalls(t *testing.T) {
	source := `REPORT ztest.
DATA lv_fm TYPE string.
lv_fm = 'Z_DYNAMIC_FM'.
CALL FUNCTION lv_fm EXPORTING iv = lv.
SUBMIT (lv_prog).
CREATE OBJECT lo TYPE (lv_class).
`

	edges := ExtractDynamicCalls(source, "PROG:ZTEST")

	if len(edges) == 0 {
		t.Fatal("Expected dynamic call edges, got none")
	}

	for _, e := range edges {
		if e.Kind != EdgeDynamic {
			t.Errorf("Expected DYNAMIC_CALL kind, got %s", e.Kind)
		}
		t.Logf("Dynamic: %s → %s [%s]", e.From, e.To, e.RefDetail)
	}

	// At least CALL FUNCTION lv_fm should be detected
	found := false
	for _, e := range edges {
		if e.RefDetail == "DYNAMIC_FM:lv_fm" {
			found = true
		}
	}
	if !found {
		t.Error("Missing dynamic FM call detection for lv_fm")
	}
}
