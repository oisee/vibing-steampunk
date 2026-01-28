// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_analysis.go contains handlers for code analysis infrastructure (call graphs, tracing).
package mcp

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Analysis Routing ---
// Routes for this module:
//   analyze: type=call_graph, type=get_call_graph, type=compare_call_graphs, type=trace_execution,
//            type=callers, type=callees, type=structure

// routeAnalysisAction routes code analysis actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeAnalysisAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "analyze" {
		return nil, false, nil
	}

	analysisType, _ := params["type"].(string)
	switch analysisType {
	case "call_graph", "get_call_graph":
		// Try object_uri from params first, then build from target
		objectURI, _ := params["object_uri"].(string)
		if objectURI == "" {
			objectURI = buildObjectURL(objectType, objectName)
		}
		if objectURI == "" {
			return newToolResultError("target (e.g., 'CLAS ZCL_TEST') or object_uri in params is required"), true, nil
		}
		args := map[string]any{"object_uri": objectURI}
		if dir, ok := params["direction"].(string); ok {
			args["direction"] = dir
		}
		if depth, ok := params["max_depth"].(float64); ok {
			args["max_depth"] = depth
		}
		if max, ok := params["max_results"].(float64); ok {
			args["max_results"] = max
		}
		if method, ok := params["method"].(string); ok {
			args["method"] = method
		}
		// Use handleGetCallGraph for raw call graph, handleAnalyzeCallGraph for analyzed version
		if analysisType == "get_call_graph" {
			result, err := s.handleGetCallGraph(ctx, newRequest(args))
			return result, true, err
		}
		result, err := s.handleAnalyzeCallGraph(ctx, newRequest(args))
		return result, true, err

	case "compare_call_graphs":
		// Try object_uri from params first, then build from target
		objectURI, _ := params["object_uri"].(string)
		if objectURI == "" {
			objectURI = buildObjectURL(objectType, objectName)
		}
		traceData, _ := params["trace_data"].(string)
		if objectURI == "" || traceData == "" {
			return newToolResultError("target or object_uri, and trace_data are required"), true, nil
		}
		result, err := s.handleCompareCallGraphs(ctx, newRequest(map[string]any{
			"object_uri": objectURI,
			"trace_data": traceData,
		}))
		return result, true, err

	case "trace_execution":
		// Try object_uri from params first, then build from target
		objectURI, _ := params["object_uri"].(string)
		if objectURI == "" {
			objectURI = buildObjectURL(objectType, objectName)
		}
		if objectURI == "" {
			return newToolResultError("target (e.g., 'CLAS ZCL_TEST') or object_uri in params is required"), true, nil
		}
		args := map[string]any{"object_uri": objectURI}
		if depth, ok := params["max_depth"].(float64); ok {
			args["max_depth"] = depth
		}
		if runTests, ok := params["run_tests"].(bool); ok {
			args["run_tests"] = runTests
		}
		if testURI, ok := params["test_object_uri"].(string); ok {
			args["test_object_uri"] = testURI
		}
		if traceUser, ok := params["trace_user"].(string); ok {
			args["trace_user"] = traceUser
		}
		result, err := s.handleTraceExecution(ctx, newRequest(args))
		return result, true, err

	case "callers":
		// Try object_uri from params first, then build from target
		objectURI, _ := params["object_uri"].(string)
		if objectURI == "" {
			objectURI = buildObjectURL(objectType, objectName)
		}
		if objectURI == "" {
			return newToolResultError("target (e.g., 'CLAS ZCL_TEST') or object_uri in params is required"), true, nil
		}
		args := map[string]any{"object_uri": objectURI}
		if depth, ok := params["max_depth"].(float64); ok {
			args["max_depth"] = depth
		}
		result, err := s.handleGetCallersOf(ctx, newRequest(args))
		return result, true, err

	case "callees":
		// Try object_uri from params first, then build from target
		objectURI, _ := params["object_uri"].(string)
		if objectURI == "" {
			objectURI = buildObjectURL(objectType, objectName)
		}
		if objectURI == "" {
			return newToolResultError("target (e.g., 'CLAS ZCL_TEST') or object_uri in params is required"), true, nil
		}
		args := map[string]any{"object_uri": objectURI}
		if depth, ok := params["max_depth"].(float64); ok {
			args["max_depth"] = depth
		}
		result, err := s.handleGetCalleesOf(ctx, newRequest(args))
		return result, true, err

	case "structure":
		// Try object_name from params first, then use objectName from target
		name, _ := params["object_name"].(string)
		if name == "" {
			name = objectName
		}
		if name == "" {
			return newToolResultError("target (e.g., 'ZCL_TEST') or object_name in params is required"), true, nil
		}
		args := map[string]any{"object_name": name}
		if maxResults, ok := params["max_results"].(float64); ok {
			args["max_results"] = maxResults
		}
		result, err := s.handleGetObjectStructure(ctx, newRequest(args))
		return result, true, err
	}

	return nil, false, nil
}

// --- Code Analysis Infrastructure Handlers ---

func (s *Server) handleGetCallGraph(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.Params.Arguments["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	opts := &adt.CallGraphOptions{
		Direction:  "callers",
		MaxDepth:   3,
		MaxResults: 100,
	}

	if dir, ok := request.Params.Arguments["direction"].(string); ok && dir != "" {
		opts.Direction = dir
	}
	if depth, ok := request.Params.Arguments["max_depth"].(float64); ok && depth > 0 {
		opts.MaxDepth = int(depth)
	}
	if max, ok := request.Params.Arguments["max_results"].(float64); ok && max > 0 {
		opts.MaxResults = int(max)
	}
	if method, ok := request.Params.Arguments["method"].(string); ok {
		opts.Method = method
	}

	graph, err := s.adtClient.GetCallGraph(ctx, objectURI, opts)
	if err != nil {
		return wrapErr("GetCallGraph", err), nil
	}

	return newToolResultJSON(graph), nil
}

func (s *Server) handleGetObjectStructure(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectName, ok := request.Params.Arguments["object_name"].(string)
	if !ok || objectName == "" {
		return newToolResultError("object_name is required"), nil
	}

	maxResults := 100
	if max, ok := request.Params.Arguments["max_results"].(float64); ok && max > 0 {
		maxResults = int(max)
	}

	structure, err := s.adtClient.GetObjectStructureCAI(ctx, objectName, maxResults)
	if err != nil {
		return wrapErr("GetObjectStructure", err), nil
	}

	return newToolResultJSON(structure), nil
}

func (s *Server) handleGetCallersOf(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.Params.Arguments["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	maxDepth := 5
	if depth, ok := request.Params.Arguments["max_depth"].(float64); ok && depth > 0 {
		maxDepth = int(depth)
	}

	graph, err := s.adtClient.GetCallersOf(ctx, objectURI, maxDepth)
	if err != nil {
		return wrapErr("GetCallersOf", err), nil
	}

	// Flatten to edges for easier consumption
	edges := adt.FlattenCallGraph(graph)
	stats := adt.AnalyzeCallGraph(graph)

	return newToolResultJSON(map[string]any{
		"root":  graph,
		"edges": edges,
		"stats": stats,
	}), nil
}

func (s *Server) handleGetCalleesOf(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.Params.Arguments["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	maxDepth := 5
	if depth, ok := request.Params.Arguments["max_depth"].(float64); ok && depth > 0 {
		maxDepth = int(depth)
	}

	graph, err := s.adtClient.GetCalleesOf(ctx, objectURI, maxDepth)
	if err != nil {
		return wrapErr("GetCalleesOf", err), nil
	}

	// Flatten to edges for easier consumption
	edges := adt.FlattenCallGraph(graph)
	stats := adt.AnalyzeCallGraph(graph)

	return newToolResultJSON(map[string]any{
		"root":  graph,
		"edges": edges,
		"stats": stats,
	}), nil
}

func (s *Server) handleAnalyzeCallGraph(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.Params.Arguments["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	direction := "callees"
	if dir, ok := request.Params.Arguments["direction"].(string); ok && dir != "" {
		direction = dir
	}

	maxDepth := 5
	if depth, ok := request.Params.Arguments["max_depth"].(float64); ok && depth > 0 {
		maxDepth = int(depth)
	}

	method := ""
	if m, ok := request.Params.Arguments["method"].(string); ok {
		method = m
	}

	graph, err := s.adtClient.GetCallGraph(ctx, objectURI, &adt.CallGraphOptions{
		Direction:  direction,
		MaxDepth:   maxDepth,
		MaxResults: 1000,
		Method:     method,
	})
	if err != nil {
		return wrapErr("AnalyzeCallGraph", err), nil
	}

	stats := adt.AnalyzeCallGraph(graph)
	edges := adt.FlattenCallGraph(graph)

	return newToolResultJSON(map[string]any{
		"object_uri": objectURI,
		"direction":  direction,
		"stats":      stats,
		"edge_count": len(edges),
		"edges":      edges,
	}), nil
}

func (s *Server) handleCompareCallGraphs(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.Params.Arguments["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	traceDataStr, ok := request.Params.Arguments["trace_data"].(string)
	if !ok || traceDataStr == "" {
		return newToolResultError("trace_data is required (JSON array of edges)"), nil
	}

	// Parse trace data
	var actualEdges []adt.CallGraphEdge
	if err := json.Unmarshal([]byte(traceDataStr), &actualEdges); err != nil {
		return wrapErr("ParseTraceData", err), nil
	}

	// Get static call graph
	graph, err := s.adtClient.GetCallGraph(ctx, objectURI, &adt.CallGraphOptions{
		Direction:  "callees",
		MaxDepth:   10,
		MaxResults: 1000,
	})
	if err != nil {
		return wrapErr("GetStaticCallGraph", err), nil
	}

	staticEdges := adt.FlattenCallGraph(graph)

	// Compare
	comparison := adt.CompareCallGraphs(staticEdges, actualEdges)

	return newToolResultJSON(map[string]any{
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
	}), nil
}

func (s *Server) handleTraceExecution(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.Params.Arguments["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	opts := &adt.TraceExecutionOptions{
		ObjectURI: objectURI,
		MaxDepth:  5,
	}

	if maxDepth, ok := request.Params.Arguments["max_depth"].(float64); ok {
		opts.MaxDepth = int(maxDepth)
	}

	if runTests, ok := request.Params.Arguments["run_tests"].(bool); ok {
		opts.RunTests = runTests
	}

	if testURI, ok := request.Params.Arguments["test_object_uri"].(string); ok && testURI != "" {
		opts.TestObjectURI = testURI
	} else if opts.RunTests {
		opts.TestObjectURI = objectURI // Default to same object
	}

	if traceUser, ok := request.Params.Arguments["trace_user"].(string); ok && traceUser != "" {
		opts.TraceUser = traceUser
	}

	result, err := s.adtClient.TraceExecution(ctx, opts)
	if err != nil {
		return wrapErr("TraceExecution", err), nil
	}

	// Build comprehensive output
	output := map[string]any{
		"object_uri": objectURI,
	}

	if result.StaticStats != nil {
		output["static_stats"] = result.StaticStats
	}

	if result.Trace != nil {
		output["trace"] = map[string]any{
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
		output["comparison"] = map[string]any{
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

	return newToolResultJSON(output), nil
}
