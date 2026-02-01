package adt

import (
	"testing"
)

func TestXMLEscape(t *testing.T) {
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
		result := XMLEscape(tt.input)
		if result != tt.expected {
			t.Errorf("XMLEscape(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

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
  sessionTitle="AVINOGRADOVA"
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
        <DEBUGGEE_USER>AVINOGRADOVA</DEBUGGEE_USER>
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
	if result.User != "AVINOGRADOVA" {
		t.Errorf("expected user 'AVINOGRADOVA', got '%s'", result.User)
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
        <DEBUGGEE_USER>AVINOGRADOVA</DEBUGGEE_USER>
        <PRG_CURR>ZTEST_MCP_CRUD</PRG_CURR>
        <LINE_CURR>20</LINE_CURR>
        <DBGEE_KIND>POSTMORTEM</DBGEE_KIND>
        <IS_ATTACH_IMPOSSIBLE>X</IS_ATTACH_IMPOSSIBLE>
        <DUMP_ID>20251205_123456_AVINOGRADOVA</DUMP_ID>
        <DUMP_DATE>20251205</DUMP_DATE>
        <DUMP_TIME>123456</DUMP_TIME>
        <DUMP_HOST>VHCALA4HCI</DUMP_HOST>
        <DUMP_UNAME>AVINOGRADOVA</DUMP_UNAME>
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
	if result.DumpID != "20251205_123456_AVINOGRADOVA" {
		t.Errorf("expected dumpId '20251205_123456_AVINOGRADOVA', got '%s'", result.DumpID)
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
