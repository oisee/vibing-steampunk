package graph

import "testing"

func testScope() *PackageScope {
	return &PackageScope{
		RootPackage: "$ZLLM",
		Packages:    []string{"$ZLLM", "$ZLLM_00", "$ZLLM_01", "$ZLLM_02", "$ZLLM_02_SUB", "$ZLLM_04", "$ZLLM_99"},
		PackageSet: map[string]bool{
			"$ZLLM":        true,
			"$ZLLM_00":     true,
			"$ZLLM_01":     true,
			"$ZLLM_02":     true,
			"$ZLLM_02_SUB": true,
			"$ZLLM_04":     true,
			"$ZLLM_99":     true,
		},
		Hierarchy: map[string]string{
			"$ZLLM_00":     "$ZLLM",
			"$ZLLM_01":     "$ZLLM",
			"$ZLLM_02":     "$ZLLM",
			"$ZLLM_02_SUB": "$ZLLM_02",
			"$ZLLM_04":     "$ZLLM",
			"$ZLLM_99":     "$ZLLM",
		},
	}
}

func TestClassifyCrossing_Upward(t *testing.T) {
	scope := testScope()
	// Child → parent
	dir := ClassifyCrossing("$ZLLM_01", "$ZLLM", scope, nil)
	if dir != CrossUpward {
		t.Fatalf("ZLLM_01→ZLLM: got %s, want UPWARD", dir)
	}
}

func TestClassifyCrossing_UpwardFromSubchild(t *testing.T) {
	scope := testScope()
	// Grandchild → parent
	dir := ClassifyCrossing("$ZLLM_02_SUB", "$ZLLM_02", scope, nil)
	if dir != CrossUpward {
		t.Fatalf("ZLLM_02_SUB→ZLLM_02: got %s, want UPWARD", dir)
	}
}

func TestClassifyCrossing_UpwardSkip(t *testing.T) {
	scope := testScope()
	opts := &CrossingOptions{
		CommonPatterns:      []string{"_00"},
		UpwardSkipThreshold: 1, // skip > 1 level = UPWARD_SKIP
	}
	// Grandchild → root (skipping ZLLM_02)
	dir := ClassifyCrossing("$ZLLM_02_SUB", "$ZLLM", scope, opts)
	if dir != CrossUpwardSkip {
		t.Fatalf("ZLLM_02_SUB→ZLLM (skip): got %s, want UPWARD_SKIP", dir)
	}
}

func TestClassifyCrossing_Common(t *testing.T) {
	scope := testScope()
	// Module → common package
	dir := ClassifyCrossing("$ZLLM_02", "$ZLLM_00", scope, nil)
	if dir != CrossCommon {
		t.Fatalf("ZLLM_02→ZLLM_00: got %s, want COMMON", dir)
	}
}

func TestClassifyCrossing_Sibling(t *testing.T) {
	scope := testScope()
	// Sibling → sibling
	dir := ClassifyCrossing("$ZLLM_02", "$ZLLM_01", scope, nil)
	if dir != CrossSibling {
		t.Fatalf("ZLLM_02→ZLLM_01: got %s, want SIBLING", dir)
	}
}

func TestClassifyCrossing_Downward(t *testing.T) {
	scope := testScope()
	// Parent → child
	dir := ClassifyCrossing("$ZLLM", "$ZLLM_01", scope, nil)
	if dir != CrossDownward {
		t.Fatalf("ZLLM→ZLLM_01: got %s, want DOWNWARD", dir)
	}
}

func TestClassifyCrossing_CommonDown(t *testing.T) {
	scope := testScope()
	// Common → specific module
	dir := ClassifyCrossing("$ZLLM_00", "$ZLLM_01", scope, nil)
	if dir != CrossCommonDown {
		t.Fatalf("ZLLM_00→ZLLM_01: got %s, want COMMON_DOWN", dir)
	}
}

func TestClassifyCrossing_External(t *testing.T) {
	scope := testScope()
	// To a package outside the hierarchy
	dir := ClassifyCrossing("$ZLLM_02", "$ZOTHER", scope, nil)
	if dir != CrossExternal {
		t.Fatalf("ZLLM_02→ZOTHER: got %s, want EXTERNAL", dir)
	}
}

func TestClassifyCrossing_Same(t *testing.T) {
	scope := testScope()
	dir := ClassifyCrossing("$ZLLM_02", "$ZLLM_02", scope, nil)
	if dir != CrossSame {
		t.Fatalf("same pkg: got %s, want SAME", dir)
	}
}

func TestAnalyzeCrossings_DetectsCircular(t *testing.T) {
	scope := testScope()
	g := New()

	g.AddNode(&Node{ID: "CLAS:ZCL_A", Name: "ZCL_A", Type: "CLAS", Package: "$ZLLM_01"})
	g.AddNode(&Node{ID: "CLAS:ZCL_B", Name: "ZCL_B", Type: "CLAS", Package: "$ZLLM_02"})

	// Bidirectional sibling dep
	g.AddEdge(&Edge{From: "CLAS:ZCL_A", To: "CLAS:ZCL_B", Kind: EdgeCalls})
	g.AddEdge(&Edge{From: "CLAS:ZCL_B", To: "CLAS:ZCL_A", Kind: EdgeCalls})

	report := AnalyzeCrossings(g, scope, nil)

	if report.Sibling != 2 {
		t.Fatalf("Sibling = %d, want 2", report.Sibling)
	}
	if len(report.Circular) != 1 {
		t.Fatalf("Circular = %d, want 1", len(report.Circular))
	}
}

func TestAnalyzeCrossings_TestPackageExempt(t *testing.T) {
	scope := &PackageScope{
		RootPackage: "$ZLLM",
		Packages:    []string{"$ZLLM", "$ZLLM_01", "$ZLLM_TEST"},
		PackageSet:  map[string]bool{"$ZLLM": true, "$ZLLM_01": true, "$ZLLM_TEST": true},
		Hierarchy:   map[string]string{"$ZLLM_01": "$ZLLM", "$ZLLM_TEST": "$ZLLM"},
	}
	g := New()
	g.AddNode(&Node{ID: "CLAS:ZCL_TEST", Name: "ZCL_TEST", Type: "CLAS", Package: "$ZLLM_TEST"})
	g.AddNode(&Node{ID: "CLAS:ZCL_PROD", Name: "ZCL_PROD", Type: "CLAS", Package: "$ZLLM_01"})
	g.AddEdge(&Edge{From: "CLAS:ZCL_TEST", To: "CLAS:ZCL_PROD", Kind: EdgeCalls})

	report := AnalyzeCrossings(g, scope, nil)
	// Test package → sibling is exempt, should not count as sibling
	if report.Sibling != 0 {
		t.Fatalf("Sibling = %d, want 0 (test package exempt)", report.Sibling)
	}
}

func TestClassifyCrossing_PrefixFallback(t *testing.T) {
	// No hierarchy — fall back to prefix-based classification
	scope := &PackageScope{
		RootPackage: "$ZLLM",
		Packages:    []string{"$ZLLM", "$ZLLM_00", "$ZLLM_01", "$ZLLM_02"},
		PackageSet:  map[string]bool{"$ZLLM": true, "$ZLLM_00": true, "$ZLLM_01": true, "$ZLLM_02": true},
		Hierarchy:   map[string]string{}, // empty — no TDEVC
	}

	// Upward by prefix
	dir := ClassifyCrossing("$ZLLM_01", "$ZLLM", scope, nil)
	if dir != CrossUpward {
		t.Fatalf("prefix upward: got %s, want UPWARD", dir)
	}

	// Downward by prefix
	dir = ClassifyCrossing("$ZLLM", "$ZLLM_01", scope, nil)
	if dir != CrossDownward {
		t.Fatalf("prefix downward: got %s, want DOWNWARD", dir)
	}

	// Sibling by prefix
	dir = ClassifyCrossing("$ZLLM_01", "$ZLLM_02", scope, nil)
	if dir != CrossSibling {
		t.Fatalf("prefix sibling: got %s, want SIBLING", dir)
	}
}

func TestGuessPackageFromName(t *testing.T) {
	tests := []struct{ name, want string }{
		{"ZCL_LLM_00_CACHE", "$ZLLM_00"},
		{"ZIF_RAY_00_NODE", "$ZRAY_00"},
		{"ZRAY_00_CCLM", "$ZRAY_00"},
		{"ZCX_LLM_00_ERROR", "$ZLLM_00"},
		{"ZCL_LLM_05_TRACE_WS", "$ZLLM_05"},
		{"ZLLM_00_NODE", "$ZLLM_00"},
		{"CL_ABAP_TYPEDESCR", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := GuessPackageFromName(tt.name)
		if got != tt.want {
			t.Errorf("GuessPackageFromName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}
