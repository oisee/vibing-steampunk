package graph

import (
	"testing"
)

// TestSlimV2_FullHierarchyScenario exercises the complete Slim V2 pipeline
// with a realistic package hierarchy and various reachability patterns.
//
// Hierarchy:
//
//	$ZHIRTEST_00              (root)
//	├── $ZHIRTEST_001         (core)
//	│   ├── ZCL_HIRT_CORE         → called from OUTSIDE scope (LIVE)
//	│   └── ZCL_HIRT_HELPER       → called only by ZCL_HIRT_CORE (INTERNAL_ONLY)
//	├── $ZHIRTEST_010         (utils)
//	│   ├── ZCL_HIRT_UTIL         → called only by ZCL_HIRT_CORE (INTERNAL_ONLY)
//	│   └── ZCL_HIRT_DEAD_UTIL    → never called (DEAD)
//	├── $ZHIRTEST_101         (old/legacy)
//	│   └── ZPROG_HIRT_ORPHAN     → never called (DEAD)
//	│   └── ZCL_HIRT_LEGACY       → called only by ZPROG_HIRT_ORPHAN (INTERNAL_ONLY, but part of dead cluster)
//	└── (root level)
//	    └── ZCL_HIRT_API          → called from OUTSIDE scope (LIVE), calls ZCL_HIRT_CORE
//
// Expected results:
//   LIVE (2):           ZCL_HIRT_API, ZCL_HIRT_CORE
//   DEAD (2):           ZCL_HIRT_DEAD_UTIL, ZPROG_HIRT_ORPHAN
//   INTERNAL_ONLY (3):  ZCL_HIRT_HELPER, ZCL_HIRT_UTIL, ZCL_HIRT_LEGACY
//
// Note: ZCL_HIRT_LEGACY is INTERNAL_ONLY (has ref from ZPROG_HIRT_ORPHAN which is in scope),
// but ZPROG_HIRT_ORPHAN itself is DEAD. This is the "dead cluster" case that V2 flags
// as INTERNAL_ONLY (warning) — true reachability analysis would mark both as unreachable.
// That's V2.1.
func TestSlimV2_FullHierarchyScenario(t *testing.T) {
	// --- Phase 1: Package scope resolution ---
	tdevc := []TDEVCRow{
		{DevClass: "$ZHIRTEST_00", ParentCL: ""},
		{DevClass: "$ZHIRTEST_001", ParentCL: "$ZHIRTEST_00"},
		{DevClass: "$ZHIRTEST_010", ParentCL: "$ZHIRTEST_00"},
		{DevClass: "$ZHIRTEST_101", ParentCL: "$ZHIRTEST_00"},
		// Unrelated packages that should NOT be included
		{DevClass: "$ZHIRTEST_999", ParentCL: ""},
		{DevClass: "$ZOTHER", ParentCL: ""},
	}

	scope := ResolvePackageScope("$ZHIRTEST_00", false, tdevc)

	// Verify scope includes correct packages
	expectedPkgs := []string{"$ZHIRTEST_00", "$ZHIRTEST_001", "$ZHIRTEST_010", "$ZHIRTEST_101"}
	for _, pkg := range expectedPkgs {
		if !scope.InScope(pkg) {
			t.Errorf("Package %s should be in scope", pkg)
		}
	}
	if scope.InScope("$ZHIRTEST_999") {
		t.Error("$ZHIRTEST_999 should NOT be in scope (not a child)")
	}
	if scope.InScope("$ZOTHER") {
		t.Error("$ZOTHER should NOT be in scope")
	}

	// --- Phase 2: Objects in scope ---
	objects := []SlimObjectInfo{
		// $ZHIRTEST_00 (root)
		{Name: "ZCL_HIRT_API", Type: "CLAS", Package: "$ZHIRTEST_00"},
		// $ZHIRTEST_001 (core)
		{Name: "ZCL_HIRT_CORE", Type: "CLAS", Package: "$ZHIRTEST_001"},
		{Name: "ZCL_HIRT_HELPER", Type: "CLAS", Package: "$ZHIRTEST_001"},
		// $ZHIRTEST_010 (utils)
		{Name: "ZCL_HIRT_UTIL", Type: "CLAS", Package: "$ZHIRTEST_010"},
		{Name: "ZCL_HIRT_DEAD_UTIL", Type: "CLAS", Package: "$ZHIRTEST_010"},
		// $ZHIRTEST_101 (legacy)
		{Name: "ZPROG_HIRT_ORPHAN", Type: "PROG", Package: "$ZHIRTEST_101"},
		{Name: "ZCL_HIRT_LEGACY", Type: "CLAS", Package: "$ZHIRTEST_101"},
	}

	// Build scope object set
	scopeObjects := make(map[string]bool)
	for _, obj := range objects {
		scopeObjects[obj.Name] = true
	}

	// --- Phase 3: Cross-references ---
	refs := []SlimRefRow{
		// EXTERNAL callers (not in scope)
		{CallerInclude: "ZCL_EXTERNAL_CONSUMER========CP", TargetName: "ZCL_HIRT_API", Source: "WBCROSSGT"},
		{CallerInclude: "ZCL_ANOTHER_EXTERNAL========CP", TargetName: "ZCL_HIRT_CORE", Source: "WBCROSSGT"},

		// INTERNAL refs (within scope)
		{CallerInclude: "ZCL_HIRT_API========CP", TargetName: "ZCL_HIRT_CORE", Source: "WBCROSSGT"},      // API → CORE
		{CallerInclude: "ZCL_HIRT_CORE========CP", TargetName: "ZCL_HIRT_HELPER", Source: "WBCROSSGT"},    // CORE → HELPER
		{CallerInclude: "ZCL_HIRT_CORE========CP", TargetName: "ZCL_HIRT_UTIL", Source: "WBCROSSGT"},      // CORE → UTIL
		{CallerInclude: "ZPROG_HIRT_ORPHAN", TargetName: "ZCL_HIRT_LEGACY", Source: "CROSS"},              // ORPHAN → LEGACY

		// Self-references (should be ignored)
		{CallerInclude: "ZCL_HIRT_CORE========CP", TargetName: "ZCL_HIRT_CORE", Source: "WBCROSSGT"},
		{CallerInclude: "ZCL_HIRT_API========CU", TargetName: "ZCL_HIRT_API", Source: "WBCROSSGT"},

		// ZCL_HIRT_DEAD_UTIL: no refs at all (truly dead)
		// ZPROG_HIRT_ORPHAN: no incoming refs (truly dead)
	}

	// --- Run Slim V2 ---
	result := ComputeSlim(objects, refs, nil, scopeObjects)

	// --- Verify results ---

	// DEAD objects: ZCL_HIRT_DEAD_UTIL (zero refs) + ZPROG_HIRT_ORPHAN (zero refs)
	if result.DeadObjectCount != 2 {
		t.Errorf("DeadObjectCount: got %d, want 2", result.DeadObjectCount)
		for _, d := range result.DeadObjects {
			t.Logf("  DEAD: %s", d.Name)
		}
	}
	deadNames := make(map[string]bool)
	for _, d := range result.DeadObjects {
		deadNames[d.Name] = true
	}
	if !deadNames["ZCL_HIRT_DEAD_UTIL"] {
		t.Error("ZCL_HIRT_DEAD_UTIL should be DEAD (zero refs)")
	}
	if !deadNames["ZPROG_HIRT_ORPHAN"] {
		t.Error("ZPROG_HIRT_ORPHAN should be DEAD (zero incoming refs)")
	}

	// INTERNAL_ONLY objects: ZCL_HIRT_HELPER, ZCL_HIRT_UTIL, ZCL_HIRT_LEGACY
	if result.InternalOnlyCount != 3 {
		t.Errorf("InternalOnlyCount: got %d, want 3", result.InternalOnlyCount)
		for _, d := range result.InternalOnly {
			t.Logf("  INTERNAL_ONLY: %s (%d internal refs)", d.Name, d.InternalRefs)
		}
	}
	internalNames := make(map[string]bool)
	for _, d := range result.InternalOnly {
		internalNames[d.Name] = true
	}
	if !internalNames["ZCL_HIRT_HELPER"] {
		t.Error("ZCL_HIRT_HELPER should be INTERNAL_ONLY (called only by CORE)")
	}
	if !internalNames["ZCL_HIRT_UTIL"] {
		t.Error("ZCL_HIRT_UTIL should be INTERNAL_ONLY (called only by CORE)")
	}
	if !internalNames["ZCL_HIRT_LEGACY"] {
		t.Error("ZCL_HIRT_LEGACY should be INTERNAL_ONLY (called only by ORPHAN which is in scope)")
	}

	// LIVE objects: ZCL_HIRT_API (external caller) + ZCL_HIRT_CORE (external caller)
	if result.LiveObjectCount != 2 {
		t.Errorf("LiveObjectCount: got %d, want 2 (API + CORE)", result.LiveObjectCount)
	}

	// Totals
	if result.TotalObjects != 7 {
		t.Errorf("TotalObjects: got %d, want 7", result.TotalObjects)
	}
	totalClassified := result.DeadObjectCount + result.InternalOnlyCount + result.LiveObjectCount
	if totalClassified != result.TotalObjects {
		t.Errorf("Classification mismatch: dead(%d) + internal(%d) + live(%d) = %d, want %d",
			result.DeadObjectCount, result.InternalOnlyCount, result.LiveObjectCount,
			totalClassified, result.TotalObjects)
	}
}

func TestSlimV2_MaskScope(t *testing.T) {
	// Test mask-based scope with hierarchy expansion
	tdevc := []TDEVCRow{
		{DevClass: "$ZHIRTEST_A", ParentCL: ""},
		{DevClass: "$ZHIRTEST_A_SUB", ParentCL: "$ZHIRTEST_A"},
		{DevClass: "$ZHIRTEST_B", ParentCL: ""},
		{DevClass: "$ZHIRTEST_B_SUB", ParentCL: "$ZHIRTEST_B"},
		{DevClass: "$ZOTHER", ParentCL: ""},
	}

	scope := ResolvePackageScope("$ZHIRTEST*", false, tdevc)

	// Should include all $ZHIRTEST* packages and their children
	if len(scope.Packages) != 4 {
		t.Errorf("Mask scope: got %d packages, want 4: %v", len(scope.Packages), scope.Packages)
	}
	if scope.InScope("$ZOTHER") {
		t.Error("$ZOTHER should not match mask")
	}
}

func TestSlimV2_DeadClusterWarning(t *testing.T) {
	// A → B → C, where A has no external refs.
	// All are INTERNAL_ONLY (A has zero external refs, B and C called only internally).
	// This is the "dead cluster" case — V2 flags them as INTERNAL_ONLY, V2.1 would mark as unreachable.
	objects := []SlimObjectInfo{
		{Name: "ZCL_CLUSTER_A", Type: "CLAS"},
		{Name: "ZCL_CLUSTER_B", Type: "CLAS"},
		{Name: "ZCL_CLUSTER_C", Type: "CLAS"},
	}
	scopeObjects := map[string]bool{
		"ZCL_CLUSTER_A": true,
		"ZCL_CLUSTER_B": true,
		"ZCL_CLUSTER_C": true,
	}
	refs := []SlimRefRow{
		{CallerInclude: "ZCL_CLUSTER_A========CP", TargetName: "ZCL_CLUSTER_B", Source: "WBCROSSGT"},
		{CallerInclude: "ZCL_CLUSTER_B========CP", TargetName: "ZCL_CLUSTER_C", Source: "WBCROSSGT"},
	}

	result := ComputeSlim(objects, refs, nil, scopeObjects)

	// A has zero refs → DEAD
	if result.DeadObjectCount != 1 {
		t.Errorf("Dead: got %d, want 1 (ZCL_CLUSTER_A has zero refs)", result.DeadObjectCount)
	}
	// B and C have internal refs → INTERNAL_ONLY
	if result.InternalOnlyCount != 2 {
		t.Errorf("Internal: got %d, want 2 (B and C have only internal refs)", result.InternalOnlyCount)
	}
	// Nobody is LIVE (no external refs on anyone)
	if result.LiveObjectCount != 0 {
		t.Errorf("Live: got %d, want 0", result.LiveObjectCount)
	}
}

func TestSlimV2_InternalRefCounts(t *testing.T) {
	// Verify that internal ref counts are reported correctly
	objects := []SlimObjectInfo{
		{Name: "ZCL_TARGET", Type: "CLAS"},
		{Name: "ZCL_CALLER1", Type: "CLAS"},
		{Name: "ZCL_CALLER2", Type: "CLAS"},
	}
	scopeObjects := map[string]bool{
		"ZCL_TARGET":  true,
		"ZCL_CALLER1": true,
		"ZCL_CALLER2": true,
	}
	refs := []SlimRefRow{
		{CallerInclude: "ZCL_CALLER1========CP", TargetName: "ZCL_TARGET", Source: "WBCROSSGT"},
		{CallerInclude: "ZCL_CALLER1========CU", TargetName: "ZCL_TARGET", Source: "WBCROSSGT"},
		{CallerInclude: "ZCL_CALLER2========CP", TargetName: "ZCL_TARGET", Source: "WBCROSSGT"},
	}

	result := ComputeSlim(objects, refs, nil, scopeObjects)

	// ZCL_TARGET should be INTERNAL_ONLY with 3 internal refs
	if result.InternalOnlyCount < 1 {
		t.Fatal("ZCL_TARGET should be INTERNAL_ONLY")
	}
	for _, entry := range result.InternalOnly {
		if entry.Name == "ZCL_TARGET" {
			if entry.InternalRefs != 3 {
				t.Errorf("ZCL_TARGET InternalRefs: got %d, want 3", entry.InternalRefs)
			}
			return
		}
	}
	t.Error("ZCL_TARGET not found in InternalOnly list")
}
