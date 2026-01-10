// ABOUTME: Integration test helpers for MCP server integration tests.
// ABOUTME: Provides getIntegrationServer() and callSAPTool() helpers for testing.

package mcp

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

var (
	envOnce   sync.Once
	envLoaded bool
)

// loadEnvFile reads a .env file and sets environment variables.
// Searches for .env in current directory and up to 5 parent directories.
func loadEnvFile() {
	envOnce.Do(func() {
		// Find .env file by walking up directories
		dir, err := os.Getwd()
		if err != nil {
			return
		}

		for range 6 {
			envPath := filepath.Join(dir, ".env")
			if _, err := os.Stat(envPath); err == nil {
				if parseEnvFile(envPath) == nil {
					envLoaded = true
					return
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	})
}

// parseEnvFile reads a .env file and sets environment variables.
func parseEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Parse KEY=VALUE
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			// Only set if not already set (env vars take precedence)
			if os.Getenv(key) == "" {
				os.Setenv(key, value)
			}
		}
	}
	return scanner.Err()
}

// getIntegrationServer creates an MCP server for integration tests.
// Loads credentials from .env file or environment variables.
func getIntegrationServer(t *testing.T) *Server {
	loadEnvFile()

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
