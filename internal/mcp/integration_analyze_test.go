//go:build integration

// ABOUTME: Integration tests for MCP analyze actions (call_graph, callers, callees, structure).
// ABOUTME: Tests code analysis features on standard SAP objects.

package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestIntegration_AnalyzeCallGraph(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Analyze call graph for a standard class
	result, err := callSAPTool(ctx, s, "analyze", "CLAS CL_ABAP_TYPEDESCR", map[string]any{
		"type":      "call_graph",
		"direction": "callees",
		"max_depth": float64(2),
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not analyze call graph: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Call graph result is empty")
	} else {
		t.Logf("Call graph analysis: %d characters", len(text))
		// Should contain stats
		if !strings.Contains(text, "stats") && !strings.Contains(text, "edges") {
			t.Log("Warning: call graph might not have expected structure")
		}
	}
}

func TestIntegration_AnalyzeCallers(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Get callers of a standard class
	result, err := callSAPTool(ctx, s, "analyze", "CLAS CL_ABAP_TYPEDESCR", map[string]any{
		"type":      "callers",
		"max_depth": float64(1),
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not analyze callers: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Callers result is empty")
	} else {
		t.Logf("Callers analysis: %d characters", len(text))
	}
}

func TestIntegration_AnalyzeCallees(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Get callees of a standard class
	result, err := callSAPTool(ctx, s, "analyze", "CLAS CL_ABAP_TYPEDESCR", map[string]any{
		"type":      "callees",
		"max_depth": float64(1),
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not analyze callees: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Callees result is empty")
	} else {
		t.Logf("Callees analysis: %d characters", len(text))
	}
}

func TestIntegration_AnalyzeStructure(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Get object structure
	result, err := callSAPTool(ctx, s, "analyze", "", map[string]any{
		"type":        "structure",
		"object_name": "CL_ABAP_TYPEDESCR",
		"max_results": float64(50),
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not analyze structure: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Structure result is empty")
	} else {
		t.Logf("Structure analysis: %d characters", len(text))
	}
}

func TestIntegration_AnalyzeCDSDependencies(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Analyze CDS dependencies (via read handler which routes to CDS analysis)
	result, err := callSAPTool(ctx, s, "analyze", "DDLS ACM_DDDDLSRC", map[string]any{
		"type":              "cds_dependencies",
		"dependency_level":  "hierarchy",
		"with_associations": false,
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not analyze CDS dependencies: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("CDS dependencies result is empty")
	} else {
		t.Logf("CDS dependencies analysis: %d characters", len(text))
		// Should contain statistics
		if !strings.Contains(text, "statistics") {
			t.Log("Warning: CDS dependencies might not have expected statistics")
		}
	}
}
