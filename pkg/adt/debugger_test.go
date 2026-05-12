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

func TestXmlEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"a > b", "a &gt; b"},
		{"a < b", "a &lt; b"},
		{"a & b", "a &amp; b"},
		{`a "quoted" b`, "a &quot;quoted&quot; b"},
		{"a 'quoted' b", "a &apos;quoted&apos; b"},
		{"<>&\"'", "&lt;&gt;&amp;&quot;&apos;"},
	}

	for _, tt := range tests {
		result := xmlEscape(tt.input)
		if result != tt.expected {
			t.Errorf("xmlEscape(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
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
		`terminalId="TERM123"`,       // terminalId attribute when set
		`ideId="myide"`,              // ideId attribute
		`systemDebugging="true"`,     // systemDebugging when true
		`deactivated="true"`,         // deactivated when true
		`xmlns:adtcore`,              // namespace declaration
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
		`systemDebugging=`,    // should be omitted when false
		`deactivated=`,        // should be omitted when false
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

// --- Debug Session Parse Tests ---

func TestParseAttachResponse(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="utf-8"?>
<dbg:attach xmlns:dbg="http://www.sap.com/adt/debugger"
  isRfc="false"
  isSameSystem="true"
  serverName="VHCALA4HCI_A4H_01"
  debugSessionId="session123"
  processId="42"
  isPostMortem="false"
  isUserAuthorizedForChanges="true"
  debuggeeSessionId="debuggee456"
  abapTraceState="OFF"
  canAdvancedTableFeatures="true"
  isNonExclusive="false"
  isNonExclusiveToggled="false"
  guiEditorGuid=""
  sessionTitle="TESTUSER"
  isSteppingPossible="true"
  isTerminationPossible="true">
  <dbg:actions>
    <dbg:action name="stepInto" style="push" group="stepping" title="Step Into"/>
    <dbg:action name="stepOver" style="push" group="stepping" title="Step Over"/>
  </dbg:actions>
  <dbg:reachedBreakpoints>
    <dbg:breakpoint id="BP001" kind="line"/>
  </dbg:reachedBreakpoints>
</dbg:attach>`

	result, err := parseAttachResponse([]byte(xmlResp))
	if err != nil {
		t.Fatalf("parseAttachResponse failed: %v", err)
	}

	// Verify debug state
	if result.DebugSessionID != "session123" {
		t.Errorf("expected debugSessionId 'session123', got '%s'", result.DebugSessionID)
	}
	if result.ProcessID != 42 {
		t.Errorf("expected processId 42, got %d", result.ProcessID)
	}
	if !result.IsSameSystem {
		t.Error("expected isSameSystem=true")
	}
	if !result.IsSteppingPossible {
		t.Error("expected isSteppingPossible=true")
	}
	if result.ServerName != "VHCALA4HCI_A4H_01" {
		t.Errorf("expected serverName 'VHCALA4HCI_A4H_01', got '%s'", result.ServerName)
	}

	// Verify actions
	if len(result.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(result.Actions))
	}
	if result.Actions[0].Name != "stepInto" {
		t.Errorf("expected first action 'stepInto', got '%s'", result.Actions[0].Name)
	}

	// Verify reached breakpoints
	if len(result.ReachedBreakpoints) != 1 {
		t.Fatalf("expected 1 reached breakpoint, got %d", len(result.ReachedBreakpoints))
	}
	if result.ReachedBreakpoints[0].ID != "BP001" {
		t.Errorf("expected breakpoint ID 'BP001', got '%s'", result.ReachedBreakpoints[0].ID)
	}
}

func TestParseStepResponse(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="utf-8"?>
<dbg:step xmlns:dbg="http://www.sap.com/adt/debugger"
  isRfc="false"
  isSameSystem="true"
  serverName="VHCALA4HCI_A4H_01"
  debugSessionId="session123"
  processId="42"
  isDebuggeeChanged="false"
  isSteppingPossible="true"
  isTerminationPossible="true">
  <dbg:settings
    systemDebugging="false"
    createExceptionObject="false"
    backgroundRFC="false"
    sharedObjectDebugging="false"
    showDataAging="false"
    updateDebugging="false"/>
  <dbg:actions>
    <dbg:action name="stepOver" style="push" group="stepping" title="Step Over"/>
  </dbg:actions>
  <dbg:reachedBreakpoints/>
</dbg:step>`

	result, err := parseStepResponse([]byte(xmlResp))
	if err != nil {
		t.Fatalf("parseStepResponse failed: %v", err)
	}

	if result.DebugSessionID != "session123" {
		t.Errorf("expected debugSessionId 'session123', got '%s'", result.DebugSessionID)
	}
	if result.IsDebuggeeChanged {
		t.Error("expected isDebuggeeChanged=false")
	}
	if !result.IsSteppingPossible {
		t.Error("expected isSteppingPossible=true")
	}
	if result.Settings.SystemDebugging {
		t.Error("expected systemDebugging=false")
	}
	if len(result.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(result.Actions))
	}
}

func TestParseStackResponse(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="utf-8"?>
<dbg:stack xmlns:dbg="http://www.sap.com/adt/debugger"
  isRfc="false"
  isSameSystem="true"
  serverName="VHCALA4HCI_A4H_01"
  debugCursorStackIndex="1">
  <dbg:stackEntry
    stackPosition="1"
    stackType="ABAP"
    stackUri="/sap/bc/adt/debugger/stack/type/ABAP/position/1"
    programName="ZTEST_MCP_CRUD"
    includeName="ZTEST_MCP_CRUD"
    line="15"
    eventType="REPORT"
    eventName="ZTEST_MCP_CRUD"
    sourceType="ABAP"
    systemProgram="false"
    isVit="false"
    uri="/sap/bc/adt/programs/programs/ZTEST_MCP_CRUD/source/main#start=15"/>
  <dbg:stackEntry
    stackPosition="2"
    stackType="ABAP"
    stackUri="/sap/bc/adt/debugger/stack/type/ABAP/position/2"
    programName="CL_ADT_RES_UNIT_TEST_RUN"
    includeName="CL_ADT_RES_UNIT_TEST_RUN"
    line="45"
    eventType="METHOD"
    eventName="POST"
    sourceType="ABAP"
    systemProgram="true"
    isVit="false"/>
</dbg:stack>`

	result, err := parseStackResponse([]byte(xmlResp))
	if err != nil {
		t.Fatalf("parseStackResponse failed: %v", err)
	}

	if !result.IsSameSystem {
		t.Error("expected isSameSystem=true")
	}
	if result.DebugCursorStackIndex != 1 {
		t.Errorf("expected debugCursorStackIndex 1, got %d", result.DebugCursorStackIndex)
	}

	if len(result.Stack) != 2 {
		t.Fatalf("expected 2 stack entries, got %d", len(result.Stack))
	}

	// Check first stack entry
	entry := result.Stack[0]
	if entry.ProgramName != "ZTEST_MCP_CRUD" {
		t.Errorf("expected programName 'ZTEST_MCP_CRUD', got '%s'", entry.ProgramName)
	}
	if entry.Line != 15 {
		t.Errorf("expected line 15, got %d", entry.Line)
	}
	if entry.StackType != "ABAP" {
		t.Errorf("expected stackType 'ABAP', got '%s'", entry.StackType)
	}
	if entry.SystemProgram {
		t.Error("expected systemProgram=false for first entry")
	}

	// Check second stack entry
	if !result.Stack[1].SystemProgram {
		t.Error("expected systemProgram=true for second entry")
	}
}

func TestParseVariablesResponse(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <STPDA_ADT_VARIABLE>
        <ID>LV_COUNT</ID>
        <NAME>LV_COUNT</NAME>
        <DECLARED_TYPE_NAME>I</DECLARED_TYPE_NAME>
        <ACTUAL_TYPE_NAME>I</ACTUAL_TYPE_NAME>
        <KIND>LOCAL</KIND>
        <INSTANTIATION_KIND></INSTANTIATION_KIND>
        <ACCESS_KIND></ACCESS_KIND>
        <META_TYPE>simple</META_TYPE>
        <PARAMETER_KIND></PARAMETER_KIND>
        <VALUE>42</VALUE>
        <HEX_VALUE>0000002A</HEX_VALUE>
        <READ_ONLY></READ_ONLY>
        <TECHNICAL_TYPE>I</TECHNICAL_TYPE>
        <LENGTH>4</LENGTH>
        <TABLE_BODY></TABLE_BODY>
        <TABLE_LINES>0</TABLE_LINES>
        <IS_VALUE_INCOMPLETE></IS_VALUE_INCOMPLETE>
        <IS_EXCEPTION></IS_EXCEPTION>
      </STPDA_ADT_VARIABLE>
      <STPDA_ADT_VARIABLE>
        <ID>LS_DATA</ID>
        <NAME>LS_DATA</NAME>
        <DECLARED_TYPE_NAME>TY_DATA</DECLARED_TYPE_NAME>
        <ACTUAL_TYPE_NAME>TY_DATA</ACTUAL_TYPE_NAME>
        <KIND>LOCAL</KIND>
        <META_TYPE>structure</META_TYPE>
        <VALUE></VALUE>
        <READ_ONLY></READ_ONLY>
        <TECHNICAL_TYPE>u</TECHNICAL_TYPE>
        <LENGTH>100</LENGTH>
        <IS_VALUE_INCOMPLETE>X</IS_VALUE_INCOMPLETE>
      </STPDA_ADT_VARIABLE>
    </DATA>
  </asx:values>
</asx:abap>`

	result, err := parseVariablesResponse([]byte(xmlResp))
	if err != nil {
		t.Fatalf("parseVariablesResponse failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 variables, got %d", len(result))
	}

	// Check first variable (simple type)
	v1 := result[0]
	if v1.Name != "LV_COUNT" {
		t.Errorf("expected name 'LV_COUNT', got '%s'", v1.Name)
	}
	if v1.MetaType != DebugMetaTypeSimple {
		t.Errorf("expected metaType 'simple', got '%s'", v1.MetaType)
	}
	if v1.Value != "42" {
		t.Errorf("expected value '42', got '%s'", v1.Value)
	}
	if v1.HexValue != "0000002A" {
		t.Errorf("expected hexValue '0000002A', got '%s'", v1.HexValue)
	}
	if v1.ReadOnly {
		t.Error("expected readOnly=false")
	}
	if v1.IsComplexType() {
		t.Error("simple type should not be complex")
	}

	// Check second variable (structure)
	v2 := result[1]
	if v2.MetaType != DebugMetaTypeStructure {
		t.Errorf("expected metaType 'structure', got '%s'", v2.MetaType)
	}
	if !v2.IsValueIncomplete {
		t.Error("expected isValueIncomplete=true")
	}
	if !v2.IsComplexType() {
		t.Error("structure should be complex type")
	}
}

func TestParseChildVariablesResponse(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <HIERARCHIES>
        <STPDA_ADT_VARIABLE_HIERARCHY>
          <PARENT_ID>@ROOT</PARENT_ID>
          <CHILD_ID>LV_COUNT</CHILD_ID>
          <CHILD_NAME>LV_COUNT</CHILD_NAME>
        </STPDA_ADT_VARIABLE_HIERARCHY>
        <STPDA_ADT_VARIABLE_HIERARCHY>
          <PARENT_ID>@ROOT</PARENT_ID>
          <CHILD_ID>LS_DATA</CHILD_ID>
          <CHILD_NAME>LS_DATA</CHILD_NAME>
        </STPDA_ADT_VARIABLE_HIERARCHY>
      </HIERARCHIES>
      <VARIABLES>
        <STPDA_ADT_VARIABLE>
          <ID>LV_COUNT</ID>
          <NAME>LV_COUNT</NAME>
          <META_TYPE>simple</META_TYPE>
          <VALUE>42</VALUE>
        </STPDA_ADT_VARIABLE>
        <STPDA_ADT_VARIABLE>
          <ID>LS_DATA</ID>
          <NAME>LS_DATA</NAME>
          <META_TYPE>structure</META_TYPE>
        </STPDA_ADT_VARIABLE>
      </VARIABLES>
    </DATA>
  </asx:values>
</asx:abap>`

	result, err := parseChildVariablesResponse([]byte(xmlResp))
	if err != nil {
		t.Fatalf("parseChildVariablesResponse failed: %v", err)
	}

	// Check hierarchies
	if len(result.Hierarchies) != 2 {
		t.Fatalf("expected 2 hierarchies, got %d", len(result.Hierarchies))
	}
	if result.Hierarchies[0].ParentID != "@ROOT" {
		t.Errorf("expected parentId '@ROOT', got '%s'", result.Hierarchies[0].ParentID)
	}
	if result.Hierarchies[0].ChildID != "LV_COUNT" {
		t.Errorf("expected childId 'LV_COUNT', got '%s'", result.Hierarchies[0].ChildID)
	}

	// Check variables
	if len(result.Variables) != 2 {
		t.Fatalf("expected 2 variables, got %d", len(result.Variables))
	}
}

func TestParseDebuggeeResponse(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <STPDA_DEBUGGEE>
        <CLIENT>001</CLIENT>
        <DEBUGGEE_ID>ABC123</DEBUGGEE_ID>
        <TERMINAL_ID>vsp-12345678</TERMINAL_ID>
        <IDE_ID>vsp</IDE_ID>
        <DEBUGGEE_USER>TESTUSER</DEBUGGEE_USER>
        <PRG_CURR>ZTEST_MCP_CRUD</PRG_CURR>
        <INCL_CURR>ZTEST_MCP_CRUD</INCL_CURR>
        <LINE_CURR>15</LINE_CURR>
        <RFCDEST></RFCDEST>
        <APPLSERVER>VHCALA4HCI</APPLSERVER>
        <SYSID>A4H</SYSID>
        <SYSNR>0</SYSNR>
        <TSTMP>20251205123456</TSTMP>
        <DBGEE_KIND>DEBUGGEE</DBGEE_KIND>
        <IS_ATTACH_IMPOSSIBLE></IS_ATTACH_IMPOSSIBLE>
        <IS_SAME_SERVER>X</IS_SAME_SERVER>
        <INSTANCE_NAME>A4H_01</INSTANCE_NAME>
      </STPDA_DEBUGGEE>
    </DATA>
  </asx:values>
</asx:abap>`

	result, err := parseDebuggeeResponse([]byte(xmlResp))
	if err != nil {
		t.Fatalf("parseDebuggeeResponse failed: %v", err)
	}

	if result.ID != "ABC123" {
		t.Errorf("expected debuggeeId 'ABC123', got '%s'", result.ID)
	}
	if result.User != "TESTUSER" {
		t.Errorf("expected user 'TESTUSER', got '%s'", result.User)
	}
	if result.Program != "ZTEST_MCP_CRUD" {
		t.Errorf("expected program 'ZTEST_MCP_CRUD', got '%s'", result.Program)
	}
	if result.Line != 15 {
		t.Errorf("expected line 15, got %d", result.Line)
	}
	if result.Kind != DebuggeeKindDebuggee {
		t.Errorf("expected kind 'debuggee', got '%s'", result.Kind)
	}
	if !result.IsAttachable {
		t.Error("expected isAttachable=true")
	}
	if !result.IsSameServer {
		t.Error("expected isSameServer=true")
	}
}

func TestParseDebuggeeResponse_PostMortem(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <STPDA_DEBUGGEE>
        <CLIENT>001</CLIENT>
        <DEBUGGEE_ID>DUMP123</DEBUGGEE_ID>
        <DEBUGGEE_USER>TESTUSER</DEBUGGEE_USER>
        <PRG_CURR>ZTEST_MCP_CRUD</PRG_CURR>
        <LINE_CURR>20</LINE_CURR>
        <DBGEE_KIND>POSTMORTEM</DBGEE_KIND>
        <IS_ATTACH_IMPOSSIBLE>X</IS_ATTACH_IMPOSSIBLE>
        <DUMP_ID>20251205_123456_TESTUSER</DUMP_ID>
        <DUMP_DATE>20251205</DUMP_DATE>
        <DUMP_TIME>123456</DUMP_TIME>
        <DUMP_HOST>VHCALA4HCI</DUMP_HOST>
        <DUMP_UNAME>TESTUSER</DUMP_UNAME>
        <DUMP_CLIENT>001</DUMP_CLIENT>
        <DUMP_URI>/sap/bc/adt/runtime/dumps/123456</DUMP_URI>
      </STPDA_DEBUGGEE>
    </DATA>
  </asx:values>
</asx:abap>`

	result, err := parseDebuggeeResponse([]byte(xmlResp))
	if err != nil {
		t.Fatalf("parseDebuggeeResponse failed: %v", err)
	}

	if result.Kind != DebuggeeKindPostMortem {
		t.Errorf("expected kind 'postmortem', got '%s'", result.Kind)
	}
	if result.IsAttachable {
		t.Error("expected isAttachable=false for post-mortem")
	}
	if result.DumpID != "20251205_123456_TESTUSER" {
		t.Errorf("expected dumpId '20251205_123456_TESTUSER', got '%s'", result.DumpID)
	}
}

func TestParseDebuggeeResponse_Empty(t *testing.T) {
	// Empty response should return nil
	result, err := parseDebuggeeResponse([]byte{})
	if err != nil {
		t.Fatalf("parseDebuggeeResponse failed: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for empty response")
	}
}

// --- Debug Session Integration Tests (Mock) ---

func TestDebuggerAttach_Mock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/core/discovery" {
			w.Header().Set("X-CSRF-Token", "test-token")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/sap/bc/adt/debugger" {
			method := r.URL.Query().Get("method")
			if method == "attach" {
				w.Header().Set("Content-Type", "application/xml")
				w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<dbg:attach xmlns:dbg="http://www.sap.com/adt/debugger"
  debugSessionId="session123"
  processId="42"
  isSteppingPossible="true"
  isTerminationPossible="true">
</dbg:attach>`))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "testuser", "testpass", WithClient("001"))
	ctx := context.Background()

	result, err := client.DebuggerAttach(ctx, "debuggee123", "testuser")
	if err != nil {
		t.Fatalf("DebuggerAttach failed: %v", err)
	}

	if result.DebugSessionID != "session123" {
		t.Errorf("expected debugSessionId 'session123', got '%s'", result.DebugSessionID)
	}
	if result.ProcessID != 42 {
		t.Errorf("expected processId 42, got %d", result.ProcessID)
	}
}

func TestDebuggerStep_Mock(t *testing.T) {
	var lastMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/core/discovery" {
			w.Header().Set("X-CSRF-Token", "test-token")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/sap/bc/adt/debugger" {
			lastMethod = r.URL.Query().Get("method")
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<dbg:step xmlns:dbg="http://www.sap.com/adt/debugger"
  debugSessionId="session123"
  isDebuggeeChanged="false"
  isSteppingPossible="true">
  <dbg:settings/>
</dbg:step>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "testuser", "testpass", WithClient("001"))
	ctx := context.Background()

	// Test stepOver
	result, err := client.DebuggerStep(ctx, DebugStepOver, "")
	if err != nil {
		t.Fatalf("DebuggerStep failed: %v", err)
	}
	if lastMethod != "stepOver" {
		t.Errorf("expected method 'stepOver', got '%s'", lastMethod)
	}
	if !result.IsSteppingPossible {
		t.Error("expected isSteppingPossible=true")
	}

	// Test stepInto
	_, err = client.DebuggerStep(ctx, DebugStepInto, "")
	if err != nil {
		t.Fatalf("DebuggerStep stepInto failed: %v", err)
	}
	if lastMethod != "stepInto" {
		t.Errorf("expected method 'stepInto', got '%s'", lastMethod)
	}

	// Test stepContinue
	_, err = client.DebuggerStep(ctx, DebugStepContinue, "")
	if err != nil {
		t.Fatalf("DebuggerStep stepContinue failed: %v", err)
	}
	if lastMethod != "stepContinue" {
		t.Errorf("expected method 'stepContinue', got '%s'", lastMethod)
	}
}

func TestDebuggerGetStack_Mock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/core/discovery" {
			w.Header().Set("X-CSRF-Token", "test-token")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodGet && r.URL.Path == "/sap/bc/adt/debugger/stack" {
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<dbg:stack xmlns:dbg="http://www.sap.com/adt/debugger"
  isSameSystem="true"
  debugCursorStackIndex="1">
  <dbg:stackEntry
    stackPosition="1"
    programName="ZTEST_MCP_CRUD"
    line="15"
    eventType="REPORT"/>
</dbg:stack>`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "testuser", "testpass", WithClient("001"))
	ctx := context.Background()

	result, err := client.DebuggerGetStack(ctx, true)
	if err != nil {
		t.Fatalf("DebuggerGetStack failed: %v", err)
	}

	if len(result.Stack) != 1 {
		t.Fatalf("expected 1 stack entry, got %d", len(result.Stack))
	}
	if result.Stack[0].ProgramName != "ZTEST_MCP_CRUD" {
		t.Errorf("expected programName 'ZTEST_MCP_CRUD', got '%s'", result.Stack[0].ProgramName)
	}
}

func TestDebuggerGetVariables_Mock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sap/bc/adt/core/discovery" {
			w.Header().Set("X-CSRF-Token", "test-token")
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodPost && r.URL.Path == "/sap/bc/adt/debugger" {
			method := r.URL.Query().Get("method")
			if method == "getVariables" {
				w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
				w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
    <DATA>
      <STPDA_ADT_VARIABLE>
        <ID>LV_COUNT</ID>
        <NAME>LV_COUNT</NAME>
        <META_TYPE>simple</META_TYPE>
        <VALUE>42</VALUE>
      </STPDA_ADT_VARIABLE>
    </DATA>
  </asx:values>
</asx:abap>`))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "testuser", "testpass", WithClient("001"))
	ctx := context.Background()

	result, err := client.DebuggerGetVariables(ctx, []string{"LV_COUNT"})
	if err != nil {
		t.Fatalf("DebuggerGetVariables failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 variable, got %d", len(result))
	}
	if result[0].Value != "42" {
		t.Errorf("expected value '42', got '%s'", result[0].Value)
	}
}

func TestDebuggerGetVariables_EmptyIDs(t *testing.T) {
	client := NewClient("http://localhost", "testuser", "testpass")
	ctx := context.Background()

	_, err := client.DebuggerGetVariables(ctx, []string{})
	if err == nil {
		t.Error("expected error for empty variable IDs")
	}
}

func TestDebugVariable_IsComplexType(t *testing.T) {
	tests := []struct {
		metaType DebugMetaType
		expected bool
	}{
		{DebugMetaTypeSimple, false},
		{DebugMetaTypeString, false},
		{DebugMetaTypeStructure, true},
		{DebugMetaTypeTable, true},
		{DebugMetaTypeDataRef, true},
		{DebugMetaTypeObjectRef, true},
		{DebugMetaTypeClass, true},
		{DebugMetaTypeObject, true},
		{DebugMetaTypeBoxRef, true},
		{DebugMetaTypeBoxedComp, false},
		{DebugMetaTypeAnonymComp, false},
		{DebugMetaTypeUnknown, false},
	}

	for _, tt := range tests {
		v := DebugVariable{MetaType: tt.metaType}
		result := v.IsComplexType()
		if result != tt.expected {
			t.Errorf("IsComplexType() for %s: expected %v, got %v", tt.metaType, tt.expected, result)
		}
	}
}

func TestDebugStepTypes(t *testing.T) {
	// Verify step type constants
	tests := []struct {
		stepType DebugStepType
		expected string
	}{
		{DebugStepInto, "stepInto"},
		{DebugStepOver, "stepOver"},
		{DebugStepReturn, "stepReturn"},
		{DebugStepContinue, "stepContinue"},
		{DebugStepRunToLine, "stepRunToLine"},
		{DebugStepJumpToLine, "stepJumpToLine"},
		{DebugTerminate, "terminateDebuggee"},
	}

	for _, tt := range tests {
		if string(tt.stepType) != tt.expected {
			t.Errorf("step type %v: expected '%s', got '%s'", tt.stepType, tt.expected, string(tt.stepType))
		}
	}
}
