// ABOUTME: XML parsing utilities for ABAP debugger responses.
// ABOUTME: Shared parsing logic for attach, step, stack, and variable responses.

package adt

import (
	"encoding/xml"
	"fmt"
)

// --- Shared XML Types for Parsing ---

// xmlAction is the shared type for action elements in attach/step responses.
type xmlAction struct {
	Name     string `xml:"name,attr"`
	Style    string `xml:"style,attr"`
	Group    string `xml:"group,attr"`
	Title    string `xml:"title,attr"`
	Link     string `xml:"link,attr"`
	Value    string `xml:"value,attr"`
	Disabled bool   `xml:"disabled,attr"`
}

// xmlVariable is the shared type for variable elements in ABAP XML responses.
type xmlVariable struct {
	ID                string `xml:"ID"`
	Name              string `xml:"NAME"`
	DeclaredTypeName  string `xml:"DECLARED_TYPE_NAME"`
	ActualTypeName    string `xml:"ACTUAL_TYPE_NAME"`
	Kind              string `xml:"KIND"`
	InstantiationKind string `xml:"INSTANTIATION_KIND"`
	AccessKind        string `xml:"ACCESS_KIND"`
	MetaType          string `xml:"META_TYPE"`
	ParameterKind     string `xml:"PARAMETER_KIND"`
	Value             string `xml:"VALUE"`
	HexValue          string `xml:"HEX_VALUE"`
	ReadOnly          string `xml:"READ_ONLY"`
	TechnicalType     string `xml:"TECHNICAL_TYPE"`
	Length            int    `xml:"LENGTH"`
	TableBody         string `xml:"TABLE_BODY"`
	TableLines        int    `xml:"TABLE_LINES"`
	IsValueIncomplete string `xml:"IS_VALUE_INCOMPLETE"`
	IsException       string `xml:"IS_EXCEPTION"`
	InheritanceLevel  int    `xml:"INHERITANCE_LEVEL"`
	InheritanceClass  string `xml:"INHERITANCE_CLASS"`
}

// convertXMLVariable converts an xmlVariable to DebugVariable.
func convertXMLVariable(v xmlVariable) DebugVariable {
	return DebugVariable{
		ID:                v.ID,
		Name:              v.Name,
		DeclaredTypeName:  v.DeclaredTypeName,
		ActualTypeName:    v.ActualTypeName,
		Kind:              v.Kind,
		InstantiationKind: v.InstantiationKind,
		AccessKind:        v.AccessKind,
		MetaType:          DebugMetaType(v.MetaType),
		ParameterKind:     v.ParameterKind,
		Value:             v.Value,
		HexValue:          v.HexValue,
		ReadOnly:          v.ReadOnly == "X",
		TechnicalType:     v.TechnicalType,
		Length:            v.Length,
		TableBody:         v.TableBody,
		TableLines:        v.TableLines,
		IsValueIncomplete: v.IsValueIncomplete == "X",
		IsException:       v.IsException == "X",
		InheritanceLevel:  v.InheritanceLevel,
		InheritanceClass:  v.InheritanceClass,
	}
}

// --- Parse Functions ---

// parseDebuggeeResponse parses the XML response containing debuggee information.
func parseDebuggeeResponse(data []byte) (*Debuggee, error) {
	if len(data) == 0 {
		return nil, nil
	}

	xmlStr := StripXMLNamespaces(string(data), "asx:", "abap:")

	// The response is in ABAP XML format: <abap><values><DATA><STPDA_DEBUGGEE>...</STPDA_DEBUGGEE></DATA></values></abap>
	type stpdaDebuggee struct {
		Client             int    `xml:"CLIENT"`
		DebuggeeID         string `xml:"DEBUGGEE_ID"`
		TerminalID         string `xml:"TERMINAL_ID"`
		IdeID              string `xml:"IDE_ID"`
		DebuggeeUser       string `xml:"DEBUGGEE_USER"`
		ProgramCurrent     string `xml:"PRG_CURR"`
		IncludeCurrent     string `xml:"INCL_CURR"`
		LineCurrent        int    `xml:"LINE_CURR"`
		RFCDest            string `xml:"RFCDEST"`
		AppServer          string `xml:"APPLSERVER"`
		SystemID           string `xml:"SYSID"`
		SystemNumber       int    `xml:"SYSNR"`
		Timestamp          int64  `xml:"TSTMP"`
		DebuggeeKind       string `xml:"DBGEE_KIND"`
		IsAttachImpossible string `xml:"IS_ATTACH_IMPOSSIBLE"`
		IsSameServer       string `xml:"IS_SAME_SERVER"`
		InstanceName       string `xml:"INSTANCE_NAME"`
		DumpID             string `xml:"DUMP_ID"`
		DumpDate           string `xml:"DUMP_DATE"`
		DumpTime           string `xml:"DUMP_TIME"`
		DumpHost           string `xml:"DUMP_HOST"`
		DumpUser           string `xml:"DUMP_UNAME"`
		DumpClient         string `xml:"DUMP_CLIENT"`
		DumpURI            string `xml:"DUMP_URI"`
	}

	type abapResponse struct {
		XMLName xml.Name `xml:"abap"`
		Values  struct {
			Data struct {
				Debuggee stpdaDebuggee `xml:"STPDA_DEBUGGEE"`
			} `xml:"DATA"`
		} `xml:"values"`
	}

	var resp abapResponse
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing debuggee XML: %w", err)
	}

	d := resp.Values.Data.Debuggee
	if d.DebuggeeID == "" {
		return nil, nil // No debuggee
	}

	debuggee := &Debuggee{
		ID:           d.DebuggeeID,
		Client:       d.Client,
		TerminalID:   d.TerminalID,
		IdeID:        d.IdeID,
		User:         d.DebuggeeUser,
		Program:      d.ProgramCurrent,
		Include:      d.IncludeCurrent,
		Line:         d.LineCurrent,
		RFCDest:      d.RFCDest,
		AppServer:    d.AppServer,
		SystemID:     d.SystemID,
		SystemNumber: d.SystemNumber,
		Timestamp:    d.Timestamp,
		InstanceName: d.InstanceName,
		DumpID:       d.DumpID,
		DumpDate:     d.DumpDate,
		DumpTime:     d.DumpTime,
		DumpHost:     d.DumpHost,
		DumpUser:     d.DumpUser,
		DumpClient:   d.DumpClient,
		DumpURI:      d.DumpURI,
	}

	// Parse kind
	switch d.DebuggeeKind {
	case "POSTMORTEM":
		debuggee.Kind = DebuggeeKindPostMortem
	case "POSTMORTEM_DIALOG":
		debuggee.Kind = DebuggeeKindPostMortemDialog
	default:
		debuggee.Kind = DebuggeeKindDebuggee
	}

	// Parse boolean flags
	debuggee.IsAttachable = d.IsAttachImpossible != "X"
	debuggee.IsSameServer = d.IsSameServer == "X"

	return debuggee, nil
}

func parseAttachResponse(data []byte) (*DebugAttachResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty attach response")
	}

	xmlStr := StripXMLNamespaces(string(data), "dbg:")

	type xmlBreakpoint struct {
		ID                               string `xml:"id,attr"`
		Kind                             string `xml:"kind,attr"`
		UnresolvableCondition            string `xml:"unresolvableCondition,attr"`
		UnresolvableConditionErrorOffset string `xml:"unresolvableConditionErrorOffset,attr"`
	}

	type xmlAttach struct {
		XMLName                    xml.Name `xml:"attach"`
		IsRFC                      bool     `xml:"isRfc,attr"`
		IsSameSystem               bool     `xml:"isSameSystem,attr"`
		ServerName                 string   `xml:"serverName,attr"`
		DebugSessionID             string   `xml:"debugSessionId,attr"`
		ProcessID                  int      `xml:"processId,attr"`
		IsPostMortem               bool     `xml:"isPostMortem,attr"`
		IsUserAuthorizedForChanges bool     `xml:"isUserAuthorizedForChanges,attr"`
		DebuggeeSessionID          string   `xml:"debuggeeSessionId,attr"`
		AbapTraceState             string   `xml:"abapTraceState,attr"`
		CanAdvancedTableFeatures   bool     `xml:"canAdvancedTableFeatures,attr"`
		IsNonExclusive             bool     `xml:"isNonExclusive,attr"`
		IsNonExclusiveToggled      bool     `xml:"isNonExclusiveToggled,attr"`
		GuiEditorGuid              string   `xml:"guiEditorGuid,attr"`
		SessionTitle               string   `xml:"sessionTitle,attr"`
		IsSteppingPossible         bool     `xml:"isSteppingPossible,attr"`
		IsTerminationPossible      bool     `xml:"isTerminationPossible,attr"`
		Actions                    struct {
			Action []xmlAction `xml:"action"`
		} `xml:"actions"`
		ReachedBreakpoints struct {
			Breakpoint []xmlBreakpoint `xml:"breakpoint"`
		} `xml:"reachedBreakpoints"`
	}

	var resp xmlAttach
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing attach response: %w", err)
	}

	result := &DebugAttachResult{
		DebugState: DebugState{
			IsRFC:                      resp.IsRFC,
			IsSameSystem:               resp.IsSameSystem,
			ServerName:                 resp.ServerName,
			DebugSessionID:             resp.DebugSessionID,
			ProcessID:                  resp.ProcessID,
			IsPostMortem:               resp.IsPostMortem,
			IsUserAuthorizedForChanges: resp.IsUserAuthorizedForChanges,
			DebuggeeSessionID:          resp.DebuggeeSessionID,
			AbapTraceState:             resp.AbapTraceState,
			CanAdvancedTableFeatures:   resp.CanAdvancedTableFeatures,
			IsNonExclusive:             resp.IsNonExclusive,
			IsNonExclusiveToggled:      resp.IsNonExclusiveToggled,
			GuiEditorGuid:              resp.GuiEditorGuid,
			SessionTitle:               resp.SessionTitle,
			IsSteppingPossible:         resp.IsSteppingPossible,
			IsTerminationPossible:      resp.IsTerminationPossible,
		},
	}

	// Parse actions
	for _, a := range resp.Actions.Action {
		result.Actions = append(result.Actions, DebugAction{
			Name:     a.Name,
			Style:    a.Style,
			Group:    a.Group,
			Title:    a.Title,
			Link:     a.Link,
			Value:    a.Value,
			Disabled: a.Disabled,
		})
	}

	// Parse reached breakpoints
	for _, bp := range resp.ReachedBreakpoints.Breakpoint {
		result.ReachedBreakpoints = append(result.ReachedBreakpoints, DebugReachedBreakpoint{
			ID:                               bp.ID,
			Kind:                             bp.Kind,
			UnresolvableCondition:            bp.UnresolvableCondition,
			UnresolvableConditionErrorOffset: bp.UnresolvableConditionErrorOffset,
		})
	}

	return result, nil
}

func parseStepResponse(data []byte) (*DebugStepResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty step response")
	}

	xmlStr := StripXMLNamespaces(string(data), "dbg:")

	type xmlSettings struct {
		SystemDebugging       bool `xml:"systemDebugging,attr"`
		CreateExceptionObject bool `xml:"createExceptionObject,attr"`
		BackgroundRFC         bool `xml:"backgroundRFC,attr"`
		SharedObjectDebugging bool `xml:"sharedObjectDebugging,attr"`
		ShowDataAging         bool `xml:"showDataAging,attr"`
		UpdateDebugging       bool `xml:"updateDebugging,attr"`
	}

	type xmlBreakpoint struct {
		ID   string `xml:"id,attr"`
		Kind string `xml:"kind,attr"`
	}

	type xmlStep struct {
		XMLName                    xml.Name    `xml:"step"`
		IsRFC                      bool        `xml:"isRfc,attr"`
		IsSameSystem               bool        `xml:"isSameSystem,attr"`
		ServerName                 string      `xml:"serverName,attr"`
		DebugSessionID             string      `xml:"debugSessionId,attr"`
		ProcessID                  int         `xml:"processId,attr"`
		IsPostMortem               bool        `xml:"isPostMortem,attr"`
		IsUserAuthorizedForChanges bool        `xml:"isUserAuthorizedForChanges,attr"`
		DebuggeeSessionID          string      `xml:"debuggeeSessionId,attr"`
		AbapTraceState             string      `xml:"abapTraceState,attr"`
		CanAdvancedTableFeatures   bool        `xml:"canAdvancedTableFeatures,attr"`
		IsNonExclusive             bool        `xml:"isNonExclusive,attr"`
		IsNonExclusiveToggled      bool        `xml:"isNonExclusiveToggled,attr"`
		GuiEditorGuid              string      `xml:"guiEditorGuid,attr"`
		SessionTitle               string      `xml:"sessionTitle,attr"`
		IsSteppingPossible         bool        `xml:"isSteppingPossible,attr"`
		IsTerminationPossible      bool        `xml:"isTerminationPossible,attr"`
		IsDebuggeeChanged          bool        `xml:"isDebuggeeChanged,attr"`
		Settings                   xmlSettings `xml:"settings"`
		Actions                    struct {
			Action []xmlAction `xml:"action"`
		} `xml:"actions"`
		ReachedBreakpoints struct {
			Breakpoint []xmlBreakpoint `xml:"breakpoint"`
		} `xml:"reachedBreakpoints"`
	}

	var resp xmlStep
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing step response: %w", err)
	}

	result := &DebugStepResult{
		DebugState: DebugState{
			IsRFC:                      resp.IsRFC,
			IsSameSystem:               resp.IsSameSystem,
			ServerName:                 resp.ServerName,
			DebugSessionID:             resp.DebugSessionID,
			ProcessID:                  resp.ProcessID,
			IsPostMortem:               resp.IsPostMortem,
			IsUserAuthorizedForChanges: resp.IsUserAuthorizedForChanges,
			DebuggeeSessionID:          resp.DebuggeeSessionID,
			AbapTraceState:             resp.AbapTraceState,
			CanAdvancedTableFeatures:   resp.CanAdvancedTableFeatures,
			IsNonExclusive:             resp.IsNonExclusive,
			IsNonExclusiveToggled:      resp.IsNonExclusiveToggled,
			GuiEditorGuid:              resp.GuiEditorGuid,
			SessionTitle:               resp.SessionTitle,
			IsSteppingPossible:         resp.IsSteppingPossible,
			IsTerminationPossible:      resp.IsTerminationPossible,
		},
		IsDebuggeeChanged: resp.IsDebuggeeChanged,
		Settings: DebugSettings{
			SystemDebugging:       resp.Settings.SystemDebugging,
			CreateExceptionObject: resp.Settings.CreateExceptionObject,
			BackgroundRFC:         resp.Settings.BackgroundRFC,
			SharedObjectDebugging: resp.Settings.SharedObjectDebugging,
			ShowDataAging:         resp.Settings.ShowDataAging,
			UpdateDebugging:       resp.Settings.UpdateDebugging,
		},
	}

	// Parse actions
	for _, a := range resp.Actions.Action {
		result.Actions = append(result.Actions, DebugAction{
			Name:     a.Name,
			Style:    a.Style,
			Group:    a.Group,
			Title:    a.Title,
			Link:     a.Link,
			Value:    a.Value,
			Disabled: a.Disabled,
		})
	}

	// Parse reached breakpoints
	for _, bp := range resp.ReachedBreakpoints.Breakpoint {
		result.ReachedBreakpoints = append(result.ReachedBreakpoints, DebugReachedBreakpoint{
			ID:   bp.ID,
			Kind: bp.Kind,
		})
	}

	return result, nil
}

func parseStackResponse(data []byte) (*DebugStackInfo, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty stack response")
	}

	xmlStr := StripXMLNamespaces(string(data), "dbg:")

	type xmlStackEntry struct {
		StackPosition int    `xml:"stackPosition,attr"`
		StackType     string `xml:"stackType,attr"`
		StackURI      string `xml:"stackUri,attr"`
		ProgramName   string `xml:"programName,attr"`
		IncludeName   string `xml:"includeName,attr"`
		Line          int    `xml:"line,attr"`
		EventType     string `xml:"eventType,attr"`
		EventName     string `xml:"eventName,attr"`
		SourceType    string `xml:"sourceType,attr"`
		SystemProgram bool   `xml:"systemProgram,attr"`
		IsVit         bool   `xml:"isVit,attr"`
		URI           string `xml:"uri,attr"`
	}

	type xmlStack struct {
		XMLName               xml.Name        `xml:"stack"`
		IsRFC                 bool            `xml:"isRfc,attr"`
		IsSameSystem          bool            `xml:"isSameSystem,attr"`
		ServerName            string          `xml:"serverName,attr"`
		DebugCursorStackIndex int             `xml:"debugCursorStackIndex,attr"`
		StackEntry            []xmlStackEntry `xml:"stackEntry"`
	}

	var resp xmlStack
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing stack response: %w", err)
	}

	result := &DebugStackInfo{
		IsRFC:                 resp.IsRFC,
		IsSameSystem:          resp.IsSameSystem,
		ServerName:            resp.ServerName,
		DebugCursorStackIndex: resp.DebugCursorStackIndex,
	}

	for _, e := range resp.StackEntry {
		result.Stack = append(result.Stack, DebugStackEntry{
			StackPosition: e.StackPosition,
			StackType:     e.StackType,
			StackURI:      e.StackURI,
			ProgramName:   e.ProgramName,
			IncludeName:   e.IncludeName,
			Line:          e.Line,
			EventType:     e.EventType,
			EventName:     e.EventName,
			SourceType:    e.SourceType,
			SystemProgram: e.SystemProgram,
			IsVit:         e.IsVit,
			URI:           e.URI,
		})
	}

	return result, nil
}

func parseVariablesResponse(data []byte) ([]DebugVariable, error) {
	if len(data) == 0 {
		return nil, nil
	}

	xmlStr := StripXMLNamespaces(string(data), "asx:")

	type xmlAbap struct {
		XMLName xml.Name `xml:"abap"`
		Values  struct {
			Data struct {
				Variables []xmlVariable `xml:"STPDA_ADT_VARIABLE"`
			} `xml:"DATA"`
		} `xml:"values"`
	}

	var resp xmlAbap
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing variables response: %w", err)
	}

	var result []DebugVariable
	for _, v := range resp.Values.Data.Variables {
		result = append(result, convertXMLVariable(v))
	}

	return result, nil
}

func parseChildVariablesResponse(data []byte) (*DebugChildVariablesInfo, error) {
	if len(data) == 0 {
		return nil, nil
	}

	xmlStr := StripXMLNamespaces(string(data), "asx:")

	type xmlHierarchy struct {
		ParentID  string `xml:"PARENT_ID"`
		ChildID   string `xml:"CHILD_ID"`
		ChildName string `xml:"CHILD_NAME"`
	}

	type xmlAbap struct {
		XMLName xml.Name `xml:"abap"`
		Values  struct {
			Data struct {
				Hierarchies struct {
					Hierarchy []xmlHierarchy `xml:"STPDA_ADT_VARIABLE_HIERARCHY"`
				} `xml:"HIERARCHIES"`
				Variables struct {
					Variable []xmlVariable `xml:"STPDA_ADT_VARIABLE"`
				} `xml:"VARIABLES"`
			} `xml:"DATA"`
		} `xml:"values"`
	}

	var resp xmlAbap
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing child variables response: %w", err)
	}

	result := &DebugChildVariablesInfo{}

	for _, h := range resp.Values.Data.Hierarchies.Hierarchy {
		result.Hierarchies = append(result.Hierarchies, DebugVariableHierarchy{
			ParentID:  h.ParentID,
			ChildID:   h.ChildID,
			ChildName: h.ChildName,
		})
	}

	for _, v := range resp.Values.Data.Variables.Variable {
		result.Variables = append(result.Variables, convertXMLVariable(v))
	}

	return result, nil
}
