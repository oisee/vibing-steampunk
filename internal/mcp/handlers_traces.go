// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_traces.go contains handlers for ABAP profiler traces (ATRA).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// traceAnalysisTypes is the routing table as data; see analysisTypes.
func (s *Server) traceAnalysisTypes() map[string]server.ToolHandlerFunc {
	return map[string]server.ToolHandlerFunc{
		"list_traces": s.handleListTraces,
		"get_trace":   s.handleGetTrace,
	}
}

// routeTracesAction routes "analyze" with trace-related types.
func (s *Server) routeTracesAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "analyze" {
		return nil, false, nil
	}
	handler, known := s.traceAnalysisTypes()[getStringParam(params, "type")]
	if !known {
		return nil, false, nil
	}
	return s.callHandler(ctx, handler, params)
}

// --- ABAP Profiler / Traces Handlers ---

func (s *Server) handleListTraces(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	opts := &adt.TraceQueryOptions{
		MaxResults: 100,
	}

	if user, ok := request.GetArguments()["user"].(string); ok && user != "" {
		opts.User = user
	}
	if procType, ok := request.GetArguments()["process_type"].(string); ok && procType != "" {
		opts.ProcessType = procType
	}
	if objType, ok := request.GetArguments()["object_type"].(string); ok && objType != "" {
		opts.ObjectType = objType
	}
	if max, ok := request.GetArguments()["max_results"].(float64); ok && max > 0 {
		opts.MaxResults = int(max)
	}

	traces, err := s.adtClient.ListTraces(ctx, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to list traces: %v", err)), nil
	}

	result, _ := json.MarshalIndent(traces, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

// defaultTraceTop keeps a hit list readable: a long run has tens of
// thousands of positions, and the few that cost the time sort to the top.
const defaultTraceTop = 50

// rawTraceLimit caps raw=true output; a hit list can be 20 MB of XML.
const rawTraceLimit = 256 * 1024

func (s *Server) handleGetTrace(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	traceID := getStringParam(args, "trace_id")
	if traceID == "" {
		return newToolResultError("trace_id is required"), nil
	}
	toolType := getStringParam(args, "tool_type")

	if raw, _ := getBoolParam(args, "raw"); raw {
		body, err := s.adtClient.GetTraceRaw(ctx, traceID, toolType)
		if err != nil {
			return newToolResultError(fmt.Sprintf("Failed to get trace: %v", err)), nil
		}
		if len(body) > rawTraceLimit {
			return mcp.NewToolResultText(fmt.Sprintf("%s\n... [truncated: showing %d of %d bytes]", body[:rawTraceLimit], rawTraceLimit, len(body))), nil
		}
		return mcp.NewToolResultText(string(body)), nil
	}

	opts := adt.TraceGetOptions{
		ToolType: toolType,
		SortBy:   getStringParam(args, "sort_by"),
		Top:      defaultTraceTop,
	}
	if top, ok := getFloatParam(args, "top"); ok {
		opts.Top = int(top) // 0 returns every entry
	}

	analysis, err := s.adtClient.GetTraceAnalysis(ctx, traceID, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get trace: %v", err)), nil
	}

	result, _ := json.MarshalIndent(analysis, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}
