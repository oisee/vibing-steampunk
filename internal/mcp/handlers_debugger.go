// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_debugger.go contains handlers for WebSocket-based debugging (via ZADT_VSP).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Debugger Routing ---
// Routes for this module:
//   debug: set_breakpoint, get_breakpoints, delete_breakpoint, call_rfc, run_report, setup

// routeDebuggerAction routes debugger-specific actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeDebuggerAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "debug" {
		return nil, false, nil
	}

	debugType := strings.ToLower(objectType)
	if debugType == "" {
		if t, ok := params["type"].(string); ok {
			debugType = strings.ToLower(t)
		}
	}

	switch debugType {
	case "set_breakpoint", "setup":
		kind, _ := params["kind"].(string)
		if kind == "" {
			kind = "line"
		}
		args := map[string]any{"kind": kind}
		if program, ok := params["program"].(string); ok {
			args["program"] = program
		}
		if line, ok := params["line"].(float64); ok {
			args["line"] = line
		}
		if method, ok := params["method"].(string); ok {
			args["method"] = method
		}
		if statement, ok := params["statement"].(string); ok {
			args["statement"] = statement
		}
		if exception, ok := params["exception"].(string); ok {
			args["exception"] = exception
		}
		result, err := s.handleSetBreakpoint(ctx, newRequest(args))
		return result, true, err

	case "get_breakpoints", "breakpoints":
		result, err := s.handleGetBreakpoints(ctx, newRequest(nil))
		return result, true, err

	case "delete_breakpoint":
		bpID, _ := params["breakpoint_id"].(string)
		if bpID == "" {
			bpID = objectName
		}
		if bpID == "" {
			return newToolResultError("breakpoint_id is required"), true, nil
		}
		result, err := s.handleDeleteBreakpoint(ctx, newRequest(map[string]any{"breakpoint_id": bpID}))
		return result, true, err

	case "call_rfc":
		function, _ := params["function"].(string)
		if function == "" {
			return newToolResultError("function is required for call_rfc"), true, nil
		}
		args := map[string]any{"function": function}
		if rfcParams, ok := params["params"].(string); ok {
			args["params"] = rfcParams
		}
		result, err := s.handleCallRFC(ctx, newRequest(args))
		return result, true, err

	case "run_report":
		report, _ := params["report"].(string)
		if report == "" {
			return newToolResultError("report is required for run_report"), true, nil
		}
		args := map[string]any{"report": report}
		if variant, ok := params["variant"].(string); ok {
			args["variant"] = variant
		}
		result, err := s.handleDebugRunReport(ctx, newRequest(args))
		return result, true, err

	case "classrun", "run_class":
		className, _ := params["class"].(string)
		if className == "" {
			className = objectName
		}
		if className == "" {
			return newToolResultError("class is required for classrun"), true, nil
		}
		result, err := s.handleDebugClassrun(ctx, className)
		return result, true, err

	// HTTP breakpoints (CL_ABAP_DEBUGGER - writes to ABDBG_BPS, checked by HTTP execution)
	case "set_http_breakpoint", "sethttpbreakpoint":
		result, err := s.handleSetHttpBreakpoint(ctx, params)
		return result, true, err

	case "get_http_breakpoints", "gethttpbreakpoints":
		result, err := s.handleGetHttpBreakpoints(ctx)
		return result, true, err

	case "delete_http_breakpoints", "deletehttpbreakpoints":
		result, err := s.handleDeleteHttpBreakpoints(ctx)
		return result, true, err
	}

	return nil, false, nil
}

// --- Debugger Session Handlers (WebSocket-based via ZADT_VSP) ---
// All breakpoint operations use WebSocket for reliable CSRF-free communication.

// ensureDebugWSClient ensures WebSocket debug client is connected.
func (s *Server) ensureDebugWSClient(ctx context.Context) error {
	if s.debugWSClient != nil && s.debugWSClient.IsConnected() {
		return nil
	}

	// Create new client
	s.debugWSClient = adt.NewDebugWebSocketClient(
		s.config.BaseURL,
		s.config.Client,
		s.config.Username,
		s.config.Password,
		s.config.InsecureSkipVerify,
	)

	return s.debugWSClient.Connect(ctx)
}

func (s *Server) handleSetBreakpoint(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get breakpoint kind (default: "line")
	kind, _ := request.Params.Arguments["kind"].(string)
	if kind == "" {
		kind = "line"
	}

	// Ensure WebSocket client is connected
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError("SetBreakpoint: WebSocket connect failed: " + err.Error() + ". Ensure ZADT_VSP is deployed and SAPC/SICF are configured."), nil
	}

	var bpID string
	var err error
	var msg strings.Builder

	switch kind {
	case "line":
		program, ok := request.Params.Arguments["program"].(string)
		if !ok || program == "" {
			return newToolResultError("program is required for line breakpoints"), nil
		}

		lineFloat, ok := request.Params.Arguments["line"].(float64)
		if !ok || lineFloat <= 0 {
			return newToolResultError("line is required and must be positive for line breakpoints"), nil
		}
		line := int(lineFloat)

		// Optional method parameter for include-relative line numbers
		method, _ := request.Params.Arguments["method"].(string)

		// Auto-convert class names to pool format (ZCL_TEST → ZCL_TEST================CP)
		originalProgram := program
		program = convertToClassPool(program)

		// Use method-aware breakpoint if method is specified
		if method != "" {
			bpID, err = s.debugWSClient.SetMethodBreakpoint(ctx, program, method, line)
			if err != nil {
				return wrapErr("SetMethodBreakpoint", err), nil
			}

			msg.WriteString("Method breakpoint set successfully!\n\n")
			fmt.Fprintf(&msg, "Breakpoint ID: %s\n", bpID)
			if program != originalProgram {
				fmt.Fprintf(&msg, "Program: %s (converted from %s)\n", program, originalProgram)
			} else {
				fmt.Fprintf(&msg, "Program: %s\n", program)
			}
			fmt.Fprintf(&msg, "Method: %s\n", method)
			fmt.Fprintf(&msg, "Line: %d (relative to method start)\n", line)
			msg.WriteString("\nℹ️  Line number is relative to the METHOD implementation, not the full class.\n")
		} else {
			bpID, err = s.debugWSClient.SetLineBreakpoint(ctx, program, line)
			if err != nil {
				return wrapErr("SetLineBreakpoint", err), nil
			}

			msg.WriteString("Line breakpoint set successfully!\n\n")
			fmt.Fprintf(&msg, "Breakpoint ID: %s\n", bpID)
			if program != originalProgram {
				fmt.Fprintf(&msg, "Program: %s (converted from %s)\n", program, originalProgram)
			} else {
				fmt.Fprintf(&msg, "Program: %s\n", program)
			}
			fmt.Fprintf(&msg, "Line: %d (pool-absolute)\n", line)
		}

	case "statement":
		statement, ok := request.Params.Arguments["statement"].(string)
		if !ok || statement == "" {
			return newToolResultError("statement is required for statement breakpoints (e.g., 'CALL FUNCTION', 'SELECT', 'LOOP')"), nil
		}

		bpID, err = s.debugWSClient.SetStatementBreakpoint(ctx, statement)
		if err != nil {
			return wrapErr("SetStatementBreakpoint", err), nil
		}

		msg.WriteString("Statement breakpoint set successfully!\n\n")
		fmt.Fprintf(&msg, "Breakpoint ID: %s\n", bpID)
		fmt.Fprintf(&msg, "Statement: %s\n", statement)
		msg.WriteString("\nThis breakpoint will trigger on ALL occurrences of this statement type.\n")

	case "exception":
		exception, ok := request.Params.Arguments["exception"].(string)
		if !ok || exception == "" {
			return newToolResultError("exception is required for exception breakpoints (e.g., 'CX_SY_ZERODIVIDE')"), nil
		}

		bpID, err = s.debugWSClient.SetExceptionBreakpoint(ctx, exception)
		if err != nil {
			return wrapErr("SetExceptionBreakpoint", err), nil
		}

		msg.WriteString("Exception breakpoint set successfully!\n\n")
		fmt.Fprintf(&msg, "Breakpoint ID: %s\n", bpID)
		fmt.Fprintf(&msg, "Exception: %s\n", exception)
		msg.WriteString("\nThis breakpoint will trigger when this exception is raised.\n")

	default:
		return newToolResultError("Invalid breakpoint kind: " + kind + ". Valid kinds: line, statement, exception"), nil
	}

	msg.WriteString("\n⚠️  IMPORTANT: Breakpoints only trigger for code executed in a DIFFERENT SAP session.\n")
	msg.WriteString("Use DebuggerListen in this session, then trigger execution from another session\n")
	msg.WriteString("(e.g., SAP GUI, HTTP request, RunUnitTests from another connection).")

	return mcp.NewToolResultText(msg.String()), nil
}

// convertToClassPool converts class/interface names to pool format for debugging.
// Example: ZCL_TEST → ZCL_TEST================CP (padded to 30 chars + CP suffix)
func convertToClassPool(program string) string {
	program = strings.ToUpper(program)

	// Already in pool format
	if strings.HasSuffix(program, "CP") && strings.Contains(program, "=") {
		return program
	}

	// Check if it looks like a class or interface name
	isClass := strings.HasPrefix(program, "ZCL_") ||
		strings.HasPrefix(program, "YCL_") ||
		strings.HasPrefix(program, "ZIF_") ||
		strings.HasPrefix(program, "YIF_") ||
		strings.HasPrefix(program, "LCL_") ||
		strings.HasPrefix(program, "LIF_") ||
		strings.Contains(program, "/CL_") ||
		strings.Contains(program, "/IF_")

	if !isClass {
		return program
	}

	// Pad to 30 chars with '=' and add 'CP' suffix
	// Total length: 30 + 2 = 32 (standard ABAP class pool naming)
	if len(program) < 30 {
		padding := 30 - len(program)
		program = program + strings.Repeat("=", padding) + "CP"
	}

	return program
}

func (s *Server) handleGetBreakpoints(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return wrapErr("GetBreakpoints/Connect", err), nil
	}

	breakpoints, err := s.debugWSClient.GetBreakpoints(ctx)
	if err != nil {
		return wrapErr("GetBreakpoints", err), nil
	}

	if len(breakpoints) == 0 {
		return mcp.NewToolResultText("No breakpoints are currently set."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Active Breakpoints (%d):\n\n", len(breakpoints))
	for i, bp := range breakpoints {
		fmt.Fprintf(&sb, "%d. ID: %v\n", i+1, bp["id"])
		if kind, ok := bp["kind"]; ok {
			fmt.Fprintf(&sb, "   Kind: %v\n", kind)
		}
		if uri, ok := bp["uri"]; ok {
			fmt.Fprintf(&sb, "   URI: %v\n", uri)
		}
		if line, ok := bp["line"]; ok {
			fmt.Fprintf(&sb, "   Line: %v\n", line)
		}
		sb.WriteString("\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleDeleteBreakpoint(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	bpID, ok := request.Params.Arguments["breakpoint_id"].(string)
	if !ok || bpID == "" {
		return newToolResultError("breakpoint_id is required"), nil
	}

	if err := s.ensureDebugWSClient(ctx); err != nil {
		return wrapErr("DeleteBreakpoint/Connect", err), nil
	}

	if err := s.debugWSClient.DeleteBreakpoint(ctx, bpID); err != nil {
		return wrapErr("DeleteBreakpoint", err), nil
	}

	return mcp.NewToolResultText("Breakpoint " + bpID + " deleted successfully."), nil
}

func (s *Server) handleCallRFC(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	function, ok := request.Params.Arguments["function"].(string)
	if !ok || function == "" {
		return newToolResultError("function is required"), nil
	}

	// Parse params if provided
	rfcParams := make(map[string]string)
	if paramsStr, ok := request.Params.Arguments["params"].(string); ok && paramsStr != "" {
		// Parse JSON params
		var rawParams map[string]any
		if err := json.Unmarshal([]byte(paramsStr), &rawParams); err != nil {
			return wrapErr("CallRFC/ParseParams", err), nil
		}
		for k, v := range rawParams {
			rfcParams[k] = fmt.Sprintf("%v", v)
		}
	}

	// Ensure WebSocket client is connected
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError("CallRFC: WebSocket connect failed: " + err.Error() + ". Ensure ZADT_VSP is deployed and SAPC/SICF are configured."), nil
	}

	result, err := s.debugWSClient.CallRFC(ctx, function, rfcParams)
	if err != nil {
		return wrapErr("CallRFC", err), nil
	}

	// Format result
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("RFC call completed.\n\nFunction: %s\nSubrc: %d\n\nResult:\n%s", function, result.Subrc, string(resultJSON))), nil
}

// handleDebugRunReport triggers report execution via WebSocket for debugging.
// This schedules a background job that CAN hit external breakpoints, then returns immediately.
// Use with DebuggerListen to catch the debuggee when it hits a breakpoint.
func (s *Server) handleDebugRunReport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	report, ok := request.Params.Arguments["report"].(string)
	if !ok || report == "" {
		return newToolResultError("report is required"), nil
	}

	variant, _ := request.Params.Arguments["variant"].(string)

	// Ensure WebSocket client is connected
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError("RunReport: WebSocket connect failed: " + err.Error() + ". Ensure ZADT_VSP is deployed and SAPC/SICF are configured."), nil
	}

	// Trigger async execution - this schedules a background job that runs in a separate
	// work process and CAN hit external breakpoints
	err := s.debugWSClient.RunReport(ctx, report, variant)
	if err != nil {
		return wrapErr("RunReport", err), nil
	}

	var sb strings.Builder
	sb.WriteString("Report execution triggered for debugging.\n\n")
	fmt.Fprintf(&sb, "Report: %s\n", report)
	if variant != "" {
		fmt.Fprintf(&sb, "Variant: %s\n", variant)
	}
	sb.WriteString("\nThe report is scheduled as a background job in a separate work process.\n")
	sb.WriteString("If it hits a breakpoint, DebuggerListen will catch the debuggee.\n\n")
	sb.WriteString("Recommended workflow:\n")
	sb.WriteString("1. Set breakpoint: debug set_breakpoint with program/line\n")
	sb.WriteString("2. Trigger execution: debug run_report (this action)\n")
	sb.WriteString("3. Wait for debuggee: debug listen\n")
	sb.WriteString("4. Attach: debug attach with debuggee_id\n")

	return mcp.NewToolResultText(sb.String()), nil
}

// handleDebugClassrun executes a class via ADT classrun endpoint.
// This runs in ICF dialog context and WILL check external breakpoints.
func (s *Server) handleDebugClassrun(ctx context.Context, className string) (*mcp.CallToolResult, error) {
	result, err := s.adtClient.RunClassrun(ctx, className)
	if err != nil {
		return wrapErr("RunClassrun", err), nil
	}

	if !result.Success {
		return newToolResultError(fmt.Sprintf("Classrun failed: %s", result.Message)), nil
	}

	var sb strings.Builder
	sb.WriteString("Class executed via ADT classrun endpoint.\n\n")
	fmt.Fprintf(&sb, "Class: %s\n", result.ClassName)
	fmt.Fprintf(&sb, "Status: HTTP %d\n\n", result.StatusCode)

	if result.Output != "" {
		sb.WriteString("Output:\n")
		sb.WriteString(result.Output)
		sb.WriteString("\n")
	}

	sb.WriteString("\nThis ran in ICF dialog context - external breakpoints SHOULD have been checked.\n")
	sb.WriteString("If breakpoint was hit, use 'debug listen' to catch the debuggee.\n")

	return mcp.NewToolResultText(sb.String()), nil
}

// --- HTTP Breakpoint Handlers (CL_ABAP_DEBUGGER - writes to ABDBG_BPS) ---
// These breakpoints are checked by HTTP-triggered execution (unlike TPDAPI which writes to ABDBG_EXTDBPS).

func (s *Server) handleSetHttpBreakpoint(ctx context.Context, params map[string]any) (*mcp.CallToolResult, error) {
	program, _ := params["program"].(string)
	if program == "" {
		return newToolResultError("program is required for HTTP breakpoint"), nil
	}

	line, _ := params["line"].(float64)
	if line <= 0 {
		return newToolResultError("line is required and must be positive for HTTP breakpoint"), nil
	}

	// Method is required for classes to resolve include name
	method, _ := params["method"].(string)

	// Ensure WebSocket client is connected
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError("SetHttpBreakpoint: WebSocket connect failed: " + err.Error()), nil
	}

	result, err := s.debugWSClient.SetHttpBreakpoint(ctx, program, int(line), method)
	if err != nil {
		return wrapErr("SetHttpBreakpoint", err), nil
	}

	var sb strings.Builder
	sb.WriteString("HTTP breakpoint set via CL_ABAP_DEBUGGER.\n\n")
	fmt.Fprintf(&sb, "Program: %s\n", program)
	if method != "" {
		fmt.Fprintf(&sb, "Method: %s\n", method)
	}
	fmt.Fprintf(&sb, "Line: %.0f\n", line)
	sb.WriteString("Table: ABDBG_BPS (checked by HTTP execution)\n\n")

	if saved, ok := result["saved"].(bool); ok && saved {
		sb.WriteString("✓ Breakpoint saved successfully.\n")
	}

	sb.WriteString("\nThis breakpoint type is checked by HTTP-triggered execution (classrun, REST calls).\n")
	sb.WriteString("Compare with: debug set_breakpoint (TPDAPI → ABDBG_EXTDBPS, Eclipse ADT style)\n")

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleGetHttpBreakpoints(ctx context.Context) (*mcp.CallToolResult, error) {
	// Ensure WebSocket client is connected
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError("GetHttpBreakpoints: WebSocket connect failed: " + err.Error()), nil
	}

	result, err := s.debugWSClient.GetHttpBreakpoints(ctx)
	if err != nil {
		return wrapErr("GetHttpBreakpoints", err), nil
	}

	var sb strings.Builder
	sb.WriteString("HTTP Breakpoints (ABDBG_BPS table):\n\n")

	if count, ok := result["count"].(float64); ok {
		fmt.Fprintf(&sb, "Total: %.0f breakpoint(s)\n\n", count)
	}
	if bps, ok := result["breakpoints"].([]any); ok {
		for i, bp := range bps {
			if bpMap, ok := bp.(map[string]any); ok {
				fmt.Fprintf(&sb, "%d. Program: %v, Line: %v\n", i+1, bpMap["program"], bpMap["line"])
			}
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleDeleteHttpBreakpoints(ctx context.Context) (*mcp.CallToolResult, error) {
	// Ensure WebSocket client is connected
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError("DeleteHttpBreakpoints: WebSocket connect failed: " + err.Error()), nil
	}

	result, err := s.debugWSClient.DeleteHttpBreakpoints(ctx)
	if err != nil {
		return wrapErr("DeleteHttpBreakpoints", err), nil
	}

	var sb strings.Builder
	sb.WriteString("HTTP breakpoints deleted.\n\n")

	if deleted, ok := result["deleted"].(bool); ok && deleted {
		sb.WriteString("✓ All HTTP breakpoints for current user have been removed from ABDBG_BPS.\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}
