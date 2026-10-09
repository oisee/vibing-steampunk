// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_analysis.go contains handlers for code analysis infrastructure (call graphs, tracing).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// countExecutable counts the edges in a list that something could actually
// have run. It exists so "untested paths" means paths.
func countExecutable(edges []adt.CallGraphEdge) int {
	n := 0
	for _, e := range edges {
		if adt.IsExecutableKind(e.CalleeKind) {
			n++
		}
	}
	return n
}

// analysisTypes is the routing table as data rather than as control flow.
//
// It is a map and not a switch so that the surface can be enumerated — `vsp
// sweep` walks it to ask whether every advertised type is reachable, and a test
// asserts the published list and this table are the same set. A switch answers
// "is this type routed?" only by running the handler, which is no use to
// anything that wants to check the surface without calling into SAP.
func (s *Server) analysisTypes() map[string]server.ToolHandlerFunc {
	return map[string]server.ToolHandlerFunc{
		"call_graph":          s.handleGetCallGraph,
		"object_structure":    s.handleGetObjectStructure,
		"callers":             s.handleGetCallersOf,
		"callees":             s.handleGetCalleesOf,
		"analyze_call_graph":  s.handleAnalyzeCallGraph,
		"compare_call_graphs": s.handleCompareCallGraphs,
		"trace_execution":     s.handleTraceExecution,
		"check_boundaries":    s.handleCheckBoundaries,
		"loads":               s.handleLoads,
		"graph_stats":         s.handleGraphStats,
		"co_change":           s.handleCoChange,
		"impact":              s.handleImpact,
		"where_used_config":   s.handleWhereUsedConfig,
		"usage_examples":      s.handleUsageExamples,
		"health":              s.handleHealth,
		"cr_history":          s.handleCRHistory,
		"tr_boundaries":       s.handleTransportBoundaries,
		"cr_boundaries":       s.handleCRBoundaries,
		// Same handler as read target="CDS_IMPACT", which help advertises
		// under this type too; it reads view_name, so the names a caller
		// writes for a CDS view are mapped onto it.
		"cds_impact": func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := paramsWithAlias(req.GetArguments(), "view_name", "cds_view", "ddls_name", "object_name", "name")
			return s.handleGetCDSImpactAnalysis(ctx, newRequest(args))
		},
	}
}

// routeAnalysisAction routes "analyze" with call graph and structure types.
func (s *Server) routeAnalysisAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "analyze" {
		return nil, false, nil
	}
	handler, known := s.analysisTypes()[getStringParam(params, "type")]
	if !known {
		return nil, false, nil
	}
	return s.callHandler(ctx, handler, params)
}

// --- Code Analysis Infrastructure Handlers ---
//
// These five used to ask /sap/bc/adt/cai/callgraph, which does not exist on
// any release we have seen: an agent calling analyze type=callers got a raw
// 404 back, and one calling type=callees got a graph with no children, which
// reads as "this object calls nothing". They now go to the two sources that do
// answer — the where-used list for the up direction, the CROSS and WBCROSSGT
// cross-reference tables for the down one — and when a direction cannot be
// served the answer says which source failed and why, never a bare status
// code.

func (s *Server) handleGetCallGraph(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	direction := "callers"
	if dir, ok := request.GetArguments()["direction"].(string); ok && dir != "" {
		direction = strings.ToLower(strings.TrimSpace(dir))
	}

	switch direction {
	case "callers":
		return s.handleGetCallersOf(ctx, request)
	case "callees":
		return s.handleGetCalleesOf(ctx, request)
	case "both":
		// Two answers rather than one merged tree: they come from different
		// sources with different confidence, and flattening them together
		// would hide which half is the graded where-used list and which half
		// is a table of activation-time references.
		up, err := s.callGraphAnswer(ctx, request, "callers")
		if err != nil {
			return newToolResultError(err.Error()), nil
		}
		down, err := s.callGraphAnswer(ctx, request, "callees")
		if err != nil {
			return newToolResultError(err.Error()), nil
		}
		result, _ := json.MarshalIndent(map[string]any{"callers": up, "callees": down}, "", "  ")
		return mcp.NewToolResultText(string(result)), nil
	}

	return newToolResultError(fmt.Sprintf(
		"direction %q is not one this can answer. It is \"callers\" (the where-used list SE84 shows), "+
			"\"callees\" (the CROSS and WBCROSSGT cross-reference tables), or \"both\".", direction)), nil
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
	answer, err := s.callGraphAnswer(ctx, request, "callers")
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	result, _ := json.MarshalIndent(answer, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetCalleesOf(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	answer, err := s.callGraphAnswer(ctx, request, "callees")
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	result, _ := json.MarshalIndent(answer, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

// callGraphAnswer is the one place either direction is answered, so that the
// tool, the analyze action and the statistics all report the same thing about
// the same object.
func (s *Server) callGraphAnswer(ctx context.Context, request mcp.CallToolRequest, direction string) (map[string]any, error) {
	objectURI, err := s.callGraphObjectURI(ctx, request)
	if err != nil {
		return nil, err
	}

	limit := defaultRows
	if max, ok := request.GetArguments()["max_results"].(float64); ok && max > 0 {
		limit = int(max)
	}

	answer := map[string]any{
		"object_uri": objectURI,
		"direction":  direction,
	}

	switch direction {
	case "callers":
		callers, unresolved, err := s.adtClient.WhereUsed(ctx, objectURI)
		if err != nil {
			return nil, fmt.Errorf("the where-used list could not be read for %s: %v", objectURI, err)
		}
		answer["source"] = sourceWhereUsed
		answer["total"] = len(callers)
		if len(unresolved) > 0 {
			// Those includes are in the list, as themselves. Said beside it so
			// an include standing where its program should be is not read as
			// the program having no callers here.
			answer["unresolved_includes"] = unresolved
			answer["gap"] = adt.UnresolvedIncludesNote(unresolved)
		}
		if len(callers) > limit {
			// Said, not left to be inferred from comparing "total" against the
			// length of the array. A reader who does not make that comparison
			// reads a truncated where-used list as the whole one.
			answer["truncated"] = truncationNote(limit, len(callers), "max_results")
			callers = callers[:limit]
		}
		answer["callers"] = callerAnswers(callers)
		if len(callers) == 0 {
			answer["note"] = emptyWhereUsedNote
		}
	case "callees":
		callees, gaps, err := s.adtClient.Callees(ctx, objectURI)
		if err != nil {
			// Unwrapped: these errors already name the object, and a second
			// sentence around them reads as two different failures.
			return nil, err
		}
		answer["source"] = sourceCrossReference
		answer["total"] = len(callees)
		if len(gaps) > 0 {
			// One of the two tables answered and the other did not. The list
			// below is real but short, and "total" would otherwise read as the
			// whole truth — so name the table that is missing from it.
			answer["unsearched"] = gaps
			answer["gap"] = adt.UnsearchedNote(gaps, 2, "cross-reference table")
		}
		if len(callees) > limit {
			answer["truncated"] = truncationNote(limit, len(callees), "max_results")
			callees = callees[:limit]
		}
		answer["callees"] = callees
		// Asked for explicitly, and returned beside the answer rather than in
		// it. "What does this reference" is about the code that runs; the
		// inactive index describes a version that does not. One list holding
		// both would describe behaviour nothing has.
		if v, ok := request.GetArguments()["include_inactive"].(bool); ok && v {
			inactive, _, ierr := s.adtClient.InactiveCallees(ctx, objectURI)
			switch {
			case ierr != nil:
				answer["inactive_error"] = ierr.Error()
			case len(inactive) > 0:
				answer["inactive"] = inactive
				answer["inactive_note"] = fmt.Sprintf(
					"%d references are recorded against an unactivated version of this object. "+
						"They are listed separately because they describe what will change when it is "+
						"activated, not what the running code does.", len(inactive))
			}
		}

		if len(callees) == 0 {
			answer["note"] = emptyCalleesNote
			if n := s.adtClient.InactiveReferenceCount(ctx, objectURI); n > 0 {
				// The empty note lists three readings; this decides between
				// them, so it replaces rather than accompanies it.
				answer["note"] = fmt.Sprintf("No row for this object's includes, but %d are "+
					"recorded against an inactive version of it, in the index SAP keeps for "+
					"objects with unactivated changes. This object does reference things; the "+
					"version activated on this system does not.", n)
				answer["inactive_references"] = n
			}
		} else {
			answer["caveat"] = calleeCaveat
		}
	default:
		return nil, fmt.Errorf("direction %q is neither \"callers\" nor \"callees\"", direction)
	}

	return answer, nil
}

// callerAnswer is one caller as this question wants it. The where-used list is
// shared with the dump impact query, whose ExposedCaller carries a distance
// from a failing statement and the unit it was reached through — neither of
// which means anything when nothing has failed, and both of which read as
// answers if they are printed anyway.
type callerAnswer struct {
	Name    string `json:"name"`
	Type    string `json:"type,omitempty"`
	URI     string `json:"uri,omitempty"`
	Package string `json:"package,omitempty"`
	// Component is the method or routine the reference sits in, which is where
	// to look rather than which object to open.
	Component string `json:"component,omitempty"`
	IsTest    bool   `json:"is_test"`
}

func callerAnswers(callers []adt.ExposedCaller) []callerAnswer {
	out := make([]callerAnswer, 0, len(callers))
	for _, c := range callers {
		out = append(out, callerAnswer{
			Name:      c.Name,
			Type:      c.Type,
			URI:       c.URI,
			Package:   c.Package,
			Component: c.Component,
			IsTest:    c.IsTest,
		})
	}
	return out
}

// callGraphObjectURI takes the object to ask about, by URI or by type and
// name. Agents reach for the names — the URI is a detail of the protocol, not
// of the question — and refusing a request that named the object perfectly
// well is a bad way to spend a turn.
func (s *Server) callGraphObjectURI(ctx context.Context, request mcp.CallToolRequest) (string, error) {
	args := request.GetArguments()
	if uri, ok := args["object_uri"].(string); ok && strings.TrimSpace(uri) != "" {
		return strings.TrimSpace(uri), nil
	}

	objType := strings.ToUpper(strings.TrimSpace(getStringParam(args, "object_type")))
	objName := strings.ToUpper(strings.TrimSpace(getStringParam(args, "object_name")))
	if objType == "" || objName == "" {
		return "", fmt.Errorf("name the object: object_uri, or object_type plus object_name " +
			"(CLAS, INTF, PROG, FUGR, FUNC)")
	}

	if objType == "FUNC" {
		// A module is addressable only under its group, and only the
		// repository knows which group that is.
		results, err := s.adtClient.SearchObject(ctx, objName, 10)
		if err != nil {
			return "", fmt.Errorf("looking up function module %s: %v", objName, err)
		}
		for _, r := range results {
			if strings.EqualFold(r.Name, objName) && strings.Contains(r.URI, "/fmodules/") {
				return r.URI, nil
			}
		}
		return "", fmt.Errorf("function module %s is not in the repository, so it has no URI to ask about", objName)
	}

	uri := buildADTObjectURL(objType, objName)
	if uri == "" {
		return "", fmt.Errorf("%s is not an object type this can address; it knows CLAS, INTF, PROG, FUGR and FUNC", objType)
	}
	return uri, nil
}

const (
	sourceWhereUsed = "the where-used list behind SE84 " +
		"(/sap/bc/adt/repository/informationsystem/usageReferences), filtered to direct references"
	sourceCrossReference = "the CROSS and WBCROSSGT cross-reference tables, which SAP fills at activation"

	// An empty where-used list and a misspelt object name produce the same 200
	// and the same empty list. Saying so is the only way an agent can tell
	// which of the two it is looking at.
	emptyWhereUsedNote = "The where-used list answered and it is empty: nobody references this object — " +
		"or the name does not exist, which reads identically here. Check the name resolves before " +
		"concluding the object is unused."
	emptyCalleesNote = "The cross-reference tables answered and hold no row for this object's includes. " +
		"That is a real answer for code that touches nothing global: they record global types, classes, " +
		"methods and external calls, so a class doing only local work and a procedural routine calling " +
		"nothing outside its own program are both legitimately empty. The other explanations are that the " +
		"object has never been activated here — the tables are filled at activation, not at save — or that " +
		"the name does not exist. Ask about a neighbour that should have references to tell the cases apart."

	// The down direction is the weaker of the two and an agent acting on it
	// should know how.
	calleeCaveat = "These are references recorded at activation, not observed calls: a dynamic " +
		"CALL METHOD (name) or a call by RFC destination appears in no row, and a reference inside " +
		"dead code appears in every one. \"calls\": true marks the rows that are an invocation " +
		"rather than a type or data reference."
)

func (s *Server) handleAnalyzeCallGraph(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, err := s.callGraphObjectURI(ctx, request)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}

	direction := "callees"
	if dir, ok := request.GetArguments()["direction"].(string); ok && dir != "" {
		direction = strings.ToLower(strings.TrimSpace(dir))
	}

	graph, err := s.adtClient.CallGraph(ctx, objectURI, &adt.CallGraphOptions{
		Direction:  direction,
		MaxResults: 1000,
	})
	if err != nil {
		return newToolResultError(err.Error()), nil
	}

	stats := adt.AnalyzeCallGraph(graph)
	edges := adt.FlattenCallGraph(graph)

	output := map[string]interface{}{
		"object_uri": objectURI,
		"direction":  direction,
		"stats":      stats,
		"edge_count": len(edges),
		"edges":      edges,
		// max_depth used to be a parameter here. It is gone rather than
		// ignored quietly: both sources are one hop, so a depth of five was
		// only ever a number in a request nobody answered.
		"depth": 1,
	}
	if note := callGraphGapNote(graph, direction); note != "" {
		// Beside the edges, not inside them: an include standing where its
		// program should be is still an edge, and a table that could not be
		// read leaves edges out. Either way the list is not the whole answer.
		output["unsearched"] = graph.Unsearched
		output["gap"] = note
	}

	result, _ := json.MarshalIndent(output, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

// callGraphGapNote says what a call graph's Unsearched means, which depends
// on the direction it was asked in: for callers it is includes whose main
// program could not be read, for callees the cross-reference tables that could
// not be.
func callGraphGapNote(graph *adt.CallGraphNode, direction string) string {
	if graph == nil || len(graph.Unsearched) == 0 {
		return ""
	}
	if direction == "callers" {
		return adt.UnresolvedIncludesNote(graph.Unsearched)
	}
	return adt.UnsearchedNote(graph.Unsearched, 2, "cross-reference table")
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

	// The static side, from the cross-reference tables. Note what this makes
	// the comparison mean: the tables record references, so an edge that is
	// "static only" may be a type this code names and never calls, not an
	// untested path.
	graph, err := s.adtClient.CallGraph(ctx, objectURI, &adt.CallGraphOptions{
		Direction:  "callees",
		MaxResults: 1000,
	})
	if err != nil {
		return newToolResultError(fmt.Sprintf("the static side of the comparison could not be read: %v", err)), nil
	}

	staticEdges := adt.FlattenCallGraph(graph)

	// Compare
	comparison := adt.CompareCallGraphs(staticEdges, actualEdges)

	output := map[string]interface{}{
		"object_uri":   objectURI,
		"static_edges": len(staticEdges),
		"actual_edges": len(actualEdges),
		"common_edges": len(comparison.CommonEdges),
		// Counted over invocations only. A type reference is not a path, and
		// calling it untested inflated this figure with things nothing could
		// ever have executed.
		"untested_paths":   countExecutable(comparison.StaticOnly),
		"executable_edges": comparison.ExecutableEdges,
		"dynamic_calls":    len(comparison.ActualOnly),
		"coverage_ratio":   comparison.CoverageRatio,
		"common":           comparison.CommonEdges,
		"static_only":      comparison.StaticOnly,
		"actual_only":      comparison.ActualOnly,
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
			"note":        result.Trace.Note,
		}
	}

	// What did not run, and why: without it a failed trace read looks like
	// code that made no calls.
	if len(result.Unsearched) > 0 {
		output["unsearched"] = result.Unsearched
	}

	if len(result.ActualEdges) > 0 {
		output["actual_edges"] = result.ActualEdges
	}

	if result.Comparison != nil {
		output["comparison"] = map[string]interface{}{
			"common_edges":     len(result.Comparison.CommonEdges),
			"untested_paths":   countExecutable(result.Comparison.StaticOnly),
			"executable_edges": result.Comparison.ExecutableEdges,
			"dynamic_calls":    len(result.Comparison.ActualOnly),
			"coverage_ratio":   result.Comparison.CoverageRatio,
			"static_only":      result.Comparison.StaticOnly,
			"actual_only":      result.Comparison.ActualOnly,
		}
	}

	if len(result.ExecutedTests) > 0 {
		output["executed_tests"] = result.ExecutedTests
	}

	output["execution_time_us"] = result.ExecutionTime

	jsonResult, _ := json.MarshalIndent(output, "", "  ")
	return mcp.NewToolResultText(string(jsonResult)), nil
}
