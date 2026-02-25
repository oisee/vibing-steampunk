// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_search.go contains handlers for object search operations.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- Search Handlers ---

func (s *Server) handleSearchObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, ok := request.GetArguments()["query"].(string)
	if !ok || query == "" {
		return newToolResultError("query is required"), nil
	}

	maxResults := 100
	if mr, ok := request.GetArguments()["maxResults"].(float64); ok && mr > 0 {
		maxResults = int(mr)
	}

	results, err := s.adtClient.SearchObject(ctx, query, maxResults)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to search: %v", err)), nil
	}

	output, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}
