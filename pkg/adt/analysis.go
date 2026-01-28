package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// --- Code Analysis Infrastructure (CAI) Operations ---

// CallGraphNode represents a node in the call graph.
type CallGraphNode struct {
	URI         string          `json:"uri"`
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Description string          `json:"description,omitempty"`
	Line        int             `json:"line,omitempty"`
	Column      int             `json:"column,omitempty"`
	Children    []CallGraphNode `json:"children,omitempty"`
}

// CallGraphOptions configures call graph retrieval.
type CallGraphOptions struct {
	Direction  string // "callers" or "callees"
	MaxDepth   int    // Maximum depth to traverse
	MaxResults int    // Maximum results to return
	Method     string // Specific method name (for classes) - if empty, analyzes all methods
}

// ProcedureID represents a CAI procedure identifier.
type ProcedureID struct {
	Program  string // Program name (e.g., ZTEST, SAPL<fugr>)
	ProcType string // PROG, METH, FUNC, FORM, etc.
	ProcName string // Empty for PROG, method/function name for others
}

// ParseObjectURIToProcID converts an ADT object URI to a CAI procedure ID.
// Examples:
//   - /sap/bc/adt/programs/programs/ztest -> {ZTEST, PROG, ""}
//   - /sap/bc/adt/oo/classes/zcl_test -> {ZCL_TEST, CLAS, ""} (whole class)
//   - /sap/bc/adt/oo/classes/zcl_test with method=DO_SOMETHING -> {ZCL_TEST, METH, DO_SOMETHING}
//   - /sap/bc/adt/functions/groups/zfugr/fmodules/zfunc -> {SAPLZFUGR, FUNC, ZFUNC}
//
// For classes, you can specify a method using params: method=METHOD_NAME
// If no method specified, returns CLAS type which triggers whole-class analysis.
func ParseObjectURIToProcID(objectURI string, params map[string]any) *ProcedureID {
	// Strip query/fragment from URI
	uri := objectURI
	if idx := strings.Index(uri, "#"); idx > 0 {
		uri = uri[:idx]
	}
	if idx := strings.Index(uri, "?"); idx > 0 {
		uri = uri[:idx]
	}

	parts := strings.Split(strings.TrimPrefix(uri, "/sap/bc/adt/"), "/")
	if len(parts) < 2 {
		return nil
	}

	switch parts[0] {
	case "programs":
		if len(parts) >= 3 && parts[1] == "programs" {
			return &ProcedureID{
				Program:  strings.ToUpper(parts[2]),
				ProcType: "PROG",
				ProcName: "",
			}
		}
	case "oo":
		if len(parts) >= 3 && parts[1] == "classes" {
			className := strings.ToUpper(parts[2])
			// Check for method parameter
			if method, ok := params["method"].(string); ok && method != "" {
				return &ProcedureID{
					Program:  className,
					ProcType: "METH",
					ProcName: strings.ToUpper(method),
				}
			}
			// No method specified: return CLAS type for whole-class analysis
			return &ProcedureID{
				Program:  className,
				ProcType: "CLAS",
				ProcName: "",
			}
		}
		if len(parts) >= 3 && parts[1] == "interfaces" {
			ifName := strings.ToUpper(parts[2])
			if method, ok := params["method"].(string); ok && method != "" {
				return &ProcedureID{
					Program:  ifName,
					ProcType: "METH",
					ProcName: strings.ToUpper(method),
				}
			}
			return nil // Interfaces don't have implementations directly
		}
	case "functions":
		if len(parts) >= 5 && parts[1] == "groups" && parts[3] == "fmodules" {
			// Function module: program is SAPL<group>
			return &ProcedureID{
				Program:  "SAPL" + strings.ToUpper(parts[2]),
				ProcType: "FUNC",
				ProcName: strings.ToUpper(parts[4]),
			}
		}
	}

	return nil
}

// GetCallGraph retrieves the call graph for an ABAP object.
// Direction can be "callers" (who calls this) or "callees" (what this calls).
// For classes without a specific method, it aggregates callees from all methods.
func (c *Client) GetCallGraph(ctx context.Context, objectURI string, opts *CallGraphOptions) (*CallGraphNode, error) {
	if opts == nil {
		opts = &CallGraphOptions{
			Direction:  "callees",
			MaxDepth:   3,
			MaxResults: 100,
		}
	}

	// Build params for method specification
	params := map[string]any{}
	if opts.Method != "" {
		params["method"] = opts.Method
	}

	// Parse object URI to procedure ID
	procID := ParseObjectURIToProcID(objectURI, params)
	if procID == nil {
		return nil, fmt.Errorf("cannot parse object URI to procedure ID: %s", objectURI)
	}

	// Handle whole-class analysis by getting all methods
	if procID.ProcType == "CLAS" {
		return c.getClassCallGraph(ctx, procID.Program, opts)
	}

	return c.getSingleProcCallGraph(ctx, procID, opts)
}

// getClassCallGraph aggregates call graphs from all methods in a class.
func (c *Client) getClassCallGraph(ctx context.Context, className string, opts *CallGraphOptions) (*CallGraphNode, error) {
	// Get class components to find all methods
	classURL := "/sap/bc/adt/oo/classes/" + strings.ToLower(className)
	components, err := c.GetClassComponents(ctx, classURL)
	if err != nil {
		return nil, fmt.Errorf("getting class components for call graph: %w", err)
	}

	// Create root node for the class
	root := &CallGraphNode{
		URI:      classURL,
		Name:     className,
		Type:     "CLAS",
		Children: []CallGraphNode{},
	}

	// Recursively find all methods in the component tree
	var methods []string
	var findMethods func(comp *ClassComponent)
	findMethods = func(comp *ClassComponent) {
		if comp == nil {
			return
		}
		if comp.Type == "CLAS/OM" || comp.Type == "method" { // CLAS/OM = class method
			methods = append(methods, comp.Name)
		}
		for i := range comp.Components {
			findMethods(&comp.Components[i])
		}
	}
	findMethods(components)

	// Query callees for each method
	for _, methodName := range methods {
		methodProcID := &ProcedureID{
			Program:  className,
			ProcType: "METH",
			ProcName: strings.ToUpper(methodName),
		}

		methodGraph, err := c.getSingleProcCallGraph(ctx, methodProcID, opts)
		if err != nil {
			// Skip methods that fail (might not have callees)
			continue
		}
		if methodGraph != nil {
			// Add method node even if no children (shows method was analyzed)
			if methodGraph.Name == "" {
				methodGraph.Name = methodName
			}
			root.Children = append(root.Children, *methodGraph)
		}
	}

	return root, nil
}

// getSingleProcCallGraph retrieves call graph for a single procedure.
func (c *Client) getSingleProcCallGraph(ctx context.Context, procID *ProcedureID, opts *CallGraphOptions) (*CallGraphNode, error) {
	params := url.Values{}
	if opts.Direction != "" {
		params.Set("direction", opts.Direction)
	}
	if opts.MaxDepth > 0 {
		params.Set("maxDepth", fmt.Sprintf("%d", opts.MaxDepth))
	}
	if opts.MaxResults > 0 {
		params.Set("maxResults", fmt.Sprintf("%d", opts.MaxResults))
	}

	// Build request body with callGraphConfig structure
	// SAP ST: default namespace for elements, prefixed namespace for attributes
	procNameAttr := ""
	if procID.ProcName != "" {
		procNameAttr = fmt.Sprintf(` cg:procName="%s"`, procID.ProcName)
	}
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<callGraphConfig xmlns="http://www.sap.com/adt/cai/callgraphconfig"
    xmlns:cg="http://www.sap.com/adt/cai/callgraphconfig"
    cg:levelExternal="%d"
    cg:collapse="false">
  <roots>
    <procId cg:program="%s" cg:procType="%s"%s/>
  </roots>
</callGraphConfig>`, opts.MaxDepth, procID.Program, procID.ProcType, procNameAttr)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/cai/callgraph", &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Accept:      "application/vnd.sap.adt.cai.callgraph.v1+xml",
		ContentType: "application/vnd.sap.adt.cai.callgraphconfig.v1+xml",
		Body:        []byte(body),
	})
	if err != nil {
		return nil, fmt.Errorf("getting call graph: %w", err)
	}

	return parseCallGraphResponse(resp.Body)
}

// callGraphNodeResponse matches the ADT CAI callGraphNode response format.
type callGraphNodeResponse struct {
	ProcType  string `xml:"processingBlockType,attr"`
	ProcName  string `xml:"processingBlockName,attr"`
	NodeIndex struct {
		ID       int `xml:"id,attr"`
		CallerID int `xml:"callerId,attr"`
		OriginID int `xml:"originId,attr"`
	} `xml:"http://www.sap.com/adt/cai/callgraph nodeIndex"`
	CAIObject struct {
		URI         string `xml:"uri,attr"`
		Type        string `xml:"type,attr"`
		Name        string `xml:"name,attr"`
		Description string `xml:"description,attr"`
		FullName    string `xml:"fullName,attr"`
	} `xml:"http://www.sap.com/adt/cai/callgraph caiObject"`
}

// parseCallGraphResponse parses the call graph XML response.
func parseCallGraphResponse(data []byte) (*CallGraphNode, error) {
	type callGraphsXML struct {
		XMLName    xml.Name `xml:"http://www.sap.com/adt/cai/callgraph callGraphs"`
		CallGraphs []struct {
			Nodes []callGraphNodeResponse `xml:"http://www.sap.com/adt/cai/callgraph callGraphNode"`
		} `xml:"http://www.sap.com/adt/cai/callgraph callGraph"`
	}

	var cgs callGraphsXML
	if err := xml.Unmarshal(data, &cgs); err != nil {
		return nil, fmt.Errorf("parsing call graph: %w", err)
	}

	if len(cgs.CallGraphs) == 0 {
		return nil, fmt.Errorf("no call graph data in response")
	}
	if len(cgs.CallGraphs[0].Nodes) == 0 {
		return nil, fmt.Errorf("call graph contains no nodes")
	}

	// Create map of nodes by ID
	nodes := make(map[int]*CallGraphNode)
	var root *CallGraphNode

	for _, n := range cgs.CallGraphs[0].Nodes {
		node := &CallGraphNode{
			URI:         n.CAIObject.URI,
			Name:        n.CAIObject.Name,
			Type:        n.ProcType,
			Description: n.CAIObject.Description,
		}
		nodes[n.NodeIndex.ID] = node

		if n.NodeIndex.CallerID == 0 {
			root = node
		}
	}

	// Build parent-child relationships
	for _, n := range cgs.CallGraphs[0].Nodes {
		if n.NodeIndex.CallerID != 0 {
			if parent, ok := nodes[n.NodeIndex.CallerID]; ok {
				if child, ok := nodes[n.NodeIndex.ID]; ok {
					parent.Children = append(parent.Children, *child)
				}
			}
		}
	}

	return root, nil
}

// GetCallersOf returns who calls the specified object (up traversal).
// This is a convenience wrapper around GetCallGraph with direction="callers".
func (c *Client) GetCallersOf(ctx context.Context, objectURI string, maxDepth int) (*CallGraphNode, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}
	return c.GetCallGraph(ctx, objectURI, &CallGraphOptions{
		Direction:  "callers",
		MaxDepth:   maxDepth,
		MaxResults: 500,
	})
}

// GetCalleesOf returns what the specified object calls (down traversal).
// This is a convenience wrapper around GetCallGraph with direction="callees".
func (c *Client) GetCalleesOf(ctx context.Context, objectURI string, maxDepth int) (*CallGraphNode, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}
	return c.GetCallGraph(ctx, objectURI, &CallGraphOptions{
		Direction:  "callees",
		MaxDepth:   maxDepth,
		MaxResults: 500,
	})
}

// CallGraphEdge represents a single edge in the call graph.
type CallGraphEdge struct {
	CallerURI  string `json:"caller_uri"`
	CallerName string `json:"caller_name"`
	CalleeURI  string `json:"callee_uri"`
	CalleeName string `json:"callee_name"`
	Line       int    `json:"line,omitempty"`
}

// FlattenCallGraph converts a hierarchical call graph to a flat list of edges.
func FlattenCallGraph(root *CallGraphNode) []CallGraphEdge {
	var edges []CallGraphEdge
	if root == nil {
		return edges
	}

	var traverse func(parent *CallGraphNode)
	traverse = func(parent *CallGraphNode) {
		for _, child := range parent.Children {
			edges = append(edges, CallGraphEdge{
				CallerURI:  parent.URI,
				CallerName: parent.Name,
				CalleeURI:  child.URI,
				CalleeName: child.Name,
				Line:       child.Line,
			})
			childCopy := child
			traverse(&childCopy)
		}
	}
	traverse(root)
	return edges
}

// CallGraphStats provides statistics about a call graph.
type CallGraphStats struct {
	TotalNodes  int            `json:"total_nodes"`
	TotalEdges  int            `json:"total_edges"`
	MaxDepth    int            `json:"max_depth"`
	NodesByType map[string]int `json:"nodes_by_type"`
	UniqueNodes []string       `json:"unique_nodes"`
}

// AnalyzeCallGraph computes statistics for a call graph.
func AnalyzeCallGraph(root *CallGraphNode) *CallGraphStats {
	stats := &CallGraphStats{
		NodesByType: make(map[string]int),
	}
	if root == nil {
		return stats
	}

	seen := make(map[string]bool)
	var maxDepth int

	var traverse func(node *CallGraphNode, depth int)
	traverse = func(node *CallGraphNode, depth int) {
		if depth > maxDepth {
			maxDepth = depth
		}
		if !seen[node.URI] {
			seen[node.URI] = true
			stats.TotalNodes++
			stats.NodesByType[node.Type]++
			stats.UniqueNodes = append(stats.UniqueNodes, node.Name)
		}
		for _, child := range node.Children {
			stats.TotalEdges++
			childCopy := child
			traverse(&childCopy, depth+1)
		}
	}
	traverse(root, 0)
	stats.MaxDepth = maxDepth
	return stats
}

// CallGraphComparison compares static and actual call graphs.
type CallGraphComparison struct {
	CommonEdges   []CallGraphEdge `json:"common_edges"`   // In both static and actual
	StaticOnly    []CallGraphEdge `json:"static_only"`    // In static but not executed
	ActualOnly    []CallGraphEdge `json:"actual_only"`    // Executed but not in static (dynamic calls)
	CoverageRatio float64         `json:"coverage_ratio"` // Actual/Static ratio
}

// CompareCallGraphs compares a static call graph with an actual execution trace.
func CompareCallGraphs(staticEdges, actualEdges []CallGraphEdge) *CallGraphComparison {
	comp := &CallGraphComparison{}

	// Build lookup sets
	staticSet := make(map[string]CallGraphEdge)
	for _, e := range staticEdges {
		key := e.CallerName + "->" + e.CalleeName
		staticSet[key] = e
	}

	actualSet := make(map[string]CallGraphEdge)
	for _, e := range actualEdges {
		key := e.CallerName + "->" + e.CalleeName
		actualSet[key] = e
	}

	// Find common and static-only
	for key, edge := range staticSet {
		if _, ok := actualSet[key]; ok {
			comp.CommonEdges = append(comp.CommonEdges, edge)
		} else {
			comp.StaticOnly = append(comp.StaticOnly, edge)
		}
	}

	// Find actual-only (dynamic calls)
	for key, edge := range actualSet {
		if _, ok := staticSet[key]; !ok {
			comp.ActualOnly = append(comp.ActualOnly, edge)
		}
	}

	// Coverage ratio
	if len(staticEdges) > 0 {
		comp.CoverageRatio = float64(len(comp.CommonEdges)) / float64(len(staticEdges))
	}

	return comp
}

// ExtractCallEdgesFromTrace converts trace entries to call graph edges.
// It analyzes Program and Event fields to identify caller-callee relationships.
func ExtractCallEdgesFromTrace(entries []TraceEntry) []CallGraphEdge {
	var edges []CallGraphEdge
	seen := make(map[string]bool)

	// Group entries by program to detect call relationships
	var prevProgram string
	for _, entry := range entries {
		if entry.Program == "" {
			continue
		}

		// Event field contains call type info (PERFORM, CALL METHOD, etc.)
		// When program changes, we have a call edge
		if prevProgram != "" && prevProgram != entry.Program {
			edgeKey := prevProgram + "->" + entry.Program
			if !seen[edgeKey] {
				seen[edgeKey] = true
				edges = append(edges, CallGraphEdge{
					CallerURI:  "/sap/bc/adt/programs/programs/" + strings.ToLower(prevProgram),
					CallerName: prevProgram,
					CalleeURI:  "/sap/bc/adt/programs/programs/" + strings.ToLower(entry.Program),
					CalleeName: entry.Program,
					Line:       entry.Line,
				})
			}
		}
		prevProgram = entry.Program
	}

	return edges
}

// TraceExecutionResult contains the result of a traced execution.
type TraceExecutionResult struct {
	// Static call graph from code analysis
	StaticGraph *CallGraphNode `json:"static_graph,omitempty"`

	// Actual trace data from runtime
	Trace *TraceAnalysis `json:"trace,omitempty"`

	// Extracted call edges from trace
	ActualEdges []CallGraphEdge `json:"actual_edges,omitempty"`

	// Comparison between static and actual
	Comparison *CallGraphComparison `json:"comparison,omitempty"`

	// Statistics
	StaticStats *CallGraphStats `json:"static_stats,omitempty"`

	// Execution info
	ExecutedTests []string `json:"executed_tests,omitempty"`
	ExecutionTime int64    `json:"execution_time_us,omitempty"`
}

// TraceExecutionOptions configures traced execution.
type TraceExecutionOptions struct {
	// ObjectURI is the starting point for static call graph
	ObjectURI string

	// MaxDepth for static call graph traversal
	MaxDepth int

	// RunTests triggers unit tests before collecting trace
	RunTests bool

	// TestObjectURI specifies which object's tests to run
	TestObjectURI string

	// TraceUser filters traces by user (optional)
	TraceUser string
}

// TraceExecution performs a traced execution and compares actual vs static call graphs.
// This is the composite tool for RCA (Root Cause Analysis).
func (c *Client) TraceExecution(ctx context.Context, opts *TraceExecutionOptions) (*TraceExecutionResult, error) {
	result := &TraceExecutionResult{}

	// Step 1: Build static call graph (callees - what gets called from the starting point)
	if opts.ObjectURI != "" {
		depth := opts.MaxDepth
		if depth <= 0 {
			depth = 5
		}

		staticGraph, err := c.GetCalleesOf(ctx, opts.ObjectURI, depth)
		if err != nil {
			// Non-fatal: continue without static graph
			result.StaticGraph = nil
		} else {
			result.StaticGraph = staticGraph
			result.StaticStats = AnalyzeCallGraph(staticGraph)
		}
	}

	// Step 2: Run unit tests if requested (to trigger execution)
	if opts.RunTests && opts.TestObjectURI != "" {
		testResult, err := c.RunUnitTests(ctx, opts.TestObjectURI, nil)
		if err == nil && testResult != nil {
			// Collect test names that ran
			for _, tc := range testResult.Classes {
				for _, tm := range tc.TestMethods {
					result.ExecutedTests = append(result.ExecutedTests,
						fmt.Sprintf("%s=>%s", tc.Name, tm.Name))
				}
			}
		}
	}

	// Step 3: Get latest trace for user
	traceUser := opts.TraceUser
	if traceUser == "" {
		// Use current user from config
		traceUser = c.config.Username
	}

	traces, err := c.ListTraces(ctx, &TraceQueryOptions{
		User:       traceUser,
		MaxResults: 5,
	})
	if err == nil && len(traces) > 0 {
		// Get the most recent trace
		latestTrace := traces[0]

		// Get hitlist analysis
		analysis, err := c.GetTrace(ctx, latestTrace.ID, "hitlist")
		if err == nil {
			result.Trace = analysis
			result.ExecutionTime = analysis.TotalTime

			// Step 4: Extract actual call edges from trace
			result.ActualEdges = ExtractCallEdgesFromTrace(analysis.Entries)

			// Step 5: Compare static vs actual if we have both
			if result.StaticGraph != nil {
				staticEdges := FlattenCallGraph(result.StaticGraph)
				result.Comparison = CompareCallGraphs(staticEdges, result.ActualEdges)
			}
		}
	}

	return result, nil
}
