package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildBreakpointRequestXML_LineBreakpoint(t *testing.T) {
	req := &BreakpointRequest{
		Scope:         BreakpointScopeExternal,
		DebuggingMode: DebuggingModeUser,
		User:          "TESTUSER",
		Breakpoints: []Breakpoint{
			NewLineBreakpoint("/sap/bc/adt/programs/programs/ZTEST/source/main", 42),
		},
	}

	xml, err := buildBreakpointRequestXML(req)
	if err != nil {
		t.Fatalf("buildBreakpointRequestXML failed: %v", err)
	}

	// Verify key elements - new format uses attributes
	if !strings.Contains(xml, `kind="line"`) {
		t.Error("missing kind=line attribute")
	}
	if !strings.Contains(xml, `adtcore:uri=`) {
		t.Error("missing adtcore:uri attribute")
	}
	if !strings.Contains(xml, `#start=42`) {
		t.Error("missing line number fragment")
	}
	if !strings.Contains(xml, `/sap/bc/adt/programs/programs/ZTEST/source/main`) {
		t.Error("missing object URI")
	}
	if !strings.Contains(xml, `scope="external"`) {
		t.Error("missing scope attribute")
	}
	if !strings.Contains(xml, `debuggingMode="user"`) {
		t.Error("missing debuggingMode attribute")
	}
	if !strings.Contains(xml, `requestUser="TESTUSER"`) {
		t.Error("missing requestUser attribute")
	}
}

func TestBuildBreakpointRequestXML_ExceptionBreakpoint(t *testing.T) {
	req := &BreakpointRequest{
		Scope:         BreakpointScopeExternal,
		DebuggingMode: DebuggingModeUser,
		User:          "TESTUSER",
		Breakpoints: []Breakpoint{
			NewExceptionBreakpoint("CX_SY_ZERODIVIDE"),
		},
	}

	xml, err := buildBreakpointRequestXML(req)
	if err != nil {
		t.Fatalf("buildBreakpointRequestXML failed: %v", err)
	}

	if !strings.Contains(xml, `kind="exception"`) {
		t.Error("missing kind=exception attribute")
	}
	if !strings.Contains(xml, `exceptionClass="CX_SY_ZERODIVIDE"`) {
		t.Error("missing exceptionClass attribute")
	}
}

func TestBuildBreakpointRequestXML_StatementBreakpoint(t *testing.T) {
	req := &BreakpointRequest{
		Scope:         BreakpointScopeExternal,
		DebuggingMode: DebuggingModeUser,
		User:          "TESTUSER",
		Breakpoints: []Breakpoint{
			NewStatementBreakpoint("CALL FUNCTION"),
		},
	}

	xml, err := buildBreakpointRequestXML(req)
	if err != nil {
		t.Fatalf("buildBreakpointRequestXML failed: %v", err)
	}

	if !strings.Contains(xml, `kind="statement"`) {
		t.Error("missing kind=statement attribute")
	}
	if !strings.Contains(xml, `statement="CALL FUNCTION"`) {
		t.Error("missing statement attribute")
	}
}

func TestBuildBreakpointRequestXML_MessageBreakpoint(t *testing.T) {
	req := &BreakpointRequest{
		Scope:         BreakpointScopeExternal,
		DebuggingMode: DebuggingModeUser,
		User:          "TESTUSER",
		Breakpoints: []Breakpoint{
			NewMessageBreakpoint("001", "E"),
		},
	}

	xml, err := buildBreakpointRequestXML(req)
	if err != nil {
		t.Fatalf("buildBreakpointRequestXML failed: %v", err)
	}

	if !strings.Contains(xml, `kind="message"`) {
		t.Error("missing kind=message attribute")
	}
	if !strings.Contains(xml, `msgId="001"`) {
		t.Error("missing msgId attribute")
	}
	if !strings.Contains(xml, `msgTy="E"`) {
		t.Error("missing msgTy attribute")
	}
}

func TestBuildBreakpointRequestXML_WithCondition(t *testing.T) {
	bp := NewLineBreakpoint("/sap/bc/adt/programs/programs/ZTEST/source/main", 42)
	bp.Condition = "lv_counter > 10"

	req := &BreakpointRequest{
		Scope:         BreakpointScopeExternal,
		DebuggingMode: DebuggingModeUser,
		User:          "TESTUSER",
		Breakpoints:   []Breakpoint{bp},
	}

	xml, err := buildBreakpointRequestXML(req)
	if err != nil {
		t.Fatalf("buildBreakpointRequestXML failed: %v", err)
	}

	// Condition should be XML-escaped in the attribute
	if !strings.Contains(xml, `condition="lv_counter &gt; 10"`) {
		t.Error("missing or incorrectly escaped condition attribute")
	}
}

func TestParseBreakpointResponse_LineBreakpoint(t *testing.T) {
	// Real response format from SAP system
	xmlResp := `<?xml version="1.0" encoding="utf-8"?>
<dbg:breakpoints xmlns:dbg="http://www.sap.com/adt/debugger">
  <breakpoint kind="line" id="KIND=0.SOURCETYPE=ABAP.MAIN_PROGRAM=ZTEST.INCLUDE=ZTEST.LINE_NR=42"
    adtcore:uri="/sap/bc/adt/programs/programs/ztest/source/main#start=42"
    adtcore:type="PROG/P"
    adtcore:name="ZTEST"
    xmlns:adtcore="http://www.sap.com/adt/core"/>
</dbg:breakpoints>`

	resp, err := parseBreakpointResponse([]byte(xmlResp))
	if err != nil {
		t.Fatalf("parseBreakpointResponse failed: %v", err)
	}

	if len(resp.Breakpoints) != 1 {
		t.Fatalf("expected 1 breakpoint, got %d", len(resp.Breakpoints))
	}

	bp := resp.Breakpoints[0]
	if bp.ID != "KIND=0.SOURCETYPE=ABAP.MAIN_PROGRAM=ZTEST.INCLUDE=ZTEST.LINE_NR=42" {
		t.Errorf("unexpected ID: %s", bp.ID)
	}
	if bp.Kind != BreakpointKindLine {
		t.Errorf("expected kind 'line', got '%s'", bp.Kind)
	}
	if bp.Line != 42 {
		t.Errorf("expected line 42, got %d", bp.Line)
	}
	if !strings.Contains(bp.URI, "/sap/bc/adt/programs/programs/ztest/source/main") {
		t.Errorf("unexpected URI: %s", bp.URI)
	}
}

func TestParseBreakpointResponse_ExceptionBreakpoint(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="utf-8"?>
<dbg:breakpoints xmlns:dbg="http://www.sap.com/adt/debugger">
  <breakpoint kind="exception" id="KIND=5.EXCEPTION_CLASS=CX_SY_ZERODIVIDE" exceptionClass="CX_SY_ZERODIVIDE"/>
</dbg:breakpoints>`

	resp, err := parseBreakpointResponse([]byte(xmlResp))
	if err != nil {
		t.Fatalf("parseBreakpointResponse failed: %v", err)
	}

	if len(resp.Breakpoints) != 1 {
		t.Fatalf("expected 1 breakpoint, got %d", len(resp.Breakpoints))
	}

	bp := resp.Breakpoints[0]
	if bp.Kind != BreakpointKindException {
		t.Errorf("expected kind 'exception', got '%s'", bp.Kind)
	}
	if bp.Exception != "CX_SY_ZERODIVIDE" {
		t.Errorf("expected exception 'CX_SY_ZERODIVIDE', got '%s'", bp.Exception)
	}
}

func TestParseBreakpointResponse_MultipleBreakpoints(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="utf-8"?>
<dbg:breakpoints xmlns:dbg="http://www.sap.com/adt/debugger" xmlns:adtcore="http://www.sap.com/adt/core">
  <breakpoint kind="line" id="BP001" adtcore:uri="/sap/bc/adt/programs/programs/ztest/source/main#start=10"/>
  <breakpoint kind="exception" id="BP002" exceptionClass="CX_SY_ZERODIVIDE"/>
  <breakpoint kind="statement" id="BP003" statement="WRITE"/>
</dbg:breakpoints>`

	resp, err := parseBreakpointResponse([]byte(xmlResp))
	if err != nil {
		t.Fatalf("parseBreakpointResponse failed: %v", err)
	}

	if len(resp.Breakpoints) != 3 {
		t.Fatalf("expected 3 breakpoints, got %d", len(resp.Breakpoints))
	}

	// Check first breakpoint (line)
	if resp.Breakpoints[0].Kind != BreakpointKindLine {
		t.Errorf("first bp: expected kind 'line', got '%s'", resp.Breakpoints[0].Kind)
	}

	// Check second breakpoint (exception)
	if resp.Breakpoints[1].Kind != BreakpointKindException {
		t.Errorf("second bp: expected kind 'exception', got '%s'", resp.Breakpoints[1].Kind)
	}

	// Check third breakpoint (statement)
	if resp.Breakpoints[2].Kind != BreakpointKindStatement {
		t.Errorf("third bp: expected kind 'statement', got '%s'", resp.Breakpoints[2].Kind)
	}
}

func TestParseBreakpointResponse_WithErrorMessage(t *testing.T) {
	// Breakpoint with error should be skipped
	xmlResp := `<?xml version="1.0" encoding="utf-8"?>
<dbg:breakpoints xmlns:dbg="http://www.sap.com/adt/debugger">
  <breakpoint kind="line" errorMessage="Cannot create a breakpoint at this position"/>
</dbg:breakpoints>`

	resp, err := parseBreakpointResponse([]byte(xmlResp))
	if err != nil {
		t.Fatalf("parseBreakpointResponse failed: %v", err)
	}

	// Breakpoint with error should be skipped
	if len(resp.Breakpoints) != 0 {
		t.Errorf("expected 0 breakpoints (error case should be skipped), got %d", len(resp.Breakpoints))
	}
}

func TestParseBreakpointResponse_EmptyResponse(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="utf-8"?>
<dbg:breakpoints xmlns:dbg="http://www.sap.com/adt/debugger">
</dbg:breakpoints>`

	resp, err := parseBreakpointResponse([]byte(xmlResp))
	if err != nil {
		t.Fatalf("parseBreakpointResponse failed: %v", err)
	}

	if len(resp.Breakpoints) != 0 {
		t.Errorf("expected 0 breakpoints, got %d", len(resp.Breakpoints))
	}
}

func TestNewLineBreakpoint(t *testing.T) {
	bp := NewLineBreakpoint("/sap/bc/adt/programs/programs/ZTEST/source/main", 42)

	if bp.Kind != BreakpointKindLine {
		t.Errorf("expected kind 'line', got '%s'", bp.Kind)
	}
	if !bp.Enabled {
		t.Error("expected enabled=true")
	}
	if bp.URI != "/sap/bc/adt/programs/programs/ZTEST/source/main" {
		t.Errorf("unexpected URI: %s", bp.URI)
	}
	if bp.Line != 42 {
		t.Errorf("expected line 42, got %d", bp.Line)
	}
}

func TestNewExceptionBreakpoint(t *testing.T) {
	bp := NewExceptionBreakpoint("CX_SY_ZERODIVIDE")

	if bp.Kind != BreakpointKindException {
		t.Errorf("expected kind 'exception', got '%s'", bp.Kind)
	}
	if !bp.Enabled {
		t.Error("expected enabled=true")
	}
	if bp.Exception != "CX_SY_ZERODIVIDE" {
		t.Errorf("expected exception 'CX_SY_ZERODIVIDE', got '%s'", bp.Exception)
	}
}

func TestNewStatementBreakpoint(t *testing.T) {
	bp := NewStatementBreakpoint("WRITE")

	if bp.Kind != BreakpointKindStatement {
		t.Errorf("expected kind 'statement', got '%s'", bp.Kind)
	}
	if !bp.Enabled {
		t.Error("expected enabled=true")
	}
	if bp.Statement != "WRITE" {
		t.Errorf("expected statement 'WRITE', got '%s'", bp.Statement)
	}
}

func TestNewMessageBreakpoint(t *testing.T) {
	bp := NewMessageBreakpoint("001", "E")

	if bp.Kind != BreakpointKindMessage {
		t.Errorf("expected kind 'message', got '%s'", bp.Kind)
	}
	if !bp.Enabled {
		t.Error("expected enabled=true")
	}
	if bp.MessageID != "001" {
		t.Errorf("expected messageId '001', got '%s'", bp.MessageID)
	}
	if bp.MessageType != "E" {
		t.Errorf("expected messageType 'E', got '%s'", bp.MessageType)
	}
}

func TestSetExternalBreakpoint_Integration(t *testing.T) {
	// Mock SAP response for successful breakpoint creation
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CSRF token request via discovery endpoint
		if r.URL.Path == "/sap/bc/adt/core/discovery" {
			w.Header().Set("X-CSRF-Token", "test-token")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/sap/bc/adt/debugger/breakpoints" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<dbg:breakpoints xmlns:dbg="http://www.sap.com/adt/debugger" xmlns:adtcore="http://www.sap.com/adt/core">
  <breakpoint kind="line" id="KIND=0.SOURCETYPE=ABAP.MAIN_PROGRAM=ZTEST.INCLUDE=ZTEST.LINE_NR=42"
    adtcore:uri="/sap/bc/adt/programs/programs/ztest/source/main#start=42"/>
</dbg:breakpoints>`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "testuser", "testpass", WithClient("001"))
	ctx := context.Background()

	req := &BreakpointRequest{
		Scope:         BreakpointScopeExternal,
		DebuggingMode: DebuggingModeUser,
		User:          "testuser",
		Breakpoints: []Breakpoint{
			NewLineBreakpoint("/sap/bc/adt/programs/programs/ZTEST/source/main", 42),
		},
	}

	resp, err := client.SetExternalBreakpoint(ctx, req)
	if err != nil {
		t.Fatalf("SetExternalBreakpoint failed: %v", err)
	}

	if len(resp.Breakpoints) != 1 {
		t.Fatalf("expected 1 breakpoint, got %d", len(resp.Breakpoints))
	}

	bp := resp.Breakpoints[0]
	if bp.Kind != BreakpointKindLine {
		t.Errorf("expected kind 'line', got '%s'", bp.Kind)
	}
	if bp.Line != 42 {
		t.Errorf("expected line 42, got %d", bp.Line)
	}
}

func TestGetExternalBreakpoints_Integration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CSRF token request via discovery endpoint
		if r.URL.Path == "/sap/bc/adt/core/discovery" {
			w.Header().Set("X-CSRF-Token", "test-token")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == "/sap/bc/adt/debugger/breakpoints" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<dbg:breakpoints xmlns:dbg="http://www.sap.com/adt/debugger" xmlns:adtcore="http://www.sap.com/adt/core">
  <breakpoint kind="line" id="BP001" adtcore:uri="/sap/bc/adt/programs/programs/ztest/source/main#start=10"/>
  <breakpoint kind="exception" id="BP002" exceptionClass="CX_SY_ZERODIVIDE"/>
</dbg:breakpoints>`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "testuser", "testpass", WithClient("001"))
	ctx := context.Background()

	resp, err := client.GetExternalBreakpoints(ctx, "testuser")
	if err != nil {
		t.Fatalf("GetExternalBreakpoints failed: %v", err)
	}

	if len(resp.Breakpoints) != 2 {
		t.Fatalf("expected 2 breakpoints, got %d", len(resp.Breakpoints))
	}
}

func TestDeleteExternalBreakpoint_Integration(t *testing.T) {
	deleteCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CSRF token request via discovery endpoint
		if r.URL.Path == "/sap/bc/adt/core/discovery" {
			w.Header().Set("X-CSRF-Token", "test-token")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/sap/bc/adt/debugger/breakpoints/") {
			deleteCount++
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "testuser", "testpass", WithClient("001"))
	ctx := context.Background()

	err := client.DeleteExternalBreakpoint(ctx, "BP001", "testuser")
	if err != nil {
		t.Fatalf("DeleteExternalBreakpoint failed: %v", err)
	}

	if deleteCount != 1 {
		t.Errorf("expected 1 delete call, got %d", deleteCount)
	}
}

func TestValidateBreakpointCondition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CSRF token request via discovery endpoint
		if r.URL.Path == "/sap/bc/adt/core/discovery" {
			w.Header().Set("X-CSRF-Token", "test-token")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/sap/bc/adt/debugger/breakpoints/conditions" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<dbg:condition xmlns:dbg="http://www.sap.com/adt/debugger" valid="true"/>`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "testuser", "testpass", WithClient("001"))
	ctx := context.Background()

	valid, msg, err := client.ValidateBreakpointCondition(ctx, "lv_counter > 10")
	if err != nil {
		t.Fatalf("ValidateBreakpointCondition failed: %v", err)
	}

	if !valid {
		t.Errorf("expected condition to be valid, got invalid with message: %s", msg)
	}
}

// TestBuildBreakpointRequestXML_WithOptionalAttributes verifies optional attributes are included when set
func TestBuildBreakpointRequestXML_WithOptionalAttributes(t *testing.T) {
	req := &BreakpointRequest{
		Scope:           BreakpointScopeExternal,
		DebuggingMode:   DebuggingModeUser,
		User:            "TESTUSER",
		TerminalID:      "TERM123",
		IdeID:           "myide",
		SystemDebugging: true,
		Deactivated:     true,
		Breakpoints: []Breakpoint{
			NewLineBreakpoint("/sap/bc/adt/programs/programs/ZTEST/source/main", 42),
		},
	}

	xml, err := buildBreakpointRequestXML(req)
	if err != nil {
		t.Fatalf("buildBreakpointRequestXML failed: %v", err)
	}

	// Verify optional attributes are included when set
	checks := []string{
		`terminalId="TERM123"`,   // terminalId attribute when set
		`ideId="myide"`,          // ideId attribute
		`systemDebugging="true"`, // systemDebugging when true
		`deactivated="true"`,     // deactivated when true
		`xmlns:adtcore`,          // namespace declaration
	}

	for _, check := range checks {
		if !strings.Contains(xml, check) {
			t.Errorf("missing expected XML content: %s\nGot XML:\n%s", check, xml)
		}
	}
}

// TestBuildBreakpointRequestXML_OmitsEmptyOptionalAttrs verifies empty optional attributes are omitted
func TestBuildBreakpointRequestXML_OmitsEmptyOptionalAttrs(t *testing.T) {
	req := &BreakpointRequest{
		Scope:         BreakpointScopeExternal,
		DebuggingMode: DebuggingModeUser,
		User:          "TESTUSER",
		// SystemDebugging false, Deactivated false - these should be omitted
		Breakpoints: []Breakpoint{
			NewLineBreakpoint("/sap/bc/adt/programs/programs/ZTEST/source/main", 42),
		},
	}

	xml, err := buildBreakpointRequestXML(req)
	if err != nil {
		t.Fatalf("buildBreakpointRequestXML failed: %v", err)
	}

	// Verify false optional attributes are NOT included
	shouldNotContain := []string{
		`systemDebugging=`, // should be omitted when false
		`deactivated=`,     // should be omitted when false
	}

	for _, check := range shouldNotContain {
		if strings.Contains(xml, check) {
			t.Errorf("should NOT contain: %s\nGot XML:\n%s", check, xml)
		}
	}

	// Should contain required attributes
	if !strings.Contains(xml, `ideId="vsp"`) {
		t.Error("missing default ideId attribute")
	}
	// terminalId should always be present (auto-generated when empty)
	if !strings.Contains(xml, `terminalId="vsp-`) {
		t.Error("missing auto-generated terminalId attribute")
	}
}
