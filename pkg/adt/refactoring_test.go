package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// newCSRFResponse creates a mock response with properly canonicalized CSRF header.
// http.Header literal keys bypass canonicalization, so we use Set() explicitly.
func newCSRFResponse() *http.Response {
	h := make(http.Header)
	h.Set("X-CSRF-Token", "test-token")
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     h,
	}
}

// --- Rename Evaluate Tests ---

func TestParseRenameEvaluateResult(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<refactoring:evaluateResult xmlns:refactoring="http://www.sap.com/adt/refactoring"
                            refactoring:feasible="true" refactoring:changeCount="3">
  <refactoring:problems>
    <refactoring:problem refactoring:severity="W" refactoring:description="Name already used in subclass ZCL_SUB"
                         refactoring:uri="/sap/bc/adt/oo/classes/ZCL_SUB" refactoring:line="10" refactoring:column="5"/>
  </refactoring:problems>
</refactoring:evaluateResult>`

	result, err := parseRenameEvaluateResult([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("parseRenameEvaluateResult failed: %v", err)
	}

	if !result.Feasible {
		t.Error("Expected feasible=true")
	}
	if result.ChangeCount != 3 {
		t.Errorf("ChangeCount = %d, want 3", result.ChangeCount)
	}
	if len(result.Problems) != 1 {
		t.Fatalf("Expected 1 problem, got %d", len(result.Problems))
	}
	if result.Problems[0].Severity != "W" {
		t.Errorf("Problem severity = %q, want %q", result.Problems[0].Severity, "W")
	}
	if result.Problems[0].Line != 10 {
		t.Errorf("Problem line = %d, want 10", result.Problems[0].Line)
	}
}

func TestParseRenameEvaluateResult_NotFeasible(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<refactoring:evaluateResult xmlns:refactoring="http://www.sap.com/adt/refactoring"
                            refactoring:feasible="false" refactoring:changeCount="0">
  <refactoring:problems>
    <refactoring:problem refactoring:severity="E" refactoring:description="Symbol cannot be renamed"/>
  </refactoring:problems>
</refactoring:evaluateResult>`

	result, err := parseRenameEvaluateResult([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Feasible {
		t.Error("Expected feasible=false")
	}
	if len(result.Problems) != 1 {
		t.Fatalf("Expected 1 problem, got %d", len(result.Problems))
	}
	if result.Problems[0].Severity != "E" {
		t.Errorf("Severity = %q, want %q", result.Problems[0].Severity, "E")
	}
}

func TestParseRenameEvaluateResult_ErrorResponse(t *testing.T) {
	xmlResponse := `<error><message>Object is locked by user OTHERDEV</message></error>`

	result, err := parseRenameEvaluateResult([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Feasible {
		t.Error("Expected feasible=false for error response")
	}
	if len(result.Problems) == 0 {
		t.Error("Expected at least one problem for error response")
	}
}

// --- Rename Preview Tests ---

func TestParseRenamePreviewResult(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<refactoring:previewResult xmlns:refactoring="http://www.sap.com/adt/refactoring">
  <refactoring:edits>
    <refactoring:change refactoring:uri="/sap/bc/adt/oo/classes/ZCL_TEST" refactoring:line="5" refactoring:column="12"
                        refactoring:length="10" refactoring:oldText="old_method" refactoring:newText="new_method"/>
    <refactoring:change refactoring:uri="/sap/bc/adt/oo/classes/ZCL_TEST" refactoring:line="20" refactoring:column="8"
                        refactoring:length="10" refactoring:oldText="old_method" refactoring:newText="new_method"/>
    <refactoring:change refactoring:uri="/sap/bc/adt/oo/classes/ZCL_CALLER" refactoring:line="15" refactoring:column="22"
                        refactoring:length="10" refactoring:oldText="old_method" refactoring:newText="new_method"/>
  </refactoring:edits>
</refactoring:previewResult>`

	result, err := parseRenamePreviewResult([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("parseRenamePreviewResult failed: %v", err)
	}

	if len(result.Changes) != 3 {
		t.Fatalf("Expected 3 changes, got %d", len(result.Changes))
	}
	if result.Changes[0].Line != 5 {
		t.Errorf("Change[0].Line = %d, want 5", result.Changes[0].Line)
	}
	if result.Changes[0].OldText != "old_method" {
		t.Errorf("Change[0].OldText = %q, want %q", result.Changes[0].OldText, "old_method")
	}
	if result.Changes[2].URI != "/sap/bc/adt/oo/classes/ZCL_CALLER" {
		t.Errorf("Change[2].URI = %q, want cross-reference URI", result.Changes[2].URI)
	}
}

func TestParseRenamePreviewResult_Empty(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<refactoring:previewResult xmlns:refactoring="http://www.sap.com/adt/refactoring">
  <refactoring:edits/>
</refactoring:previewResult>`

	result, err := parseRenamePreviewResult([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Changes) != 0 {
		t.Errorf("Expected 0 changes, got %d", len(result.Changes))
	}
}

// --- Rename Execute Tests ---

func TestParseRenameExecuteResult(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<refactoring:executeResult xmlns:refactoring="http://www.sap.com/adt/refactoring"
                           refactoring:success="true" refactoring:message="Rename completed successfully">
  <refactoring:affectedObjects>
    <refactoring:object refactoring:uri="/sap/bc/adt/oo/classes/ZCL_TEST"/>
    <refactoring:object refactoring:uri="/sap/bc/adt/oo/classes/ZCL_CALLER"/>
  </refactoring:affectedObjects>
</refactoring:executeResult>`

	result, err := parseRenameExecuteResult([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("parseRenameExecuteResult failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success=true")
	}
	if len(result.AffectedObjects) != 2 {
		t.Fatalf("Expected 2 affected objects, got %d", len(result.AffectedObjects))
	}
	if result.Message != "Rename completed successfully" {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestParseRenameExecuteResult_UnparsableSuccess(t *testing.T) {
	// Sometimes server returns non-standard XML on success
	result, err := parseRenameExecuteResult([]byte("OK"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("Expected success=true for unparsable response")
	}
	if !strings.Contains(result.Message, "OK") {
		t.Errorf("Message should include raw response, got %q", result.Message)
	}
}

// --- Extract Method Tests ---

func TestParseExtractMethodEvaluateResult(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<refactoring:evaluateResult xmlns:refactoring="http://www.sap.com/adt/refactoring"
                            refactoring:feasible="true" refactoring:returnType="STRING">
  <refactoring:parameters>
    <refactoring:parameter refactoring:name="IV_INPUT" refactoring:type="STRING" refactoring:direction="IMPORTING"/>
    <refactoring:parameter refactoring:name="RV_RESULT" refactoring:type="STRING" refactoring:direction="RETURNING"/>
  </refactoring:parameters>
</refactoring:evaluateResult>`

	result, err := parseExtractMethodEvaluateResult([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("parseExtractMethodEvaluateResult failed: %v", err)
	}

	if !result.Feasible {
		t.Error("Expected feasible=true")
	}
	if result.ReturnType != "STRING" {
		t.Errorf("ReturnType = %q, want %q", result.ReturnType, "STRING")
	}
	if len(result.Parameters) != 2 {
		t.Fatalf("Expected 2 parameters, got %d", len(result.Parameters))
	}
	if result.Parameters[0].Name != "IV_INPUT" {
		t.Errorf("Param[0].Name = %q, want %q", result.Parameters[0].Name, "IV_INPUT")
	}
	if result.Parameters[0].Direction != "IMPORTING" {
		t.Errorf("Param[0].Direction = %q, want %q", result.Parameters[0].Direction, "IMPORTING")
	}
}

func TestParseExtractMethodEvaluateResult_NotFeasible(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<refactoring:evaluateResult xmlns:refactoring="http://www.sap.com/adt/refactoring"
                            refactoring:feasible="false">
  <refactoring:problems>
    <refactoring:problem refactoring:severity="E" refactoring:description="Selection contains RETURN statement"/>
  </refactoring:problems>
</refactoring:evaluateResult>`

	result, err := parseExtractMethodEvaluateResult([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Feasible {
		t.Error("Expected feasible=false")
	}
	if len(result.Problems) != 1 {
		t.Fatalf("Expected 1 problem, got %d", len(result.Problems))
	}
}

func TestParseExtractMethodPreviewResult(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<refactoring:previewResult xmlns:refactoring="http://www.sap.com/adt/refactoring">
  <refactoring:newMethodSource>METHOD new_method.
  rv_result = iv_input.
ENDMETHOD.</refactoring:newMethodSource>
  <refactoring:modifiedSource>METHOD old_method.
  rv_result = new_method( iv_input ).
ENDMETHOD.</refactoring:modifiedSource>
  <refactoring:callSite>new_method( iv_input )</refactoring:callSite>
</refactoring:previewResult>`

	result, err := parseExtractMethodPreviewResult([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("parseExtractMethodPreviewResult failed: %v", err)
	}

	if !strings.Contains(result.NewMethodSource, "METHOD new_method") {
		t.Error("NewMethodSource should contain new method definition")
	}
	if !strings.Contains(result.CallSite, "new_method") {
		t.Error("CallSite should contain method call")
	}
}

func TestParseExtractMethodExecuteResult(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<refactoring:executeResult xmlns:refactoring="http://www.sap.com/adt/refactoring"
                           refactoring:success="true" refactoring:message="Method extracted successfully"/>`

	result, err := parseExtractMethodExecuteResult([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Error("Expected success=true")
	}
}

// --- Client-level integration tests with mock transport ---

func TestClient_RenameEvaluate(t *testing.T) {
	evalXML := `<?xml version="1.0" encoding="UTF-8"?>
<refactoring:evaluateResult xmlns:refactoring="http://www.sap.com/adt/refactoring"
                            refactoring:feasible="true" refactoring:changeCount="2"/>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/core/discovery":     newCSRFResponse(),
			"/sap/bc/adt/refactoring/rename": newTestResponse(evalXML),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.RenameEvaluate(context.Background(), "/sap/bc/adt/oo/classes/ZCL_TEST", 10, 5, "CLASS zcl_test DEFINITION.", "new_name")
	if err != nil {
		t.Fatalf("RenameEvaluate failed: %v", err)
	}

	if !result.Feasible {
		t.Error("Expected feasible=true")
	}
	if result.ChangeCount != 2 {
		t.Errorf("ChangeCount = %d, want 2", result.ChangeCount)
	}

	// Verify request was made
	if len(mock.requests) == 0 {
		t.Fatal("No requests made")
	}
	req := mock.requests[len(mock.requests)-1]
	if req.Method != http.MethodPost {
		t.Errorf("Method = %q, want POST", req.Method)
	}
	if !strings.Contains(req.URL.String(), "rename") {
		t.Errorf("URL = %q, should contain 'rename'", req.URL.String())
	}
}

func TestClient_RenameEvaluate_ReadOnly(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Safety.ReadOnly = true
	transport := NewTransportWithClient(cfg, &mockTransportClient{responses: map[string]*http.Response{}})
	client := NewClientWithTransport(cfg, transport)

	_, err := client.RenameEvaluate(context.Background(), "/sap/bc/adt/oo/classes/ZCL_TEST", 1, 1, "source", "new_name")
	if err == nil {
		t.Error("Expected error for read-only mode")
	}
}

func TestClient_ExtractMethodEvaluate(t *testing.T) {
	evalXML := `<?xml version="1.0" encoding="UTF-8"?>
<refactoring:evaluateResult xmlns:refactoring="http://www.sap.com/adt/refactoring"
                            refactoring:feasible="true" refactoring:returnType="I">
  <refactoring:parameters>
    <refactoring:parameter refactoring:name="IV_X" refactoring:type="I" refactoring:direction="IMPORTING"/>
  </refactoring:parameters>
</refactoring:evaluateResult>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/core/discovery":            newCSRFResponse(),
			"/sap/bc/adt/refactoring/extractmethod": newTestResponse(evalXML),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.ExtractMethodEvaluate(context.Background(), "/sap/bc/adt/oo/classes/ZCL_TEST", 10, 1, 15, 80, "source code", "extract_me")
	if err != nil {
		t.Fatalf("ExtractMethodEvaluate failed: %v", err)
	}

	if !result.Feasible {
		t.Error("Expected feasible=true")
	}
	if len(result.Parameters) != 1 {
		t.Fatalf("Expected 1 parameter, got %d", len(result.Parameters))
	}
	if result.Parameters[0].Name != "IV_X" {
		t.Errorf("Param name = %q, want %q", result.Parameters[0].Name, "IV_X")
	}
}
