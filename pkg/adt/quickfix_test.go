package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// --- Quick Fix Proposals Tests ---

func TestParseQuickFixProposals(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<quickfix:proposals xmlns:quickfix="http://www.sap.com/adt/quickfix">
  <quickfix:proposal quickfix:id="QF001" quickfix:title="Add missing IMPORTING parameter"
                     quickfix:description="Adds the required parameter IV_NAME to method call"/>
  <quickfix:proposal quickfix:id="QF002" quickfix:title="Remove unused variable"
                     quickfix:description="Removes the declaration of LV_UNUSED"/>
</quickfix:proposals>`

	result, err := parseQuickFixProposals([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("parseQuickFixProposals failed: %v", err)
	}

	if len(result.Proposals) != 2 {
		t.Fatalf("Expected 2 proposals, got %d", len(result.Proposals))
	}
	if result.Proposals[0].ID != "QF001" {
		t.Errorf("Proposal[0].ID = %q, want %q", result.Proposals[0].ID, "QF001")
	}
	if result.Proposals[0].Title != "Add missing IMPORTING parameter" {
		t.Errorf("Proposal[0].Title = %q", result.Proposals[0].Title)
	}
	if result.Proposals[1].ID != "QF002" {
		t.Errorf("Proposal[1].ID = %q, want %q", result.Proposals[1].ID, "QF002")
	}
}

func TestParseQuickFixProposals_Empty(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<quickfix:proposals xmlns:quickfix="http://www.sap.com/adt/quickfix"/>`

	result, err := parseQuickFixProposals([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Proposals) != 0 {
		t.Errorf("Expected 0 proposals, got %d", len(result.Proposals))
	}
}

// --- Quick Fix Apply Tests ---

func TestParseQuickFixApplyResult_XMLResponse(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<quickfix:result xmlns:quickfix="http://www.sap.com/adt/quickfix" quickfix:status="OK">
  <quickfix:newSource>REPORT ztest.
DATA lv_name TYPE string.
WRITE lv_name.</quickfix:newSource>
  <quickfix:message>Quick fix applied successfully</quickfix:message>
</quickfix:result>`

	result, err := parseQuickFixApplyResult([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("parseQuickFixApplyResult failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success=true")
	}
	if !strings.Contains(result.NewSource, "REPORT ztest") {
		t.Error("NewSource should contain ABAP code")
	}
	if result.Message != "Quick fix applied successfully" {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestParseQuickFixApplyResult_PlainTextResponse(t *testing.T) {
	// Some quickfix endpoints return plain text (the fixed source directly)
	plainSource := `REPORT ztest.
DATA lv_name TYPE string.
WRITE lv_name.`

	result, err := parseQuickFixApplyResult([]byte(plainSource))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Success {
		t.Error("Expected success=true for plain text response")
	}
	if !strings.Contains(result.NewSource, "REPORT ztest") {
		t.Error("NewSource should contain the source code")
	}
}

// --- ATC QuickFix Tests ---

func TestParseATCQuickFixDetails(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<atcquickfix:quickfixDetails xmlns:atcquickfix="http://www.sap.com/adt/atc/quickfix"
                              atcquickfix:title="Replace deprecated statement"
                              atcquickfix:description="Replace MOVE with assignment operator"
                              atcquickfix:canAutoFix="true"/>`

	result, err := parseATCQuickFixDetails([]byte(xmlResponse), "FINDING123")
	if err != nil {
		t.Fatalf("parseATCQuickFixDetails failed: %v", err)
	}

	if result.FindingID != "FINDING123" {
		t.Errorf("FindingID = %q, want %q", result.FindingID, "FINDING123")
	}
	if result.Title != "Replace deprecated statement" {
		t.Errorf("Title = %q", result.Title)
	}
	if !result.CanAutoFix {
		t.Error("Expected canAutoFix=true")
	}
}

func TestParseATCQuickFixDetails_UnparsableResponse(t *testing.T) {
	// When parsing fails, should return basic info without error
	result, err := parseATCQuickFixDetails([]byte("unexpected response"), "FINDING456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.FindingID != "FINDING456" {
		t.Errorf("FindingID = %q, want %q", result.FindingID, "FINDING456")
	}
	if result.CanAutoFix {
		t.Error("Expected canAutoFix=false for unparsable response")
	}
}

func TestParseATCQuickFixApplyResult(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<atcquickfix:applyResult xmlns:atcquickfix="http://www.sap.com/adt/atc/quickfix"
                          atcquickfix:success="true" atcquickfix:message="Fix applied to 3 locations"/>`

	result, err := parseATCQuickFixApplyResult([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("parseATCQuickFixApplyResult failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success=true")
	}
	if result.Message != "Fix applied to 3 locations" {
		t.Errorf("Message = %q", result.Message)
	}
}

func TestParseATCQuickFixApplyResult_Unparsable(t *testing.T) {
	result, err := parseATCQuickFixApplyResult([]byte("OK"))
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

// --- Client-level tests ---

func TestClient_GetQuickFixProposals(t *testing.T) {
	proposalsXML := `<?xml version="1.0" encoding="UTF-8"?>
<quickfix:proposals xmlns:quickfix="http://www.sap.com/adt/quickfix">
  <quickfix:proposal quickfix:id="QF001" quickfix:title="Add TYPE declaration"/>
</quickfix:proposals>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/core/discovery":     newCSRFResponse(),
			"/sap/bc/adt/quickfix/proposals": newTestResponse(proposalsXML),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GetQuickFixProposals(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", 5, 10, "REPORT ztest.")
	if err != nil {
		t.Fatalf("GetQuickFixProposals failed: %v", err)
	}

	if len(result.Proposals) != 1 {
		t.Fatalf("Expected 1 proposal, got %d", len(result.Proposals))
	}
	if result.Proposals[0].ID != "QF001" {
		t.Errorf("Proposal ID = %q, want %q", result.Proposals[0].ID, "QF001")
	}

	// Verify request
	if len(mock.requests) == 0 {
		t.Fatal("No requests made")
	}
	req := mock.requests[len(mock.requests)-1]
	if req.Method != http.MethodPost {
		t.Errorf("Method = %q, want POST", req.Method)
	}
}

func TestClient_ApplyQuickFix(t *testing.T) {
	applyResult := `REPORT ztest.
DATA lv_fixed TYPE string.
WRITE lv_fixed.`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/core/discovery":  newCSRFResponse(),
			"/sap/bc/adt/quickfix/apply": newTestResponse(applyResult),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.ApplyQuickFix(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "QF001", 5, 10, "REPORT ztest.")
	if err != nil {
		t.Fatalf("ApplyQuickFix failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success=true")
	}
	if !strings.Contains(result.NewSource, "lv_fixed") {
		t.Error("NewSource should contain fixed code")
	}
}

func TestClient_ApplyQuickFix_ReadOnly(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Safety.ReadOnly = true
	transport := NewTransportWithClient(cfg, &mockTransportClient{responses: map[string]*http.Response{}})
	client := NewClientWithTransport(cfg, transport)

	_, err := client.ApplyQuickFix(context.Background(), "/sap/bc/adt/programs/programs/ZTEST", "QF001", 1, 1, "source")
	if err == nil {
		t.Error("Expected error for read-only mode")
	}
}

func TestClient_GetATCQuickFixDetails(t *testing.T) {
	detailsXML := `<?xml version="1.0" encoding="UTF-8"?>
<atcquickfix:quickfixDetails xmlns:atcquickfix="http://www.sap.com/adt/atc/quickfix"
                              atcquickfix:title="Fix issue"
                              atcquickfix:canAutoFix="true"/>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/atc/quickfix/FINDING123": newTestResponse(detailsXML),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GetATCQuickFixDetails(context.Background(), "FINDING123")
	if err != nil {
		t.Fatalf("GetATCQuickFixDetails failed: %v", err)
	}

	if result.FindingID != "FINDING123" {
		t.Errorf("FindingID = %q", result.FindingID)
	}
	if !result.CanAutoFix {
		t.Error("Expected canAutoFix=true")
	}
}

func TestClient_ApplyATCQuickFix(t *testing.T) {
	applyXML := `<?xml version="1.0" encoding="UTF-8"?>
<atcquickfix:applyResult xmlns:atcquickfix="http://www.sap.com/adt/atc/quickfix"
                          atcquickfix:success="true" atcquickfix:message="Applied"/>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/core/discovery":                newCSRFResponse(),
			"/sap/bc/adt/atc/quickfix/FINDING123/apply": newTestResponse(applyXML),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.ApplyATCQuickFix(context.Background(), "FINDING123")
	if err != nil {
		t.Fatalf("ApplyATCQuickFix failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success=true")
	}
}

func TestClient_ApplyATCQuickFix_ReadOnly(t *testing.T) {
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	cfg.Safety.ReadOnly = true
	transport := NewTransportWithClient(cfg, &mockTransportClient{responses: map[string]*http.Response{}})
	client := NewClientWithTransport(cfg, transport)

	_, err := client.ApplyATCQuickFix(context.Background(), "FINDING123")
	if err == nil {
		t.Error("Expected error for read-only mode")
	}
}
