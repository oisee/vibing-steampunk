// ABOUTME: Integration test helpers for MCP server integration tests.
// ABOUTME: Provides getIntegrationServer() and callSAPTool() helpers for testing.

package mcp

import (
	"context"
	"os"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/testutil"
)

// getIntegrationServer creates an MCP server for integration tests.
// Loads credentials from .env file or environment variables.
func getIntegrationServer(t *testing.T) *Server {
	testutil.LoadEnv()

	url := os.Getenv("SAP_URL")
	user := os.Getenv("SAP_USER")
	pass := os.Getenv("SAP_PASSWORD")

	if url == "" || user == "" || pass == "" {
		t.Skip("SAP_URL, SAP_USER, SAP_PASSWORD required for integration tests (set in .env or environment)")
	}

	client := os.Getenv("SAP_CLIENT")
	if client == "" {
		client = "001"
	}
	lang := os.Getenv("SAP_LANGUAGE")
	if lang == "" {
		lang = "EN"
	}

	cfg := &Config{
		BaseURL:            url,
		Username:           user,
		Password:           pass,
		Client:             client,
		Language:           lang,
		InsecureSkipVerify: os.Getenv("SAP_INSECURE") == "true",
	}

	return NewServer(cfg)
}

// callSAPTool calls the universal SAP tool with given action, target, and params.
// Returns the result and any error. Handles error formatting for test assertions.
func callSAPTool(ctx context.Context, s *Server, action, target string, params map[string]any) (*mcp.CallToolResult, error) {
	args := map[string]any{
		"action": action,
		"target": target,
	}
	if params != nil {
		args["params"] = params
	}

	return s.handleUniversalTool(ctx, newRequest(args))
}

// getResultText extracts the text content from a tool result.
func getResultText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if textContent, ok := result.Content[0].(mcp.TextContent); ok {
		return textContent.Text
	}
	return ""
}

// isErrorResult checks if the result indicates an error.
func isErrorResult(result *mcp.CallToolResult) bool {
	if result == nil {
		return true
	}
	return result.IsError
}
