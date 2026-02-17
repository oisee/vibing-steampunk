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

	result, _ := json.MarshalIndent(info, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetInstalledComponents(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	components, err := s.adtClient.GetInstalledComponents(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get installed components: %v", err)), nil
	}

	result, _ := json.MarshalIndent(components, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetConnectionInfo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Return current connection info for introspection
	info := map[string]interface{}{
		"user":   s.config.Username,
		"url":    s.config.BaseURL,
		"client": s.config.Client,
		"mode":   s.config.Mode,
	}

	// Add feature summary
	info["features"] = s.featureProber.FeatureSummary(ctx)

	// Add debugger status
	info["debugger_user"] = strings.ToUpper(s.config.Username) // Debugger uses uppercase

	result, _ := json.MarshalIndent(info, "", "  ")
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

	result, _ := json.MarshalIndent(output, "", "  ")
	return mcp.NewToolResultText(string(result)), nil
}

func (s *Server) handleGetAbapHelp(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	keyword, _ := request.Params.Arguments["keyword"].(string)
	if keyword == "" {
		return newToolResultError("keyword is required"), nil
	}

	helpResult, err := s.adtClient.GetAbapHelp(ctx, keyword)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetAbapHelp failed: %v", err)), nil
	}

	// Try to get real documentation from SAP system via WebSocket (ZADT_VSP)
	// Use ensureWSConnected like GitExport does
	if errResult := s.ensureWSConnected(ctx, "GetAbapHelp"); errResult == nil {
		wsHelp, err := s.amdpWSClient.GetAbapDocumentation(ctx, keyword)
		if err == nil && wsHelp.Found && wsHelp.HTML != "" {
			helpResult.Documentation = wsHelp.HTML
		}
	}

	// Format output for LLM consumption
	var sb strings.Builder
	fmt.Fprintf(&sb, "ABAP Keyword: %s\n\n", helpResult.Keyword)
	fmt.Fprintf(&sb, "Documentation URL:\n  %s\n\n", helpResult.URL)
	fmt.Fprintf(&sb, "Search Query:\n  %s\n", helpResult.SearchQuery)

	if helpResult.Documentation != "" {
		fmt.Fprintf(&sb, "\n---\nDocumentation from SAP system:\n\n%s", helpResult.Documentation)
	} else {
		fmt.Fprintf(&sb, "\n---\nNote: For full documentation, use the URL above or WebSearch with the provided query.")
	}

	return mcp.NewToolResultText(sb.String()), nil
}
