// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_traces.go contains handlers for ABAP profiler traces (ATRA).
package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Traces Routing ---
// Routes for this module:
//   analyze: type=traces (list), type=trace with trace_id in params

// routeTracesAction routes ABAP profiler trace actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeTracesAction(ctx context.Context, action, _, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "analyze" {
		return nil, false, nil
	}

	analysisType, _ := params["type"].(string)
	switch analysisType {
	case "traces":
		// ListTraces
		args := map[string]any{}
		if user, ok := params["user"].(string); ok {
			args["user"] = user
		}
		if procType, ok := params["process_type"].(string); ok {
			args["process_type"] = procType
		}
		if objType, ok := params["object_type"].(string); ok {
			args["object_type"] = objType
		}
		if maxResults, ok := params["max_results"].(float64); ok {
			args["max_results"] = maxResults
		}
		result, err := s.handleListTraces(ctx, newRequest(args))
		return result, true, err

	case "trace":
		// GetTrace - requires trace_id
		traceID, _ := params["trace_id"].(string)
		if traceID == "" {
			traceID = objectName
		}
		if traceID == "" {
			return newToolResultError("trace_id is required for trace analysis"), true, nil
		}
		args := map[string]any{"trace_id": traceID}
		if toolType, ok := params["tool_type"].(string); ok {
			args["tool_type"] = toolType
		}
		result, err := s.handleGetTrace(ctx, newRequest(args))
		return result, true, err
	}

	return nil, false, nil
}

// --- ABAP Profiler / Traces Handlers ---

func (s *Server) handleListTraces(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	opts := &adt.TraceQueryOptions{
		MaxResults: 100,
	}

	if user, ok := request.Params.Arguments["user"].(string); ok && user != "" {
		opts.User = user
	}
	if procType, ok := request.Params.Arguments["process_type"].(string); ok && procType != "" {
		opts.ProcessType = procType
	}
	if objType, ok := request.Params.Arguments["object_type"].(string); ok && objType != "" {
		opts.ObjectType = objType
	}
	if max, ok := request.Params.Arguments["max_results"].(float64); ok && max > 0 {
		opts.MaxResults = int(max)
	}

	traces, err := s.adtClient.ListTraces(ctx, opts)
	if err != nil {
		return wrapErr("ListTraces", err), nil
	}

	return newToolResultJSON(traces), nil
}

func (s *Server) handleGetTrace(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	traceID, ok := request.Params.Arguments["trace_id"].(string)
	if !ok || traceID == "" {
		return newToolResultError("trace_id is required"), nil
	}

	toolType := "hitlist"
	if tt, ok := request.Params.Arguments["tool_type"].(string); ok && tt != "" {
		toolType = tt
	}

	analysis, err := s.adtClient.GetTrace(ctx, traceID, toolType)
	if err != nil {
		return wrapErr("GetTrace", err), nil
	}

	return newToolResultJSON(analysis), nil
}
