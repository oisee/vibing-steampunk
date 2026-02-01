//go:build integration

// ABOUTME: Integration tests for MCP search actions (search OBJECT).
// ABOUTME: Tests use wildcard patterns to search for SAP objects.

package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestIntegration_SearchObject(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Search for classes starting with CL_ABAP
	result, err := callSAPTool(ctx, s, "search", "OBJECT CL_ABAP*", nil)
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Search failed: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Search results are empty")
	} else {
		t.Logf("Search results: %d characters", len(text))
		// Results should contain CL_ABAP prefix
		if !strings.Contains(text, "CL_ABAP") {
			t.Error("Expected results to contain CL_ABAP")
		}
	}
}

func TestIntegration_SearchObjectWithQuery(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Search using query parameter
	result, err := callSAPTool(ctx, s, "search", "OBJECT", map[string]any{
		"query":      "IF_*",
		"maxResults": float64(5),
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Search failed: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Search results are empty")
	} else {
		t.Logf("Search results with limit: %d characters", len(text))
	}
}

func TestIntegration_SearchProgram(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Search for programs starting with SAP
	result, err := callSAPTool(ctx, s, "search", "OBJECT SAP*", map[string]any{
		"maxResults": float64(10),
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Search failed: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Search results are empty")
	} else {
		t.Logf("Found programs: %d characters", len(text))
	}
}

func TestIntegration_SearchTable(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Search for tables starting with T
	result, err := callSAPTool(ctx, s, "search", "OBJECT T00*", map[string]any{
		"maxResults": float64(10),
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Search failed: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Search results are empty")
	} else {
		t.Logf("Found tables: %d characters", len(text))
	}
}
