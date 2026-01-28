// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_dumps.go contains handlers for runtime errors (short dumps / RABAX).
package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Dumps Routing ---
// Routes for this module:
//   analyze: type=dumps (list), type=dump with dump_id in params

// routeDumpsAction routes runtime errors/short dumps actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeDumpsAction(ctx context.Context, action, _, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "analyze" {
		return nil, false, nil
	}

	analysisType, _ := params["type"].(string)
	switch analysisType {
	case "dumps":
		// ListDumps
		args := map[string]any{}
		if user, ok := params["user"].(string); ok {
			args["user"] = user
		}
		if excType, ok := params["exception_type"].(string); ok {
			args["exception_type"] = excType
		}
		if prog, ok := params["program"].(string); ok {
			args["program"] = prog
		}
		if pkg, ok := params["package"].(string); ok {
			args["package"] = pkg
		}
		if dateFrom, ok := params["date_from"].(string); ok {
			args["date_from"] = dateFrom
		}
		if dateTo, ok := params["date_to"].(string); ok {
			args["date_to"] = dateTo
		}
		if maxResults, ok := params["max_results"].(float64); ok {
			args["max_results"] = maxResults
		}
		result, err := s.handleListDumps(ctx, newRequest(args))
		return result, true, err

	case "dump":
		// GetDump - requires dump_id
		dumpID, _ := params["dump_id"].(string)
		if dumpID == "" {
			dumpID = objectName
		}
		if dumpID == "" {
			return newToolResultError("dump_id is required for dump analysis"), true, nil
		}
		result, err := s.handleGetDump(ctx, newRequest(map[string]any{"dump_id": dumpID}))
		return result, true, err
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
		return wrapErr("GetDumps", err), nil
	}

	return newToolResultJSON(dumps), nil
}

func (s *Server) handleGetDump(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dumpID, ok := request.Params.Arguments["dump_id"].(string)
	if !ok || dumpID == "" {
		return newToolResultError("dump_id is required"), nil
	}

	dump, err := s.adtClient.GetDump(ctx, dumpID)
	if err != nil {
		return wrapErr("GetDump", err), nil
	}

	return newToolResultJSON(dump), nil
}
