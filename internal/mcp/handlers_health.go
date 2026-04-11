package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/graph"
)

type healthScope struct {
	Kind       string `json:"kind"`
	Package    string `json:"package,omitempty"`
	ObjectType string `json:"object_type,omitempty"`
	ObjectName string `json:"object_name,omitempty"`
}

type healthSummary struct {
	Status   string `json:"status"`
	Headline string `json:"headline"`
}

type healthSignal struct {
	Status  string         `json:"status"`
	Details map[string]any `json:"details,omitempty"`
}

type healthResult struct {
	Scope   healthScope             `json:"scope"`
	Summary healthSummary           `json:"summary"`
	Signals map[string]healthSignal `json:"signals"`
}

func (s *Server) handleHealth(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	pkg := strings.ToUpper(strings.TrimSpace(getStringParam(args, "package")))
	objType := strings.ToUpper(strings.TrimSpace(getStringParam(args, "object_type")))
	objName := strings.ToUpper(strings.TrimSpace(getStringParam(args, "object_name")))
	parent := strings.ToUpper(strings.TrimSpace(getStringParam(args, "parent")))

	if s.adtClient == nil {
		return newToolResultError("SAP connection required for health"), nil
	}
	if pkg == "" && (objType == "" || objName == "") {
		return newToolResultError("provide either package or object_type + object_name"), nil
	}

	result := &healthResult{
		Signals: make(map[string]healthSignal),
	}

	if pkg != "" {
		result.Scope = healthScope{Kind: "package", Package: pkg}
		s.populatePackageHealth(ctx, pkg, result)
	} else {
		result.Scope = healthScope{Kind: "object", ObjectType: objType, ObjectName: objName}
		s.populateObjectHealth(ctx, objType, objName, parent, result)
	}

	result.Summary = summarizeHealth(result.Signals)

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("JSON marshal error: %v", err)), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) populatePackageHealth(ctx context.Context, pkg string, result *healthResult) {
	result.Signals["tests"] = s.collectPackageTests(ctx, pkg)
	result.Signals["atc"] = s.collectPackageATC(ctx, pkg)
	result.Signals["boundaries"] = s.collectPackageBoundaries(ctx, pkg)
	result.Signals["staleness"] = s.collectPackageStaleness(ctx, pkg)
}

func (s *Server) populateObjectHealth(ctx context.Context, objType, objName, parent string, result *healthResult) {
	result.Signals["tests"] = s.collectObjectTests(ctx, objType, objName)
	result.Signals["atc"] = s.collectObjectATC(ctx, objType, objName)
	result.Signals["boundaries"] = s.collectObjectBoundaries(ctx, objType, objName, parent)
	result.Signals["staleness"] = s.collectObjectStaleness(ctx, objType, objName, parent)
}

func (s *Server) collectObjectTests(ctx context.Context, objType, objName string) healthSignal {
	objectURL := buildHealthObjectURL(objType, objName, "")
	if objectURL == "" {
		return healthSignal{Status: "UNKNOWN"}
	}
	result, err := s.adtClient.RunUnitTests(ctx, objectURL, nil)
	if err != nil {
		return healthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}
	classCount, methodCount, alertCount := summarizeUnitTests(result)
	status := "PASS"
	if classCount == 0 {
		status = "NONE"
	}
	if alertCount > 0 {
		status = "FAIL"
	}
	return healthSignal{Status: status, Details: map[string]any{
		"classes": classCount,
		"methods": methodCount,
		"alerts":  alertCount,
	}}
}

func (s *Server) collectPackageTests(ctx context.Context, pkg string) healthSignal {
	content, err := s.adtClient.GetPackage(ctx, pkg)
	if err != nil {
		return healthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}

	var testClasses []adt.PackageObject
	for _, obj := range content.Objects {
		if strings.ToUpper(obj.Type) == "CLAS" && graph.IsTestCaller(obj.Name, "") {
			testClasses = append(testClasses, obj)
		}
	}
	if len(testClasses) == 0 {
		return healthSignal{Status: "NONE"}
	}

	limit := 5
	if len(testClasses) < limit {
		limit = len(testClasses)
	}
	totalClasses := 0
	totalMethods := 0
	totalAlerts := 0
	for _, obj := range testClasses[:limit] {
		objectURL := buildHealthObjectURL("CLAS", obj.Name, "")
		result, err := s.adtClient.RunUnitTests(ctx, objectURL, nil)
		if err != nil {
			continue
		}
		c, m, a := summarizeUnitTests(result)
		totalClasses += c
		totalMethods += m
		totalAlerts += a
	}

	status := "PASS"
	if totalClasses == 0 {
		status = "NONE"
	}
	if totalAlerts > 0 {
		status = "FAIL"
	}
	return healthSignal{Status: status, Details: map[string]any{
		"test_classes_found": len(testClasses),
		"test_classes_run":   limit,
		"classes":            totalClasses,
		"methods":            totalMethods,
		"alerts":             totalAlerts,
	}}
}

func summarizeUnitTests(result *adt.UnitTestResult) (classCount, methodCount, alertCount int) {
	if result == nil {
		return 0, 0, 0
	}
	classCount = len(result.Classes)
	for _, c := range result.Classes {
		methodCount += len(c.TestMethods)
		alertCount += len(c.Alerts)
		for _, m := range c.TestMethods {
			alertCount += len(m.Alerts)
		}
	}
	return classCount, methodCount, alertCount
}

func (s *Server) collectObjectATC(ctx context.Context, objType, objName string) healthSignal {
	objectURL := buildHealthObjectURL(objType, objName, "")
	if objectURL == "" {
		return healthSignal{Status: "UNKNOWN"}
	}
	result, err := s.adtClient.RunATCCheck(ctx, objectURL, "", 100)
	if err != nil {
		return healthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}
	total, errors, warnings, infos := summarizeATC(result)
	status := "CLEAN"
	if total > 0 {
		status = "FINDINGS"
	}
	return healthSignal{Status: status, Details: map[string]any{
		"findings": total,
		"errors":   errors,
		"warnings": warnings,
		"infos":    infos,
	}}
}

func (s *Server) collectPackageATC(ctx context.Context, pkg string) healthSignal {
	objectURL := "/sap/bc/adt/packages/" + strings.ToLower(pkg)
	result, err := s.adtClient.RunATCCheck(ctx, objectURL, "", 200)
	if err != nil {
		return healthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}
	total, errors, warnings, infos := summarizeATC(result)
	status := "CLEAN"
	if total > 0 {
		status = "FINDINGS"
	}
	return healthSignal{Status: status, Details: map[string]any{
		"findings": total,
		"errors":   errors,
		"warnings": warnings,
		"infos":    infos,
	}}
}

func summarizeATC(result *adt.ATCWorklist) (total, errors, warnings, infos int) {
	if result == nil {
		return 0, 0, 0, 0
	}
	for _, obj := range result.Objects {
		total += len(obj.Findings)
		for _, f := range obj.Findings {
			switch f.Priority {
			case 1:
				errors++
			case 2:
				warnings++
			default:
				infos++
			}
		}
	}
	return total, errors, warnings, infos
}

func (s *Server) collectObjectBoundaries(ctx context.Context, objType, objName, parent string) healthSignal {
	if objType != "CLAS" && objType != "PROG" && objType != "INTF" {
		return healthSignal{Status: "UNKNOWN"}
	}
	source, err := s.adtClient.GetSource(ctx, objType, objName, nil)
	if err != nil || source == "" {
		return healthSignal{Status: "ERROR", Details: map[string]any{"message": "failed to read source"}}
	}

	g := graph.New()
	nodeID := graph.NodeID(objType, objName)
	g.AddNode(&graph.Node{ID: nodeID, Name: objName, Type: objType})
	edges := graph.ExtractDepsFromSource(source, nodeID)
	dynEdges := graph.ExtractDynamicCalls(source, nodeID)
	for _, e := range append(edges, dynEdges...) {
		g.AddEdge(e)
		parts := strings.SplitN(e.To, ":", 2)
		if len(parts) == 2 {
			g.AddNode(&graph.Node{ID: e.To, Name: parts[1], Type: parts[0]})
		}
	}
	s.resolvePackages(ctx, g)
	n := g.GetNode(nodeID)
	if n == nil || n.Package == "" {
		return healthSignal{Status: "UNKNOWN"}
	}
	report := g.CheckBoundaries(n.Package, &graph.BoundaryOptions{IncludeDynamic: true})
	status := "CLEAN"
	if report.Violations > 0 {
		status = "VIOLATIONS"
	}
	return healthSignal{Status: status, Details: map[string]any{
		"violations":       report.Violations,
		"crossed_packages": report.CrossedPackages,
		"dynamic":          report.Dynamic,
	}}
}

func (s *Server) collectPackageBoundaries(ctx context.Context, pkg string) healthSignal {
	g := graph.New()
	content, err := s.adtClient.GetPackage(ctx, pkg)
	if err != nil {
		return healthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}

	count := 0
	for _, obj := range content.Objects {
		objType := strings.ToUpper(obj.Type)
		if objType != "CLAS" && objType != "PROG" && objType != "INTF" {
			continue
		}
		if count >= 30 {
			break
		}
		source, err := s.adtClient.GetSource(ctx, objType, obj.Name, nil)
		if err != nil || source == "" {
			continue
		}
		nodeID := graph.NodeID(objType, obj.Name)
		g.AddNode(&graph.Node{ID: nodeID, Name: obj.Name, Type: objType, Package: pkg})
		edges := graph.ExtractDepsFromSource(source, nodeID)
		dynEdges := graph.ExtractDynamicCalls(source, nodeID)
		for _, e := range append(edges, dynEdges...) {
			g.AddEdge(e)
			parts := strings.SplitN(e.To, ":", 2)
			if len(parts) == 2 {
				g.AddNode(&graph.Node{ID: e.To, Name: parts[1], Type: parts[0]})
			}
		}
		count++
	}
	s.resolvePackages(ctx, g)
	report := g.CheckBoundaries(pkg, &graph.BoundaryOptions{IncludeDynamic: true})
	status := "CLEAN"
	if report.Violations > 0 {
		status = "VIOLATIONS"
	}
	return healthSignal{Status: status, Details: map[string]any{
		"scanned_objects":   count,
		"violations":        report.Violations,
		"crossed_packages":  report.CrossedPackages,
		"violating_objects": report.ViolatingObjects,
	}}
}

func (s *Server) collectObjectStaleness(ctx context.Context, objType, objName, parent string) healthSignal {
	revs, err := s.adtClient.GetRevisions(ctx, objType, objName, &adt.GetSourceOptions{Parent: parent})
	if err != nil {
		return healthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}
	return stalenessFromRevisions(revs)
}

func (s *Server) collectPackageStaleness(ctx context.Context, pkg string) healthSignal {
	content, err := s.adtClient.GetPackage(ctx, pkg)
	if err != nil {
		return healthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}

	var newest time.Time
	checked := 0
	for _, obj := range content.Objects {
		objType := strings.ToUpper(obj.Type)
		if objType != "CLAS" && objType != "PROG" && objType != "INTF" {
			continue
		}
		if checked >= 10 {
			break
		}
		revs, err := s.adtClient.GetRevisions(ctx, objType, obj.Name, nil)
		if err != nil || len(revs) == 0 {
			continue
		}
		tm, err := time.Parse(time.RFC3339, revs[0].Date)
		if err != nil {
			continue
		}
		if tm.After(newest) {
			newest = tm
		}
		checked++
	}
	if newest.IsZero() {
		return healthSignal{Status: "UNKNOWN"}
	}
	return stalenessFromTime(newest, checked)
}

func stalenessFromRevisions(revs []adt.Revision) healthSignal {
	if len(revs) == 0 {
		return healthSignal{Status: "UNKNOWN"}
	}
	tm, err := time.Parse(time.RFC3339, revs[0].Date)
	if err != nil {
		return healthSignal{Status: "ERROR", Details: map[string]any{"message": err.Error()}}
	}
	return stalenessFromTime(tm, 1)
}

func stalenessFromTime(tm time.Time, checked int) healthSignal {
	ageDays := int(time.Since(tm).Hours() / 24)
	status := "ACTIVE"
	switch {
	case ageDays > 365:
		status = "STALE"
	case ageDays > 90:
		status = "AGING"
	}
	return healthSignal{Status: status, Details: map[string]any{
		"last_changed": tm.Format(time.RFC3339),
		"age_days":     ageDays,
		"checked":      checked,
	}}
}

func summarizeHealth(signals map[string]healthSignal) healthSummary {
	if signals["tests"].Status == "FAIL" {
		return healthSummary{Status: "BAD", Headline: "Unit tests are failing"}
	}
	if signals["boundaries"].Status == "VIOLATIONS" {
		return healthSummary{Status: "WARN", Headline: "Boundary violations detected"}
	}
	if signals["atc"].Status == "FINDINGS" {
		return healthSummary{Status: "WARN", Headline: "ATC findings detected"}
	}
	if signals["staleness"].Status == "STALE" {
		return healthSummary{Status: "WARN", Headline: "Object or package appears stale"}
	}
	return healthSummary{Status: "GOOD", Headline: "No major health issues detected"}
}

func buildHealthObjectURL(objType, objName, parent string) string {
	switch objType {
	case "CLAS", "PROG", "INTF", "FUGR":
		return buildADTObjectURL(objType, objName)
	case "FUNC":
		if parent == "" {
			return ""
		}
		return fmt.Sprintf("/sap/bc/adt/functions/groups/%s/fmodules/%s", strings.ToLower(parent), strings.ToLower(objName))
	default:
		return ""
	}
}
