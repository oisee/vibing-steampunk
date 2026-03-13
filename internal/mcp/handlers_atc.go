// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_atc.go contains handlers for ATC (ABAP Test Cockpit) operations.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- ATC Handlers ---

func (s *Server) handleRunATCCheck(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.GetArguments()["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	variant := ""
	if v, ok := request.GetArguments()["variant"].(string); ok {
		variant = v
	}

	maxResults := 100
	if mr, ok := request.GetArguments()["max_results"].(float64); ok && mr > 0 {
		maxResults = int(mr)
	}

	result, err := s.adtClient.RunATCCheck(ctx, objectURL, variant, maxResults)
	if err != nil {
		return newToolResultError(fmt.Sprintf("ATC check failed: %v", err)), nil
	}

	// Format output with summary
	type summary struct {
		TotalObjects  int `json:"totalObjects"`
		TotalFindings int `json:"totalFindings"`
		Errors        int `json:"errors"`
		Warnings      int `json:"warnings"`
		Infos         int `json:"infos"`
	}
	type output struct {
		Summary  summary          `json:"summary"`
		Worklist *adt.ATCWorklist `json:"worklist"`
	}

	sum := summary{TotalObjects: len(result.Objects)}
	for _, obj := range result.Objects {
		sum.TotalFindings += len(obj.Findings)
		for _, f := range obj.Findings {
			switch f.Priority {
			case 1:
				sum.Errors++
			case 2:
				sum.Warnings++
			default:
				sum.Infos++
			}
		}
	}

	out := output{Summary: sum, Worklist: result}
	outputJSON, _ := json.MarshalIndent(out, "", "  ")
	return mcp.NewToolResultText(string(outputJSON)), nil
}

func (s *Server) handleGetATCCustomizing(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := s.adtClient.GetATCCustomizing(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get ATC customizing: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}
