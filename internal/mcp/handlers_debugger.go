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
	kind, _ := request.GetArguments()["kind"].(string)
	if kind == "" {
		kind = "line"
	}

	// Ensure WebSocket client is connected
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError(fmt.Sprintf("Failed to connect to ZADT_VSP WebSocket: %v. Ensure ZADT_VSP is deployed and SAPC/SICF are configured.", err)), nil
	}

	var bpID string
	var err error
	var msg strings.Builder

	switch kind {
	case "line":
		program, ok := request.GetArguments()["program"].(string)
		if !ok || program == "" {
			return newToolResultError("program is required for line breakpoints"), nil
		}

		lineFloat, ok := request.GetArguments()["line"].(float64)
		if !ok || lineFloat <= 0 {
			return newToolResultError("line is required and must be positive for line breakpoints"), nil
		}
		line := int(lineFloat)

		// Optional method parameter for include-relative line numbers
		method, _ := request.GetArguments()["method"].(string)

		// Auto-convert class names to pool format (ZCL_TEST → ZCL_TEST================CP)
		originalProgram := program
		program = convertToClassPool(program)

		// Use method-aware breakpoint if method is specified
		if method != "" {
			bpID, err = s.debugWSClient.SetMethodBreakpoint(ctx, program, method, line)
			if err != nil {
				return newToolResultError(fmt.Sprintf("SetMethodBreakpoint failed: %v", err)), nil
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
				return newToolResultError(fmt.Sprintf("SetLineBreakpoint failed: %v", err)), nil
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
		statement, ok := request.GetArguments()["statement"].(string)
		if !ok || statement == "" {
			return newToolResultError("statement is required for statement breakpoints (e.g., 'CALL FUNCTION', 'SELECT', 'LOOP')"), nil
		}

		bpID, err = s.debugWSClient.SetStatementBreakpoint(ctx, statement)
		if err != nil {
			return newToolResultError(fmt.Sprintf("SetStatementBreakpoint failed: %v", err)), nil
		}

		msg.WriteString("Statement breakpoint set successfully!\n\n")
		fmt.Fprintf(&msg, "Breakpoint ID: %s\n", bpID)
		fmt.Fprintf(&msg, "Statement: %s\n", statement)
		msg.WriteString("\nThis breakpoint will trigger on ALL occurrences of this statement type.\n")

	case "exception":
		exception, ok := request.GetArguments()["exception"].(string)
		if !ok || exception == "" {
			return newToolResultError("exception is required for exception breakpoints (e.g., 'CX_SY_ZERODIVIDE')"), nil
		}

		bpID, err = s.debugWSClient.SetExceptionBreakpoint(ctx, exception)
		if err != nil {
			return newToolResultError(fmt.Sprintf("SetExceptionBreakpoint failed: %v", err)), nil
		}

		msg.WriteString("Exception breakpoint set successfully!\n\n")
		fmt.Fprintf(&msg, "Breakpoint ID: %s\n", bpID)
		fmt.Fprintf(&msg, "Exception: %s\n", exception)
		msg.WriteString("\nThis breakpoint will trigger when this exception is raised.\n")

	default:
		return newToolResultError(fmt.Sprintf("Invalid breakpoint kind: %s. Valid kinds: line, statement, exception", kind)), nil
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
		return newToolResultError(fmt.Sprintf("Failed to connect to ZADT_VSP WebSocket: %v", err)), nil
	}

	breakpoints, err := s.debugWSClient.GetBreakpoints(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetBreakpoints failed: %v", err)), nil
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
	bpID, ok := request.GetArguments()["breakpoint_id"].(string)
	if !ok || bpID == "" {
		return newToolResultError("breakpoint_id is required"), nil
	}

	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError(fmt.Sprintf("Failed to connect to ZADT_VSP WebSocket: %v", err)), nil
	}

	if err := s.debugWSClient.DeleteBreakpoint(ctx, bpID); err != nil {
		return newToolResultError(fmt.Sprintf("DeleteBreakpoint failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Breakpoint %s deleted successfully.", bpID)), nil
}

func (s *Server) handleCallRFC(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	function, ok := request.GetArguments()["function"].(string)
	if !ok || function == "" {
		return newToolResultError("function is required"), nil
	}

	// Parse params if provided
	params := make(map[string]string)
	if paramsStr, ok := request.GetArguments()["params"].(string); ok && paramsStr != "" {
		// Parse JSON params
		var rawParams map[string]interface{}
		if err := json.Unmarshal([]byte(paramsStr), &rawParams); err != nil {
			return newToolResultError(fmt.Sprintf("Invalid params JSON: %v", err)), nil
		}
		for k, v := range rawParams {
			params[k] = fmt.Sprintf("%v", v)
		}
	}

	// Ensure WebSocket client is connected
	if err := s.ensureDebugWSClient(ctx); err != nil {
		return newToolResultError(fmt.Sprintf("Failed to connect to ZADT_VSP WebSocket: %v. Ensure ZADT_VSP is deployed and SAPC/SICF are configured.", err)), nil
	}

	result, err := s.debugWSClient.CallRFC(ctx, function, params)
	if err != nil {
		return newToolResultError(fmt.Sprintf("CallRFC failed: %v", err)), nil
	}

	// Format result
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(fmt.Sprintf("RFC call completed.\n\nFunction: %s\nSubrc: %d\n\nResult:\n%s", function, result.Subrc, string(resultJSON))), nil
}
