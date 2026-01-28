// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_amdp.go contains handlers for AMDP (HANA) SQLScript debugging.
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- AMDP Routing ---
// Routes for this module:
//   debug: amdp_start, amdp_stop, amdp_resume, amdp_step, amdp_variables, amdp_breakpoint, amdp_breakpoints

// routeAMDPAction routes AMDP debugger actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeAMDPAction(ctx context.Context, action, _, _ string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "debug" {
		return nil, false, nil
	}

	debugType, _ := params["type"].(string)
	switch debugType {
	case "amdp_start":
		args := map[string]any{}
		if cascadeMode, ok := params["cascade_mode"].(string); ok {
			args["cascade_mode"] = cascadeMode
		}
		result, err := s.handleAMDPDebuggerStart(ctx, newRequest(args))
		return result, true, err

	case "amdp_stop":
		result, err := s.handleAMDPDebuggerStop(ctx, newRequest(nil))
		return result, true, err

	case "amdp_resume":
		result, err := s.handleAMDPDebuggerResume(ctx, newRequest(nil))
		return result, true, err

	case "amdp_step":
		stepType, _ := params["step_type"].(string)
		if stepType == "" {
			return newToolResultError("amdp_step requires 'step_type' in params"), true, nil
		}
		result, err := s.handleAMDPDebuggerStep(ctx, newRequest(map[string]any{"step_type": stepType}))
		return result, true, err

	case "amdp_variables":
		result, err := s.handleAMDPGetVariables(ctx, newRequest(nil))
		return result, true, err

	case "amdp_breakpoint":
		procName, _ := params["proc_name"].(string)
		line, _ := params["line"].(float64)
		if procName == "" || line == 0 {
			return newToolResultError("amdp_breakpoint requires 'proc_name' and 'line' in params"), true, nil
		}
		result, err := s.handleAMDPSetBreakpoint(ctx, newRequest(map[string]any{"proc_name": procName, "line": line}))
		return result, true, err

	case "amdp_breakpoints":
		result, err := s.handleAMDPGetBreakpoints(ctx, newRequest(nil))
		return result, true, err
	}

	return nil, false, nil
}

// --- AMDP (HANA) Debugger Handlers ---

// requireActiveAMDPSession checks if there's an active AMDP debug session.
// Returns error result if no session, nil if session is active.
func (s *Server) requireActiveAMDPSession() *mcp.CallToolResult {
	if s.amdpWSClient == nil || !s.amdpWSClient.IsActive() {
		return newToolResultError("No active AMDP session. Use AMDPDebuggerStart first.")
	}
	return nil
}

func (s *Server) handleAMDPDebuggerStart(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Check if session already active
	if s.amdpWSClient != nil {
		if s.amdpWSClient.IsActive() {
			return newToolResultError("AMDP session already active. Use AMDPDebuggerStop first."), nil
		}
		// Close orphaned connection (connected but not active)
		s.amdpWSClient.Close()
	}

	// Create WebSocket-based AMDP client (connects to ZADT_VSP)
	s.amdpWSClient = adt.NewAMDPWebSocketClient(
		s.config.BaseURL,
		s.config.Client,
		s.config.Username,
		s.config.Password,
		s.config.InsecureSkipVerify,
	)

	// Connect to ZADT_VSP WebSocket
	if err := s.amdpWSClient.Connect(ctx); err != nil {
		s.amdpWSClient = nil
		return wrapErr("AMDPDebuggerStart/Connect", err), nil
	}

	// Start AMDP debug session
	cascadeMode := "FULL"
	if cm, ok := request.Params.Arguments["cascade_mode"].(string); ok && cm != "" {
		cascadeMode = cm
	}

	if err := s.amdpWSClient.Start(ctx, cascadeMode); err != nil {
		s.amdpWSClient.Close()
		s.amdpWSClient = nil
		return wrapErr("AMDPDebuggerStart", err), nil
	}

	var sb strings.Builder
	sb.WriteString("AMDP Debug Session Started (WebSocket via ZADT_VSP)\n\n")
	fmt.Fprintf(&sb, "Cascade Mode: %s\n", cascadeMode)
	sb.WriteString("\nSession uses WebSocket connection to ZADT_VSP APC handler.")
	sb.WriteString("\nUse AMDPDebuggerStep, AMDPGetVariables to interact.")
	sb.WriteString("\nUse AMDPDebuggerStop to terminate the session.")

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleAMDPDebuggerResume(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if errResult := s.requireActiveAMDPSession(); errResult != nil {
		return errResult, nil
	}

	result, err := s.amdpWSClient.Resume(ctx)
	if err != nil {
		return wrapErr("AMDPDebuggerResume", err), nil
	}

	var sb strings.Builder
	sb.WriteString("AMDP Debugger Resume\n\n")

	if len(result.Events) == 0 {
		sb.WriteString("Waiting for debuggee... (no events yet)\n")
		sb.WriteString("Execute AMDP code to trigger breakpoints.\n")
	} else {
		for i, event := range result.Events {
			fmt.Fprintf(&sb, "Event %d: %s\n", i+1, event.Kind)
			if event.ContextID != "" {
				fmt.Fprintf(&sb, "  Context ID: %s\n", event.ContextID)
			}
			if event.Kind == "on_break" {
				sb.WriteString("\n=== BREAKPOINT HIT ===\n")
				if event.ABAPPosition != nil {
					fmt.Fprintf(&sb, "  ABAP Position: %s/%s line %d\n",
						event.ABAPPosition.Program,
						event.ABAPPosition.Include,
						event.ABAPPosition.Line)
				}
				if event.NativePosition != nil {
					fmt.Fprintf(&sb, "  Native Position: %s.%s line %d\n",
						event.NativePosition.Schema,
						event.NativePosition.Name,
						event.NativePosition.Line)
				}
				fmt.Fprintf(&sb, "  Variable Count: %d\n", event.VariableCount)
				fmt.Fprintf(&sb, "  Stack Depth: %d\n", event.StackDepth)
			}
			if event.Aborted {
				sb.WriteString("  Session was aborted\n")
			}
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleAMDPDebuggerStop(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// WebSocket-based Stop via ZADT_VSP
	if s.amdpWSClient == nil {
		return mcp.NewToolResultText("No AMDP session active."), nil
	}

	// Stop AMDP debug session
	if err := s.amdpWSClient.Stop(ctx); err != nil {
		// Close connection anyway
		s.amdpWSClient.Close()
		s.amdpWSClient = nil
		return wrapErr("AMDPDebuggerStop", err), nil
	}

	// Close WebSocket connection
	s.amdpWSClient.Close()
	s.amdpWSClient = nil

	return mcp.NewToolResultText("AMDP debug session stopped. WebSocket connection closed."), nil
}

func (s *Server) handleAMDPDebuggerStep(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if errResult := s.requireActiveAMDPSession(); errResult != nil {
		return errResult, nil
	}

	stepType, ok := request.Params.Arguments["step_type"].(string)
	if !ok || stepType == "" {
		return newToolResultError("step_type is required"), nil
	}

	// Execute step via WebSocket
	if err := s.amdpWSClient.Step(ctx, stepType); err != nil {
		return wrapErr("AMDPDebuggerStep", err), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Step executed: %s\n\n", stepType)
	sb.WriteString("Use AMDPDebuggerResume to check for breakpoint events.\n")

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleAMDPGetVariables(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if errResult := s.requireActiveAMDPSession(); errResult != nil {
		return errResult, nil
	}

	result, err := s.amdpWSClient.GetVariables(ctx)
	if err != nil {
		return wrapErr("AMDPGetVariables", err), nil
	}

	var sb strings.Builder
	sb.WriteString("AMDP Variables:\n\n")

	if len(result.Variables) == 0 {
		sb.WriteString("No variables available (session may not be at breakpoint)\n")
	} else {
		for _, v := range result.Variables {
			if v.Type == "table" {
				fmt.Fprintf(&sb, "  %s: [TABLE %d rows]\n", v.Name, v.Rows)
			} else {
				fmt.Fprintf(&sb, "  %s = %s (%s)\n", v.Name, v.Value, v.Type)
			}
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleAMDPSetBreakpoint(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if errResult := s.requireActiveAMDPSession(); errResult != nil {
		return errResult, nil
	}

	procName, _ := request.Params.Arguments["proc_name"].(string)
	if procName == "" {
		return newToolResultError("proc_name is required"), nil
	}

	lineFloat, ok := request.Params.Arguments["line"].(float64)
	if !ok {
		return newToolResultError("line is required"), nil
	}
	line := int(lineFloat)

	// Set breakpoint via WebSocket
	if err := s.amdpWSClient.SetBreakpoint(ctx, procName, line); err != nil {
		return wrapErr("AMDPSetBreakpoint", err), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("AMDP breakpoint set at %s line %d", procName, line)), nil
}

func (s *Server) handleAMDPGetBreakpoints(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if errResult := s.requireActiveAMDPSession(); errResult != nil {
		return errResult, nil
	}

	result, err := s.amdpWSClient.GetBreakpoints(ctx)
	if err != nil {
		return wrapErr("AMDPGetBreakpoints", err), nil
	}

	var sb strings.Builder
	sb.WriteString("AMDP Breakpoints:\n\n")

	if len(result.Breakpoints) == 0 {
		sb.WriteString("No breakpoints set.\n")
	} else {
		for i, bp := range result.Breakpoints {
			enabled := "enabled"
			if !bp.Enabled {
				enabled = "disabled"
			}
			fmt.Fprintf(&sb, "  %d. %s line %d (%s)\n", i+1, bp.ObjectName, bp.Line, enabled)
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}
