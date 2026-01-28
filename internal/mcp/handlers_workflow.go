// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_workflow.go contains handlers for high-level workflow operations.
package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- Workflow Routing ---
// Routes for this module:
//   create: type=write_program, type=write_class, type=create_activate_program, type=create_class_with_tests

// routeWorkflowAction routes high-level workflow actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeWorkflowAction(ctx context.Context, action, _, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "create" {
		return nil, false, nil
	}

	createType, _ := params["type"].(string)
	switch createType {
	case "write_program":
		programName, _ := params["program_name"].(string)
		if programName == "" {
			programName = objectName
		}
		if programName == "" {
			return newToolResultError("program_name is required"), true, nil
		}
		source, _ := params["source"].(string)
		if source == "" {
			return newToolResultError("source is required"), true, nil
		}
		args := map[string]any{"program_name": programName, "source": source}
		if transport, ok := params["transport"].(string); ok {
			args["transport"] = transport
		}
		result, err := s.handleWriteProgram(ctx, newRequest(args))
		return result, true, err

	case "write_class":
		className, _ := params["class_name"].(string)
		if className == "" {
			className = objectName
		}
		if className == "" {
			return newToolResultError("class_name is required"), true, nil
		}
		source, _ := params["source"].(string)
		if source == "" {
			return newToolResultError("source is required"), true, nil
		}
		args := map[string]any{"class_name": className, "source": source}
		if transport, ok := params["transport"].(string); ok {
			args["transport"] = transport
		}
		result, err := s.handleWriteClass(ctx, newRequest(args))
		return result, true, err

	case "create_activate_program":
		programName, _ := params["program_name"].(string)
		description, _ := params["description"].(string)
		packageName, _ := params["package_name"].(string)
		source, _ := params["source"].(string)
		if programName == "" || description == "" || packageName == "" || source == "" {
			return newToolResultError("program_name, description, package_name, and source are required"), true, nil
		}
		args := map[string]any{
			"program_name": programName,
			"description":  description,
			"package_name": packageName,
			"source":       source,
		}
		if transport, ok := params["transport"].(string); ok {
			args["transport"] = transport
		}
		result, err := s.handleCreateAndActivateProgram(ctx, newRequest(args))
		return result, true, err

	case "create_class_with_tests":
		className, _ := params["class_name"].(string)
		description, _ := params["description"].(string)
		packageName, _ := params["package_name"].(string)
		classSource, _ := params["class_source"].(string)
		testSource, _ := params["test_source"].(string)
		if className == "" || description == "" || packageName == "" || classSource == "" || testSource == "" {
			return newToolResultError("class_name, description, package_name, class_source, and test_source are required"), true, nil
		}
		args := map[string]any{
			"class_name":   className,
			"description":  description,
			"package_name": packageName,
			"class_source": classSource,
			"test_source":  testSource,
		}
		if transport, ok := params["transport"].(string); ok {
			args["transport"] = transport
		}
		result, err := s.handleCreateClassWithTests(ctx, newRequest(args))
		return result, true, err
	}

	return nil, false, nil
}

// --- Workflow Handlers ---

func (s *Server) handleWriteProgram(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	programName, ok := request.Params.Arguments["program_name"].(string)
	if !ok || programName == "" {
		return newToolResultError("program_name is required"), nil
	}

	source, ok := request.Params.Arguments["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	transport := ""
	if t, ok := request.Params.Arguments["transport"].(string); ok {
		transport = t
	}

	result, err := s.adtClient.WriteProgram(ctx, programName, source, transport)
	if err != nil {
		return wrapErr("WriteProgram", err), nil
	}
	return newToolResultJSON(result), nil
}

func (s *Server) handleWriteClass(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	className, ok := request.Params.Arguments["class_name"].(string)
	if !ok || className == "" {
		return newToolResultError("class_name is required"), nil
	}

	source, ok := request.Params.Arguments["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	transport := ""
	if t, ok := request.Params.Arguments["transport"].(string); ok {
		transport = t
	}

	result, err := s.adtClient.WriteClass(ctx, className, source, transport)
	if err != nil {
		return wrapErr("WriteClass", err), nil
	}
	return newToolResultJSON(result), nil
}

func (s *Server) handleCreateAndActivateProgram(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	programName, ok := request.Params.Arguments["program_name"].(string)
	if !ok || programName == "" {
		return newToolResultError("program_name is required"), nil
	}

	description, ok := request.Params.Arguments["description"].(string)
	if !ok || description == "" {
		return newToolResultError("description is required"), nil
	}

	packageName, ok := request.Params.Arguments["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}

	source, ok := request.Params.Arguments["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	transport := ""
	if t, ok := request.Params.Arguments["transport"].(string); ok {
		transport = t
	}

	result, err := s.adtClient.CreateAndActivateProgram(ctx, programName, description, packageName, source, transport)
	if err != nil {
		return wrapErr("CreateAndActivateProgram", err), nil
	}
	return newToolResultJSON(result), nil
}

func (s *Server) handleCreateClassWithTests(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	className, ok := request.Params.Arguments["class_name"].(string)
	if !ok || className == "" {
		return newToolResultError("class_name is required"), nil
	}

	description, ok := request.Params.Arguments["description"].(string)
	if !ok || description == "" {
		return newToolResultError("description is required"), nil
	}

	packageName, ok := request.Params.Arguments["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}

	classSource, ok := request.Params.Arguments["class_source"].(string)
	if !ok || classSource == "" {
		return newToolResultError("class_source is required"), nil
	}

	testSource, ok := request.Params.Arguments["test_source"].(string)
	if !ok || testSource == "" {
		return newToolResultError("test_source is required"), nil
	}

	transport := ""
	if t, ok := request.Params.Arguments["transport"].(string); ok {
		transport = t
	}

	result, err := s.adtClient.CreateClassWithTests(ctx, className, description, packageName, classSource, testSource, transport)
	if err != nil {
		return wrapErr("CreateClassWithTests", err), nil
	}
	return newToolResultJSON(result), nil
}
