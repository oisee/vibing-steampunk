package graph

import (
	"testing"
)

func TestComputeRenamePreview_Basic(t *testing.T) {
	refs := []RenameRefRow{
		{CallerInclude: "ZCL_CONSUMER========CP", TargetName: "ZCL_OLD", RefType: "TY", Source: "WBCROSSGT"},
		{CallerInclude: "ZCL_CONSUMER========CP", TargetName: "ZCL_OLD", RefType: "ME", Source: "WBCROSSGT"},
		{CallerInclude: "ZREPORT", TargetName: "ZCL_OLD", RefType: "TY", Source: "WBCROSSGT"},
	}

	result := ComputeRenamePreview("CLAS", "ZCL_OLD", "ZCL_NEW", refs)

	if result.OldName != "ZCL_OLD" {
		t.Errorf("OldName: got %q", result.OldName)
	}
	if result.NewName != "ZCL_NEW" {
		t.Errorf("NewName: got %q", result.NewName)
	}
	if result.AffectedCount != 2 {
		t.Errorf("AffectedCount: got %d, want 2 (ZCL_CONSUMER + ZREPORT)", result.AffectedCount)
	}
	if result.TotalRefs != 3 {
		t.Errorf("TotalRefs: got %d, want 3", result.TotalRefs)
	}
}

func TestComputeRenamePreview_SelfRefSkipped(t *testing.T) {
	refs := []RenameRefRow{
		{CallerInclude: "ZCL_OLD========CP", TargetName: "ZCL_OLD", Source: "WBCROSSGT"},
		{CallerInclude: "ZCL_OLD========CU", TargetName: "ZCL_OLD", Source: "WBCROSSGT"},
		{CallerInclude: "ZCL_EXTERNAL========CP", TargetName: "ZCL_OLD", Source: "WBCROSSGT"},
	}

	result := ComputeRenamePreview("CLAS", "ZCL_OLD", "ZCL_NEW", refs)

	if result.AffectedCount != 1 {
		t.Errorf("Self-refs skipped: got %d affected, want 1", result.AffectedCount)
	}
	if result.Refs[0].CallerName != "ZCL_EXTERNAL" {
		t.Errorf("Expected ZCL_EXTERNAL, got %q", result.Refs[0].CallerName)
	}
}

func TestComputeRenamePreview_Confidence(t *testing.T) {
	refs := []RenameRefRow{
		// Exact match: TargetName = ZCL_OLD exactly
		{CallerInclude: "ZCL_EXACT========CP", TargetName: "ZCL_OLD", Source: "WBCROSSGT"},
		// Prefix match: TargetName starts with ZCL_OLD but is longer (from LIKE query)
		{CallerInclude: "ZCL_PREFIX========CP", TargetName: "ZCL_OLD_HELPER", Source: "WBCROSSGT"},
	}

	result := ComputeRenamePreview("CLAS", "ZCL_OLD", "ZCL_NEW", refs)

	if len(result.Refs) != 2 {
		t.Fatalf("Expected 2 refs, got %d", len(result.Refs))
	}
	// HIGH should sort first
	if result.Refs[0].Confidence != "HIGH" {
		t.Errorf("First ref should be HIGH confidence, got %q", result.Refs[0].Confidence)
	}
	if result.Refs[1].Confidence != "MEDIUM" {
		t.Errorf("Second ref should be MEDIUM confidence (prefix match), got %q", result.Refs[1].Confidence)
	}
}

func TestComputeRenamePreview_MultiSource(t *testing.T) {
	refs := []RenameRefRow{
		{CallerInclude: "ZREPORT", TargetName: "Z_MY_FM", RefType: "FU", Source: "CROSS"},
		{CallerInclude: "ZREPORT", TargetName: "Z_MY_FM", RefType: "TY", Source: "WBCROSSGT"},
	}

	result := ComputeRenamePreview("FUNC", "Z_MY_FM", "Z_NEW_FM", refs)

	if result.AffectedCount != 1 {
		t.Errorf("Same caller from two sources: got %d affected, want 1", result.AffectedCount)
	}
	if result.Refs[0].RefCount != 2 {
		t.Errorf("RefCount should combine both sources: got %d, want 2", result.Refs[0].RefCount)
	}
	// Source should show both
	if result.Refs[0].Source != "CROSS+WBCROSSGT" {
		t.Errorf("Source: got %q, want CROSS+WBCROSSGT", result.Refs[0].Source)
	}
}

func TestComputeRenamePreview_Risks_CLAS(t *testing.T) {
	result := ComputeRenamePreview("CLAS", "ZCL_OLD", "ZCL_NEW", nil)

	riskKinds := make(map[string]bool)
	for _, r := range result.Risks {
		riskKinds[r.Kind] = true
	}
	if !riskKinds["DYNAMIC_CALL"] {
		t.Error("CLAS rename should warn about dynamic calls")
	}
	if !riskKinds["STRING_LITERAL"] {
		t.Error("Should warn about string literals")
	}
	if !riskKinds["CONFIG_REF"] {
		t.Error("Should warn about config references")
	}
}

func TestComputeRenamePreview_Risks_FUNC(t *testing.T) {
	result := ComputeRenamePreview("FUNC", "Z_OLD_FM", "Z_NEW_FM", nil)

	found := false
	for _, r := range result.Risks {
		if r.Kind == "DYNAMIC_CALL" && containsStr(r.Description, "CALL FUNCTION") {
			found = true
		}
	}
	if !found {
		t.Error("FUNC rename should warn about dynamic CALL FUNCTION")
	}
}

func TestComputeRenamePreview_Risks_NameOverflow(t *testing.T) {
	result := ComputeRenamePreview("CLAS", "ZCL_SHORT", "ZCL_THIS_NAME_IS_WAY_TOO_LONG_FOR_SAP_OBJECTS", nil)

	found := false
	for _, r := range result.Risks {
		if r.Kind == "NAME_OVERFLOW" {
			found = true
		}
	}
	if !found {
		t.Error("Should warn about name overflow >30 chars")
	}
}

func TestComputeRenamePreview_Empty(t *testing.T) {
	result := ComputeRenamePreview("CLAS", "ZCL_ORPHAN", "ZCL_NEW", nil)

	if result.AffectedCount != 0 {
		t.Errorf("No refs: got %d affected", result.AffectedCount)
	}
	// Risks should still be present
	if len(result.Risks) == 0 {
		t.Error("Risks should always be generated even with no refs")
	}
}

func TestComputeRenamePreview_Sorting(t *testing.T) {
	refs := []RenameRefRow{
		{CallerInclude: "ZCL_LOW========CP", TargetName: "ZCL_OLD", Source: "WBCROSSGT"},
		{CallerInclude: "ZCL_HIGH========CP", TargetName: "ZCL_OLD", Source: "WBCROSSGT"},
		{CallerInclude: "ZCL_HIGH========CP", TargetName: "ZCL_OLD", Source: "CROSS"},
		{CallerInclude: "ZCL_HIGH========CP", TargetName: "ZCL_OLD", Source: "WBCROSSGT"},
	}

	result := ComputeRenamePreview("CLAS", "ZCL_OLD", "ZCL_NEW", refs)

	if len(result.Refs) != 2 {
		t.Fatalf("Expected 2 callers, got %d", len(result.Refs))
	}
	// ZCL_HIGH has 3 refs, ZCL_LOW has 1 — HIGH should be first (both are HIGH confidence, so by ref count)
	if result.Refs[0].CallerName != "ZCL_HIGH" {
		t.Errorf("First should be ZCL_HIGH (more refs), got %q", result.Refs[0].CallerName)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstr(s, substr))
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
