package graph

import (
	"testing"
)

func TestIsCustomObject(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		// Custom: Z/Y prefix
		{"ZCL_MY_CLASS", true},
		{"YCL_OTHER", true},
		{"ZREPORT", true},
		{"Y_FUNC_MODULE", true},

		// Custom: /Z.../ /Y.../ namespace
		{"/ZVENDOR/CL_FOO", true},
		{"/YPARTNER/IF_BAR", true},

		// Standard: everything else
		{"CL_SALV_TABLE", false},
		{"CL_GUI_ALV_GRID", false},
		{"IF_HTTP_CLIENT", false},
		{"BAPI_MATERIAL_GET_DETAIL", false},
		{"/SAP/CL_SOMETHING", false},
		{"/UI5/CL_ROUTER", false},
		{"/BOBF/CL_TRA_MANAGER", false},

		// Edge cases
		{"", false},
		{"A", false},
		{"/", false},
	}
	for _, tt := range tests {
		if got := IsCustomObject(tt.name); got != tt.want {
			t.Errorf("IsCustomObject(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestComputeAPISurface_Basic(t *testing.T) {
	customObjects := map[string]bool{
		"ZCL_ORDER":  true,
		"ZCL_TRAVEL": true,
		"ZREPORT":    true,
	}
	rows := []APISurfaceRow{
		// ZCL_ORDER references CL_SALV_TABLE (2 times)
		{Include: "ZCL_ORDER========CP", RefName: "CL_SALV_TABLE", RefType: "TY", Source: "WBCROSSGT"},
		{Include: "ZCL_ORDER========CP", RefName: "CL_SALV_TABLE", RefType: "ME", Source: "WBCROSSGT"},
		// ZCL_TRAVEL also references CL_SALV_TABLE
		{Include: "ZCL_TRAVEL========CP", RefName: "CL_SALV_TABLE", RefType: "TY", Source: "WBCROSSGT"},
		// ZCL_ORDER references IF_HTTP_CLIENT
		{Include: "ZCL_ORDER========CP", RefName: "IF_HTTP_CLIENT", RefType: "TY", Source: "WBCROSSGT"},
		// ZREPORT calls BAPI_MATERIAL_GET_DETAIL
		{Include: "ZREPORT", RefName: "BAPI_MATERIAL_GET_DETAIL", RefType: "FU", Source: "CROSS"},
	}

	result := ComputeAPISurface(rows, customObjects, 50)

	if result.TotalCustomObjects != 3 {
		t.Errorf("TotalCustomObjects: got %d, want 3", result.TotalCustomObjects)
	}
	if result.UniqueStandardAPIs != 3 {
		t.Errorf("UniqueStandardAPIs: got %d, want 3", result.UniqueStandardAPIs)
	}
	if result.TotalCrossings != 5 {
		t.Errorf("TotalCrossings: got %d, want 5", result.TotalCrossings)
	}

	// CL_SALV_TABLE should be first (2 callers, 3 usages)
	if len(result.TopAPIs) < 1 {
		t.Fatal("Expected at least 1 API entry")
	}
	top := result.TopAPIs[0]
	if top.Name != "CL_SALV_TABLE" {
		t.Errorf("Top API: got %q, want CL_SALV_TABLE", top.Name)
	}
	if top.CallerCount != 2 {
		t.Errorf("CL_SALV_TABLE CallerCount: got %d, want 2", top.CallerCount)
	}
	if top.UsageCount != 3 {
		t.Errorf("CL_SALV_TABLE UsageCount: got %d, want 3", top.UsageCount)
	}
}

func TestComputeAPISurface_SkipsCustomTargets(t *testing.T) {
	customObjects := map[string]bool{"ZCL_A": true}
	rows := []APISurfaceRow{
		{Include: "ZCL_A========CP", RefName: "ZCL_B", RefType: "TY", Source: "WBCROSSGT"},       // custom→custom: skip
		{Include: "ZCL_A========CP", RefName: "CL_STANDARD", RefType: "TY", Source: "WBCROSSGT"}, // custom→standard: keep
	}

	result := ComputeAPISurface(rows, customObjects, 50)

	if result.UniqueStandardAPIs != 1 {
		t.Errorf("Should only have 1 standard API (custom targets filtered): got %d", result.UniqueStandardAPIs)
	}
	if result.TopAPIs[0].Name != "CL_STANDARD" {
		t.Errorf("Expected CL_STANDARD, got %q", result.TopAPIs[0].Name)
	}
}

func TestComputeAPISurface_SkipsOutOfScopeCallers(t *testing.T) {
	// Only ZCL_A is in scope
	customObjects := map[string]bool{"ZCL_A": true}
	rows := []APISurfaceRow{
		{Include: "ZCL_A========CP", RefName: "CL_STD", RefType: "TY", Source: "WBCROSSGT"},
		{Include: "ZCL_B========CP", RefName: "CL_STD", RefType: "TY", Source: "WBCROSSGT"}, // ZCL_B not in scope
	}

	result := ComputeAPISurface(rows, customObjects, 50)

	if result.TopAPIs[0].CallerCount != 1 {
		t.Errorf("CallerCount should be 1 (only ZCL_A in scope), got %d", result.TopAPIs[0].CallerCount)
	}
}

func TestComputeAPISurface_Ranking(t *testing.T) {
	customObjects := map[string]bool{"ZCL_A": true, "ZCL_B": true, "ZCL_C": true}
	rows := []APISurfaceRow{
		// CL_LOW: 1 caller, 1 usage
		{Include: "ZCL_A========CP", RefName: "CL_LOW", RefType: "TY", Source: "WBCROSSGT"},
		// CL_HIGH: 3 callers, 3 usages
		{Include: "ZCL_A========CP", RefName: "CL_HIGH", RefType: "TY", Source: "WBCROSSGT"},
		{Include: "ZCL_B========CP", RefName: "CL_HIGH", RefType: "TY", Source: "WBCROSSGT"},
		{Include: "ZCL_C========CP", RefName: "CL_HIGH", RefType: "TY", Source: "WBCROSSGT"},
		// CL_MID: 2 callers, 2 usages
		{Include: "ZCL_A========CP", RefName: "CL_MID", RefType: "TY", Source: "WBCROSSGT"},
		{Include: "ZCL_B========CP", RefName: "CL_MID", RefType: "TY", Source: "WBCROSSGT"},
	}

	result := ComputeAPISurface(rows, customObjects, 50)

	if len(result.TopAPIs) != 3 {
		t.Fatalf("Expected 3 APIs, got %d", len(result.TopAPIs))
	}
	// Order: CL_HIGH (3), CL_MID (2), CL_LOW (1)
	if result.TopAPIs[0].Name != "CL_HIGH" {
		t.Errorf("First: got %q, want CL_HIGH", result.TopAPIs[0].Name)
	}
	if result.TopAPIs[1].Name != "CL_MID" {
		t.Errorf("Second: got %q, want CL_MID", result.TopAPIs[1].Name)
	}
	if result.TopAPIs[2].Name != "CL_LOW" {
		t.Errorf("Third: got %q, want CL_LOW", result.TopAPIs[2].Name)
	}
}

func TestComputeAPISurface_TopNCap(t *testing.T) {
	customObjects := map[string]bool{"ZCL_A": true}
	var rows []APISurfaceRow
	for i := 0; i < 100; i++ {
		name := "CL_STD_" + string(rune('A'+i%26)) + string(rune('A'+i/26))
		rows = append(rows, APISurfaceRow{Include: "ZCL_A========CP", RefName: name, RefType: "TY", Source: "WBCROSSGT"})
	}

	result := ComputeAPISurface(rows, customObjects, 10)

	if len(result.TopAPIs) != 10 {
		t.Errorf("TopN cap: got %d, want 10", len(result.TopAPIs))
	}
	if result.UniqueStandardAPIs != 100 {
		t.Errorf("UniqueStandardAPIs should reflect total: got %d, want 100", result.UniqueStandardAPIs)
	}
}

func TestComputeAPISurface_Empty(t *testing.T) {
	result := ComputeAPISurface(nil, nil, 50)

	if result.UniqueStandardAPIs != 0 {
		t.Errorf("Empty: got %d APIs", result.UniqueStandardAPIs)
	}
	if result.TotalCrossings != 0 {
		t.Errorf("Empty: got %d crossings", result.TotalCrossings)
	}
}

func TestComputeAPISurface_NamespaceHandling(t *testing.T) {
	customObjects := map[string]bool{"ZCL_A": true}
	rows := []APISurfaceRow{
		{Include: "ZCL_A========CP", RefName: "/SAP/CL_STD", RefType: "TY", Source: "WBCROSSGT"},    // standard namespace → keep
		{Include: "ZCL_A========CP", RefName: "/UI5/CL_ROUTER", RefType: "TY", Source: "WBCROSSGT"}, // standard namespace → keep
		{Include: "ZCL_A========CP", RefName: "/ZVENDOR/CL_X", RefType: "TY", Source: "WBCROSSGT"},  // customer namespace → skip
	}

	result := ComputeAPISurface(rows, customObjects, 50)

	if result.UniqueStandardAPIs != 2 {
		t.Errorf("Namespace: expected 2 standard APIs (/SAP/ and /UI5/), got %d", result.UniqueStandardAPIs)
	}
	for _, api := range result.TopAPIs {
		if api.Name == "/ZVENDOR/CL_X" {
			t.Error("/ZVENDOR/ should be classified as custom and filtered out")
		}
	}
}

func TestComputeAPISurface_CallerListCapped(t *testing.T) {
	customObjects := make(map[string]bool)
	var rows []APISurfaceRow
	for i := 0; i < 20; i++ {
		name := "ZCL_CALLER_" + string(rune('A'+i))
		customObjects[name] = true
		rows = append(rows, APISurfaceRow{Include: name + "========CP", RefName: "CL_POPULAR", RefType: "TY", Source: "WBCROSSGT"})
	}

	result := ComputeAPISurface(rows, customObjects, 50)

	if len(result.TopAPIs[0].Callers) != 10 {
		t.Errorf("Callers list should be capped at 10, got %d", len(result.TopAPIs[0].Callers))
	}
	if result.TopAPIs[0].CallerCount != 20 {
		t.Errorf("CallerCount should reflect total: got %d, want 20", result.TopAPIs[0].CallerCount)
	}
}

func TestComputeAPISurface_SkipsNoise(t *testing.T) {
	customObjects := map[string]bool{"ZCL_A": true}
	rows := []APISurfaceRow{
		{Include: "ZCL_A========CP", RefName: "SY", RefType: "TY", Source: "WBCROSSGT"},
		{Include: "ZCL_A========CP", RefName: "SYST", RefType: "TY", Source: "WBCROSSGT"},
		{Include: "ZCL_A========CP", RefName: "SYST_SUBRC", RefType: "TY", Source: "WBCROSSGT"},
		{Include: "ZCL_A========CP", RefName: "ABAP_BOOL", RefType: "TY", Source: "WBCROSSGT"},
		{Include: "ZCL_A========CP", RefName: "ABAP_TRUE", RefType: "TY", Source: "WBCROSSGT"},
		{Include: "ZCL_A========CP", RefName: "ABAP_FALSE", RefType: "TY", Source: "WBCROSSGT"},
		{Include: "ZCL_A========CP", RefName: "CL_HTTP_CLIENT", RefType: "TY", Source: "WBCROSSGT"},
	}

	result := ComputeAPISurface(rows, customObjects, 50)
	if result.UniqueStandardAPIs != 1 {
		t.Fatalf("Expected only 1 kept API after noise filtering, got %d", result.UniqueStandardAPIs)
	}
	if got := result.TopAPIs[0].Name; got != "CL_HTTP_CLIENT" {
		t.Fatalf("Top API = %q, want CL_HTTP_CLIENT", got)
	}
}
