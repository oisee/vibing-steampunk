package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleGetCDSImpactAnalysis returns reverse dependencies (where-used) for a CDS view.
func (s *Server) handleGetCDSImpactAnalysis(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	viewName, ok := request.Params.Arguments["view_name"].(string)
	if !ok || viewName == "" {
		return newToolResultError("view_name is required"), nil
	}

	result, err := s.adtClient.GetCDSImpactAnalysis(ctx, viewName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetCDSImpactAnalysis failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

// handleGetCDSElementInfo returns metadata for all elements (fields) of a CDS view.
func (s *Server) handleGetCDSElementInfo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	viewName, ok := request.Params.Arguments["view_name"].(string)
	if !ok || viewName == "" {
		return newToolResultError("view_name is required"), nil
	}

	result, err := s.adtClient.GetCDSElementInfo(ctx, viewName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetCDSElementInfo failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}
