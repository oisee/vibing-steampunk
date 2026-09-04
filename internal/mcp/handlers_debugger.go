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
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// routeDebuggerAction routes "debug" sub-actions for the WebSocket-based debugger.
func (s *Server) routeDebuggerAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "debug" {
		return nil, false, nil
	}
	switch objectType {
	case "SET_BREAKPOINT":
		return s.callHandler(ctx, s.handleSetBreakpoint, params)
	case "GET_BREAKPOINTS":
		return s.callHandler(ctx, s.handleGetBreakpoints, params)
	case "DELETE_BREAKPOINT":
		return s.callHandler(ctx, s.handleDeleteBreakpoint, params)
	case "CALL_RFC":
		return s.callHandler(ctx, s.handleCallRFC, params)
	case "MOVE":
		return s.callHandler(ctx, s.handleMoveObject, params)
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
	s.applyWSAuth(s.debugWSClient.SetCookies)
	if s.config.ClientCertProvider != nil {
		s.debugWSClient.SetClientCertProvider(s.config.ClientCertProvider)
	} else if s.config.ClientCert != nil {
		// cert mode: without this the dial falls back to basic auth with an
		// empty password and the bridge 401s (same wiring as the AMDP client)
		s.debugWSClient.SetClientCert(s.config.ClientCert)
	}

	return s.debugWSClient.Connect(ctx)
}

// The breakpoint half of the debugger, through SAP's own ADT resource on the
// session the server holds. It replaced two earlier routes: the stateless REST
// client, which answered 403 because a CSRF token cannot survive a session it
// does not have, and the ZADT_VSP WebSocket, which existed to supply the session
// the stateless client lacked. Neither is needed once the server holds one.
//
// A line is the line in the source as ADT serves it — the same numbering vsp's
// read tools show — so the old method-relative 'method' parameter is gone; name
// the object and give the line you can see.

func (s *Server) handleSetBreakpoint(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sess, err := s.debugger(ctx)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}

	args := request.GetArguments()
	kind, _ := args["kind"].(string)
	if kind == "" {
		kind = "line"
	}

	var bp adt.Breakpoint
	switch kind {
	case "line":
		program, _ := args["program"].(string)
		if program == "" {
			return newToolResultError("program is required for line breakpoints"), nil
		}
		lineFloat, ok := args["line"].(float64)
		if !ok || lineFloat <= 0 {
			return newToolResultError("line is required and must be positive for line breakpoints"), nil
		}
		uri, err := sess.dbg.ResolveSourceURI(ctx, program)
		if err != nil {
			return newToolResultError(err.Error()), nil
		}
		condition, _ := args["condition"].(string)
		bp = adt.Breakpoint{Kind: adt.BreakpointKindLine, URI: uri, Line: int(lineFloat), Condition: condition}

	case "statement":
		statement, _ := args["statement"].(string)
		if statement == "" {
			return newToolResultError("statement is required for statement breakpoints (e.g., 'CALL FUNCTION', 'SELECT', 'LOOP')"), nil
		}
		bp = adt.Breakpoint{Kind: adt.BreakpointKindStatement, Statement: statement}

	case "exception":
		exception, _ := args["exception"].(string)
		if exception == "" {
			return newToolResultError("exception is required for exception breakpoints (e.g., 'CX_SY_ZERODIVIDE')"), nil
		}
		bp = adt.Breakpoint{Kind: adt.BreakpointKindException, Exception: exception}

	default:
		return newToolResultError(fmt.Sprintf("Invalid breakpoint kind: %s. Valid kinds: line, statement, exception", kind)), nil
	}

	bps, err := sess.dbg.ADTAdd(ctx, bp)
	if err != nil {
		return newToolResultError(fmt.Sprintf("SetBreakpoint failed over %s: %v", sess.route, err)), nil
	}

	var msg strings.Builder
	fmt.Fprintf(&msg, "Breakpoint set. This client now has %d registered:\n\n", len(bps))
	msg.WriteString(saprfc.FormatBreakpoints(bps))
	msg.WriteString("\nBreakpoints only fire for code running in ANOTHER session. Start DebuggerListen, " +
		"then trigger the code from SAP GUI, an HTTP request or a separate RFC call.")
	return mcp.NewToolResultText(msg.String()), nil
}

func (s *Server) handleGetBreakpoints(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sess, err := s.debugger(ctx)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	bps, err := sess.dbg.ADTBreakpoints(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetBreakpoints failed: %v", err)), nil
	}
	// SAP answers the breakpoint GET with an empty body, so this is the set this
	// server registered, not everything the user has anywhere. Saying so is the
	// difference between a useful answer and a misleading one.
	return mcp.NewToolResultText(saprfc.FormatBreakpoints(bps) +
		"\n(The set this session registered. ADT does not report breakpoints made by other clients.)"), nil
}

func (s *Server) handleDeleteBreakpoint(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	bpID, _ := request.GetArguments()["breakpoint_id"].(string)
	if bpID == "" {
		return newToolResultError("breakpoint_id is required (or 'all')"), nil
	}
	sess, err := s.debugger(ctx)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	if strings.EqualFold(bpID, "all") {
		if err := sess.dbg.ADTClearBreakpoints(ctx); err != nil {
			return newToolResultError(fmt.Sprintf("DeleteBreakpoint failed: %v", err)), nil
		}
		return mcp.NewToolResultText("All breakpoints registered by this session were removed."), nil
	}
	bps, err := sess.dbg.ADTDropBreakpoint(ctx, bpID)
	if err != nil {
		return newToolResultError(fmt.Sprintf("DeleteBreakpoint failed: %v", err)), nil
	}
	return mcp.NewToolResultText("Breakpoint removed. Remaining:\n\n" + saprfc.FormatBreakpoints(bps)), nil
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
