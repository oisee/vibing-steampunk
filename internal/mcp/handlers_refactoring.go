// Package mcp provides MCP handlers for ADT refactoring tools.
// Uses correct ADT API patterns: POST /sap/bc/adt/refactorings with ?step= and ?rel=.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handleRenameEvaluate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.GetArguments()["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required (include #start=line,col;end=line,col fragment)"), nil
	}

	result, err := s.adtClient.RenameEvaluate(ctx, objectURI)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Rename evaluate failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleRenameExecute(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.GetArguments()["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	newName, ok := request.GetArguments()["new_name"].(string)
	if !ok || newName == "" {
		return newToolResultError("new_name is required"), nil
	}

	transport := ""
	if v, ok := request.GetArguments()["transport"].(string); ok {
		transport = v
	}

	result, err := s.adtClient.RenameExecute(ctx, objectURI, newName, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Rename execute failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleGetQuickFixProposals(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.GetArguments()["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	source := ""
	if v, ok := request.GetArguments()["source"].(string); ok {
		source = v
	}

	result, err := s.adtClient.GetQuickFixProposals(ctx, objectURI, source)
	if err != nil {
		return newToolResultError(fmt.Sprintf("QuickFix evaluation failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}
