// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_classinclude.go contains handlers for class include operations (testclasses, locals_def, etc.).
package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/vinchacho/vibing-steampunk/pkg/adt"
)

// routeClassIncludeAction routes class include operations.
func (s *Server) routeClassIncludeAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	switch {
	case action == "read" && objectType == "CLAS_INCLUDE":
		return s.callHandler(ctx, s.handleGetClassInclude, map[string]any{
			"class_name":   objectName,
			"include_type": getStringParam(params, "include_type"),
		})
	case action == "create" && objectType == "CLAS_TEST_INCLUDE":
		return s.callHandler(ctx, s.handleCreateTestInclude, params)
	case action == "edit" && objectType == "CLAS_INCLUDE":
		return s.callHandler(ctx, s.handleUpdateClassInclude, params)
	}
	return nil, false, nil
}

// --- Class Include Handlers ---

func (s *Server) handleGetClassInclude(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	className, ok := request.GetArguments()["class_name"].(string)
	if !ok || className == "" {
		return newToolResultError("class_name is required"), nil
	}

	includeType, ok := request.GetArguments()["include_type"].(string)
	if !ok || includeType == "" {
		return newToolResultError("include_type is required"), nil
	}

	source, err := s.adtClient.GetClassInclude(ctx, className, adt.ClassIncludeType(includeType))
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to get class include: %v", err)), nil
	}

	return mcp.NewToolResultText(source), nil
}

func (s *Server) handleCreateTestInclude(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	className, ok := request.GetArguments()["class_name"].(string)
	if !ok || className == "" {
		return newToolResultError("class_name is required"), nil
	}

	lockHandle, ok := request.GetArguments()["lock_handle"].(string)
	if !ok || lockHandle == "" {
		return newToolResultError("lock_handle is required"), nil
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	err := s.adtClient.CreateTestInclude(ctx, className, lockHandle, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to create test include: %v", err)), nil
	}

	return mcp.NewToolResultText("Test include created successfully"), nil
}

func (s *Server) handleUpdateClassInclude(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	className, ok := request.GetArguments()["class_name"].(string)
	if !ok || className == "" {
		return newToolResultError("class_name is required"), nil
	}

	includeType, ok := request.GetArguments()["include_type"].(string)
	if !ok || includeType == "" {
		return newToolResultError("include_type is required"), nil
	}

	source, ok := request.GetArguments()["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	lockHandle, ok := request.GetArguments()["lock_handle"].(string)
	if !ok || lockHandle == "" {
		return newToolResultError("lock_handle is required"), nil
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	err := s.adtClient.UpdateClassInclude(ctx, className, adt.ClassIncludeType(includeType), source, lockHandle, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to update class include: %v", err)), nil
	}

	return mcp.NewToolResultText("Class include updated successfully"), nil
}
