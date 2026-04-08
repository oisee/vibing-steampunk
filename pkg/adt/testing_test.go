package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// newCSRFResponse creates a mock response with CSRF header for testing tests.
func newCSRFResponse() *http.Response {
	h := make(http.Header)
	h.Set("X-CSRF-Token", "test-token")
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     h,
	}
}

// --- Coverage Tests ---

func TestParseCoverageResult(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<aunit:runResult xmlns:aunit="http://www.sap.com/adt/aunit">
  <coverage>
    <statement>
      <node uri="/sap/bc/adt/oo/classes/ZCL_TEST" type="CLAS" name="ZCL_TEST" total="50" covered="35" percentage="70.0"/>
      <node uri="/sap/bc/adt/oo/classes/ZCL_HELPER" type="CLAS" name="ZCL_HELPER" total="30" covered="20" percentage="66.7"/>
    </statement>
    <branch>
      <node uri="/sap/bc/adt/oo/classes/ZCL_TEST" type="CLAS" name="ZCL_TEST" total="10" covered="7" percentage="70.0"/>
    </branch>
    <procedure>
      <node uri="/sap/bc/adt/oo/classes/ZCL_TEST" type="CLAS" name="ZCL_TEST" total="5" covered="4" percentage="80.0"/>
    </procedure>
  </coverage>
</aunit:runResult>`

	result, err := parseCoverageResult([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("parseCoverageResult failed: %v", err)
	}

	if result.Statements.Total != 80 {
		t.Errorf("Statements.Total = %d, want 80", result.Statements.Total)
	}
	if result.Statements.Covered != 55 {
		t.Errorf("Statements.Covered = %d, want 55", result.Statements.Covered)
	}
	if result.Statements.Percent < 68 || result.Statements.Percent > 69 {
		t.Errorf("Statements.Percent = %.1f, want ~68.75", result.Statements.Percent)
	}
	if result.Branches.Total != 10 {
		t.Errorf("Branches.Total = %d, want 10", result.Branches.Total)
	}
	if result.Procedures.Total != 5 {
		t.Errorf("Procedures.Total = %d, want 5", result.Procedures.Total)
	}
	if len(result.SourceCoverage) != 2 {
		t.Errorf("SourceCoverage count = %d, want 2", len(result.SourceCoverage))
	}
	sc := result.SourceCoverage["/sap/bc/adt/oo/classes/ZCL_TEST"]
	if sc == nil {
		t.Fatal("Expected source coverage for ZCL_TEST")
	}
	if sc.Statements.Total != 50 {
		t.Errorf("ZCL_TEST total = %d, want 50", sc.Statements.Total)
	}
}

func TestParseCoverageResult_Empty(t *testing.T) {
	result, err := parseCoverageResult([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Statements.Total != 0 {
		t.Errorf("Expected 0 total for empty response")
	}
}

func TestParseCoverageResult_NoCoverage(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<aunit:runResult xmlns:aunit="http://www.sap.com/adt/aunit">
  <program>
    <testClasses/>
  </program>
</aunit:runResult>`

	result, err := parseCoverageResult([]byte(xmlResponse))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Statements.Total != 0 {
		t.Errorf("Expected 0 total when no coverage data")
	}
}

func TestClient_GetCodeCoverage(t *testing.T) {
	coverageXML := `<?xml version="1.0" encoding="UTF-8"?>
<aunit:runResult xmlns:aunit="http://www.sap.com/adt/aunit">
  <coverage>
    <statement>
      <node uri="/sap/bc/adt/oo/classes/ZCL_TEST" type="CLAS" name="ZCL_TEST" total="20" covered="15" percentage="75.0"/>
    </statement>
  </coverage>
</aunit:runResult>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/core/discovery":    newCSRFResponse(),
			"/sap/bc/adt/abapunit/testruns": newTestResponse(coverageXML),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GetCodeCoverage(context.Background(), "/sap/bc/adt/oo/classes/ZCL_TEST", nil)
	if err != nil {
		t.Fatalf("GetCodeCoverage failed: %v", err)
	}

	if result.Statements.Total != 20 {
		t.Errorf("Statements.Total = %d, want 20", result.Statements.Total)
	}
	if result.Statements.Covered != 15 {
		t.Errorf("Statements.Covered = %d, want 15", result.Statements.Covered)
	}

	// Verify coverage=true was sent
	if len(mock.requests) == 0 {
		t.Fatal("No requests made")
	}
	lastReq := mock.requests[len(mock.requests)-1]
	if lastReq.Body == nil {
		t.Fatal("Expected request body")
	}
	bodyBytes, _ := io.ReadAll(lastReq.Body)
	if !strings.Contains(string(bodyBytes), `coverage active="true"`) {
		t.Error("Request body should contain coverage active=true")
	}
}

// --- Check Run Results Tests ---

func TestParseCheckRunResult(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun" xmlns:adtcore="http://www.sap.com/adt/core">
  <chkrun:checkReport chkrun:uri="/sap/bc/adt/programs/programs/ZTEST" chkrun:status="processed" chkrun:reporter="abapCheckRun">
    <chkrun:checkMessageList>
      <chkrun:checkMessage chkrun:uri="/sap/bc/adt/programs/programs/ZTEST#start=5,1" chkrun:type="E" chkrun:line="5" chkrun:column="1">
        <chkrun:shortText>Variable LV_X is not defined</chkrun:shortText>
      </chkrun:checkMessage>
      <chkrun:checkMessage chkrun:uri="/sap/bc/adt/programs/programs/ZTEST#start=10,1" chkrun:type="W" chkrun:line="10" chkrun:column="1">
        <chkrun:shortText>Variable LV_Y is never used</chkrun:shortText>
      </chkrun:checkMessage>
      <chkrun:checkMessage chkrun:uri="/sap/bc/adt/programs/programs/ZTEST#start=15,1" chkrun:type="I" chkrun:line="15" chkrun:column="1">
        <chkrun:shortText>Consider using inline declaration</chkrun:shortText>
      </chkrun:checkMessage>
    </chkrun:checkMessageList>
  </chkrun:checkReport>
</chkrun:checkRunReports>`

	result, err := parseCheckRunResult([]byte(xmlResponse), "RUN123")
	if err != nil {
		t.Fatalf("parseCheckRunResult failed: %v", err)
	}

	if result.CheckRunID != "RUN123" {
		t.Errorf("CheckRunID = %q, want %q", result.CheckRunID, "RUN123")
	}
	if result.Status != "processed" {
		t.Errorf("Status = %q, want %q", result.Status, "processed")
	}
	if len(result.Messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(result.Messages))
	}
	if result.Messages[0].Type != "E" {
		t.Errorf("Message[0].Type = %q, want %q", result.Messages[0].Type, "E")
	}
	if result.Messages[0].Line != 5 {
		t.Errorf("Message[0].Line = %d, want 5", result.Messages[0].Line)
	}
	if !strings.Contains(result.Messages[0].Text, "LV_X") {
		t.Errorf("Message[0].Text = %q, should contain LV_X", result.Messages[0].Text)
	}
	if result.Summary.Errors != 1 {
		t.Errorf("Summary.Errors = %d, want 1", result.Summary.Errors)
	}
	if result.Summary.Warnings != 1 {
		t.Errorf("Summary.Warnings = %d, want 1", result.Summary.Warnings)
	}
	if result.Summary.Info != 1 {
		t.Errorf("Summary.Info = %d, want 1", result.Summary.Info)
	}
	if result.Summary.Total != 3 {
		t.Errorf("Summary.Total = %d, want 3", result.Summary.Total)
	}
}

func TestParseCheckRunResult_Empty(t *testing.T) {
	result, err := parseCheckRunResult([]byte(""), "RUN456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CheckRunID != "RUN456" {
		t.Errorf("CheckRunID = %q", result.CheckRunID)
	}
	if result.Status != "empty" {
		t.Errorf("Status = %q, want %q", result.Status, "empty")
	}
}

func TestParseCheckRunResult_NoMessages(t *testing.T) {
	xmlResponse := `<?xml version="1.0" encoding="UTF-8"?>
<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun">
  <chkrun:checkReport chkrun:status="clean">
    <chkrun:checkMessageList/>
  </chkrun:checkReport>
</chkrun:checkRunReports>`

	result, err := parseCheckRunResult([]byte(xmlResponse), "RUN789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Messages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(result.Messages))
	}
	if result.Summary.Total != 0 {
		t.Errorf("Summary.Total = %d, want 0", result.Summary.Total)
	}
}

func TestClient_GetCheckRunResults(t *testing.T) {
	checkRunXML := `<?xml version="1.0" encoding="UTF-8"?>
<chkrun:checkRunReports xmlns:chkrun="http://www.sap.com/adt/checkrun">
  <chkrun:checkReport chkrun:status="processed">
    <chkrun:checkMessageList>
      <chkrun:checkMessage chkrun:type="E" chkrun:line="1">
        <chkrun:shortText>Syntax error</chkrun:shortText>
      </chkrun:checkMessage>
    </chkrun:checkMessageList>
  </chkrun:checkReport>
</chkrun:checkRunReports>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/checkruns/RUN123": newTestResponse(checkRunXML),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	transport := NewTransportWithClient(cfg, mock)
	client := NewClientWithTransport(cfg, transport)

	result, err := client.GetCheckRunResults(context.Background(), "RUN123")
	if err != nil {
		t.Fatalf("GetCheckRunResults failed: %v", err)
	}

	if result.Summary.Errors != 1 {
		t.Errorf("Errors = %d, want 1", result.Summary.Errors)
	}
}
