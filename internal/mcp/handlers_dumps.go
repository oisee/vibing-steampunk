// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_dumps.go contains handlers for runtime errors (short dumps / RABAX).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Runtime Errors / Short Dumps Handlers ---

func (s *Server) handleListDumps(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	opts := &adt.DumpQueryOptions{
		MaxResults: 100,
	}

	if user, ok := request.GetArguments()["user"].(string); ok && user != "" {
		opts.User = user
	}
	if excType, ok := request.GetArguments()["exception_type"].(string); ok && excType != "" {
		opts.ExceptionType = excType
	}
	if prog, ok := request.GetArguments()["program"].(string); ok && prog != "" {
		opts.Program = prog
	}
	if pkg, ok := request.GetArguments()["package"].(string); ok && pkg != "" {
		opts.Package = pkg
	}
	if dateFrom, ok := request.GetArguments()["date_from"].(string); ok && dateFrom != "" {
		opts.DateFrom = dateFrom
	}
	if dateTo, ok := request.GetArguments()["date_to"].(string); ok && dateTo != "" {
		opts.DateTo = dateTo
	}
	if max, ok := request.GetArguments()["max_results"].(float64); ok && max > 0 {
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
	dumpID, ok := request.GetArguments()["dump_id"].(string)
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
