// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_refactoring.go contains handlers for ADT refactoring operations
// (Rename, Extract Method, Quick Fix, ATC QuickFix).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- Refactoring Handlers ---

func (s *Server) handleRenameRefactoring(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, _ := request.Params.Arguments["object_uri"].(string)
	step, _ := request.Params.Arguments["step"].(string)
	newName, _ := request.Params.Arguments["new_name"].(string)
	source, _ := request.Params.Arguments["source"].(string)

	if objectURI == "" || step == "" || newName == "" {
		return newToolResultError("object_uri, step, and new_name are required"), nil
	}

	line := 1
	col := 1
	if l, ok := request.Params.Arguments["line"].(float64); ok {
		line = int(l)
	}
	if c, ok := request.Params.Arguments["column"].(float64); ok {
		col = int(c)
	}

	switch step {
	case "evaluate":
		if source == "" {
			return newToolResultError("source is required for evaluate step"), nil
		}
		result, err := s.adtClient.RenameEvaluate(ctx, objectURI, line, col, source, newName)
		if err != nil {
			return newToolResultError(fmt.Sprintf("RenameRefactoring evaluate failed: %v", err)), nil
		}
		output, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(output)), nil

	case "preview":
		if source == "" {
			return newToolResultError("source is required for preview step"), nil
		}
		result, err := s.adtClient.RenamePreview(ctx, objectURI, line, col, source, newName)
		if err != nil {
			return newToolResultError(fmt.Sprintf("RenameRefactoring preview failed: %v", err)), nil
		}
		output, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(output)), nil

	case "execute":
		if source == "" {
			return newToolResultError("source is required for execute step"), nil
		}
		result, err := s.adtClient.RenameExecute(ctx, objectURI, line, col, source, newName)
		if err != nil {
			return newToolResultError(fmt.Sprintf("RenameRefactoring execute failed: %v", err)), nil
		}
		output, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(output)), nil

	default:
		return newToolResultError(fmt.Sprintf("invalid step '%s': must be evaluate, preview, or execute", step)), nil
	}
}

func (s *Server) handleExtractMethod(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, _ := request.Params.Arguments["object_uri"].(string)
	step, _ := request.Params.Arguments["step"].(string)
	methodName, _ := request.Params.Arguments["method_name"].(string)
	source, _ := request.Params.Arguments["source"].(string)

	if objectURI == "" || step == "" || methodName == "" {
		return newToolResultError("object_uri, step, and method_name are required"), nil
	}
	if source == "" {
		return newToolResultError("source is required"), nil
	}

	startLine := 1
	startCol := 1
	endLine := 1
	endCol := 1
	if v, ok := request.Params.Arguments["start_line"].(float64); ok {
		startLine = int(v)
	}
	if v, ok := request.Params.Arguments["start_col"].(float64); ok {
		startCol = int(v)
	}
	if v, ok := request.Params.Arguments["end_line"].(float64); ok {
		endLine = int(v)
	}
	if v, ok := request.Params.Arguments["end_col"].(float64); ok {
		endCol = int(v)
	}

	switch step {
	case "evaluate":
		result, err := s.adtClient.ExtractMethodEvaluate(ctx, objectURI, startLine, startCol, endLine, endCol, source, methodName)
		if err != nil {
			return newToolResultError(fmt.Sprintf("ExtractMethod evaluate failed: %v", err)), nil
		}
		output, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(output)), nil

	case "preview":
		result, err := s.adtClient.ExtractMethodPreview(ctx, objectURI, startLine, startCol, endLine, endCol, source, methodName)
		if err != nil {
			return newToolResultError(fmt.Sprintf("ExtractMethod preview failed: %v", err)), nil
		}
		output, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(output)), nil

	case "execute":
		result, err := s.adtClient.ExtractMethodExecute(ctx, objectURI, startLine, startCol, endLine, endCol, source, methodName)
		if err != nil {
			return newToolResultError(fmt.Sprintf("ExtractMethod execute failed: %v", err)), nil
		}
		output, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(output)), nil

	default:
		return newToolResultError(fmt.Sprintf("invalid step '%s': must be evaluate, preview, or execute", step)), nil
	}
}

// --- Quick Fix Handlers ---

func (s *Server) handleGetQuickFixProposals(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, _ := request.Params.Arguments["object_uri"].(string)
	source, _ := request.Params.Arguments["source"].(string)

	if objectURI == "" || source == "" {
		return newToolResultError("object_uri and source are required"), nil
	}

	line := 1
	col := 1
	if l, ok := request.Params.Arguments["line"].(float64); ok {
		line = int(l)
	}
	if c, ok := request.Params.Arguments["column"].(float64); ok {
		col = int(c)
	}

	result, err := s.adtClient.GetQuickFixProposals(ctx, objectURI, line, col, source)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetQuickFixProposals failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleApplyQuickFix(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, _ := request.Params.Arguments["object_uri"].(string)
	proposalID, _ := request.Params.Arguments["proposal_id"].(string)
	source, _ := request.Params.Arguments["source"].(string)

	if objectURI == "" || proposalID == "" || source == "" {
		return newToolResultError("object_uri, proposal_id, and source are required"), nil
	}

	line := 1
	col := 1
	if l, ok := request.Params.Arguments["line"].(float64); ok {
		line = int(l)
	}
	if c, ok := request.Params.Arguments["column"].(float64); ok {
		col = int(c)
	}

	result, err := s.adtClient.ApplyQuickFix(ctx, objectURI, proposalID, line, col, source)
	if err != nil {
		return newToolResultError(fmt.Sprintf("ApplyQuickFix failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

// --- ATC Quick Fix Handlers ---

func (s *Server) handleApplyATCQuickFix(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	findingID, _ := request.Params.Arguments["finding_id"].(string)

	if findingID == "" {
		return newToolResultError("finding_id is required (from RunATCCheck finding's quickfixInfo field)"), nil
	}

	step, _ := request.Params.Arguments["step"].(string)
	if step == "" {
		step = "apply"
	}

	switch step {
	case "details":
		result, err := s.adtClient.GetATCQuickFixDetails(ctx, findingID)
		if err != nil {
			return newToolResultError(fmt.Sprintf("GetATCQuickFixDetails failed: %v", err)), nil
		}
		output, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(output)), nil

	case "apply":
		result, err := s.adtClient.ApplyATCQuickFix(ctx, findingID)
		if err != nil {
			return newToolResultError(fmt.Sprintf("ApplyATCQuickFix failed: %v", err)), nil
		}
		output, _ := json.MarshalIndent(result, "", "  ")
		return mcp.NewToolResultText(string(output)), nil

	default:
		return newToolResultError(fmt.Sprintf("invalid step '%s': must be details or apply", step)), nil
	}
}
