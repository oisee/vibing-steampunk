// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_devtools.go contains handlers for development tools (syntax check, activation, unit tests).
package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- DevTools Routing ---
// Routes for this module:
//   test: type=syntax_check, type=unit (unit tests)
//   deploy: activate, activate_package

// routeDevToolsAction routes development tool actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeDevToolsAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	switch action {
	case "test":
		testType, _ := params["type"].(string)
		switch testType {
		case "syntax_check":
			objectURL, _ := params["object_url"].(string)
			content, _ := params["content"].(string)
			if objectURL == "" || content == "" {
				return newToolResultError("syntax_check requires object_url and content in params"), true, nil
			}
			result, err := s.handleSyntaxCheck(ctx, newRequest(map[string]any{
				"object_url": objectURL,
				"content":    content,
			}))
			return result, true, err

		case "unit", "":
			objectURL, _ := params["object_url"].(string)
			if objectURL == "" {
				return newToolResultError("object_url is required for unit tests"), true, nil
			}
			args := map[string]any{"object_url": objectURL}
			if dangerous, ok := params["include_dangerous"].(bool); ok {
				args["include_dangerous"] = dangerous
			}
			if long, ok := params["include_long"].(bool); ok {
				args["include_long"] = long
			}
			result, err := s.handleRunUnitTests(ctx, newRequest(args))
			return result, true, err
		}

	case "deploy":
		switch objectType {
		case "activate":
			objectURL, _ := params["object_url"].(string)
			objName := objectName
			if objName == "" {
				objName, _ = params["object_name"].(string)
			}
			if objectURL == "" || objName == "" {
				return newToolResultError("object_url and object_name are required for activate"), true, nil
			}
			result, err := s.handleActivate(ctx, newRequest(map[string]any{
				"object_url":  objectURL,
				"object_name": objName,
			}))
			return result, true, err

		case "activate_package":
			pkg, _ := params["package"].(string)
			if pkg == "" {
				pkg = objectName
			}
			args := map[string]any{}
			if pkg != "" {
				args["package"] = pkg
			}
			if maxObjects, ok := params["max_objects"].(float64); ok {
				args["max_objects"] = maxObjects
			}
			result, err := s.handleActivatePackage(ctx, newRequest(args))
			return result, true, err
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
		return wrapErr("SyntaxCheck", err), nil
	}
	return newToolResultJSON(results), nil
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
		return wrapErr("Activate", err), nil
	}
	return newToolResultJSON(result), nil
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
		return wrapErr("ActivatePackage", err), nil
	}
	return newToolResultJSON(result), nil
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
		return wrapErr("RunUnitTests", err), nil
	}
	return newToolResultJSON(result), nil
}
