// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_search.go contains handlers for object search operations.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// routeSearchAction routes "search" action.
func (s *Server) routeSearchAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "search" {
		return nil, false, nil
	}
	// Target is the query string; could be "TYPE NAME" or just a query
	query := objectType
	if objectName != "" {
		query = objectType + " " + objectName
	}
	if query == "" {
		query = getStringParam(params, "query")
	}
	if query == "" {
		return nil, false, nil
	}
	args := map[string]any{"query": query}
	if v, ok := getFloatParam(params, "maxResults"); ok {
		args["maxResults"] = v
	}
	if v, ok := getFloatParam(params, "max_results"); ok {
		args["maxResults"] = v
	}
	if v, ok := getFloatParam(params, "max"); ok {
		args["maxResults"] = v
	}
	// Pass object type for server-side filtering so max applies after the
	// type filter (mirrors the CLI --type path).
	if v := getStringParam(params, "type"); v != "" {
		args["objectType"] = v
	}
	if v := getStringParam(params, "objectType"); v != "" {
		args["objectType"] = v
	}
	return s.callHandler(ctx, s.handleSearchObject, args)
}

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

	objectType, _ := request.GetArguments()["objectType"].(string)

	results, err := s.adtClient.SearchObjectByType(ctx, query, objectType, maxResults)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to search: %v", err)), nil
	}

	output, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}
