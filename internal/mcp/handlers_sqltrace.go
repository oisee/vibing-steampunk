// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_sqltrace.go contains handlers for SQL trace (ST05).
package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- SQL Trace Routing ---
// Routes for this module:
//   analyze: type=sql_trace_state, type=sql_traces

// routeSQLTraceAction routes SQL trace actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeSQLTraceAction(ctx context.Context, action, _, _ string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "analyze" {
		return nil, false, nil
	}

	analysisType, _ := params["type"].(string)
	switch analysisType {
	case "sql_trace_state":
		result, err := s.handleGetSQLTraceState(ctx, newRequest(nil))
		return result, true, err

	case "sql_traces":
		args := map[string]any{}
		if user, ok := params["user"].(string); ok {
			args["user"] = user
		}
		if maxResults, ok := params["max_results"].(float64); ok {
			args["max_results"] = maxResults
		}
		result, err := s.handleListSQLTraces(ctx, newRequest(args))
		return result, true, err
	}

	return nil, false, nil
}

// --- SQL Trace (ST05) Handlers ---

func (s *Server) handleGetSQLTraceState(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state, err := s.adtClient.GetSQLTraceState(ctx)
	if err != nil {
		return wrapErr("GetSQLTraceState", err), nil
	}

	return newToolResultJSON(state), nil
}

func (s *Server) handleListSQLTraces(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user := ""
	maxResults := 100

	if u, ok := request.Params.Arguments["user"].(string); ok {
		user = u
	}
	if max, ok := request.Params.Arguments["max_results"].(float64); ok && max > 0 {
		maxResults = int(max)
	}

	traces, err := s.adtClient.ListSQLTraces(ctx, user, maxResults)
	if err != nil {
		return wrapErr("ListSQLTraces", err), nil
	}

	return newToolResultJSON(traces), nil
}
