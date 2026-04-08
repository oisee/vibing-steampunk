// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_workflow.go contains handlers for high-level workflow operations.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// routeWorkflowAction routes workflow operations for high-level create/edit.
func (s *Server) routeWorkflowAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action == "edit" {
		editType := getStringParam(params, "type")
		switch editType {
		case "write_program":
			return s.callHandler(ctx, s.handleWriteProgram, params)
		case "write_class":
			return s.callHandler(ctx, s.handleWriteClass, params)
		}
	}
	if action == "create" {
		switch objectType {
		case "PROGRAM":
			return s.callHandler(ctx, s.handleCreateAndActivateProgram, params)
		case "CLASS_WITH_TESTS":
			return s.callHandler(ctx, s.handleCreateClassWithTests, params)
		}
	}
	return nil, false, nil
}

// --- Workflow Handlers ---

func (s *Server) handleWriteProgram(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	programName, ok := request.GetArguments()["program_name"].(string)
	if !ok || programName == "" {
		return newToolResultError("program_name is required"), nil
	}

	source, ok := request.GetArguments()["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	result, err := s.adtClient.WriteProgram(ctx, programName, source, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("WriteProgram failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleWriteClass(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	className, ok := request.GetArguments()["class_name"].(string)
	if !ok || className == "" {
		return newToolResultError("class_name is required"), nil
	}

	source, ok := request.GetArguments()["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	result, err := s.adtClient.WriteClass(ctx, className, source, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("WriteClass failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleCreateAndActivateProgram(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	programName, ok := request.GetArguments()["program_name"].(string)
	if !ok || programName == "" {
		return newToolResultError("program_name is required"), nil
	}

	description, ok := request.GetArguments()["description"].(string)
	if !ok || description == "" {
		return newToolResultError("description is required"), nil
	}

	packageName, ok := request.GetArguments()["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}

	source, ok := request.GetArguments()["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	result, err := s.adtClient.CreateAndActivateProgram(ctx, programName, description, packageName, source, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("CreateAndActivateProgram failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleCreateClassWithTests(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	className, ok := request.GetArguments()["class_name"].(string)
	if !ok || className == "" {
		return newToolResultError("class_name is required"), nil
	}

	description, ok := request.GetArguments()["description"].(string)
	if !ok || description == "" {
		return newToolResultError("description is required"), nil
	}

	packageName, ok := request.GetArguments()["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}

	classSource, ok := request.GetArguments()["class_source"].(string)
	if !ok || classSource == "" {
		return newToolResultError("class_source is required"), nil
	}

	testSource, ok := request.GetArguments()["test_source"].(string)
	if !ok || testSource == "" {
		return newToolResultError("test_source is required"), nil
	}

	transport := ""
	if t, ok := request.GetArguments()["transport"].(string); ok {
		transport = t
	}

	result, err := s.adtClient.CreateClassWithTests(ctx, className, description, packageName, classSource, testSource, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("CreateClassWithTests failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}
