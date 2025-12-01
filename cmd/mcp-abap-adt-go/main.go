// mcp-abap-adt-go is an MCP server providing ABAP Development Tools (ADT) functionality.
package main

import (
	"fmt"
	"os"

	"github.com/vibingsteamer/mcp-abap-adt-go/internal/mcp"
)

func main() {
	// Get configuration from environment variables
	cfg := &mcp.Config{
		BaseURL:            getEnv("SAP_URL", ""),
		Username:           getEnv("SAP_USER", ""),
		Password:           getEnv("SAP_PASSWORD", ""),
		Client:             getEnv("SAP_CLIENT", "001"),
		Language:           getEnv("SAP_LANGUAGE", "EN"),
		InsecureSkipVerify: getEnv("SAP_INSECURE", "false") == "true",
	}

	// Validate required configuration
	if cfg.BaseURL == "" {
		fmt.Fprintln(os.Stderr, "Error: SAP_URL environment variable is required")
		os.Exit(1)
	}
	if cfg.Username == "" {
		fmt.Fprintln(os.Stderr, "Error: SAP_USER environment variable is required")
		os.Exit(1)
	}
	if cfg.Password == "" {
		fmt.Fprintln(os.Stderr, "Error: SAP_PASSWORD environment variable is required")
		os.Exit(1)
	}

	// Create and start MCP server
	server := mcp.NewServer(cfg)
	if err := server.ServeStdio(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
