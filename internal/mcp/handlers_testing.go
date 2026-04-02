// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_testing.go contains handlers for testing & quality operations
// (Code Coverage, SQL Explain Plan, Check Run Results).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Code Coverage Handler ---

func (s *Server) handleGetCodeCoverage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.Params.Arguments["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	flags := adt.DefaultUnitTestFlags()
	if includeDangerous, ok := request.Params.Arguments["include_dangerous"].(bool); ok && includeDangerous {
		flags.Dangerous = true
	}
	if includeLong, ok := request.Params.Arguments["include_long"].(bool); ok && includeLong {
		flags.Long = true
	}

	result, err := s.adtClient.GetCodeCoverage(ctx, objectURL, &flags)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetCodeCoverage failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

// --- SQL Explain Plan Handler ---

func (s *Server) handleGetSQLExplainPlan(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sqlQuery, ok := request.Params.Arguments["sql_query"].(string)
	if !ok || sqlQuery == "" {
		return newToolResultError("sql_query is required"), nil
	}

	result, err := s.adtClient.GetSQLExplainPlan(ctx, sqlQuery)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetSQLExplainPlan failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

// --- Check Run Results Handler ---

func (s *Server) handleGetCheckRunResults(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	checkRunID, ok := request.Params.Arguments["check_run_id"].(string)
	if !ok || checkRunID == "" {
		return newToolResultError("check_run_id is required"), nil
	}

	result, err := s.adtClient.GetCheckRunResults(ctx, checkRunID)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetCheckRunResults failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}
