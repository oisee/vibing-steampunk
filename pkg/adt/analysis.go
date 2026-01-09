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
}

// GetCallGraph retrieves the call graph for an ABAP object.
// Direction can be "callers" (who calls this) or "callees" (what this calls).
func (c *Client) GetCallGraph(ctx context.Context, objectURI string, opts *CallGraphOptions) (*CallGraphNode, error) {
	if opts == nil {
		opts = &CallGraphOptions{
			Direction:  "callees",
			MaxDepth:   3,
			MaxResults: 100,
		}
	}

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

	// Build request body with object URI
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<cai:callGraphRequest xmlns:cai="http://www.sap.com/adt/cai">
  <cai:objectUri>%s</cai:objectUri>
</cai:callGraphRequest>`, objectURI)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/cai/callgraph", &RequestOptions{
		Method:      http.MethodPost,
		Query:       params,
		Accept:      "application/xml",
		ContentType: "application/xml",
		Body:        []byte(body),
	})
	if err != nil {
		return nil, fmt.Errorf("getting call graph: %w", err)
	}

	return parseCallGraphResponse(resp.Body)
}

// callGraphNodeXML is used for parsing call graph XML responses.
type callGraphNodeXML struct {
	URI         string             `xml:"uri,attr"`
	Name        string             `xml:"name,attr"`
	Type        string             `xml:"type,attr"`
	Description string             `xml:"description,attr"`
	Line        int                `xml:"line,attr"`
	Column      int                `xml:"column,attr"`
	Children    []callGraphNodeXML `xml:"node"`
}

// parseCallGraphResponse parses the call graph XML response.
func parseCallGraphResponse(data []byte) (*CallGraphNode, error) {
	type callGraphXML struct {
		XMLName xml.Name         `xml:"callGraph"`
		Root    callGraphNodeXML `xml:"node"`
	}

	var cg callGraphXML
	if err := xml.Unmarshal(data, &cg); err != nil {
		return nil, fmt.Errorf("parsing call graph: %w", err)
	}

	return convertCallGraphNode(&cg.Root), nil
}

func convertCallGraphNode(n *callGraphNodeXML) *CallGraphNode {
	if n == nil {
		return nil
	}
	node := &CallGraphNode{
		URI:         n.URI,
		Name:        n.Name,
		Type:        n.Type,
		Description: n.Description,
		Line:        n.Line,
		Column:      n.Column,
	}
	for _, child := range n.Children {
		childCopy := child
		node.Children = append(node.Children, *convertCallGraphNode(&childCopy))
	}
	return node
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
