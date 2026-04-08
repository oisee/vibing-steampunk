package graph

import (
	"testing"
)

func TestResolvePackageScope_Exact(t *testing.T) {
	scope := ResolvePackageScope("$ZLLM", true, nil)

	if scope.Method != "exact" {
		t.Errorf("Method: got %q, want exact", scope.Method)
	}
	if len(scope.Packages) != 1 {
		t.Errorf("Packages: got %d, want 1", len(scope.Packages))
	}
	if !scope.InScope("$ZLLM") {
		t.Error("$ZLLM should be in scope")
	}
	if scope.InScope("$ZLLM_CORE") {
		t.Error("$ZLLM_CORE should NOT be in exact scope")
	}
}

func TestResolvePackageScope_Hierarchy(t *testing.T) {
	tdevc := []TDEVCRow{
		{DevClass: "$ZLLM", ParentCL: ""},
		{DevClass: "$ZLLM_CORE", ParentCL: "$ZLLM"},
		{DevClass: "$ZLLM_UI", ParentCL: "$ZLLM"},
		{DevClass: "$ZLLM_TEST", ParentCL: "$ZLLM"},
		{DevClass: "$ZLLM_CORE_DEEP", ParentCL: "$ZLLM_CORE"},
		{DevClass: "$ZOTHER", ParentCL: ""},           // unrelated
		{DevClass: "$ZLLM2", ParentCL: ""},             // prefix match but NOT child
	}

	scope := ResolvePackageScope("$ZLLM", false, tdevc)

	if scope.Method != "hierarchy" {
		t.Errorf("Method: got %q, want hierarchy", scope.Method)
	}

	// Should include: $ZLLM, $ZLLM_CORE, $ZLLM_UI, $ZLLM_TEST, $ZLLM_CORE_DEEP
	expected := []string{"$ZLLM", "$ZLLM_CORE", "$ZLLM_CORE_DEEP", "$ZLLM_TEST", "$ZLLM_UI"}
	if len(scope.Packages) != len(expected) {
		t.Errorf("Packages: got %d, want %d: %v", len(scope.Packages), len(expected), scope.Packages)
	}
	for _, pkg := range expected {
		if !scope.InScope(pkg) {
			t.Errorf("%s should be in scope", pkg)
		}
	}

	// Should NOT include unrelated packages
	if scope.InScope("$ZOTHER") {
		t.Error("$ZOTHER should NOT be in scope (unrelated)")
	}
	if scope.InScope("$ZLLM2") {
		t.Error("$ZLLM2 should NOT be in scope (not a child, just prefix match)")
	}
}

func TestResolvePackageScope_Mask(t *testing.T) {
	tdevc := []TDEVCRow{
		{DevClass: "$ZLLM_A", ParentCL: ""},
		{DevClass: "$ZLLM_A_SUB", ParentCL: "$ZLLM_A"},
		{DevClass: "$ZLLM_B", ParentCL: ""},
		{DevClass: "$ZOTHER", ParentCL: ""},
	}

	scope := ResolvePackageScope("$ZLLM*", false, tdevc)

	if scope.Method != "mask" {
		t.Errorf("Method: got %q, want mask", scope.Method)
	}

	// Should include $ZLLM_A (matches mask) + $ZLLM_A_SUB (child) + $ZLLM_B (matches mask)
	if !scope.InScope("$ZLLM_A") {
		t.Error("$ZLLM_A should be in scope (matches mask)")
	}
	if !scope.InScope("$ZLLM_A_SUB") {
		t.Error("$ZLLM_A_SUB should be in scope (child of match)")
	}
	if !scope.InScope("$ZLLM_B") {
		t.Error("$ZLLM_B should be in scope (matches mask)")
	}
	if scope.InScope("$ZOTHER") {
		t.Error("$ZOTHER should NOT be in scope")
	}
}

func TestResolvePackageScope_NoTDEVC_Fallback(t *testing.T) {
	// TDEVC not available — fallback to mask-based
	scope := ResolvePackageScope("$ZLLM", false, nil)

	if scope.Method != "mask" {
		t.Errorf("Method: got %q, want mask (fallback)", scope.Method)
	}
	if !scope.InScope("$ZLLM") {
		t.Error("$ZLLM should be in fallback scope")
	}
}

func TestResolvePackageScope_PrefixFallback(t *testing.T) {
	// Root package doesn't exist in TDEVC, but children with similar prefix do
	// This simulates $ZHIRTEST where packages are $ZHIRTEST_00, $ZHIRTEST_001, etc.
	tdevc := []TDEVCRow{
		{DevClass: "$ZHIRTEST_00", ParentCL: ""},
		{DevClass: "$ZHIRTEST_001", ParentCL: ""},  // no PARENTCL set
		{DevClass: "$ZHIRTEST_010", ParentCL: ""},
		{DevClass: "$ZHIRTEST_101", ParentCL: ""},
		{DevClass: "$ZOTHER", ParentCL: ""},
	}

	scope := ResolvePackageScope("$ZHIRTEST", false, tdevc)

	if scope.Method != "prefix" {
		t.Errorf("Method: got %q, want prefix (root not in TDEVC)", scope.Method)
	}
	// Should include all $ZHIRTEST* packages
	if len(scope.Packages) != 4 {
		t.Errorf("Prefix scope: got %d packages, want 4: %v", len(scope.Packages), scope.Packages)
	}
	if scope.InScope("$ZOTHER") {
		t.Error("$ZOTHER should NOT be in scope")
	}
	for _, pkg := range []string{"$ZHIRTEST_00", "$ZHIRTEST_001", "$ZHIRTEST_010", "$ZHIRTEST_101"} {
		if !scope.InScope(pkg) {
			t.Errorf("%s should be in prefix scope", pkg)
		}
	}
}

func TestResolvePackageScope_DeepHierarchy(t *testing.T) {
	// 4 levels deep
	tdevc := []TDEVCRow{
		{DevClass: "$Z", ParentCL: ""},
		{DevClass: "$Z_L1", ParentCL: "$Z"},
		{DevClass: "$Z_L2", ParentCL: "$Z_L1"},
		{DevClass: "$Z_L3", ParentCL: "$Z_L2"},
	}

	scope := ResolvePackageScope("$Z", false, tdevc)

	if len(scope.Packages) != 4 {
		t.Errorf("Deep hierarchy: got %d packages, want 4", len(scope.Packages))
	}
	if !scope.InScope("$Z_L3") {
		t.Error("Deepest child should be in scope")
	}
}

func TestClassifyRefs_InternalExternal(t *testing.T) {
	scopeObjects := map[string]bool{
		"ZCL_A": true,
		"ZCL_B": true,
	}
	refs := []SlimRefRow{
		{CallerInclude: "ZCL_A========CP", TargetName: "ZCL_B", Source: "WBCROSSGT"},       // internal
		{CallerInclude: "ZCL_EXTERNAL========CP", TargetName: "ZCL_B", Source: "WBCROSSGT"}, // external
		{CallerInclude: "ZCL_B========CP", TargetName: "ZCL_B", Source: "WBCROSSGT"},        // self-ref → skipped
	}

	internal, external := ClassifyRefs(refs, scopeObjects)

	if len(internal) != 1 {
		t.Errorf("Internal: got %d, want 1", len(internal))
	}
	if len(external) != 1 {
		t.Errorf("External: got %d, want 1", len(external))
	}
}

func TestClassifyRefs_AllExternal(t *testing.T) {
	scopeObjects := map[string]bool{"ZCL_TARGET": true}
	refs := []SlimRefRow{
		{CallerInclude: "ZCL_OUTSIDE========CP", TargetName: "ZCL_TARGET"},
	}

	internal, external := ClassifyRefs(refs, scopeObjects)

	if len(internal) != 0 {
		t.Errorf("Internal: got %d, want 0", len(internal))
	}
	if len(external) != 1 {
		t.Errorf("External: got %d, want 1", len(external))
	}
}
