// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_classinclude.go contains handlers for class include operations (testclasses, locals_def, etc.).
package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Class Include Routing ---
// Routes for this module:
//   read:   CLAS_INCLUDE <class_name> (with include_type in params)
//   edit:   CLAS_INCLUDE <class_name> (with include_type, source, lock_handle in params)
//   create: CLAS_TEST_INCLUDE <class_name> (with lock_handle in params)

// routeClassIncludeAction routes class include actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeClassIncludeAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	switch action {
	case "read":
		if objectType == "CLAS_INCLUDE" {
			className := objectName
			if className == "" {
				return newToolResultError("class_name is required"), true, nil
			}
			includeType, _ := params["include_type"].(string)
			if includeType == "" {
				return newToolResultError("include_type is required in params (testclasses, locals_def, locals_imp, macros)"), true, nil
			}
			result, err := s.handleGetClassInclude(ctx, newRequest(map[string]any{
				"class_name":   className,
				"include_type": includeType,
			}))
			return result, true, err
		}

	case "edit":
		if objectType == "CLAS_INCLUDE" {
			className := objectName
			if className == "" {
				return newToolResultError("class_name is required"), true, nil
			}
			includeType, _ := params["include_type"].(string)
			source, _ := params["source"].(string)
			lockHandle, _ := params["lock_handle"].(string)
			if includeType == "" || source == "" || lockHandle == "" {
				return newToolResultError("include_type, source, and lock_handle are required in params"), true, nil
			}
			args := map[string]any{
				"class_name":   className,
				"include_type": includeType,
				"source":       source,
				"lock_handle":  lockHandle,
			}
			if transport, ok := params["transport"].(string); ok {
				args["transport"] = transport
			}
			result, err := s.handleUpdateClassInclude(ctx, newRequest(args))
			return result, true, err
		}

	case "create":
		if objectType == "CLAS_TEST_INCLUDE" {
			className := objectName
			if className == "" {
				return newToolResultError("class_name is required"), true, nil
			}
			lockHandle, _ := params["lock_handle"].(string)
			if lockHandle == "" {
				return newToolResultError("lock_handle is required in params"), true, nil
			}
			args := map[string]any{
				"class_name":  className,
				"lock_handle": lockHandle,
			}
			if transport, ok := params["transport"].(string); ok {
				args["transport"] = transport
			}
			result, err := s.handleCreateTestInclude(ctx, newRequest(args))
			return result, true, err
		}
	}

	return nil, false, nil
}

// --- Class Include Handlers ---

func (s *Server) handleGetClassInclude(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	className, ok := request.Params.Arguments["class_name"].(string)
	if !ok || className == "" {
		return newToolResultError("class_name is required"), nil
	}

	includeType, ok := request.Params.Arguments["include_type"].(string)
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
	className, ok := request.Params.Arguments["class_name"].(string)
	if !ok || className == "" {
		return newToolResultError("class_name is required"), nil
	}

	lockHandle, ok := request.Params.Arguments["lock_handle"].(string)
	if !ok || lockHandle == "" {
		return newToolResultError("lock_handle is required"), nil
	}

	transport := ""
	if t, ok := request.Params.Arguments["transport"].(string); ok {
		transport = t
	}

	err := s.adtClient.CreateTestInclude(ctx, className, lockHandle, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to create test include: %v", err)), nil
	}

	return mcp.NewToolResultText("Test include created successfully"), nil
}

func (s *Server) handleUpdateClassInclude(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	className, ok := request.Params.Arguments["class_name"].(string)
	if !ok || className == "" {
		return newToolResultError("class_name is required"), nil
	}

	includeType, ok := request.Params.Arguments["include_type"].(string)
	if !ok || includeType == "" {
		return newToolResultError("include_type is required"), nil
	}

	source, ok := request.Params.Arguments["source"].(string)
	if !ok || source == "" {
		return newToolResultError("source is required"), nil
	}

	lockHandle, ok := request.Params.Arguments["lock_handle"].(string)
	if !ok || lockHandle == "" {
		return newToolResultError("lock_handle is required"), nil
	}

	transport := ""
	if t, ok := request.Params.Arguments["transport"].(string); ok {
		transport = t
	}

	err := s.adtClient.UpdateClassInclude(ctx, className, adt.ClassIncludeType(includeType), source, lockHandle, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to update class include: %v", err)), nil
	}

	return mcp.NewToolResultText("Class include updated successfully"), nil
}
