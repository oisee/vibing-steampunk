// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_traces.go contains handlers for ABAP profiler traces (ATRA).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

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

func (s *Server) handleGetTrace(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	traceID, ok := request.GetArguments()["trace_id"].(string)
	if !ok || traceID == "" {
		return newToolResultError("trace_id is required"), nil
	}

	toolType := "hitlist"
	if tt, ok := request.GetArguments()["tool_type"].(string); ok && tt != "" {
		toolType = tt
	}

	analysis, err := s.adtClient.GetTrace(ctx, traceID, toolType)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get trace: %v", err)), nil
	}

	result, _ := json.MarshalIndent(analysis, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}
