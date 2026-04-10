package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
)

// handleCheckBoundaries performs package boundary analysis.
// It can work in two modes:
//   - Online (with SAP): reads source from SAP, enriches with TADIR packages
//   - Offline (parser-only): analyzes provided source code
func (s *Server) handleCheckBoundaries(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	pkg := getStringParam(args, "package")
	objectName := getStringParam(args, "object")
	whitelistStr := getStringParam(args, "whitelist")
	format := getStringParam(args, "format")
	sourceCode := getStringParam(args, "source")
	depth := 1
	if d, ok := args["depth"].(float64); ok && d > 0 {
		depth = int(d)
	}

	if pkg == "" && objectName == "" && sourceCode == "" {
		return newToolResultError("Provide 'package', 'object', or 'source' parameter"), nil
	}

	// Parse whitelist
	var whitelist []string
	if whitelistStr != "" {
		for _, w := range strings.Split(whitelistStr, ",") {
			w = strings.TrimSpace(w)
			if w != "" {
				whitelist = append(whitelist, w)
			}
		}
	}

	g := graph.New()

	// Mode 1: Offline parser analysis (source code provided directly)
	if sourceCode != "" {
		nodeID := graph.NodeID("PROG", "SOURCE")
		if objectName != "" {
			nodeID = graph.NodeID("PROG", objectName)
		}

		g.AddNode(&graph.Node{
			ID:      nodeID,
			Name:    strings.ToUpper(objectName),
			Type:    "PROG",
			Package: strings.ToUpper(pkg),
		})

		edges := graph.ExtractDepsFromSource(sourceCode, nodeID)
		dynEdges := graph.ExtractDynamicCalls(sourceCode, nodeID)
		for _, e := range append(edges, dynEdges...) {
			g.AddEdge(e)
			// Add target node (package unknown without SAP)
			g.AddNode(&graph.Node{
				ID:   e.To,
				Name: strings.SplitN(e.To, ":", 2)[1],
				Type: strings.SplitN(e.To, ":", 2)[0],
			})
		}

		// If we have SAP connection, resolve packages via TADIR
		if s.adtClient != nil && pkg != "" {
			s.resolvePackages(ctx, g)
		}

		report := g.CheckBoundaries(pkg, &graph.BoundaryOptions{
			Whitelist:      whitelist,
			IncludeDynamic: true,
			IncludeStandard: format == "full",
		})

		return formatBoundaryResult(report, format)
	}

	// Mode 2: Online — read source from SAP for a specific object
	if objectName != "" && s.adtClient != nil {
		source, err := s.adtClient.GetSource(ctx, "", objectName, nil)
		if err != nil {
			return newToolResultError(fmt.Sprintf("Failed to read source for %s: %v", objectName, err)), nil
		}

		// Determine object type and package from SAP
		objPkg := pkg
		objType := "PROG"

		nodeID := graph.NodeID(objType, objectName)
		g.AddNode(&graph.Node{
			ID:      nodeID,
			Name:    strings.ToUpper(objectName),
			Type:    objType,
			Package: strings.ToUpper(objPkg),
		})

		edges := graph.ExtractDepsFromSource(source, nodeID)
		dynEdges := graph.ExtractDynamicCalls(source, nodeID)
		for _, e := range append(edges, dynEdges...) {
			g.AddEdge(e)
			g.AddNode(&graph.Node{
				ID:   e.To,
				Name: strings.SplitN(e.To, ":", 2)[1],
				Type: strings.SplitN(e.To, ":", 2)[0],
			})
		}

		// Resolve packages
		s.resolvePackages(ctx, g)

		// Use resolved package if not provided
		if objPkg == "" {
			if n := g.GetNode(nodeID); n != nil && n.Package != "" {
				objPkg = n.Package
			}
		}

		report := g.CheckBoundaries(objPkg, &graph.BoundaryOptions{
			Whitelist:      whitelist,
			IncludeDynamic: true,
			IncludeStandard: format == "full",
		})

		return formatBoundaryResult(report, format)
	}

	// Mode 3: Package-level analysis — read all objects in package
	if pkg != "" && s.adtClient != nil {
		// Get package contents
		pkgContent, err := s.adtClient.GetPackage(ctx, pkg)
		if err != nil {
			return newToolResultError(fmt.Sprintf("Failed to list package %s: %v", pkg, err)), nil
		}

		maxObjects := 50
		if depth > 1 {
			maxObjects = 20
		}
		count := 0

		for _, obj := range pkgContent.Objects {
			if count >= maxObjects {
				break
			}

			// Only analyze source-bearing objects
			objType := strings.ToUpper(obj.Type)
			if objType != "CLAS" && objType != "PROG" && objType != "FUGR" && objType != "INTF" {
				continue
			}

			source, err := s.adtClient.GetSource(ctx, "", obj.Name, nil)
			if err != nil {
				continue // Skip unreadable objects
			}

			nodeID := graph.NodeID(objType, obj.Name)
			g.AddNode(&graph.Node{
				ID:      nodeID,
				Name:    obj.Name,
				Type:    objType,
				Package: strings.ToUpper(pkg),
			})

			edges := graph.ExtractDepsFromSource(source, nodeID)
			dynEdges := graph.ExtractDynamicCalls(source, nodeID)
			for _, e := range append(edges, dynEdges...) {
				g.AddEdge(e)
				g.AddNode(&graph.Node{
					ID:   e.To,
					Name: strings.SplitN(e.To, ":", 2)[1],
					Type: strings.SplitN(e.To, ":", 2)[0],
				})
			}
			count++
		}

		// Resolve packages for all nodes
		s.resolvePackages(ctx, g)

		report := g.CheckBoundaries(strings.ToUpper(pkg), &graph.BoundaryOptions{
			Whitelist:      whitelist,
			IncludeDynamic: true,
			IncludeStandard: format == "full",
		})

		return formatBoundaryResult(report, format)
	}

	return newToolResultError("SAP connection required for online analysis. Provide 'source' for offline mode."), nil
}

// resolvePackages queries TADIR to fill in missing package info for nodes.
func (s *Server) resolvePackages(ctx context.Context, g *graph.Graph) {
	// Collect nodes without packages
	var names []string
	nodesByName := make(map[string][]*graph.Node)

	for _, n := range g.Nodes() {
		if n.Package == "" && !graph.IsStandardObject(n.Name) && !strings.HasPrefix(n.ID, "DYNAMIC:") {
			names = append(names, n.Name)
			nodesByName[strings.ToUpper(n.Name)] = append(nodesByName[strings.ToUpper(n.Name)], n)
		}
	}

	if len(names) == 0 {
		return
	}

	// Batch query TADIR (up to 100 at a time)
	batchSize := 100
	for i := 0; i < len(names); i += batchSize {
		end := i + batchSize
		if end > len(names) {
			end = len(names)
		}
		batch := names[i:end]

		// Build IN clause
		quoted := make([]string, len(batch))
		for j, n := range batch {
			quoted[j] = "'" + strings.ToUpper(n) + "'"
		}
		inClause := strings.Join(quoted, ",")

		query := fmt.Sprintf("SELECT obj_name, devclass FROM tadir WHERE pgmid = 'R3TR' AND obj_name IN (%s)", inClause)

		result, err := s.adtClient.RunQuery(ctx, query, 0)
		if err != nil {
			continue // Best effort
		}

		// Parse result and update nodes
		for _, row := range result.Rows {
			objName := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"])))
			devclass := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["DEVCLASS"])))
			if nodes, ok := nodesByName[objName]; ok {
				for _, n := range nodes {
					if n.Package == "" {
						n.Package = devclass
					}
				}
			}
		}
	}
}

// handleGraphStats returns statistics about the current in-memory graph.
func (s *Server) handleGraphStats(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// For now, build a fresh graph from provided source
	args := request.GetArguments()
	sourceCode := getStringParam(args, "source")

	if sourceCode == "" {
		return newToolResultError("Provide 'source' parameter with ABAP source code"), nil
	}

	g := graph.New()
	nodeID := graph.NodeID("PROG", "SOURCE")
	g.AddNode(&graph.Node{ID: nodeID, Name: "SOURCE", Type: "PROG"})

	edges := graph.ExtractDepsFromSource(sourceCode, nodeID)
	dynEdges := graph.ExtractDynamicCalls(sourceCode, nodeID)
	for _, e := range append(edges, dynEdges...) {
		g.AddEdge(e)
		g.AddNode(&graph.Node{
			ID:   e.To,
			Name: strings.SplitN(e.To, ":", 2)[1],
			Type: strings.SplitN(e.To, ":", 2)[0],
		})
	}

	stats := g.Stats()
	result, _ := json.MarshalIndent(stats, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

// handleCoChange performs transport-based co-change analysis.
// MCP: SAP(action="analyze", params={"type": "co_change", "object_type": "CLAS", "object_name": "ZCL_FOO", "top_n": 20})
func (s *Server) handleCoChange(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	objType := strings.ToUpper(getStringParam(args, "object_type"))
	objName := strings.ToUpper(getStringParam(args, "object_name"))
	topN := 20
	if t, ok := getFloatParam(args, "top_n"); ok && t > 0 {
		topN = int(t)
	}

	if objType == "" || objName == "" {
		return newToolResultError("object_type and object_name are required. Example: SAP(action=\"analyze\", params={\"type\": \"co_change\", \"object_type\": \"CLAS\", \"object_name\": \"ZCL_FOO\"})"), nil
	}
	if s.adtClient == nil {
		return newToolResultError("SAP connection required for co-change analysis"), nil
	}

	headers, objects, err := s.fetchTransportData(ctx, objType, objName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("co_change failed: %v", err)), nil
	}
	if len(headers) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No transports found for %s %s.", objType, objName)), nil
	}

	g := graph.BuildTransportGraph(headers, objects)
	targetNodeID := graph.NodeID(objType, objName)
	result := graph.WhatChangesWith(g, targetNodeID, topN)

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("JSON marshal error: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// fetchTransportData performs bounded E070/E071 acquisition for a single object.
// Returns headers and objects suitable for graph.BuildTransportGraph.
// Acquisition steps:
//  1. E071: find transports containing the target object
//  2. E070: resolve task→request hierarchy for those transports
//  3. E070: fetch parent request headers if only tasks were found
//  4. E070: fetch ALL child tasks of resolved parent requests
//  5. E071: fetch all objects across parent requests + all child tasks
func (s *Server) fetchTransportData(ctx context.Context, objType, objName string) ([]graph.TransportHeader, []graph.TransportObject, error) {
	// Step 1: Find transports containing this object
	e071Query := fmt.Sprintf(
		"SELECT TRKORR, PGMID, OBJECT, OBJ_NAME FROM E071 WHERE PGMID = 'R3TR' AND OBJECT = '%s' AND OBJ_NAME = '%s'",
		objType, objName)
	e071Result, err := s.adtClient.RunQuery(ctx, e071Query, 200)
	if err != nil {
		return nil, nil, fmt.Errorf("E071 query: %w", err)
	}
	if e071Result == nil || len(e071Result.Rows) == 0 {
		return nil, nil, nil
	}

	// Collect transport numbers
	trNums := make(map[string]bool)
	for _, row := range e071Result.Rows {
		tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
		if tr != "" {
			trNums[tr] = true
		}
	}

	// Step 2: Resolve E070 headers
	trList := quoteKeys(trNums)
	e070Query := fmt.Sprintf(
		"SELECT TRKORR, STRKORR, TRFUNCTION, TRSTATUS, AS4USER, AS4DATE FROM E070 WHERE TRKORR IN (%s)",
		strings.Join(trList, ","))
	e070Result, err := s.adtClient.RunQuery(ctx, e070Query, 500)
	if err != nil {
		return nil, nil, fmt.Errorf("E070 query: %w", err)
	}

	var headers []graph.TransportHeader
	requestNums := make(map[string]bool)
	headerSeen := make(map[string]bool)

	for _, row := range e070Result.Rows {
		h := parseTransportHeader(row)
		headers = append(headers, h)
		headerSeen[h.TRKORR] = true
		if h.IsRequest() {
			requestNums[h.TRKORR] = true
		} else if h.STRKORR != "" {
			requestNums[h.STRKORR] = true
		}
	}

	// Step 2b: CR-level expansion via E070A attribute (if configured)
	// When a transport attribute is set, find all TRs sharing the same
	// attribute value (CR ID), expanding the co-change boundary beyond
	// a single transport request to the full change request scope.
	if attr := s.config.TransportAttribute; attr != "" && len(requestNums) > 0 {
		reqList := quoteKeys(requestNums)
		attrQuery := fmt.Sprintf(
			"SELECT TRKORR, REFERENCE FROM E070A WHERE ATTRIBUTE = '%s' AND TRKORR IN (%s)",
			attr, strings.Join(reqList, ","))
		attrResult, err := s.adtClient.RunQuery(ctx, attrQuery, 500)
		if err == nil && attrResult != nil {
			// Collect all CR references for our transports
			crRefs := make(map[string]bool)
			for _, row := range attrResult.Rows {
				ref := strings.TrimSpace(fmt.Sprintf("%v", row["REFERENCE"]))
				if ref != "" {
					crRefs[ref] = true
				}
			}
			// Find sibling TRs that share the same CR references
			if len(crRefs) > 0 {
				refList := make([]string, 0, len(crRefs))
				for ref := range crRefs {
					refList = append(refList, "'"+ref+"'")
				}
				siblingAttrQuery := fmt.Sprintf(
					"SELECT TRKORR FROM E070A WHERE ATTRIBUTE = '%s' AND REFERENCE IN (%s)",
					attr, strings.Join(refList, ","))
				siblingAttrResult, err := s.adtClient.RunQuery(ctx, siblingAttrQuery, 1000)
				if err == nil && siblingAttrResult != nil {
					// Resolve these new TRs through E070 to get headers + parent mapping
					newTRs := make(map[string]bool)
					for _, row := range siblingAttrResult.Rows {
						tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
						if tr != "" && !trNums[tr] {
							newTRs[tr] = true
						}
					}
					if len(newTRs) > 0 {
						newTRList := quoteKeys(newTRs)
						crE070Query := fmt.Sprintf(
							"SELECT TRKORR, STRKORR, TRFUNCTION, TRSTATUS, AS4USER, AS4DATE FROM E070 WHERE TRKORR IN (%s)",
							strings.Join(newTRList, ","))
						crE070Result, err := s.adtClient.RunQuery(ctx, crE070Query, 500)
						if err == nil && crE070Result != nil {
							for _, row := range crE070Result.Rows {
								h := parseTransportHeader(row)
								if !headerSeen[h.TRKORR] {
									headers = append(headers, h)
									headerSeen[h.TRKORR] = true
									trNums[h.TRKORR] = true
								}
								if h.IsRequest() {
									requestNums[h.TRKORR] = true
								} else if h.STRKORR != "" {
									requestNums[h.STRKORR] = true
								}
							}
						}
					}
				}
			}
		}
	}

	// Step 3: Fetch missing parent request headers
	var missingParents []string
	for rn := range requestNums {
		if !headerSeen[rn] {
			missingParents = append(missingParents, "'"+rn+"'")
		}
	}
	if len(missingParents) > 0 {
		parentQuery := fmt.Sprintf(
			"SELECT TRKORR, STRKORR, TRFUNCTION, TRSTATUS, AS4USER, AS4DATE FROM E070 WHERE TRKORR IN (%s)",
			strings.Join(missingParents, ","))
		parentResult, err := s.adtClient.RunQuery(ctx, parentQuery, 100)
		if err == nil && parentResult != nil {
			for _, row := range parentResult.Rows {
				h := parseTransportHeader(row)
				if !headerSeen[h.TRKORR] {
					headers = append(headers, h)
					headerSeen[h.TRKORR] = true
					requestNums[h.TRKORR] = true
				}
			}
		}
	}

	// Step 4: Fetch ALL child tasks of resolved parent requests
	if len(requestNums) > 0 {
		parentList := quoteKeys(requestNums)
		childQuery := fmt.Sprintf(
			"SELECT TRKORR, STRKORR, TRFUNCTION, TRSTATUS, AS4USER, AS4DATE FROM E070 WHERE STRKORR IN (%s)",
			strings.Join(parentList, ","))
		childResult, err := s.adtClient.RunQuery(ctx, childQuery, 500)
		if err == nil && childResult != nil {
			for _, row := range childResult.Rows {
				h := parseTransportHeader(row)
				if !headerSeen[h.TRKORR] {
					headers = append(headers, h)
					headerSeen[h.TRKORR] = true
				}
			}
		}
	}

	// Step 5: Fetch all E071 objects for all known transports
	allTRNums := make(map[string]bool)
	for _, h := range headers {
		allTRNums[h.TRKORR] = true
	}
	for rn := range requestNums {
		allTRNums[rn] = true
	}

	allTRList := quoteKeys(allTRNums)
	siblingQuery := fmt.Sprintf(
		"SELECT TRKORR, PGMID, OBJECT, OBJ_NAME FROM E071 WHERE TRKORR IN (%s) AND PGMID = 'R3TR'",
		strings.Join(allTRList, ","))
	siblingResult, err := s.adtClient.RunQuery(ctx, siblingQuery, 2000)
	if err != nil {
		return nil, nil, fmt.Errorf("sibling E071 query: %w", err)
	}

	var objects []graph.TransportObject
	if siblingResult != nil {
		for _, row := range siblingResult.Rows {
			objects = append(objects, graph.TransportObject{
				TRKORR:  strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"])),
				PGMID:   strings.TrimSpace(fmt.Sprintf("%v", row["PGMID"])),
				Object:  strings.TrimSpace(fmt.Sprintf("%v", row["OBJECT"])),
				ObjName: strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"])),
			})
		}
	}

	return headers, objects, nil
}

// parseTransportHeader extracts a TransportHeader from a RunQuery row.
func parseTransportHeader(row map[string]any) graph.TransportHeader {
	return graph.TransportHeader{
		TRKORR:     strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"])),
		STRKORR:    strings.TrimSpace(fmt.Sprintf("%v", row["STRKORR"])),
		TRFUNCTION: strings.TrimSpace(fmt.Sprintf("%v", row["TRFUNCTION"])),
		TRSTATUS:   strings.TrimSpace(fmt.Sprintf("%v", row["TRSTATUS"])),
		AS4USER:    strings.TrimSpace(fmt.Sprintf("%v", row["AS4USER"])),
		AS4DATE:    strings.TrimSpace(fmt.Sprintf("%v", row["AS4DATE"])),
	}
}

// quoteKeys converts a set of keys to SQL-quoted strings for IN clauses.
func quoteKeys(m map[string]bool) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, "'"+k+"'")
	}
	return result
}

// handleImpact performs reverse-dependency impact analysis using WBCROSSGT/CROSS
// with optional parser-based source augmentation.
//
// MCP: SAP(action="analyze", params={"type": "impact", "object_type": "CLAS", "object_name": "ZCL_FOO"})
// Optional params:
//   - "max_depth": int (default 3, max 5)
//   - "edge_kinds": "CALLS,REFERENCES,CO_TRANSPORTED" (comma-separated filter)
//   - "include_source_analysis": bool (default false) — augment with parser edges
//   - "include_co_change": bool (default false) — augment with transport co-change edges
//
// Data sources:
//   - WBCROSSGT + CROSS tables: reverse cross-references (backbone, always used)
//   - Parser/source analysis (when include_source_analysis=true): fetches source of
//     discovered objects and runs ExtractDepsFromSource to find edges that CROSS tables
//     miss (PERFORM IN PROGRAM, local include refs, static method calls within same include).
//     Dynamic calls (CALL FUNCTION variable) are flagged as DYNAMIC_CALL but do NOT
//     extend the frontier since targets are unresolved.
//   - Transport co-change (when include_co_change=true): fetches transport history,
//     materializes CO_TRANSPORTED edges between objects shipped together. Surfaces
//     workbench↔customizing correlations invisible to cross-reference tables.
//     When transport_attribute is configured, expands to CR-level correlation via E070A.
func (s *Server) handleImpact(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	objType := strings.ToUpper(getStringParam(args, "object_type"))
	objName := strings.ToUpper(getStringParam(args, "object_name"))
	maxDepth := 3
	if d, ok := getFloatParam(args, "max_depth"); ok && d > 0 {
		maxDepth = int(d)
	}
	if maxDepth > 5 {
		maxDepth = 5 // safety bound
	}
	includeSourceAnalysis := false
	if v, ok := getBoolParam(args, "include_source_analysis"); ok {
		includeSourceAnalysis = v
	}
	includeCoChange := false
	if v, ok := getBoolParam(args, "include_co_change"); ok {
		includeCoChange = v
	}

	if objType == "" || objName == "" {
		return newToolResultError("object_type and object_name are required. Example: SAP(action=\"analyze\", params={\"type\": \"impact\", \"object_type\": \"CLAS\", \"object_name\": \"ZCL_FOO\"})"), nil
	}
	if s.adtClient == nil {
		return newToolResultError("SAP connection required for impact analysis"), nil
	}

	// Parse optional edge kind filter
	var edgeKinds []graph.EdgeKind
	if kindsStr := getStringParam(args, "edge_kinds"); kindsStr != "" {
		for _, k := range strings.Split(kindsStr, ",") {
			k = strings.TrimSpace(k)
			if k != "" {
				edgeKinds = append(edgeKinds, graph.EdgeKind(k))
			}
		}
	}

	// Build reverse-dependency graph via multi-hop WBCROSSGT/CROSS acquisition
	g, err := s.fetchReverseDeps(ctx, objType, objName, maxDepth)
	if err != nil {
		return newToolResultError(fmt.Sprintf("impact query failed: %v", err)), nil
	}

	// Optional: parser-based source augmentation
	if includeSourceAnalysis {
		s.augmentGraphWithParser(ctx, g)
	}

	// Optional: co-change augmentation via transport history
	// Merges transport graph into the structural graph and materializes
	// CO_TRANSPORTED edges — weaker than CALLS/REFERENCES but surfaces
	// workbench↔customizing correlations that cross-reference tables miss.
	if includeCoChange {
		s.augmentGraphWithCoChange(ctx, g, objType, objName)
	}

	targetNodeID := graph.NodeID(objType, objName)
	opts := &graph.ImpactOptions{
		MaxDepth:  maxDepth,
		EdgeKinds: edgeKinds,
	}
	result := graph.Impact(g, targetNodeID, opts)

	// Enrich with package info
	s.resolvePackages(ctx, g)
	// Re-read package info into results after resolution
	for i, entry := range result.Entries {
		if n := g.GetNode(entry.NodeID); n != nil && n.Package != "" {
			result.Entries[i].Package = n.Package
		}
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("JSON marshal error: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// augmentGraphWithParser fetches source for source-bearing objects already in the graph
// and adds parser-derived edges. This catches gaps that CROSS/WBCROSSGT miss:
//   - PERFORM sub IN PROGRAM prog (cross-program call within same include)
//   - Static method calls within the same include (no cross-include boundary)
//   - INCLUDE statements
//   - Dynamic calls (flagged as DYNAMIC_CALL, not treated as resolved dependencies)
//
// Only objects with source-bearing types (CLAS, PROG, INTF, FUGR) are fetched.
// Parser edges are additive: they never remove or contradict CROSS/WBCROSSGT edges.
// New target nodes discovered by the parser are added to the graph but do NOT
// trigger further CROSS/WBCROSSGT expansion (that's the backbone's job).
func (s *Server) augmentGraphWithParser(ctx context.Context, g *graph.Graph) {
	// Collect source-bearing nodes already in the graph
	type sourceTarget struct {
		nodeID  string
		objType string
		objName string
	}
	var targets []sourceTarget

	for _, n := range g.Nodes() {
		switch n.Type {
		case "CLAS", "PROG", "INTF", "FUGR":
			targets = append(targets, sourceTarget{n.ID, n.Type, n.Name})
		}
	}

	// Limit to avoid excessive source fetches
	maxSourceFetches := 30
	if len(targets) > maxSourceFetches {
		targets = targets[:maxSourceFetches]
	}

	for _, t := range targets {
		// Build ADT URL
		objURL := buildADTObjectURL(t.objType, t.objName)
		if objURL == "" {
			continue
		}

		// Fetch source via GetSource with explicit type for correct dispatch
		source, err := s.adtClient.GetSource(ctx, t.objType, t.objName, nil)
		if err != nil || source == "" {
			continue // best effort — skip unreadable objects
		}

		// Extract static deps
		edges := graph.ExtractDepsFromSource(source, t.nodeID)
		for _, e := range edges {
			// Add target node if not already in graph
			parts := strings.SplitN(e.To, ":", 2)
			if len(parts) == 2 {
				g.AddNode(&graph.Node{
					ID:   e.To,
					Name: parts[1],
					Type: parts[0],
				})
			}
			g.AddEdge(e)
		}

		// Extract dynamic calls (flagged, not resolved)
		dynEdges := graph.ExtractDynamicCalls(source, t.nodeID)
		for _, e := range dynEdges {
			// Dynamic targets get nodes but are clearly marked
			g.AddNode(&graph.Node{
				ID:   e.To,
				Name: e.To,
				Type: "DYNAMIC",
			})
			g.AddEdge(e)
		}
	}
}

// augmentGraphWithCoChange merges transport-based co-change data into an existing
// structural dependency graph. It fetches transport history for the target object,
// builds the transport sub-graph, and materializes CO_TRANSPORTED edges between
// objects that were shipped together (at TR or CR level when transport_attribute
// is configured). These edges are weaker than structural CALLS/REFERENCES edges
// but surface workbench↔customizing correlations invisible to cross-reference tables.
func (s *Server) augmentGraphWithCoChange(ctx context.Context, g *graph.Graph, objType, objName string) {
	headers, objects, err := s.fetchTransportData(ctx, objType, objName)
	if err != nil || len(headers) == 0 {
		return // best effort
	}

	tg := graph.BuildTransportGraph(headers, objects)

	// Determine source based on whether CR-level expansion was used
	source := graph.SourceE071
	if s.config.TransportAttribute != "" {
		source = graph.SourceE070A
	}

	// Materialize co-transported edges (min 1 shared TR)
	graph.MaterializeCoTransported(tg, 1, source)

	// Merge CO_TRANSPORTED edges into the structural graph
	for _, e := range tg.Edges() {
		if e.Kind != graph.EdgeCoTransported {
			continue
		}
		// Ensure both nodes exist in the target graph
		if fromNode := tg.GetNode(e.From); fromNode != nil {
			g.AddNode(&graph.Node{
				ID:   fromNode.ID,
				Name: fromNode.Name,
				Type: fromNode.Type,
			})
		}
		if toNode := tg.GetNode(e.To); toNode != nil {
			g.AddNode(&graph.Node{
				ID:   toNode.ID,
				Name: toNode.Name,
				Type: toNode.Type,
			})
		}
		g.AddEdge(e)
	}
}

// fetchReverseDeps builds a graph of reverse dependencies via WBCROSSGT/CROSS.
// Multi-hop: at each depth level, queries "who references these objects?" and
// adds the results to the graph. Bounded by maxDepth and per-query row limits.
func (s *Server) fetchReverseDeps(ctx context.Context, objType, objName string, maxDepth int) (*graph.Graph, error) {
	g := graph.New()

	// Seed the root node
	rootID := graph.NodeID(objType, objName)
	g.AddNode(&graph.Node{
		ID:   rootID,
		Name: objName,
		Type: objType,
	})

	// BFS: each level queries for callers of the current frontier
	frontier := []struct {
		name    string
		objType string
	}{{objName, objType}}

	visited := map[string]bool{objName: true}

	for depth := 0; depth < maxDepth && len(frontier) > 0; depth++ {
		var nextFrontier []struct {
			name    string
			objType string
		}

		for _, obj := range frontier {
			// Query WBCROSSGT: who references this object? (reverse direction)
			wbQuery := fmt.Sprintf(
				"SELECT INCLUDE, OTYPE, NAME FROM WBCROSSGT WHERE NAME LIKE '%s%%'", obj.name)
			wbResult, err := s.adtClient.RunQuery(ctx, wbQuery, 300)
			if err == nil && wbResult != nil {
				for _, row := range wbResult.Rows {
					include := strings.TrimSpace(fmt.Sprintf("%v", row["INCLUDE"]))
					if include == "" || strings.Contains(include, "\\") {
						continue
					}

					// Normalize include to object-level
					fromID, fromType, fromName := graph.NormalizeInclude(include)
					if strings.EqualFold(fromName, obj.name) {
						continue // skip self-ref
					}

					g.AddNode(&graph.Node{ID: fromID, Name: fromName, Type: fromType})
					targetID := graph.NodeID(obj.objType, obj.name)
					g.AddEdge(&graph.Edge{
						From:       fromID,
						To:         targetID,
						Kind:       graph.EdgeReferences,
						Source:     graph.SourceWBCROSSGT,
						RawInclude: include,
					})

					if !visited[fromName] {
						visited[fromName] = true
						nextFrontier = append(nextFrontier, struct {
							name    string
							objType string
						}{fromName, fromType})
					}
				}
			}

			// Query CROSS: who calls this? (reverse direction)
			crossQuery := fmt.Sprintf(
				"SELECT INCLUDE, TYPE, NAME FROM CROSS WHERE NAME LIKE '%s%%'", obj.name)
			crossResult, err := s.adtClient.RunQuery(ctx, crossQuery, 300)
			if err == nil && crossResult != nil {
				for _, row := range crossResult.Rows {
					include := strings.TrimSpace(fmt.Sprintf("%v", row["INCLUDE"]))
					refType := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["TYPE"])))
					if include == "" {
						continue
					}

					fromID, fromType, fromName := graph.NormalizeInclude(include)
					if strings.EqualFold(fromName, obj.name) {
						continue
					}

					g.AddNode(&graph.Node{ID: fromID, Name: fromName, Type: fromType})
					targetID := graph.NodeID(obj.objType, obj.name)

					// FU/PR → CALLS, others → REFERENCES
					edgeKind := graph.EdgeReferences
					if refType == "FU" || refType == "PR" {
						edgeKind = graph.EdgeCalls
					}

					g.AddEdge(&graph.Edge{
						From:       fromID,
						To:         targetID,
						Kind:       edgeKind,
						Source:     graph.SourceCROSS,
						RawInclude: include,
						RefDetail:  "TYPE:" + refType,
					})

					if !visited[fromName] {
						visited[fromName] = true
						nextFrontier = append(nextFrontier, struct {
							name    string
							objType string
						}{fromName, fromType})
					}
				}
			}
		}

		frontier = nextFrontier
	}

	return g, nil
}

// handleWhereUsedConfig finds programs that read a specific TVARVC/STVARV variable.
// MCP: SAP(action="analyze", params={"type": "where_used_config", "variable": "ZKEKEKE"})
// Optional: "grep": false to skip source confirmation (faster, MEDIUM confidence only)
//
// Data source: CROSS table (who references TVARVC) + optional source grep for confirmation.
// This is a heuristic query. Confidence: HIGH = grep-confirmed, MEDIUM = CROSS-only.
func (s *Server) handleWhereUsedConfig(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	variable := strings.ToUpper(strings.TrimSpace(getStringParam(args, "variable")))
	doGrep := true
	if g, ok := getBoolParam(args, "grep"); ok {
		doGrep = g
	}

	if variable == "" {
		return newToolResultError("variable is required. Example: SAP(action=\"analyze\", params={\"type\": \"where_used_config\", \"variable\": \"ZKEKEKE\"})"), nil
	}
	if s.adtClient == nil {
		return newToolResultError("SAP connection required for where-used-config"), nil
	}

	refs, err := s.fetchConfigRefs(ctx, variable, doGrep)
	if err != nil {
		return newToolResultError(fmt.Sprintf("where_used_config failed: %v", err)), nil
	}

	g := graph.BuildConfigGraph(
		[]graph.TVARVCVariable{{Name: variable}},
		refs,
	)

	// Resolve packages
	s.resolvePackages(ctx, g)

	result := graph.WhereUsedConfig(g, variable)

	// Backfill package info
	for i, r := range result.Readers {
		if n := g.GetNode(r.NodeID); n != nil && n.Package != "" {
			result.Readers[i].Package = n.Package
		}
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("JSON marshal error: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// handleUsageExamples returns concrete caller snippets for a target object.
// MCP examples:
//   SAP(action="analyze", params={"type":"usage_examples","object_type":"FUNC","object_name":"Z_MY_FM"})
//   SAP(action="analyze", params={"type":"usage_examples","object_type":"CLAS","object_name":"ZCL_API","method":"GET_DATA"})
//   SAP(action="analyze", params={"type":"usage_examples","object_type":"PROG","object_name":"ZLEGACY","form":"BUILD_OUTPUT"})
//   SAP(action="analyze", params={"type":"usage_examples","object_type":"PROG","object_name":"ZBATCH_RUN","submit":true})
func (s *Server) handleUsageExamples(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	target, err := usageTargetFromArgs(args)
	if err != nil {
		return newToolResultError(err.Error()), nil
	}
	if s.adtClient == nil {
		return newToolResultError("SAP connection required for usage_examples"), nil
	}

	topN := 5
	if t, ok := getFloatParam(args, "top_n"); ok && t > 0 {
		topN = int(t)
	}

	callers, err := s.fetchUsageCallerSources(ctx, target, topN)
	if err != nil {
		return newToolResultError(fmt.Sprintf("usage_examples failed: %v", err)), nil
	}

	result := graph.FindUsageExamples(target, callers, topN)
	s.backfillUsagePackages(ctx, callers, result)

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("JSON marshal error: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

// fetchConfigRefs finds programs referencing a TVARVC variable via CROSS + optional grep.
// Steps:
//  1. CROSS WHERE NAME = 'TVARVC' AND TYPE = 'D' → programs that read TVARVC table
//  2. NormalizeInclude → deduplicate to object level
//  3. For each candidate, grep source for literal variable name (if doGrep=true)
//  4. Return TVARVCReference slice with Confirmed flag
func (s *Server) fetchConfigRefs(ctx context.Context, variable string, doGrep bool) ([]graph.TVARVCReference, error) {
	// Step 1: Find programs that reference TVARVC table
	crossQuery := "SELECT INCLUDE, TYPE, NAME FROM CROSS WHERE NAME = 'TVARVC' AND TYPE = 'D'"
	crossResult, err := s.adtClient.RunQuery(ctx, crossQuery, 500)
	if err != nil {
		return nil, fmt.Errorf("CROSS query: %w", err)
	}
	if crossResult == nil || len(crossResult.Rows) == 0 {
		return nil, nil
	}

	// Step 2: Normalize includes → deduplicate to object level
	type candidate struct {
		objType string
		objName string
	}
	seen := make(map[string]bool)
	var candidates []candidate

	for _, row := range crossResult.Rows {
		include := strings.TrimSpace(fmt.Sprintf("%v", row["INCLUDE"]))
		if include == "" {
			continue
		}
		_, objType, objName := graph.NormalizeInclude(include)
		key := objType + ":" + objName
		if !seen[key] {
			seen[key] = true
			candidates = append(candidates, candidate{objType, objName})
		}
	}

	// Step 3: Grep each candidate's source for the variable name
	var refs []graph.TVARVCReference
	for _, c := range candidates {
		confirmed := false

		if doGrep {
			// Build ADT URL for grep
			objURL := buildADTObjectURL(c.objType, c.objName)
			if objURL != "" {
				grepResult, err := s.adtClient.GrepObject(ctx, objURL, variable, true, 0)
				if err == nil && grepResult != nil && len(grepResult.Matches) > 0 {
					confirmed = true
				}
			}
		}

		refs = append(refs, graph.TVARVCReference{
			VariableName: variable,
			ObjectType:   c.objType,
			ObjectName:   c.objName,
			Confirmed:    confirmed,
		})
	}

	return refs, nil
}

type usageCallerCandidate struct {
	NodeID  string
	Name    string
	Type    string
	Package string
	IsTest  bool
	Parent  string
}

func usageTargetFromArgs(args map[string]any) (graph.UsageTarget, error) {
	target := graph.UsageTarget{
		ObjectType: strings.ToUpper(strings.TrimSpace(getStringParam(args, "object_type"))),
		ObjectName: strings.ToUpper(strings.TrimSpace(getStringParam(args, "object_name"))),
		Method:     strings.ToUpper(strings.TrimSpace(getStringParam(args, "method"))),
		Form:       strings.ToUpper(strings.TrimSpace(getStringParam(args, "form"))),
	}
	submit, _ := getBoolParam(args, "submit")

	if target.ObjectType == "" || target.ObjectName == "" {
		return target, fmt.Errorf("object_type and object_name are required. Example: SAP(action=\"analyze\", params={\"type\": \"usage_examples\", \"object_type\": \"FUNC\", \"object_name\": \"Z_MY_FM\"})")
	}
	if target.ObjectType == "PROG" && target.Form == "" && !submit {
		return target, fmt.Errorf("for PROG targets, provide either form=\"FORM_NAME\" or submit=true")
	}
	if submit {
		target.ObjectType = "SUBMIT"
	}
	return target, nil
}

func (s *Server) fetchUsageCallerSources(ctx context.Context, target graph.UsageTarget, topN int) ([]graph.CallerSource, error) {
	maxCandidates := topN * 4
	if maxCandidates < 12 {
		maxCandidates = 12
	}

	cands, err := s.fetchUsageCandidatesViaADT(ctx, target, maxCandidates)
	if err != nil || len(cands) == 0 {
		fallback, ferr := s.fetchUsageCandidatesFallback(ctx, target, maxCandidates)
		if ferr != nil && len(cands) == 0 {
			return nil, ferr
		}
		if len(cands) == 0 {
			cands = fallback
		}
	}

	var callers []graph.CallerSource
	for _, c := range cands {
		source, err := s.fetchUsageCandidateSource(ctx, c)
		if err != nil || strings.TrimSpace(source) == "" {
			continue
		}
		callers = append(callers, graph.CallerSource{
			NodeID:  c.NodeID,
			Name:    c.Name,
			Type:    c.Type,
			Package: c.Package,
			IsTest:  c.IsTest,
			Source:  source,
		})
		if len(callers) >= maxCandidates {
			break
		}
	}

	return callers, nil
}

func (s *Server) fetchUsageCandidatesViaADT(ctx context.Context, target graph.UsageTarget, maxCandidates int) ([]usageCallerCandidate, error) {
	targetURI, err := s.resolveUsageTargetURI(ctx, target)
	if err != nil || targetURI == "" {
		return nil, err
	}

	root, err := s.adtClient.GetCallersOf(ctx, targetURI, 1)
	if err != nil || root == nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var out []usageCallerCandidate
	for _, child := range root.Children {
		cand, ok := usageCandidateFromCallGraphNode(child)
		if !ok || seen[cand.NodeID] {
			continue
		}
		seen[cand.NodeID] = true
		out = append(out, cand)
		if len(out) >= maxCandidates {
			break
		}
	}
	return out, nil
}

func (s *Server) resolveUsageTargetURI(ctx context.Context, target graph.UsageTarget) (string, error) {
	switch target.ObjectType {
	case "FUNC":
		results, err := s.adtClient.SearchObject(ctx, target.ObjectName, 10)
		if err != nil {
			return "", err
		}
		for _, r := range results {
			if strings.EqualFold(r.Name, target.ObjectName) && strings.Contains(r.URI, "/fmodules/") {
				return r.URI, nil
			}
		}
		return "", fmt.Errorf("function module %s not found via SearchObject", target.ObjectName)
	case "CLAS":
		classURL := buildADTObjectURL("CLAS", target.ObjectName)
		if target.Method == "" {
			return classURL, nil
		}
		comps, err := s.adtClient.GetClassComponents(ctx, classURL)
		if err == nil {
			if href := findClassComponentHref(comps, target.Method); href != "" {
				return href, nil
			}
		}
		return classURL, nil
	case "INTF":
		intfURL := buildADTObjectURL("INTF", target.ObjectName)
		if target.Method == "" {
			return intfURL, nil
		}
		structure, err := s.adtClient.GetObjectStructureCAI(ctx, target.ObjectName, 200)
		if err == nil {
			if href := findObjectExplorerHref(structure, target.Method); href != "" {
				return href, nil
			}
		}
		return intfURL, nil
	case "PROG", "SUBMIT":
		return buildADTObjectURL("PROG", target.ObjectName), nil
	default:
		return "", fmt.Errorf("unsupported usage_examples object_type %s", target.ObjectType)
	}
}

func findClassComponentHref(comp *adt.ClassComponent, method string) string {
	if comp == nil {
		return ""
	}
	if strings.EqualFold(comp.Name, method) && strings.Contains(strings.ToUpper(comp.Type), "METH") && comp.Href != "" {
		return comp.Href
	}
	for i := range comp.Components {
		if href := findClassComponentHref(&comp.Components[i], method); href != "" {
			return href
		}
	}
	return ""
}

func findObjectExplorerHref(node *adt.ObjectExplorerNode, name string) string {
	if node == nil {
		return ""
	}
	if strings.EqualFold(node.Name, name) && node.URI != "" {
		return node.URI
	}
	for i := range node.Children {
		if href := findObjectExplorerHref(&node.Children[i], name); href != "" {
			return href
		}
	}
	return ""
}

func usageCandidateFromCallGraphNode(node adt.CallGraphNode) (usageCallerCandidate, bool) {
	objType, name, parent := usageTypeNameFromURI(node.URI, node.Name)
	if objType == "" || name == "" {
		return usageCallerCandidate{}, false
	}
	return usageCallerCandidate{
		NodeID: graph.NodeID(objType, name),
		Name:   strings.ToUpper(name),
		Type:   objType,
		Parent: strings.ToUpper(parent),
		IsTest: graph.IsTestCaller(name, ""),
	}, true
}

func usageTypeNameFromURI(uri, fallbackName string) (objType, name, parent string) {
	lowerURI := strings.ToLower(uri)
	switch {
	case strings.Contains(lowerURI, "/oo/classes/"):
		objType = "CLAS"
	case strings.Contains(lowerURI, "/oo/interfaces/"):
		objType = "INTF"
	case strings.Contains(lowerURI, "/programs/programs/"):
		objType = "PROG"
	case strings.Contains(lowerURI, "/functions/groups/") && strings.Contains(lowerURI, "/fmodules/"):
		objType = "FUNC"
	default:
		return "", "", ""
	}

	if fallbackName != "" {
		name = strings.ToUpper(fallbackName)
	}
	if objType == "FUNC" {
		parts := strings.Split(lowerURI, "/")
		for i := range parts {
			if parts[i] == "groups" && i+1 < len(parts) {
				parent = strings.ToUpper(parts[i+1])
			}
			if parts[i] == "fmodules" && i+1 < len(parts) {
				name = strings.ToUpper(parts[i+1])
			}
		}
	}
	return objType, name, parent
}

func (s *Server) fetchUsageCandidatesFallback(ctx context.Context, target graph.UsageTarget, maxCandidates int) ([]usageCallerCandidate, error) {
	var queries []string

	switch target.ObjectType {
	case "FUNC":
		queries = append(queries, fmt.Sprintf("SELECT INCLUDE, TYPE, NAME FROM CROSS WHERE NAME = '%s' AND TYPE = 'FU'", target.ObjectName))
	case "SUBMIT":
		queries = append(queries, fmt.Sprintf("SELECT INCLUDE, TYPE, NAME FROM CROSS WHERE NAME = '%s' AND TYPE = 'PR'", target.ObjectName))
	case "PROG":
		if target.Form != "" {
			queries = append(queries, fmt.Sprintf("SELECT INCLUDE, TYPE, NAME FROM CROSS WHERE NAME = '%s' AND TYPE = 'SU'", target.Form))
		} else {
			queries = append(queries, fmt.Sprintf("SELECT INCLUDE, TYPE, NAME FROM CROSS WHERE NAME = '%s' AND TYPE = 'PR'", target.ObjectName))
		}
	case "CLAS", "INTF":
		queries = append(queries, fmt.Sprintf("SELECT INCLUDE, OTYPE, NAME FROM WBCROSSGT WHERE NAME LIKE '%s%%'", target.ObjectName))
		queries = append(queries, fmt.Sprintf("SELECT INCLUDE, TYPE AS OTYPE, NAME FROM CROSS WHERE NAME LIKE '%s%%'", target.ObjectName))
	}

	seen := make(map[string]bool)
	var out []usageCallerCandidate
	for _, query := range queries {
		result, err := s.adtClient.RunQuery(ctx, query, maxCandidates*2)
		if err != nil || result == nil {
			continue
		}
		for _, row := range result.Rows {
			include := strings.TrimSpace(fmt.Sprintf("%v", row["INCLUDE"]))
			if include == "" {
				continue
			}
			_, objType, objName := graph.NormalizeInclude(include)
			if objType == "FUGR" {
				continue // no source snippets from FUGR metadata in v1
			}
			nodeID := graph.NodeID(objType, objName)
			if seen[nodeID] {
				continue
			}
			seen[nodeID] = true
			out = append(out, usageCallerCandidate{
				NodeID: nodeID,
				Name:   objName,
				Type:   objType,
				IsTest: graph.IsTestCaller(objName, ""),
			})
			if len(out) >= maxCandidates {
				return out, nil
			}
		}
	}

	return out, nil
}

func (s *Server) fetchUsageCandidateSource(ctx context.Context, cand usageCallerCandidate) (string, error) {
	switch cand.Type {
	case "CLAS", "PROG", "INTF":
		return s.adtClient.GetSource(ctx, cand.Type, cand.Name, nil)
	case "FUNC":
		if cand.Parent == "" {
			return "", fmt.Errorf("missing function group parent for %s", cand.Name)
		}
		return s.adtClient.GetSource(ctx, "FUNC", cand.Name, &adt.GetSourceOptions{Parent: cand.Parent})
	default:
		return "", fmt.Errorf("unsupported caller source type %s", cand.Type)
	}
}

func (s *Server) backfillUsagePackages(ctx context.Context, callers []graph.CallerSource, result *graph.UsageExamplesResult) {
	if result == nil || len(result.Examples) == 0 {
		return
	}
	g := graph.New()
	for _, c := range callers {
		g.AddNode(&graph.Node{ID: c.NodeID, Name: c.Name, Type: c.Type, Package: c.Package})
	}
	s.resolvePackages(ctx, g)
	for i, ex := range result.Examples {
		if ex.Package != "" {
			continue
		}
		if n := g.GetNode(ex.CallerID); n != nil && n.Package != "" {
			result.Examples[i].Package = n.Package
		}
	}
}

// buildADTObjectURL constructs an ADT URL for an object by type and name.
func buildADTObjectURL(objType, objName string) string {
	name := strings.ToLower(objName)
	switch objType {
	case "CLAS":
		return "/sap/bc/adt/oo/classes/" + name
	case "PROG":
		return "/sap/bc/adt/programs/programs/" + name
	case "INTF":
		return "/sap/bc/adt/oo/interfaces/" + name
	case "FUGR":
		return "/sap/bc/adt/functions/groups/" + name
	default:
		// Can't build URL for unknown types — skip grep
		return ""
	}
}

func formatBoundaryResult(report *graph.BoundaryReport, format string) (*mcp.CallToolResult, error) {
	switch format {
	case "json":
		result, _ := json.MarshalIndent(report, "", "  ")
		return mcp.NewToolResultText(string(result)), nil
	default:
		return mcp.NewToolResultText(report.FormatText()), nil
	}
}
