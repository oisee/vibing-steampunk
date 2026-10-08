// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_debugger_legacy.go drives the ABAP debugger through SAP's own ADT
// resources on the session the server holds (handlers_debug_session.go).
//
// It used to drive them through the stateless ADT client, which is why these
// tools shipped disabled: listen and attach each got their own session, so the
// attach never found the debuggee the listen had caught. Nothing about the
// resources was wrong — only the session under them.
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// routeDebuggerLegacyAction routes "debug" sub-actions for the session debugger.
func (s *Server) routeDebuggerLegacyAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "debug" {
		return nil, false, nil
	}
	switch objectType {
	case "LISTEN":
		return s.callHandler(ctx, s.handleDebuggerListen, params)
	case "ATTACH":
		return s.callHandler(ctx, s.handleDebuggerAttach, params)
	case "DETACH":
		return s.callHandler(ctx, s.handleDebuggerDetach, params)
	case "STEP":
		return s.callHandler(ctx, s.handleDebuggerStep, params)
	case "GET_STACK":
		return s.callHandler(ctx, s.handleDebuggerGetStack, params)
	case "GET_VARIABLES":
		return s.callHandler(ctx, s.handleDebuggerGetVariables, params)
	}
	return nil, false, nil
}

// handleDebuggerListen waits for a debuggee and attaches to it in one call.
//
// Listen and attach are one tool rather than two because the debuggee is only
// attachable while it waits: a caller that reads an id out of one result and
// passes it to the next call loses that race often enough to be useless.
// DebuggerAttach remains for the case where a caller has an id already.
func (s *Server) handleDebuggerListen(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sess, err := s.debugger(ctx)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	user, _ := request.GetArguments()["user"].(string)
	if user == "" {
		user = sess.user
	}
	timeout := 60
	if t, ok := request.GetArguments()["timeout"].(float64); ok && t > 0 {
		timeout = int(t)
		if timeout > 240 {
			timeout = 240
		}
	}

	who, _, err := sess.dbg.ADTCatch(ctx, user, saprfc.IDEID, saprfc.TerminalID, timeout)
	if err != nil {
		return newToolResultError(fmt.Sprintf("DebuggerListen failed over %s: %v", sess.route, err)), nil
	}
	if who == nil {
		return mcp.NewToolResultText(fmt.Sprintf(
			"No debuggee stopped within %ds. Breakpoints only fire for code running in another session — "+
				"trigger the code from SAP GUI, an HTTP request or a separate RFC call while this listener waits.", timeout)), nil
	}

	info, err := sess.dbg.StackInfo(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("attached to %s but could not read the stack: %v", who.ID, err)), nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Attached to %s (%s) over %s.\n\n", who.ID, who.User, sess.route)
	fmt.Fprintf(&sb, "Stopped in %s %s at %s/%s:%d\n\n", who.Type, who.Name, who.Program, who.Include, who.Line)
	sb.WriteString(saprfc.FormatStack(info))
	sb.WriteString("\nDebuggerGetVariables reads the locals; DebuggerStep moves; DebuggerDetach releases the debuggee.")
	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleDebuggerAttach(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	debuggeeID, ok := request.GetArguments()["debuggee_id"].(string)
	if !ok || debuggeeID == "" {
		return newToolResultError("debuggee_id is required"), nil
	}
	sess, err := s.debugger(ctx)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	user, _ := request.GetArguments()["user"].(string)
	if user == "" {
		user = sess.user
	}

	if _, err := sess.dbg.ADTAttach(ctx, debuggeeID, user); err != nil {
		return newToolResultError(fmt.Sprintf("DebuggerAttach failed: %v", err)), nil
	}
	info, err := sess.dbg.StackInfo(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("attached but could not read the stack: %v", err)), nil
	}
	return mcp.NewToolResultText("Attached to " + debuggeeID + ".\n\n" + saprfc.FormatStack(info)), nil
}

func (s *Server) handleDebuggerDetach(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.debugMu.Lock()
	open := s.debugSess != nil
	s.debugMu.Unlock()
	if !open {
		return mcp.NewToolResultText("No debug session is open."), nil
	}
	s.closeDebugSession(ctx)
	return mcp.NewToolResultText("Debuggee released and the debug session closed."), nil
}

func (s *Server) handleDebuggerStep(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	stepType, _ := request.GetArguments()["step_type"].(string)
	switch stepType {
	case "stepInto", "stepOver", "stepReturn", "stepContinue", "stepRunToLine", "stepJumpToLine", "terminateDebuggee":
	case "":
		return newToolResultError("step_type is required"), nil
	default:
		return newToolResultError(fmt.Sprintf("Invalid step_type: %s. Valid values: stepInto, stepOver, stepReturn, stepContinue, stepRunToLine, stepJumpToLine", stepType)), nil
	}

	// The two steps that go somewhere need to be told where. Without the target
	// SAP refuses with "Parameter uri could not be found", which reads like a
	// server fault rather than a missing argument, so say it here instead.
	uri, _ := request.GetArguments()["uri"].(string)
	uri = strings.TrimSpace(uri)
	if (stepType == "stepRunToLine" || stepType == "stepJumpToLine") && uri == "" {
		return newToolResultError(stepType + " needs uri: the target line's source URI, e.g. /sap/bc/adt/oo/classes/zcl_demo/source/main#start=42"), nil
	}

	sess, err := s.debugger(ctx)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	if _, err := sess.dbg.ADTStepTo(ctx, stepType, uri); err != nil {
		// A continue or terminate that lets the program finish leaves nothing
		// to step: SAP answers that the debuggee ended or that no session is
		// attached. That is the step succeeding, not failing — DetachDebuggee
		// and the DAP server read the same answers the same way.
		if (stepType == "stepContinue" || stepType == "terminateDebuggee") && debuggeeGone(err) {
			return mcp.NewToolResultText(stepType + " executed: the program ran to completion and the debuggee is no longer attached."), nil
		}
		return newToolResultError(fmt.Sprintf("DebuggerStep failed: %v", err)), nil
	}
	// Where it landed is the only part of a step worth reporting.
	info, err := sess.dbg.StackInfo(ctx)
	if err != nil {
		return mcp.NewToolResultText(stepType + " executed; the stack could not be read: " + err.Error()), nil
	}
	return mcp.NewToolResultText(stepType + " executed.\n\n" + saprfc.FormatStack(info)), nil
}

func (s *Server) handleDebuggerGetStack(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sess, err := s.debugger(ctx)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	info, err := sess.dbg.StackInfo(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("DebuggerGetStack failed: %v", err)), nil
	}
	return mcp.NewToolResultText(saprfc.FormatStack(info)), nil
}

// handleDebuggerGetVariables reads the stopped frame's own variables by default,
// and named ones when asked. The walk from @ROOT to the locals is done here
// rather than left to the caller: the ids that address the tree are handed out
// by the level above, so "read the variables" is two calls, not one, and a
// caller should not have to know that.
func (s *Server) handleDebuggerGetVariables(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sess, err := s.debugger(ctx)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}

	var ids []string
	if raw, ok := request.GetArguments()["variable_ids"].([]interface{}); ok {
		for _, v := range raw {
			if str, ok := v.(string); ok && str != "" {
				ids = append(ids, str)
			}
		}
	}

	if len(ids) == 0 || (len(ids) == 1 && strings.EqualFold(ids[0], "@ROOT")) {
		vars, err := sess.dbg.Locals(ctx)
		if err != nil {
			return newToolResultError(fmt.Sprintf("DebuggerGetVariables failed: %v", err)), nil
		}
		return mcp.NewToolResultText("Locals of the stopped frame:\n\n" + saprfc.FormatVariables(vars)), nil
	}

	// One composite id expands to its components; anything else is read by name.
	if len(ids) == 1 && strings.HasPrefix(ids[0], "@") {
		info, err := sess.dbg.Expand(ctx, ids[0])
		if err != nil {
			return newToolResultError(fmt.Sprintf("DebuggerGetVariables failed: %v", err)), nil
		}
		if info == nil {
			return mcp.NewToolResultText("Nothing under " + ids[0] + "."), nil
		}
		return mcp.NewToolResultText(saprfc.FormatVariables(info.Variables)), nil
	}

	vars, err := sess.dbg.Vars(ctx, ids)
	if err != nil {
		return newToolResultError(fmt.Sprintf("DebuggerGetVariables failed: %v", err)), nil
	}
	return mcp.NewToolResultText(saprfc.FormatVariables(vars)), nil
}

// debuggeeGone reports whether a step failed only because there is nothing
// left to step: the program ran to its end (debuggeeEnded) or the session has
// no debuggee any more (noSessionAttached). SAP carries both in the exception
// it answers with, on the RFC route and over HTTPS alike.
func debuggeeGone(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "debuggeeEnded") || strings.Contains(msg, "noSessionAttached")
}
