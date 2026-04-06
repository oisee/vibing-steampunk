package graph

import (
	"testing"
)

func buildConfigTestGraph() *Graph {
	vars := []TVARVCVariable{
		{Name: "ZKEKEKE", Type: "P"},
		{Name: "ZEMPTY"},
	}
	refs := []TVARVCReference{
		{VariableName: "ZKEKEKE", ObjectType: "PROG", ObjectName: "ZREPORT_A", Confirmed: true},
		{VariableName: "ZKEKEKE", ObjectType: "CLAS", ObjectName: "ZCL_ORDER", Confirmed: false},
		{VariableName: "ZKEKEKE", ObjectType: "PROG", ObjectName: "ZREPORT_B", Confirmed: true},
		// No refs for ZEMPTY
	}
	return BuildConfigGraph(vars, refs)
}

func TestWhereUsedConfig_Basic(t *testing.T) {
	g := buildConfigTestGraph()

	result := WhereUsedConfig(g, "ZKEKEKE")

	if result.Variable != "TVARVC:ZKEKEKE" {
		t.Errorf("Variable: got %q, want TVARVC:ZKEKEKE", result.Variable)
	}
	if result.VariableName != "ZKEKEKE" {
		t.Errorf("VariableName: got %q, want ZKEKEKE", result.VariableName)
	}
	if !result.Found {
		t.Error("Found: should be true")
	}
	if len(result.Readers) != 3 {
		t.Errorf("Readers: got %d, want 3", len(result.Readers))
	}
}

func TestWhereUsedConfig_CanonicalInput(t *testing.T) {
	g := buildConfigTestGraph()

	// Accept canonical TVARVC:ZKEKEKE input
	result := WhereUsedConfig(g, "TVARVC:ZKEKEKE")

	if result.Variable != "TVARVC:ZKEKEKE" {
		t.Errorf("Variable: got %q", result.Variable)
	}
	if result.VariableName != "ZKEKEKE" {
		t.Errorf("VariableName: got %q", result.VariableName)
	}
	if len(result.Readers) != 3 {
		t.Errorf("Readers: got %d, want 3", len(result.Readers))
	}
}

func TestWhereUsedConfig_CaseInsensitiveInput(t *testing.T) {
	g := buildConfigTestGraph()

	result := WhereUsedConfig(g, "zkekeke")
	if len(result.Readers) != 3 {
		t.Errorf("Lowercase input: got %d readers, want 3", len(result.Readers))
	}

	result2 := WhereUsedConfig(g, "tvarvc:zkekeke")
	if len(result2.Readers) != 3 {
		t.Errorf("Lowercase canonical input: got %d readers, want 3", len(result2.Readers))
	}
}

func TestWhereUsedConfig_NoReaders(t *testing.T) {
	g := buildConfigTestGraph()

	result := WhereUsedConfig(g, "ZEMPTY")

	if !result.Found {
		t.Error("Found: ZEMPTY exists, should be true")
	}
	if len(result.Readers) != 0 {
		t.Errorf("Readers: got %d, want 0", len(result.Readers))
	}
}

func TestWhereUsedConfig_NonexistentVariable(t *testing.T) {
	g := buildConfigTestGraph()

	result := WhereUsedConfig(g, "ZGHOST")

	if result.Found {
		t.Error("Found: ZGHOST doesn't exist, should be false")
	}
	if len(result.Readers) != 0 {
		t.Errorf("Readers: got %d, want 0", len(result.Readers))
	}
	if result.Variable != "TVARVC:ZGHOST" {
		t.Errorf("Variable: got %q, want TVARVC:ZGHOST", result.Variable)
	}
}

func TestWhereUsedConfig_ConfidencePropagation(t *testing.T) {
	g := buildConfigTestGraph()

	result := WhereUsedConfig(g, "ZKEKEKE")

	// HIGH-confidence readers should sort first
	if len(result.Readers) < 2 {
		t.Fatal("Need at least 2 readers for this test")
	}
	if result.Readers[0].Confidence != "HIGH" {
		t.Errorf("First reader should be HIGH confidence, got %q", result.Readers[0].Confidence)
	}

	// ZCL_ORDER should be MEDIUM
	for _, r := range result.Readers {
		if r.NodeID == "CLAS:ZCL_ORDER" && r.Confidence != "MEDIUM" {
			t.Errorf("ZCL_ORDER confidence: got %q, want MEDIUM", r.Confidence)
		}
	}
}

func TestWhereUsedConfig_DuplicateEvidenceCollapsed(t *testing.T) {
	// Same object, two edges (MEDIUM then HIGH) → collapsed to one entry
	refs := []TVARVCReference{
		{VariableName: "ZVAR", ObjectType: "PROG", ObjectName: "ZPROG", Confirmed: false},
		{VariableName: "ZVAR", ObjectType: "PROG", ObjectName: "ZPROG", Confirmed: true},
	}
	g := BuildConfigGraph(nil, refs)

	result := WhereUsedConfig(g, "ZVAR")

	if len(result.Readers) != 1 {
		t.Fatalf("Readers: got %d, want 1 (duplicates collapsed)", len(result.Readers))
	}
	r := result.Readers[0]
	if r.Confidence != "HIGH" {
		t.Errorf("Confidence should be promoted to HIGH, got %q", r.Confidence)
	}
	if r.EdgeCount != 2 {
		t.Errorf("EdgeCount should be 2 (both evidence pieces), got %d", r.EdgeCount)
	}
}

func TestWhereUsedConfig_NodeDetails(t *testing.T) {
	g := buildConfigTestGraph()

	result := WhereUsedConfig(g, "ZKEKEKE")

	for _, r := range result.Readers {
		if r.NodeID == "PROG:ZREPORT_A" {
			if r.Name != "ZREPORT_A" {
				t.Errorf("Name: got %q, want ZREPORT_A", r.Name)
			}
			if r.Type != NodePROG {
				t.Errorf("Type: got %q, want %q", r.Type, NodePROG)
			}
			return
		}
	}
	t.Error("ZREPORT_A not found in readers")
}

func TestWhereUsedConfig_SortStability(t *testing.T) {
	// Multiple HIGH readers should be sorted alphabetically by node ID
	refs := []TVARVCReference{
		{VariableName: "ZVAR", ObjectType: "PROG", ObjectName: "ZPROG_C", Confirmed: true},
		{VariableName: "ZVAR", ObjectType: "PROG", ObjectName: "ZPROG_A", Confirmed: true},
		{VariableName: "ZVAR", ObjectType: "PROG", ObjectName: "ZPROG_B", Confirmed: true},
	}
	g := BuildConfigGraph(nil, refs)

	result := WhereUsedConfig(g, "ZVAR")

	if len(result.Readers) != 3 {
		t.Fatalf("Readers: got %d, want 3", len(result.Readers))
	}
	if result.Readers[0].NodeID != "PROG:ZPROG_A" {
		t.Errorf("First: got %q, want PROG:ZPROG_A", result.Readers[0].NodeID)
	}
	if result.Readers[1].NodeID != "PROG:ZPROG_B" {
		t.Errorf("Second: got %q, want PROG:ZPROG_B", result.Readers[1].NodeID)
	}
	if result.Readers[2].NodeID != "PROG:ZPROG_C" {
		t.Errorf("Third: got %q, want PROG:ZPROG_C", result.Readers[2].NodeID)
	}
}
