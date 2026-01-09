// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_system.go contains handlers for system information operations.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- System Information Handlers ---

func (s *Server) handleGetSystemInfo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	info, err := s.adtClient.GetSystemInfo(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get system info: %v", err)), nil
	}

	result, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to marshal system info: %v", err)), nil
	}
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetInstalledComponents(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	components, err := s.adtClient.GetInstalledComponents(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get installed components: %v", err)), nil
	}

	result, err := json.MarshalIndent(components, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to marshal components: %v", err)), nil
	}
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetConnectionInfo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Return current connection info for introspection
	info := map[string]any{
		"user":   s.config.Username,
		"url":    s.config.BaseURL,
		"client": s.config.Client,
		"mode":   s.config.Mode,
	}

	// Add feature summary
	info["features"] = s.featureProber.FeatureSummary(ctx)

	// Add debugger status
	info["debugger_user"] = strings.ToUpper(s.config.Username) // Debugger uses uppercase

	result, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to marshal connection info: %v", err)), nil
	}
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetFeatures(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Probe all features
	results := s.featureProber.ProbeAll(ctx)

	// Format output
	type featureOutput struct {
		Features map[string]*adt.FeatureStatus `json:"features"`
		Summary  string                        `json:"summary"`
	}

	output := featureOutput{
		Features: make(map[string]*adt.FeatureStatus),
		Summary:  s.featureProber.FeatureSummary(ctx),
	}

	for id, status := range results {
		output.Features[string(id)] = status
	}

	result, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to marshal features: %v", err)), nil
	}
	return mcp.NewToolResultText(string(result)), nil
}
