// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_codeanalysis.go contains handlers for code analysis tools
// (AnalyzeABAPCode, CheckRegression).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- AnalyzeABAPCode Handler ---

func (s *Server) handleAnalyzeABAPCode(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, _ := request.Params.Arguments["object_uri"].(string)
	source, _ := request.Params.Arguments["source"].(string)

	if objectURI == "" && source == "" {
		return newToolResultError("either object_uri or source is required"), nil
	}

	result, err := s.adtClient.AnalyzeABAPCode(ctx, objectURI, source)
	if err != nil {
		return newToolResultError(fmt.Sprintf("AnalyzeABAPCode failed: %v", err)), nil
	}

	output, err2 := json.MarshalIndent(result, "", "  ")
	if err2 != nil {
		return newToolResultError(fmt.Sprintf("failed to serialize result: %v", err2)), nil
	}
	return mcp.NewToolResultText(string(output)), nil
}

// --- CheckRegression Handler ---

func (s *Server) handleCheckRegression(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.Params.Arguments["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	baseVersion, _ := request.Params.Arguments["base_version"].(string)

	result, err := s.adtClient.CheckRegression(ctx, objectURI, baseVersion)
	if err != nil {
		return newToolResultError(fmt.Sprintf("CheckRegression failed: %v", err)), nil
	}

	output, err2 := json.MarshalIndent(result, "", "  ")
	if err2 != nil {
		return newToolResultError(fmt.Sprintf("failed to serialize result: %v", err2)), nil
	}
	return mcp.NewToolResultText(string(output)), nil
}
