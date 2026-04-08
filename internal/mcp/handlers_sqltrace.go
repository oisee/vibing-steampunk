// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_sqltrace.go contains handlers for SQL trace (ST05).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// routeSQLTraceAction routes "analyze" with SQL trace types.
func (s *Server) routeSQLTraceAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "analyze" {
		return nil, false, nil
	}
	analysisType := getStringParam(params, "type")
	switch analysisType {
	case "sql_trace_state":
		return s.callHandler(ctx, s.handleGetSQLTraceState, params)
	case "list_sql_traces":
		return s.callHandler(ctx, s.handleListSQLTraces, params)
	}
	return nil, false, nil
}

// --- SQL Trace (ST05) Handlers ---

func (s *Server) handleGetSQLTraceState(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state, err := s.adtClient.GetSQLTraceState(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get SQL trace state: %v", err)), nil
	}

	result, _ := json.MarshalIndent(state, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleListSQLTraces(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	user := ""
	maxResults := 100

	if u, ok := request.GetArguments()["user"].(string); ok {
		user = u
	}
	if max, ok := request.GetArguments()["max_results"].(float64); ok && max > 0 {
		maxResults = int(max)
	}

	traces, err := s.adtClient.ListSQLTraces(ctx, user, maxResults)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to list SQL traces: %v", err)), nil
	}

	result, _ := json.MarshalIndent(traces, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}
