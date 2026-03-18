// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_sqltrace.go contains handlers for SQL trace (ST05).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

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
