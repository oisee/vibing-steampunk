// Package mcp provides MCP handlers for ABAP code analysis tools.
// AnalyzeABAPCode uses the native Go abaplint lexer+parser for analysis.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handleAnalyzeABAPCode(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectType, _ := request.GetArguments()["object_type"].(string)
	objectName, _ := request.GetArguments()["object_name"].(string)
	source, _ := request.GetArguments()["source"].(string)

	if source == "" {
		if objectName == "" {
			return newToolResultError("either object_name (with object_type) or source is required"), nil
		}
		if objectType == "" {
			return newToolResultError("object_type is required when object_name is provided"), nil
		}
	}

	result, err := s.adtClient.AnalyzeABAPCode(ctx, objectType, objectName, source)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Code analysis failed: %v", err)), nil
	}

	output, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to encode result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(output)), nil
}
