// Package mcp provides the MCP server implementation for ABAP ADT tools.
package mcp

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// getString extracts a string parameter from a map
func getString(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// newRequest creates a CallToolRequest with the given arguments
func newRequest(args map[string]any) mcp.CallToolRequest {
	var req mcp.CallToolRequest
	req.Params.Arguments = args
	return req
}

// registerUniversalTool registers the single "SAP" tool that routes to all operations.
// This reduces MCP schema overhead from ~14,200 tokens to ~150 tokens.
func (s *Server) registerUniversalTool() {
	s.mcpServer.AddTool(mcp.NewTool("SAP",
		mcp.WithDescription("SAP ABAP development. Common: read/edit CLAS|INTF|PROG <name> (edit needs params.source), search <query>, create OBJECT"),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Operation: read|edit|create|delete|search|grep|debug|query|analyze|test|system"),
		),
		mcp.WithString("target",
			mcp.Description("Target object (e.g., 'CLAS ZCL_TEST', 'PROG ZREPORT', 'TABLE SFLIGHT') or action-specific target"),
		),
		mcp.WithObject("params",
			mcp.Description("Action-specific parameters as JSON object"),
		),
	), s.handleUniversalTool)
}

// handleUniversalTool routes actions to internal handlers.
func (s *Server) handleUniversalTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := getString(request.Params.Arguments, "action")
	target := getString(request.Params.Arguments, "target")
	params := getObject(request.Params.Arguments, "params")

	// Handle help action first
	if action == "help" {
		return handleHelp(target), nil
	}

	// Parse target for sub-routers
	objectType, objectName := parseTarget(target)

	// Try module sub-routers first (each module handles its own routes)
	if result, handled, err := s.routeReportAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeAMDPAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeTransportAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeUI5Action(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeTracesAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeDumpsAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeSQLTraceAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeDebuggerAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeGrepAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeServiceBindingAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeGitAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeFileIOAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeDevToolsAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeATCAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeClassIncludeAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeInstallAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeWorkflowAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeAnalysisAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeCodeIntelAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeReadAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeCRUDAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeSearchAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeDebuggerLegacyAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	if result, handled, err := s.routeSystemAction(ctx, action, objectType, objectName, params); handled {
		return result, err
	}
	// No sub-router handled this request - return error with guidance
	return newToolResultError(getUnhandledErrorMessage(action, objectType, objectName)), nil
}

// parseTarget parses "TYPE NAME" format (e.g., "CLAS ZCL_TEST")
// All parts are uppercased for consistency with SAP conventions.
// Single-word targets become the type with empty name.
func parseTarget(target string) (objectType, objectName string) {
	parts := strings.Fields(target)
	if len(parts) >= 2 {
		return strings.ToUpper(parts[0]), strings.ToUpper(parts[1])
	}
	if len(parts) == 1 {
		return strings.ToUpper(parts[0]), ""
	}
	return "", ""
}

// getObject extracts an object parameter as map[string]any
func getObject(args map[string]any, key string) map[string]any {
	if v, ok := args[key]; ok {
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}
	return make(map[string]any)
}
