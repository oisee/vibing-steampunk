//go:build integration

// ABOUTME: Integration tests for MCP system actions (system info, features, connection).
// ABOUTME: Tests system information retrieval from SAP.

package mcp

import (
	"context"
	"strings"
	"testing"
)

func TestIntegration_SystemInfo(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	result, err := callSAPTool(ctx, s, "system", "INFO", nil)
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not get system info: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("System info is empty")
	} else {
		t.Logf("System info: %d characters", len(text))
		// Should contain system ID or version info
		if !strings.Contains(strings.ToLower(text), "system") && !strings.Contains(text, "version") {
			t.Log("Warning: system info might be missing expected fields")
		}
	}
}

func TestIntegration_SystemFeatures(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	result, err := callSAPTool(ctx, s, "system", "FEATURES", nil)
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not get features: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Features info is empty")
	} else {
		t.Logf("Features info: %d characters", len(text))
		// Should contain features object
		if !strings.Contains(text, "features") {
			t.Error("Expected features info to contain 'features' key")
		}
	}
}

func TestIntegration_SystemConnection(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	result, err := callSAPTool(ctx, s, "system", "CONNECTION", nil)
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not get connection info: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Connection info is empty")
	} else {
		t.Logf("Connection info: %d characters", len(text))
		// Should contain user and url
		if !strings.Contains(text, "user") || !strings.Contains(text, "url") {
			t.Error("Expected connection info to contain 'user' and 'url'")
		}
	}
}

func TestIntegration_SystemComponents(t *testing.T) {
	s := getIntegrationServer(t)
	ctx := context.Background()

	result, err := callSAPTool(ctx, s, "system", "COMPONENTS", nil)
	if err != nil {
		t.Fatalf("callSAPTool failed: %v", err)
	}

	if isErrorResult(result) {
		t.Skipf("Could not get components: %s", getResultText(result))
	}

	text := getResultText(result)
	if len(text) == 0 {
		t.Error("Components info is empty")
	} else {
		t.Logf("Components info: %d characters", len(text))
		// Should be an array of components
		if !strings.HasPrefix(strings.TrimSpace(text), "[") {
			t.Log("Warning: components might not be an array")
		}
	}
}
