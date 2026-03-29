// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_dumps.go contains handlers for runtime errors (short dumps / RABAX).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/vinchacho/vibing-steampunk/pkg/adt"
)

// routeDumpsAction routes "analyze" with dump-related types.
func (s *Server) routeDumpsAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "analyze" {
		return nil, false, nil
	}
	analysisType := getStringParam(params, "type")
	switch analysisType {
	case "list_dumps":
		return s.callHandler(ctx, s.handleListDumps, params)
	case "get_dump":
		return s.callHandler(ctx, s.handleGetDump, params)
	}
	return nil, false, nil
}

// --- Runtime Errors / Short Dumps Handlers ---

func (s *Server) handleListDumps(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	opts := &adt.DumpQueryOptions{
		MaxResults: 100,
	}

	if user, ok := request.Params.Arguments["user"].(string); ok && user != "" {
		opts.User = user
	}
	if excType, ok := request.Params.Arguments["exception_type"].(string); ok && excType != "" {
		opts.ExceptionType = excType
	}
	if prog, ok := request.Params.Arguments["program"].(string); ok && prog != "" {
		opts.Program = prog
	}
	if pkg, ok := request.Params.Arguments["package"].(string); ok && pkg != "" {
		opts.Package = pkg
	}
	if dateFrom, ok := request.Params.Arguments["date_from"].(string); ok && dateFrom != "" {
		opts.DateFrom = dateFrom
	}
	if dateTo, ok := request.Params.Arguments["date_to"].(string); ok && dateTo != "" {
		opts.DateTo = dateTo
	}
	if max, ok := request.Params.Arguments["max_results"].(float64); ok && max > 0 {
		opts.MaxResults = int(max)
	}

	dumps, err := s.adtClient.GetDumps(ctx, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get dumps: %v", err)), nil
	}

	result, _ := json.MarshalIndent(dumps, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetDump(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dumpID, ok := request.Params.Arguments["dump_id"].(string)
	if !ok || dumpID == "" {
		return newToolResultError("dump_id is required"), nil
	}

	dump, err := s.adtClient.GetDump(ctx, dumpID)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get dump: %v", err)), nil
	}

	result, _ := json.MarshalIndent(dump, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}
