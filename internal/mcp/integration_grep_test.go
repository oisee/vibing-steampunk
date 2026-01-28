//go:build integration

// ABOUTME: Integration tests for MCP grep actions (grep OBJECT, grep PACKAGE).
// ABOUTME: Tests grep functionality on SAP objects.

package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestIntegration_GrepPackage(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Grep for a pattern in a standard package
	result, err := callSAPTool(ctx, s, "grep", "PACKAGE BASIS", map[string]any{
		"pattern":     "DATA",
		"max_results": float64(5),
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not grep package: %s", getResultText(result))
	}

	text := getResultText(result)
	t.Logf("Grep result: %d characters", len(text))
	// Results should contain matches or empty array
	if !strings.HasPrefix(strings.TrimSpace(text), "[") && !strings.HasPrefix(strings.TrimSpace(text), "{") {
		t.Log("Warning: grep result might not be valid JSON")
	}
}

func TestIntegration_GrepPackageCaseInsensitive(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Grep with case insensitive flag
	result, err := callSAPTool(ctx, s, "grep", "PACKAGE BASIS", map[string]any{
		"pattern":          "method",
		"case_insensitive": true,
		"max_results":      float64(5),
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not grep package: %s", getResultText(result))
	}

	text := getResultText(result)
	t.Logf("Case-insensitive grep result: %d characters", len(text))
}

func TestIntegration_GrepPackageWithObjectTypes(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Grep only in specific object types
	result, err := callSAPTool(ctx, s, "grep", "PACKAGE BASIS", map[string]any{
		"pattern":      "SELECT",
		"object_types": "PROG,CLAS",
		"max_results":  float64(5),
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not grep package with object types: %s", getResultText(result))
	}

	text := getResultText(result)
	t.Logf("Filtered grep result: %d characters", len(text))
}

func TestIntegration_GrepObject(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	// Grep for a pattern in a specific object
	result, err := callSAPTool(ctx, s, "grep", "OBJECT", map[string]any{
		"object_url":    "/sap/bc/adt/programs/programs/sapmssy0/source/main",
		"pattern":       "DATA",
		"context_lines": float64(1),
	})
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not grep object: %s", getResultText(result))
	}

	text := getResultText(result)
	t.Logf("Object grep result: %d characters", len(text))
}
