package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/graph"
)

// --- CR History ---

// CRHistoryEntry represents one CR that touched an object.
type CRHistoryEntry struct {
	CRID       string   `json:"cr_id"`
	Transports []string `json:"transports"`
	Users      []string `json:"users,omitempty"`
	Dates      []string `json:"dates,omitempty"`
}

// CRHistoryResult is the result of a CR history lookup.
type CRHistoryResult struct {
	ObjectType string           `json:"object_type"`
	ObjectName string           `json:"object_name"`
	Attribute  string           `json:"attribute"`
	CRs        []CRHistoryEntry `json:"crs"`
	Transports []string         `json:"transports"` // All transports (for objects without CR attribute)
}

// handleCRHistory finds all CRs (via E070A) where an object was touched.
// MCP: SAP(action="analyze", params={"type": "cr_history", "object_type": "CLAS", "object_name": "ZCL_FOO"})
func (s *Server) handleCRHistory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	objType := strings.ToUpper(getStringParam(args, "object_type"))
	objName := strings.ToUpper(getStringParam(args, "object_name"))

	if objType == "" || objName == "" {
		return newToolResultError("object_type and object_name are required"), nil
	}
	if s.adtClient == nil {
		return newToolResultError("SAP connection required"), nil
	}

	attr := s.config.TransportAttribute

	// Step 1: Find transports containing this object
	// Split into two queries — SAP freestyle doesn't support complex OR clauses
	trSet := make(map[string]bool)

	e071R3TR := fmt.Sprintf(
		"SELECT TRKORR FROM E071 WHERE PGMID = 'R3TR' AND OBJECT = '%s' AND OBJ_NAME = '%s'",
		objType, objName)
	r3trResult, err := s.adtClient.RunQuery(ctx, e071R3TR, 500)
	if err != nil {
		return newToolResultError(fmt.Sprintf("E071 R3TR query failed: %v", err)), nil
	}
	for _, row := range r3trResult.Rows {
		tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
		if tr != "" {
			trSet[tr] = true
		}
	}

	// LIMU query — best effort (prefix match for sub-object entries)
	e071LIMU := fmt.Sprintf(
		"SELECT TRKORR FROM E071 WHERE PGMID = 'LIMU' AND OBJ_NAME LIKE '%s%%'",
		objName)
	limuResult, _ := s.adtClient.RunQuery(ctx, e071LIMU, 500)
	if limuResult != nil {
		for _, row := range limuResult.Rows {
			tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
			if tr != "" {
				trSet[tr] = true
			}
		}
	}

	if len(trSet) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No transports found for %s %s.", objType, objName)), nil
	}

	// Step 2: Resolve task→request hierarchy via E070
	trList := quoteKeys(trSet)
	e070Query := fmt.Sprintf(
		"SELECT TRKORR, STRKORR, AS4USER, AS4DATE FROM E070 WHERE TRKORR IN (%s)",
		strings.Join(trList, ","))
	e070Result, err := s.adtClient.RunQuery(ctx, e070Query, 500)

	requestSet := make(map[string]bool)
	trMeta := make(map[string]struct{ user, date string })

	if err == nil && e070Result != nil {
		for _, row := range e070Result.Rows {
			tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
			parent := strings.TrimSpace(fmt.Sprintf("%v", row["STRKORR"]))
			user := strings.TrimSpace(fmt.Sprintf("%v", row["AS4USER"]))
			date := strings.TrimSpace(fmt.Sprintf("%v", row["AS4DATE"]))
			trMeta[tr] = struct{ user, date string }{user, date}
			if parent != "" {
				requestSet[parent] = true
			} else {
				requestSet[tr] = true
			}
		}
	}

	result := &CRHistoryResult{
		ObjectType: objType,
		ObjectName: objName,
		Attribute:  attr,
	}

	// Collect all transport numbers
	for tr := range trSet {
		result.Transports = append(result.Transports, tr)
	}

	// Step 3: If transport attribute is configured, look up CRs via E070A
	if attr != "" && len(requestSet) > 0 {
		reqList := quoteKeys(requestSet)
		attrQuery := fmt.Sprintf(
			"SELECT TRKORR, REFERENCE FROM E070A WHERE ATTRIBUTE = '%s' AND TRKORR IN (%s)",
			attr, strings.Join(reqList, ","))
		attrResult, err := s.adtClient.RunQuery(ctx, attrQuery, 500)

		if err == nil && attrResult != nil {
			// Group transports by CR reference
			crMap := make(map[string][]string) // CR ID → transport list
			for _, row := range attrResult.Rows {
				tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
				ref := strings.TrimSpace(fmt.Sprintf("%v", row["REFERENCE"]))
				if ref != "" {
					crMap[ref] = append(crMap[ref], tr)
				}
			}

			for crID, trs := range crMap {
				entry := CRHistoryEntry{CRID: crID, Transports: trs}
				// Collect unique users and dates
				userSet := make(map[string]bool)
				dateSet := make(map[string]bool)
				for _, tr := range trs {
					if meta, ok := trMeta[tr]; ok {
						if meta.user != "" {
							userSet[meta.user] = true
						}
						if meta.date != "" {
							dateSet[meta.date] = true
						}
					}
				}
				for u := range userSet {
					entry.Users = append(entry.Users, u)
				}
				for d := range dateSet {
					entry.Dates = append(entry.Dates, d)
				}
				result.CRs = append(result.CRs, entry)
			}
		}
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// --- Transport Boundaries ---

// handleTransportBoundaries checks if a set of transports is self-consistent.
// For each object in the transport(s), fetches its dependencies and checks
// whether all targets are also in the transport set.
// MCP: SAP(action="analyze", params={"type": "tr_boundaries", "transports": "A4HK900001,A4HK900002"})
func (s *Server) handleTransportBoundaries(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	transports := getStringParam(args, "transports")

	if transports == "" {
		return newToolResultError("transports is required (comma-separated TR numbers)"), nil
	}
	if s.adtClient == nil {
		return newToolResultError("SAP connection required"), nil
	}

	trList := splitAndUpper(transports)
	return s.analyzeTransportBoundaries(ctx, trList, strings.Join(trList, ","))
}

// handleCRBoundaries resolves transports from a CR ID (via E070A), then
// runs the same boundary analysis as tr_boundaries.
// MCP: SAP(action="analyze", params={"type": "cr_boundaries", "cr_id": "JIRA-123"})
func (s *Server) handleCRBoundaries(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	crID := strings.TrimSpace(getStringParam(args, "cr_id"))

	if crID == "" {
		return newToolResultError("cr_id is required"), nil
	}
	if s.adtClient == nil {
		return newToolResultError("SAP connection required"), nil
	}

	attr := s.config.TransportAttribute
	if attr == "" {
		return newToolResultError("transport_attribute not configured. Set SAP_TRANSPORT_ATTRIBUTE or transport_attribute in .vsp.json"), nil
	}

	// Step 1: Find all transports for this CR via E070A
	attrQuery := fmt.Sprintf(
		"SELECT TRKORR FROM E070A WHERE ATTRIBUTE = '%s' AND REFERENCE = '%s'",
		attr, crID)
	attrResult, err := s.adtClient.RunQuery(ctx, attrQuery, 500)
	if err != nil {
		return newToolResultError(fmt.Sprintf("E070A query failed: %v", err)), nil
	}

	var trList []string
	for _, row := range attrResult.Rows {
		tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
		if tr != "" {
			trList = append(trList, tr)
		}
	}

	if len(trList) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No transports found for CR %s (attribute: %s).", crID, attr)), nil
	}

	// Step 2: Also get child tasks for these requests
	reqQuoted := make([]string, len(trList))
	for i, tr := range trList {
		reqQuoted[i] = "'" + tr + "'"
	}
	taskQuery := fmt.Sprintf(
		"SELECT TRKORR FROM E070 WHERE STRKORR IN (%s)",
		strings.Join(reqQuoted, ","))
	taskResult, err := s.adtClient.RunQuery(ctx, taskQuery, 500)
	if err == nil && taskResult != nil {
		for _, row := range taskResult.Rows {
			tr := strings.TrimSpace(fmt.Sprintf("%v", row["TRKORR"]))
			if tr != "" {
				trList = append(trList, tr)
			}
		}
	}

	label := fmt.Sprintf("CR:%s (%s)", crID, attr)
	return s.analyzeTransportBoundaries(ctx, trList, label)
}

// analyzeTransportBoundaries is the shared core: given a list of transport numbers,
// fetches objects, builds dependency graph, and runs boundary analysis.
func (s *Server) analyzeTransportBoundaries(ctx context.Context, trList []string, label string) (*mcp.CallToolResult, error) {
	// Step 1: Get all objects in these transports
	trQuoted := make([]string, len(trList))
	for i, tr := range trList {
		trQuoted[i] = "'" + strings.ToUpper(tr) + "'"
	}
	e071Query := fmt.Sprintf(
		"SELECT TRKORR, PGMID, OBJECT, OBJ_NAME FROM E071 WHERE PGMID = 'R3TR' AND TRKORR IN (%s)",
		strings.Join(trQuoted, ","))
	e071Result, err := s.adtClient.RunQuery(ctx, e071Query, 2000)
	if err != nil {
		return newToolResultError(fmt.Sprintf("E071 query failed: %v", err)), nil
	}

	// Build transport scope
	trSet := make(map[string]bool)
	for _, tr := range trList {
		trSet[strings.ToUpper(tr)] = true
	}

	type objKey struct{ objType, objName string }
	objectSet := make(map[objKey]bool)
	scopeObjects := make(map[string]bool)

	for _, row := range e071Result.Rows {
		objType := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJECT"])))
		objName := strings.ToUpper(strings.TrimSpace(fmt.Sprintf("%v", row["OBJ_NAME"])))
		if objType == "" || objName == "" {
			continue
		}
		objectSet[objKey{objType, objName}] = true
		scopeObjects[graph.NodeID(objType, objName)] = true
	}

	if len(objectSet) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No objects found in transports: %s", label)), nil
	}

	scope := &graph.TransportScope{
		Label:      label,
		Transports: trSet,
		Objects:    scopeObjects,
	}

	// Step 2: For each source-bearing object, fetch source and extract deps
	g := graph.New()

	maxObjects := 50
	count := 0
	for obj := range objectSet {
		if count >= maxObjects {
			break
		}

		// Add node
		nodeID := graph.NodeID(obj.objType, obj.objName)
		g.AddNode(&graph.Node{ID: nodeID, Name: obj.objName, Type: obj.objType})

		// Only analyze source-bearing types
		if obj.objType != "CLAS" && obj.objType != "PROG" && obj.objType != "FUGR" && obj.objType != "INTF" {
			continue
		}

		source, err := s.adtClient.GetSource(ctx, obj.objType, obj.objName, nil)
		if err != nil {
			continue
		}

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

	// Resolve packages for context in the report
	s.resolvePackages(ctx, g)

	// Step 3: Run boundary analysis
	report := graph.AnalyzeTransportBoundaries(g, scope)

	data, _ := json.MarshalIndent(report, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

// splitAndUpper splits a comma-separated string and uppercases each part.
func splitAndUpper(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
