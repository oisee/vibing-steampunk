// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_universal.go implements the single-tool "universal" mode.
// Instead of 122 individual tools, it registers a single SAP(action, target, params) tool.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerUniversalTool registers a single SAP tool that routes to all handlers.
func (s *Server) registerUniversalTool() {
	s.mcpServer.AddTool(mcp.NewTool("SAP",
		mcp.WithDescription(`SAP ABAP development: read/edit/create/test/analyze/debug objects on a live SAP system.

common target types: CLAS, PROG, INTF, FUNC, FUGR, DDLS, TABL, DEVC, BDEF, SRVD
actions: read, edit, create, delete, search, query, grep, test, analyze, debug, system, help
some actions (analyze, test, debug, system, help) use params only — no target needed.

SAP(action="read", target="CLAS ZCL_TEST")  — source + dependency context
SAP(action="read", target="CLAS ZCL_TEST", params={"method": "GET_DATA"})  — one method + context
SAP(action="edit", target="CLAS ZCL_TEST", params={"source": "..."})  — auto lock/activate
SAP(action="edit", target="CLAS ZCL_TEST", params={"method": "X", "source": "METHOD x.\nENDMETHOD."})
SAP(action="search", target="ZCL_*")
SAP(action="analyze", params={"type": "check_boundaries", "package": "$ZDEV"})
SAP(action="help") — full docs; SAP(action="help", target="tips") — best practices`),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Action to perform: read, edit, create, delete, search, query, grep, test, analyze, debug, system, help"),
		),
		mcp.WithString("target",
			mcp.Description("Target object as 'TYPE NAME' (e.g. 'CLAS ZCL_TEST', 'PROG ZREPORT'). Some actions don't need a target."),
		),
		mcp.WithObject("params",
			mcp.Description("Action-specific parameters as a JSON object"),
		),
	), s.handleUniversalTool)
}

// handleUniversalTool dispatches universal SAP(action, target, params) calls to domain-specific route functions.
func (s *Server) handleUniversalTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action, _ := request.GetArguments()["action"].(string)
	if action == "" {
		return newToolResultError("action is required. Use SAP(action=\"help\") for documentation."), nil
	}
	action = strings.ToLower(action)

	target, _ := request.GetArguments()["target"].(string)

	// Extract params as map
	params := getObject(request.GetArguments(), "params")
	if params == nil {
		params = make(map[string]any)
	}

	// Help action
	if action == "help" {
		return handleHelp(target), nil
	}

	// Parse target into type and name
	objectType, objectName := parseTarget(target)

	// Chain through all route functions; return on first match
	type routeFunc func(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error)

	routes := []routeFunc{
		s.routeSourceAction,
		s.routeReadAction,
		s.routeSearchAction,
		s.routeGrepAction,
		s.routeCodeIntelAction,
		s.routeDevToolsAction,
		s.routeATCAction,
		s.routeCRUDAction,
		s.routeClassIncludeAction,
		s.routeWorkflowAction,
		s.routeFileIOAction,
		s.routeDebuggerAction,
		s.routeDebuggerLegacyAction,
		s.routeAMDPAction,
		s.routeUI5Action,
		s.routeTransportAction,
		s.routeGitAction,
		s.routeReportAction,
		s.routeInstallAction,
		s.routeSystemAction,
		s.routeDumpsAction,
		s.routeTracesAction,
		s.routeSQLTraceAction,
		s.routeAnalysisAction,
		s.routeContextAction,
		s.routeServiceBindingAction,
	}

	for _, route := range routes {
		result, handled, err := route(ctx, action, objectType, objectName, params)
		if handled {
			if err != nil {
				return wrapErr(action, err), nil
			}
			return result, nil
		}
	}

	// Nothing matched
	return newToolResultError(getUnhandledErrorMessage(action, objectType, objectName)), nil
}

// parseTarget splits "TYPE NAME" into objectType and objectName, uppercasing both.
func parseTarget(target string) (objectType, objectName string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", ""
	}
	parts := strings.SplitN(target, " ", 2)
	objectType = strings.ToUpper(strings.TrimSpace(parts[0]))
	if len(parts) > 1 {
		objectName = strings.ToUpper(strings.TrimSpace(parts[1]))
	}
	return
}

// getObject extracts a nested object (map[string]any) from args.
func getObject(args map[string]any, key string) map[string]any {
	if v, ok := args[key]; ok {
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}
	return nil
}

// getStringParam extracts a string value from a map.
func getStringParam(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getFloatParam extracts a float64 value from a map.
func getFloatParam(args map[string]any, key string) (float64, bool) {
	if v, ok := args[key]; ok {
		if f, ok := v.(float64); ok {
			return f, true
		}
	}
	return 0, false
}

// getBoolParam extracts a bool value from a map.
func getBoolParam(args map[string]any, key string) (bool, bool) {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b, true
		}
	}
	return false, false
}

// newRequest creates an mcp.CallToolRequest with the given arguments map.
func newRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

// wrapErr wraps an error into a tool result.
func wrapErr(op string, err error) *mcp.CallToolResult {
	return newToolResultError(fmt.Sprintf("%s failed: %v", op, err))
}

// newToolResultJSON marshals a value to JSON and returns it as a tool result.
func newToolResultJSON(v any) *mcp.CallToolResult {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("JSON marshal error: %v", err))
	}
	return mcp.NewToolResultText(string(data))
}

// handlerFunc is the type signature of all existing handler methods.
type handlerFunc func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)

// callHandler is a convenience function that calls an existing handler with constructed args.
func (s *Server) callHandler(ctx context.Context, handler server.ToolHandlerFunc, args map[string]any) (*mcp.CallToolResult, bool, error) {
	result, err := handler(ctx, newRequest(args))
	return result, true, err
}
