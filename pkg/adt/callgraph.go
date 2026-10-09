package adt

import (
	"context"
	"fmt"
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
	// Unsearched names what could not be read while building this node. A
	// child list is only as complete as the sources behind it, and an empty
	// or short one means nothing until you know whether a source was missing.
	Unsearched []Unsearched `json:"unsearched,omitempty"`
}

// CallGraphOptions configures call graph retrieval.
type CallGraphOptions struct {
	Direction string // "callers" or "callees"
	// MaxDepth is accepted and ignored. Both sources CallGraph reads are one
	// hop by construction — see the note on CallGraph in callees.go — and a
	// field that quietly does nothing is better than one that suggests a
	// traversal happened.
	MaxDepth   int
	MaxResults int // Maximum results to return
}

// The three methods that used to stand here — GetCallGraph, GetCallersOf and
// GetCalleesOf — asked /sap/bc/adt/cai/callgraph, and that resource does not
// exist. It is in the discovery document of none of 7.50, 7.57 and 7.58 and
// answers 404 "No suitable resource found" in both directions, checked with a
// CSRF token in hand so it is the resource that is missing and not the
// request. Everything built on them therefore reported that no object calls
// anything, on every system, silently — which is the worst way for a
// dependency query to be wrong.
//
// They are deleted rather than deprecated so that nothing can build on them
// again. What replaces them is in callees.go: WhereUsed for the up direction
// (the where-used list behind SE84), Callees for the down direction (the
// CROSS and WBCROSSGT cross-reference tables), and CallGraph over the two for
// callers who want the node shape below.

// CallGraphEdge represents a single edge in the call graph.
type CallGraphEdge struct {
	CallerURI  string `json:"caller_uri"`
	CallerName string `json:"caller_name"`
	CalleeURI  string `json:"callee_uri"`
	CalleeName string `json:"callee_name"`
	// CalleeKind is what the cross-reference row said the callee is — method,
	// function module, type, data. It decides whether an edge is something that
	// could execute, which a coverage figure has to know: a reference to a type
	// is not a path anything could take.
	CalleeKind string `json:"callee_kind,omitempty"`
	Line       int    `json:"line,omitempty"`
}

// IsExecutableKind reports whether a callee kind is something a run could
// actually reach. The vocabulary is wbCrossKind's and crossKind's, kept in one
// place so a coverage figure and a callee list cannot drift apart.
func IsExecutableKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "method", "function module", "report", "transaction", "subroutine", "program", "dialog module":
		return true
	default:
		return false
	}
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
				CalleeKind: child.Type,
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

	// Keyed by identity, not by URI. The callee side builds children from
	// cross-reference rows, which name an object but carry no ADT path, so every
	// child arrived with URI "" — and a dedup on that key folded all of them
	// into one. The result was a graph reporting two nodes beside its own list
	// of twenty-seven edges, in an answer that showed both.
	key := func(n *CallGraphNode) string {
		if n.URI != "" {
			return "uri:" + n.URI
		}
		return "name:" + n.Type + ":" + n.Name
	}

	var traverse func(node *CallGraphNode, depth int)
	traverse = func(node *CallGraphNode, depth int) {
		if depth > maxDepth {
			maxDepth = depth
		}
		if k := key(node); !seen[k] {
			seen[k] = true
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
	CommonEdges []CallGraphEdge `json:"common_edges"` // In both static and actual
	StaticOnly  []CallGraphEdge `json:"static_only"`  // In static but not executed
	ActualOnly  []CallGraphEdge `json:"actual_only"`  // Executed but not in static (dynamic calls)
	// CoverageRatio is executed invocations over recorded invocations, or -1
	// when nothing callable was recorded at all — which is not the same as
	// nothing having run.
	CoverageRatio float64 `json:"coverage_ratio"`
	// ExecutableEdges is how many of the static edges were invocations rather
	// than type or data references. The denominator, stated so a reader can
	// see what the ratio is of.
	ExecutableEdges int `json:"executable_edges"`
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
	// Only the edges something could execute count towards coverage. Most of a
	// class's static edges are type and data references — ABAP_BOOL, SYST,
	// TADIR — and counting those as paths that were never taken produced
	// figures like 0.037 for a run that exercised everything callable.
	executable := 0
	for _, e := range staticEdges {
		if IsExecutableKind(e.CalleeKind) {
			executable++
		}
	}
	comp.ExecutableEdges = executable
	commonExecutable := 0
	for _, e := range comp.CommonEdges {
		if IsExecutableKind(e.CalleeKind) {
			commonExecutable++
		}
	}
	if executable > 0 {
		comp.CoverageRatio = float64(commonExecutable) / float64(executable)
	} else if len(staticEdges) > 0 {
		// Nothing callable was recorded, so there is no coverage to report. A
		// zero here would read as "none of it ran".
		comp.CoverageRatio = -1
	}

	return comp
}

// ExtractCallEdgesFromTrace converts trace entries to call graph edges. A
// hit-list or call-tree entry names both its calling and its called program,
// so each entry whose two differ is one edge. Edges are between programs, as
// they were before: a call inside one class pool yields none.
func ExtractCallEdgesFromTrace(entries []TraceEntry) []CallGraphEdge {
	var edges []CallGraphEdge
	seen := make(map[string]bool)

	for _, entry := range entries {
		caller, callerURI := traceProgramRef(entry.Program)
		callee, calleeURI := traceProgramRef(entry.CalledProgram)
		if caller == "" || callee == "" || caller == callee {
			continue
		}
		edgeKey := caller + "->" + callee
		if seen[edgeKey] {
			continue
		}
		seen[edgeKey] = true
		edges = append(edges, CallGraphEdge{
			CallerURI:  callerURI,
			CallerName: caller,
			CalleeURI:  calleeURI,
			CalleeName: callee,
			Line:       entry.Line,
		})
	}

	return edges
}

// traceProgramRef turns a trace's main program (ZCL_X=====CP, ZREPORT) into
// a name and an ADT URI.
func traceProgramRef(mainProgram string) (string, string) {
	name, kind, _ := strings.Cut(mainProgram, "=")
	if name == "" {
		return "", ""
	}
	if strings.HasSuffix(strings.TrimLeft(kind, "="), "CP") {
		return name, "/sap/bc/adt/oo/classes/" + strings.ToLower(name)
	}
	return name, "/sap/bc/adt/programs/programs/" + strings.ToLower(name)
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

	// Unsearched names the steps that did not run. Every field above is
	// omitempty, so a run where nothing worked marshals to almost nothing and
	// reads as "there was nothing to report". Comparison is the point of this
	// call, and it is absent both when static and actual agree and when the
	// trace never arrived.
	Unsearched []Unsearched `json:"unsearched,omitempty"`

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

	// Step 1: Build static call graph (callees - what gets called from the
	// starting point). One hop, from the cross-reference tables: the recursive
	// resource this used to ask does not exist, and the comparison below only
	// needs the edges leaving the object under trace.
	if opts.ObjectURI != "" {
		staticGraph, err := c.CallGraph(ctx, opts.ObjectURI, &CallGraphOptions{
			Direction:  "callees",
			MaxResults: 500,
		})
		if err != nil {
			// Non-fatal for the run, but not for the reader: without the static
			// half there is nothing to compare the trace against, and the
			// comparison is simply absent rather than wrong.
			result.StaticGraph = nil
			result.Unsearched = append(result.Unsearched, Unsearched{
				Object: "static call graph", Reason: err.Error()})
		} else {
			result.StaticGraph = staticGraph
			result.StaticStats = AnalyzeCallGraph(staticGraph)
		}
	}

	// Step 2: Run unit tests if requested (to trigger execution)
	if opts.RunTests && opts.TestObjectURI != "" {
		testResult, err := c.RunUnitTests(ctx, opts.TestObjectURI, nil)
		if err != nil || testResult == nil {
			// The tests were asked for in order to make something run. If they
			// did not, the trace below is of whatever else happened to execute,
			// which is not what the caller asked to see.
			result.Unsearched = append(result.Unsearched, Unsearched{
				Object: "unit tests " + opts.TestObjectURI, Reason: errOrEmpty(err, "no test result came back")})
		} else {
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
	if err != nil || len(traces) == 0 {
		// No trace means no actual edges, so the comparison cannot be made.
		// Saying so is the difference between "the code ran as predicted" and
		// "nobody looked".
		result.Unsearched = append(result.Unsearched, Unsearched{
			Object: "runtime trace for " + traceUser,
			Reason: errOrEmpty(err, "no trace was recorded for this user; the code under trace may not have run")})
	} else {
		// Get the most recent trace
		latestTrace := traces[0]

		// Get hitlist analysis
		analysis, err := c.GetTrace(ctx, latestTrace.ID, "hitlist")
		switch {
		case err != nil:
			result.Unsearched = append(result.Unsearched, Unsearched{
				Object: "trace " + latestTrace.ID, Reason: err.Error()})
		case analysis.Note != "" && len(analysis.Entries) == 0:
			// Nothing parsed is not nothing ran, so nothing is compared.
			result.Trace = analysis
			result.Unsearched = append(result.Unsearched, Unsearched{
				Object: "trace " + latestTrace.ID, Reason: analysis.Note})
		default:
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
