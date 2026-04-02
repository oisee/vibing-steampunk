package adt

import (
	"context"
	"net/http"
	"testing"
)

func TestGetImpactAnalysis_StaticOnly(t *testing.T) {
	// Mock FindReferences response (real SAP ADT XML format)
	refsXML := `<?xml version="1.0" encoding="UTF-8"?>
<usageReferences:usageReferenceResult xmlns:usageReferences="http://www.sap.com/adt/ris/usageReferences" xmlns:adtcore="http://www.sap.com/adt/core">
  <usageReferences:referencedObjects>
    <usageReferences:referencedObject usageReferences:uri="/sap/bc/adt/programs/programs/zprog1" usageReferences:isResult="true">
      <adtcore:adtObject adtcore:uri="/sap/bc/adt/programs/programs/zprog1" adtcore:type="PROG/P" adtcore:name="ZPROG1">
        <adtcore:packageRef adtcore:name="$TMP"/>
      </adtcore:adtObject>
    </usageReferences:referencedObject>
    <usageReferences:referencedObject usageReferences:uri="/sap/bc/adt/oo/classes/zcl_consumer" usageReferences:isResult="true">
      <adtcore:adtObject adtcore:uri="/sap/bc/adt/oo/classes/zcl_consumer" adtcore:type="CLAS/OC" adtcore:name="ZCL_CONSUMER">
        <adtcore:packageRef adtcore:name="$TMP"/>
      </adtcore:adtObject>
    </usageReferences:referencedObject>
  </usageReferences:referencedObjects>
</usageReferences:usageReferenceResult>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"usageReferences": newTestResponse(refsXML),
			"discovery":       newTestResponse("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	transport.csrfToken = "test-token"
	client := NewClientWithTransport(cfg, transport)

	opts := ImpactAnalysisOptions{StaticRefs: true}
	result, err := client.GetImpactAnalysis(context.Background(), "/sap/bc/adt/oo/classes/zcl_test", "ZCL_TEST", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.DirectConsumers) == 0 {
		t.Error("expected direct consumers from FindReferences")
	}
	if len(result.Layers) != 1 || result.Layers[0] != "static_references" {
		t.Errorf("expected [static_references] layer, got %v", result.Layers)
	}
	if result.Summary.RiskLevel == "" {
		t.Error("expected non-empty risk level")
	}
}

func TestGetImpactAnalysis_EmptyResult(t *testing.T) {
	emptyRefsXML := `<?xml version="1.0" encoding="UTF-8"?>
<usageReferences:usageReferenceResult xmlns:usageReferences="http://www.sap.com/adt/ris/usageReferences">
  <usageReferences:referencedObjects/>
</usageReferences:usageReferenceResult>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"usageReferences": newTestResponse(emptyRefsXML),
			"discovery":       newTestResponse("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	transport.csrfToken = "test-token"
	client := NewClientWithTransport(cfg, transport)

	opts := ImpactAnalysisOptions{StaticRefs: true}
	result, err := client.GetImpactAnalysis(context.Background(), "/sap/bc/adt/oo/classes/zcl_orphan", "ZCL_ORPHAN", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.DirectConsumers) != 0 {
		t.Errorf("expected no consumers, got %d", len(result.DirectConsumers))
	}
	if result.Summary.RiskLevel != "low" {
		t.Errorf("expected low risk, got %q", result.Summary.RiskLevel)
	}
}

func TestGetImpactAnalysis_MaxResults(t *testing.T) {
	// Create a response with many references (3 objects, cap at 2)
	refsXML := `<?xml version="1.0" encoding="UTF-8"?>
<usageReferences:usageReferenceResult xmlns:usageReferences="http://www.sap.com/adt/ris/usageReferences" xmlns:adtcore="http://www.sap.com/adt/core">
  <usageReferences:referencedObjects>
    <usageReferences:referencedObject usageReferences:uri="/sap/bc/adt/programs/programs/zprog1" usageReferences:isResult="true">
      <adtcore:adtObject adtcore:uri="/sap/bc/adt/programs/programs/zprog1" adtcore:type="PROG/P" adtcore:name="ZPROG1">
        <adtcore:packageRef adtcore:name="$TMP"/>
      </adtcore:adtObject>
    </usageReferences:referencedObject>
    <usageReferences:referencedObject usageReferences:uri="/sap/bc/adt/programs/programs/zprog2" usageReferences:isResult="true">
      <adtcore:adtObject adtcore:uri="/sap/bc/adt/programs/programs/zprog2" adtcore:type="PROG/P" adtcore:name="ZPROG2">
        <adtcore:packageRef adtcore:name="$TMP"/>
      </adtcore:adtObject>
    </usageReferences:referencedObject>
    <usageReferences:referencedObject usageReferences:uri="/sap/bc/adt/programs/programs/zprog3" usageReferences:isResult="true">
      <adtcore:adtObject adtcore:uri="/sap/bc/adt/programs/programs/zprog3" adtcore:type="PROG/P" adtcore:name="ZPROG3">
        <adtcore:packageRef adtcore:name="$TMP"/>
      </adtcore:adtObject>
    </usageReferences:referencedObject>
  </usageReferences:referencedObjects>
</usageReferences:usageReferenceResult>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"usageReferences": newTestResponse(refsXML),
			"discovery":       newTestResponse("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	transport.csrfToken = "test-token"
	client := NewClientWithTransport(cfg, transport)

	opts := ImpactAnalysisOptions{StaticRefs: true, MaxResults: 2}
	result, err := client.GetImpactAnalysis(context.Background(), "/sap/bc/adt/oo/classes/zcl_test", "ZCL_TEST", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.DirectConsumers) > 2 {
		t.Errorf("expected at most 2 consumers (MaxResults=2), got %d", len(result.DirectConsumers))
	}
	if !result.Summary.Truncated {
		t.Error("expected Truncated=true when results are capped")
	}
}

func TestDetectConfigPatterns_Source(t *testing.T) {
	source := `CLASS zcl_test DEFINITION.
  PUBLIC SECTION.
    METHODS: do_something.
ENDCLASS.

CLASS zcl_test IMPLEMENTATION.
  METHOD do_something.
    DATA: lo_badi TYPE REF TO if_ex_something.
    GET BADI lo_badi.
    CALL BADI lo_badi->execute( ).
    ENHANCEMENT-SECTION sec_01 SPOTS zspot_test.
    ENDENHANCEMENT-SECTION.
  ENDMETHOD.
ENDCLASS.`

	risks := DetectConfigPatterns(source, "ZCL_TEST")

	types := make(map[string]bool)
	for _, r := range risks {
		types[r.Type] = true
	}

	if !types["badi"] {
		t.Error("expected badi risk from GET BADI")
	}
	if !types["enhancement"] {
		t.Error("expected enhancement risk from ENHANCEMENT-SECTION")
	}
}

func TestDetectConfigPatterns_BAdIInterface(t *testing.T) {
	source := `CLASS zcl_impl DEFINITION.
  PUBLIC SECTION.
    INTERFACES: if_ex_customer_check.
ENDCLASS.`

	risks := DetectConfigPatterns(source, "ZCL_IMPL")

	found := false
	for _, r := range risks {
		if r.Type == "badi" && r.Source == "source_analysis" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected badi risk from IF_EX_ interface")
	}
}

func TestDetectConfigPatterns_UserExit(t *testing.T) {
	source := `FORM user_exit.
  CALL FUNCTION 'EXIT_SAPLMR1M_001'
    EXPORTING iv_param = lv_val.
ENDFORM.`

	risks := DetectConfigPatterns(source, "ZTEST_PROG")

	found := false
	for _, r := range risks {
		if r.Type == "user_exit" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected user_exit risk from EXIT_ function call")
	}
}

func TestSanitizeForSQL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ZCL_TEST", "ZCL_TEST"},
		{"/NAMESPACE/CL_TEST", "/NAMESPACE/CL_TEST"},
		{"ZCL_TEST'; DROP TABLE --", "ZCL_TESTDROPTABLE"},
		{"", ""},
		{"hello world", "helloworld"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := reSQLSanitize.ReplaceAllString(tt.input, "")
			if got != tt.expected {
				t.Errorf("sanitize(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestImpactSummary_RiskLevel(t *testing.T) {
	tests := []struct {
		name          string
		dynamic       int
		config        int
		total         int
		expectedLevel string
	}{
		{"low", 0, 0, 5, "low"},
		{"medium", 0, 0, 15, "medium"},
		{"high", 0, 0, 60, "high"},
		{"critical-dynamic", 1, 0, 5, "critical"},
		{"critical-config", 0, 1, 5, "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level := CalculateRiskLevel(tt.dynamic, tt.config, tt.total)
			if level != tt.expectedLevel {
				t.Errorf("CalculateRiskLevel(%d, %d, %d) = %q, want %q",
					tt.dynamic, tt.config, tt.total, level, tt.expectedLevel)
			}
		})
	}
}
