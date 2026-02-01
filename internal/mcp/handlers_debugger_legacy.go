// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_debugger_legacy.go contains handlers for legacy REST-based debugging.
// These use REST API which works for Listen/Attach/Step but not for breakpoints.
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Legacy Debugger Routing ---
// Routes for this module:
//   debug: listen, attach, detach, step, get_stack, get_variables

// routeDebuggerLegacyAction routes legacy REST-based debugger operations.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeDebuggerLegacyAction(ctx context.Context, action, objectType, _ string, params map[string]any) (*mcp.CallToolResult, bool, error) {
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
	case "listen":
		args := map[string]any{}
		if user, ok := params["user"].(string); ok {
			args["user"] = user
		}
		if timeout, ok := params["timeout"].(float64); ok {
			args["timeout"] = timeout
		}
		result, err := s.handleDebuggerListen(ctx, newRequest(args))
		return result, true, err

	case "attach":
		debuggeeID, _ := params["debuggee_id"].(string)
		if debuggeeID == "" {
			return newToolResultError("debuggee_id is required in params"), true, nil
		}
		args := map[string]any{"debuggee_id": debuggeeID}
		if user, ok := params["user"].(string); ok {
			args["user"] = user
		}
		result, err := s.handleDebuggerAttach(ctx, newRequest(args))
		return result, true, err

	case "detach":
		result, err := s.handleDebuggerDetach(ctx, newRequest(nil))
		return result, true, err

	case "step":
		stepType, _ := params["step_type"].(string)
		if stepType == "" {
			return newToolResultError("step_type is required in params (stepInto, stepOver, stepReturn, stepContinue)"), true, nil
		}
		args := map[string]any{"step_type": stepType}
		if uri, ok := params["uri"].(string); ok {
			args["uri"] = uri
		}
		result, err := s.handleDebuggerStep(ctx, newRequest(args))
		return result, true, err

	case "get_stack":
		result, err := s.handleDebuggerGetStack(ctx, newRequest(nil))
		return result, true, err

	case "get_variables":
		args := map[string]any{}
		if varIDs, ok := params["variable_ids"].([]any); ok {
			args["variable_ids"] = varIDs
		}
		result, err := s.handleDebuggerGetVariables(ctx, newRequest(args))
		return result, true, err
	}

	return nil, false, nil
}

// --- Legacy REST-based Debugger Handlers (fallback) ---

func (s *Server) handleDebuggerListen(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user, _ := request.Params.Arguments["user"].(string)
	if user == "" {
		user = s.config.Username // Default to connection user
	}
	timeout := 60 // default
	if t, ok := request.Params.Arguments["timeout"].(float64); ok && t > 0 {
		timeout = int(t)
		if timeout > 240 {
			timeout = 240 // max 240 seconds
		}
	}

	result, err := s.adtClient.DebuggerListen(ctx, &adt.ListenOptions{
		DebuggingMode:  adt.DebuggingModeUser,
		User:           user,
		TimeoutSeconds: timeout,
	})
	if err != nil {
		return wrapErr("DebuggerListen", err), nil
	}

	if result.TimedOut {
		return mcp.NewToolResultText("Listener timed out - no debuggee hit a breakpoint within the timeout period."), nil
	}

	if result.Conflict != nil {
		return mcp.NewToolResultText(fmt.Sprintf("Listener conflict detected: %s (user: %s)",
			result.Conflict.ConflictText, result.Conflict.IdeUser)), nil
	}

	if result.Debuggee != nil {
		var sb strings.Builder
		sb.WriteString("Debuggee caught!\n\n")
		fmt.Fprintf(&sb, "Debuggee ID: %s\n", result.Debuggee.ID)
		fmt.Fprintf(&sb, "User: %s\n", result.Debuggee.User)
		fmt.Fprintf(&sb, "Program: %s\n", result.Debuggee.Program)
		fmt.Fprintf(&sb, "Include: %s\n", result.Debuggee.Include)
		fmt.Fprintf(&sb, "Line: %d\n", result.Debuggee.Line)
		fmt.Fprintf(&sb, "Kind: %s\n", result.Debuggee.Kind)
		fmt.Fprintf(&sb, "Attachable: %v\n", result.Debuggee.IsAttachable)
		fmt.Fprintf(&sb, "App Server: %s\n", result.Debuggee.AppServer)
		sb.WriteString("\nUse DebuggerAttach with the debuggee_id to attach to this session.")
		return mcp.NewToolResultText(sb.String()), nil
	}

	return mcp.NewToolResultText("Listener returned with no result."), nil
}

func (s *Server) handleDebuggerAttach(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	debuggeeID, ok := request.Params.Arguments["debuggee_id"].(string)
	if !ok || debuggeeID == "" {
		return newToolResultError("debuggee_id is required"), nil
	}

	user, _ := request.Params.Arguments["user"].(string)
	if user == "" {
		user = s.config.Username // Default to connection user
	}

	result, err := s.adtClient.DebuggerAttach(ctx, debuggeeID, user)
	if err != nil {
		return wrapErr("DebuggerAttach", err), nil
	}

	var sb strings.Builder
	sb.WriteString("Successfully attached to debuggee!\n\n")
	fmt.Fprintf(&sb, "Debug Session ID: %s\n", result.DebugSessionID)
	fmt.Fprintf(&sb, "Process ID: %d\n", result.ProcessID)
	fmt.Fprintf(&sb, "Server: %s\n", result.ServerName)
	fmt.Fprintf(&sb, "Stepping Possible: %v\n", result.IsSteppingPossible)
	fmt.Fprintf(&sb, "Termination Possible: %v\n", result.IsTerminationPossible)

	if len(result.ReachedBreakpoints) > 0 {
		sb.WriteString("\nReached Breakpoints:\n")
		for _, bp := range result.ReachedBreakpoints {
			fmt.Fprintf(&sb, "  - ID: %s (kind: %s)\n", bp.ID, bp.Kind)
		}
	}

	if len(result.Actions) > 0 {
		sb.WriteString("\nAvailable Actions:\n")
		for _, action := range result.Actions {
			fmt.Fprintf(&sb, "  - %s: %s\n", action.Name, action.Title)
		}
	}

	sb.WriteString("\nUse DebuggerGetStack to see the call stack, DebuggerGetVariables to inspect variables.")
	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleDebuggerDetach(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	err := s.adtClient.DebuggerDetach(ctx)
	if err != nil {
		return wrapErr("DebuggerDetach", err), nil
	}

	return mcp.NewToolResultText("Successfully detached from debug session."), nil
}

func (s *Server) handleDebuggerStep(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stepTypeStr, ok := request.Params.Arguments["step_type"].(string)
	if !ok || stepTypeStr == "" {
		return newToolResultError("step_type is required"), nil
	}

	// Map string to step type
	var stepType adt.DebugStepType
	switch stepTypeStr {
	case "stepInto":
		stepType = adt.DebugStepInto
	case "stepOver":
		stepType = adt.DebugStepOver
	case "stepReturn":
		stepType = adt.DebugStepReturn
	case "stepContinue":
		stepType = adt.DebugStepContinue
	case "stepRunToLine":
		stepType = adt.DebugStepRunToLine
	case "stepJumpToLine":
		stepType = adt.DebugStepJumpToLine
	default:
		return newToolResultError("Invalid step_type: " + stepTypeStr + ". Valid values: stepInto, stepOver, stepReturn, stepContinue, stepRunToLine, stepJumpToLine"), nil
	}

	uri, _ := request.Params.Arguments["uri"].(string)

	result, err := s.adtClient.DebuggerStep(ctx, stepType, uri)
	if err != nil {
		return wrapErr("DebuggerStep", err), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Step '%s' executed.\n\n", stepTypeStr)
	fmt.Fprintf(&sb, "Session: %s\n", result.DebugSessionID)
	fmt.Fprintf(&sb, "Debuggee Changed: %v\n", result.IsDebuggeeChanged)
	fmt.Fprintf(&sb, "Stepping Possible: %v\n", result.IsSteppingPossible)

	if len(result.ReachedBreakpoints) > 0 {
		sb.WriteString("\nReached Breakpoints:\n")
		for _, bp := range result.ReachedBreakpoints {
			fmt.Fprintf(&sb, "  - ID: %s (kind: %s)\n", bp.ID, bp.Kind)
		}
	}

	sb.WriteString("\nUse DebuggerGetStack to see current position, DebuggerGetVariables to inspect variables.")
	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleDebuggerGetStack(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := s.adtClient.DebuggerGetStack(ctx, true)
	if err != nil {
		return wrapErr("DebuggerGetStack", err), nil
	}

	var sb strings.Builder
	sb.WriteString("Call Stack:\n\n")
	fmt.Fprintf(&sb, "Server: %s\n", result.ServerName)
	fmt.Fprintf(&sb, "Current Stack Index: %d\n\n", result.DebugCursorStackIndex)

	for i, entry := range result.Stack {
		marker := "  "
		if entry.StackPosition == result.DebugCursorStackIndex {
			marker = "→ "
		}
		fmt.Fprintf(&sb, "%s[%d] %s::%s (line %d)\n",
			marker, entry.StackPosition, entry.ProgramName, entry.EventName, entry.Line)
		fmt.Fprintf(&sb, "      Type: %s, Include: %s\n", entry.EventType, entry.IncludeName)
		if entry.SystemProgram {
			sb.WriteString("      (system program)\n")
		}
		if i < len(result.Stack)-1 {
			sb.WriteString("\n")
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleDebuggerGetVariables(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Parse variable_ids from request
	var variableIDs []string

	if ids, ok := request.Params.Arguments["variable_ids"].([]any); ok {
		for _, id := range ids {
			if s, ok := id.(string); ok {
				variableIDs = append(variableIDs, s)
			}
		}
	}

	// Default to @ROOT if no IDs specified
	if len(variableIDs) == 0 {
		variableIDs = []string{"@ROOT"}
	}

	// If @ROOT is requested, use GetChildVariables for top-level vars
	if len(variableIDs) == 1 && variableIDs[0] == "@ROOT" {
		result, err := s.adtClient.DebuggerGetChildVariables(ctx, []string{"@ROOT", "@DATAAGING"})
		if err != nil {
			return wrapErr("DebuggerGetVariables", err), nil
		}

		var sb strings.Builder
		sb.WriteString("Variables:\n\n")

		for _, v := range result.Variables {
			fmt.Fprintf(&sb, "%s: %s = %s\n", v.Name, v.DeclaredTypeName, v.Value)
			fmt.Fprintf(&sb, "  MetaType: %s, Kind: %s\n", v.MetaType, v.Kind)
			if v.IsComplexType() {
				fmt.Fprintf(&sb, "  (complex type - use variable ID '%s' to expand)\n", v.ID)
			}
		}

		return mcp.NewToolResultText(sb.String()), nil
	}

	// Get specific variables
	result, err := s.adtClient.DebuggerGetVariables(ctx, variableIDs)
	if err != nil {
		return wrapErr("DebuggerGetVariables", err), nil
	}

	var sb strings.Builder
	sb.WriteString("Variables:\n\n")

	for _, v := range result {
		fmt.Fprintf(&sb, "%s: %s = %s\n", v.Name, v.DeclaredTypeName, v.Value)
		fmt.Fprintf(&sb, "  ID: %s\n", v.ID)
		fmt.Fprintf(&sb, "  MetaType: %s, Kind: %s\n", v.MetaType, v.Kind)
		if v.HexValue != "" {
			fmt.Fprintf(&sb, "  Hex: %s\n", v.HexValue)
		}
		if v.TableLines > 0 {
			fmt.Fprintf(&sb, "  Table Lines: %d\n", v.TableLines)
		}
		if v.IsComplexType() {
			sb.WriteString("  (complex type - expandable)\n")
		}
		sb.WriteString("\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}
