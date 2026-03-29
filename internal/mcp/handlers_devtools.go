// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_devtools.go contains handlers for development tools (syntax check, activation, unit tests).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/vinchacho/vibing-steampunk/pkg/adt"
)

// routeDevToolsAction routes "test" (unit tests), "analyze" (syntax check), "edit" (activate), and "analyze" (execute_abap).
func (s *Server) routeDevToolsAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action == "test" {
		analysisType := getStringParam(params, "type")
		if analysisType == "" || analysisType == "unit" {
			// Unit tests
			objectURL := getStringParam(params, "object_url")
			if objectURL == "" {
				return nil, false, nil
			}
			args := map[string]any{"object_url": objectURL}
			if v, ok := getBoolParam(params, "include_dangerous"); ok {
				args["include_dangerous"] = v
			}
			if v, ok := getBoolParam(params, "include_long"); ok {
				args["include_long"] = v
			}
			return s.callHandler(ctx, s.handleRunUnitTests, args)
		}
	}

	if action == "analyze" {
		analysisType := getStringParam(params, "type")
		switch analysisType {
		case "syntax_check":
			return s.callHandler(ctx, s.handleSyntaxCheck, params)
		case "execute_abap":
			return s.callHandler(ctx, s.handleExecuteABAP, params)
		}
	}

	if action == "edit" {
		switch objectType {
		case "ACTIVATE":
			return s.callHandler(ctx, s.handleActivate, params)
		case "ACTIVATE_PACKAGE":
			return s.callHandler(ctx, s.handleActivatePackage, params)
		}
	}

	return nil, false, nil
}

// --- Development Tool Handlers ---

func (s *Server) handleSyntaxCheck(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.Params.Arguments["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	content, ok := request.Params.Arguments["content"].(string)
	if !ok || content == "" {
		return newToolResultError("content is required"), nil
	}

	results, err := s.adtClient.SyntaxCheck(ctx, objectURL, content)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Syntax check failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleActivate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.Params.Arguments["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	objectName, ok := request.Params.Arguments["object_name"].(string)
	if !ok || objectName == "" {
		return newToolResultError("object_name is required"), nil
	}

	result, err := s.adtClient.Activate(ctx, objectURL, objectName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Activation failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleActivatePackage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	packageName := ""
	if pkg, ok := request.Params.Arguments["package"].(string); ok {
		packageName = pkg
	}

	maxObjects := 100
	if max, ok := request.Params.Arguments["max_objects"].(float64); ok && max > 0 {
		maxObjects = int(max)
	}

	result, err := s.adtClient.ActivatePackage(ctx, packageName, maxObjects)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Batch activation failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleRunUnitTests(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.Params.Arguments["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	// Build flags from optional parameters
	flags := adt.DefaultUnitTestFlags()

	if includeDangerous, ok := request.Params.Arguments["include_dangerous"].(bool); ok && includeDangerous {
		flags.Dangerous = true
	}

	if includeLong, ok := request.Params.Arguments["include_long"].(bool); ok && includeLong {
		flags.Long = true
	}

	result, err := s.adtClient.RunUnitTests(ctx, objectURL, &flags)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Unit test run failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}
