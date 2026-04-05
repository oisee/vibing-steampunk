// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_analysis.go contains handlers for code analysis infrastructure (call graphs, tracing).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// routeAnalysisAction routes "analyze" with call graph and structure types.
func (s *Server) routeAnalysisAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "analyze" {
		return nil, false, nil
	}
	analysisType := getStringParam(params, "type")
	switch analysisType {
	case "call_graph":
		return s.callHandler(ctx, s.handleGetCallGraph, params)
	case "object_structure":
		return s.callHandler(ctx, s.handleGetObjectStructure, params)
	case "callers":
		return s.callHandler(ctx, s.handleGetCallersOf, params)
	case "callees":
		return s.callHandler(ctx, s.handleGetCalleesOf, params)
	case "analyze_call_graph":
		return s.callHandler(ctx, s.handleAnalyzeCallGraph, params)
	case "compare_call_graphs":
		return s.callHandler(ctx, s.handleCompareCallGraphs, params)
	case "trace_execution":
		return s.callHandler(ctx, s.handleTraceExecution, params)
	case "check_boundaries":
		return s.callHandler(ctx, s.handleCheckBoundaries, params)
	case "graph_stats":
		return s.callHandler(ctx, s.handleGraphStats, params)
	case "co_change":
		return s.callHandler(ctx, s.handleCoChange, params)
	case "impact":
		return s.callHandler(ctx, s.handleImpact, params)
	}
	return nil, false, nil
}

// --- Code Analysis Infrastructure Handlers ---

func (s *Server) handleGetCallGraph(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.GetArguments()["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	opts := &adt.CallGraphOptions{
		Direction:  "callers",
		MaxDepth:   3,
		MaxResults: 100,
	}

	if dir, ok := request.GetArguments()["direction"].(string); ok && dir != "" {
		opts.Direction = dir
	}
	if depth, ok := request.GetArguments()["max_depth"].(float64); ok && depth > 0 {
		opts.MaxDepth = int(depth)
	}
	if max, ok := request.GetArguments()["max_results"].(float64); ok && max > 0 {
		opts.MaxResults = int(max)
	}

	graph, err := s.adtClient.GetCallGraph(ctx, objectURI, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get call graph: %v", err)), nil
	}

	result, _ := json.MarshalIndent(graph, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetObjectStructure(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectName, ok := request.GetArguments()["object_name"].(string)
	if !ok || objectName == "" {
		return newToolResultError("object_name is required"), nil
	}

	maxResults := 100
	if max, ok := request.GetArguments()["max_results"].(float64); ok && max > 0 {
		maxResults = int(max)
	}

	structure, err := s.adtClient.GetObjectStructureCAI(ctx, objectName, maxResults)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get object structure: %v", err)), nil
	}

	result, _ := json.MarshalIndent(structure, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetCallersOf(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.GetArguments()["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	maxDepth := 5
	if depth, ok := request.GetArguments()["max_depth"].(float64); ok && depth > 0 {
		maxDepth = int(depth)
	}

	graph, err := s.adtClient.GetCallersOf(ctx, objectURI, maxDepth)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get callers: %v", err)), nil
	}

	// Flatten to edges for easier consumption
	edges := adt.FlattenCallGraph(graph)
	stats := adt.AnalyzeCallGraph(graph)

	output := map[string]interface{}{
		"root":  graph,
		"edges": edges,
		"stats": stats,
	}

	result, _ := json.MarshalIndent(output, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetCalleesOf(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.GetArguments()["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	maxDepth := 5
	if depth, ok := request.GetArguments()["max_depth"].(float64); ok && depth > 0 {
		maxDepth = int(depth)
	}

	graph, err := s.adtClient.GetCalleesOf(ctx, objectURI, maxDepth)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get callees: %v", err)), nil
	}

	// Flatten to edges for easier consumption
	edges := adt.FlattenCallGraph(graph)
	stats := adt.AnalyzeCallGraph(graph)

	output := map[string]interface{}{
		"root":  graph,
		"edges": edges,
		"stats": stats,
	}

	result, _ := json.MarshalIndent(output, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleAnalyzeCallGraph(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.GetArguments()["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	direction := "callees"
	if dir, ok := request.GetArguments()["direction"].(string); ok && dir != "" {
		direction = dir
	}

	maxDepth := 5
	if depth, ok := request.GetArguments()["max_depth"].(float64); ok && depth > 0 {
		maxDepth = int(depth)
	}

	graph, err := s.adtClient.GetCallGraph(ctx, objectURI, &adt.CallGraphOptions{
		Direction:  direction,
		MaxDepth:   maxDepth,
		MaxResults: 1000,
	})
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get call graph: %v", err)), nil
	}

	stats := adt.AnalyzeCallGraph(graph)
	edges := adt.FlattenCallGraph(graph)

	output := map[string]interface{}{
		"object_uri": objectURI,
		"direction":  direction,
		"stats":      stats,
		"edge_count": len(edges),
		"edges":      edges,
	}

	result, _ := json.MarshalIndent(output, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleCompareCallGraphs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.GetArguments()["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	traceDataStr, ok := request.GetArguments()["trace_data"].(string)
	if !ok || traceDataStr == "" {
		return newToolResultError("trace_data is required (JSON array of edges)"), nil
	}

	// Parse trace data
	var actualEdges []adt.CallGraphEdge
	if err := json.Unmarshal([]byte(traceDataStr), &actualEdges); err != nil {
		return newToolResultError(fmt.Sprintf("Failed to parse trace_data: %v", err)), nil
	}

	// Get static call graph
	graph, err := s.adtClient.GetCallGraph(ctx, objectURI, &adt.CallGraphOptions{
		Direction:  "callees",
		MaxDepth:   10,
		MaxResults: 1000,
	})
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get static call graph: %v", err)), nil
	}

	staticEdges := adt.FlattenCallGraph(graph)

	// Compare
	comparison := adt.CompareCallGraphs(staticEdges, actualEdges)

	output := map[string]interface{}{
		"object_uri":     objectURI,
		"static_edges":   len(staticEdges),
		"actual_edges":   len(actualEdges),
		"common_edges":   len(comparison.CommonEdges),
		"untested_paths": len(comparison.StaticOnly),
		"dynamic_calls":  len(comparison.ActualOnly),
		"coverage_ratio": comparison.CoverageRatio,
		"common":         comparison.CommonEdges,
		"static_only":    comparison.StaticOnly,
		"actual_only":    comparison.ActualOnly,
	}

	result, _ := json.MarshalIndent(output, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleTraceExecution(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.GetArguments()["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	opts := &adt.TraceExecutionOptions{
		ObjectURI: objectURI,
		MaxDepth:  5,
	}

	if maxDepth, ok := request.GetArguments()["max_depth"].(float64); ok {
		opts.MaxDepth = int(maxDepth)
	}

	if runTests, ok := request.GetArguments()["run_tests"].(bool); ok {
		opts.RunTests = runTests
	}

	if testURI, ok := request.GetArguments()["test_object_uri"].(string); ok && testURI != "" {
		opts.TestObjectURI = testURI
	} else if opts.RunTests {
		opts.TestObjectURI = objectURI // Default to same object
	}

	if traceUser, ok := request.GetArguments()["trace_user"].(string); ok && traceUser != "" {
		opts.TraceUser = traceUser
	}

	result, err := s.adtClient.TraceExecution(ctx, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Trace execution failed: %v", err)), nil
	}

	// Build comprehensive output
	output := map[string]interface{}{
		"object_uri": objectURI,
	}

	if result.StaticStats != nil {
		output["static_stats"] = result.StaticStats
	}

	if result.Trace != nil {
		output["trace"] = map[string]interface{}{
			"id":          result.Trace.TraceID,
			"total_time":  result.Trace.TotalTime,
			"total_calls": result.Trace.TotalCalls,
			"entries":     len(result.Trace.Entries),
		}
	}

	if len(result.ActualEdges) > 0 {
		output["actual_edges"] = result.ActualEdges
	}

	if result.Comparison != nil {
		output["comparison"] = map[string]interface{}{
			"common_edges":   len(result.Comparison.CommonEdges),
			"untested_paths": len(result.Comparison.StaticOnly),
			"dynamic_calls":  len(result.Comparison.ActualOnly),
			"coverage_ratio": result.Comparison.CoverageRatio,
			"static_only":    result.Comparison.StaticOnly,
			"actual_only":    result.Comparison.ActualOnly,
		}
	}

	if len(result.ExecutedTests) > 0 {
		output["executed_tests"] = result.ExecutedTests
	}

	output["execution_time_us"] = result.ExecutionTime

	jsonResult, _ := json.MarshalIndent(output, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}
