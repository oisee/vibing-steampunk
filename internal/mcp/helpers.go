// Package mcp provides the MCP server implementation for ABAP ADT tools.
// helpers.go contains shared utility functions used across handlers.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// newToolResultError creates an error result for tool execution failures.
func newToolResultError(message string) *mcp.CallToolResult {
	result := mcp.NewToolResultText(message)
	result.IsError = true
	return result
}

// newToolResultJSON creates a successful result with JSON-formatted output.
func newToolResultJSON(v any) *mcp.CallToolResult {
	output, _ := json.MarshalIndent(v, "", "  ")
	return mcp.NewToolResultText(string(output))
}

// wrapErr creates an error result with consistent "operation failed" format.
func wrapErr(op string, err error) *mcp.CallToolResult {
	return newToolResultError(fmt.Sprintf("%s failed: %v", op, err))
}

// --- Parameter extraction helpers ---

// getStr extracts a string parameter, returning empty string if not found.
func getStr(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

// getInt extracts an integer parameter from float64, returning default if not found.
func getInt(args map[string]any, key string, def int) int {
	if v, ok := args[key].(float64); ok {
		return int(v)
	}
	return def
}

// getBool extracts a boolean parameter, returning default if not found.
func getBool(args map[string]any, key string, def bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return def
}

// requireStr extracts a required string parameter, returning error result if missing.
func requireStr(args map[string]any, key string) (string, *mcp.CallToolResult) {
	if v, ok := args[key].(string); ok && v != "" {
		return v, nil
	}
	return "", newToolResultError(key + " is required")
}

// ensureWSConnected ensures the WebSocket client is connected, creating it if needed.
// Returns error result if connection fails, nil on success.
func (s *Server) ensureWSConnected(ctx context.Context, toolName string) *mcp.CallToolResult {
	if s.amdpWSClient == nil || !s.amdpWSClient.IsConnected() {
		s.amdpWSClient = adt.NewAMDPWebSocketClient(
			s.config.BaseURL, s.config.Client, s.config.Username, s.config.Password, s.config.InsecureSkipVerify,
		)
		if err := s.amdpWSClient.Connect(ctx); err != nil {
			s.amdpWSClient = nil
			return newToolResultError(fmt.Sprintf("%s: WebSocket connect failed: %v", toolName, err))
		}
	}
	return nil
}

// copyOptional copies optional string params from src to dst if present.
func copyOptional(dst, src map[string]any, keys ...string) {
	for _, key := range keys {
		if v, ok := src[key].(string); ok && v != "" {
			dst[key] = v
		}
	}
}

// copyOptionalBool copies optional bool params from src to dst if present.
func copyOptionalBool(dst, src map[string]any, keys ...string) {
	for _, key := range keys {
		if v, ok := src[key].(bool); ok {
			dst[key] = v
		}
	}
}
