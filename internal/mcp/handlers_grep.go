// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_grep.go contains handlers for grep/search operations on ABAP objects.
package mcp

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- Grep Routing ---
// Routes for this module:
//   grep: OBJECT <url> (with pattern in params), PACKAGE <name> (with pattern in params)

// routeGrepAction routes grep-specific actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeGrepAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "grep" {
		return nil, false, nil
	}

	pattern, _ := params["pattern"].(string)
	if pattern == "" {
		// Pattern might be in target position for simple grep commands
		pattern = objectName
	}

	switch objectType {
	case "OBJECT":
		// GrepObject: grep OBJECT <url> with pattern in params
		objectURL, _ := params["object_url"].(string)
		if objectURL == "" {
			return newToolResultError("object_url is required in params for OBJECT grep"), true, nil
		}
		if pattern == "" {
			return newToolResultError("pattern is required"), true, nil
		}
		args := map[string]any{
			"object_url": objectURL,
			"pattern":    pattern,
		}
		if ci, ok := params["case_insensitive"].(bool); ok {
			args["case_insensitive"] = ci
		}
		if cl, ok := params["context_lines"].(float64); ok {
			args["context_lines"] = cl
		}
		result, err := s.handleGrepObject(ctx, newRequest(args))
		return result, true, err

	case "PACKAGE":
		// GrepPackage: grep PACKAGE <name> with pattern in params
		packageName := objectName
		if packageName == "" {
			return newToolResultError("package name is required"), true, nil
		}
		pattern, _ := params["pattern"].(string)
		if pattern == "" {
			return newToolResultError("pattern is required in params for PACKAGE grep"), true, nil
		}
		args := map[string]any{
			"package_name": packageName,
			"pattern":      pattern,
		}
		if ci, ok := params["case_insensitive"].(bool); ok {
			args["case_insensitive"] = ci
		}
		if ot, ok := params["object_types"].(string); ok {
			args["object_types"] = ot
		}
		if mr, ok := params["max_results"].(float64); ok {
			args["max_results"] = mr
		}
		result, err := s.handleGrepPackage(ctx, newRequest(args))
		return result, true, err
	}

	return nil, false, nil
}

// --- Grep/Search Handlers ---

func (s *Server) handleGrepObject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.Params.Arguments["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	pattern, ok := request.Params.Arguments["pattern"].(string)
	if !ok || pattern == "" {
		return newToolResultError("pattern is required"), nil
	}

	caseInsensitive := false
	if ci, ok := request.Params.Arguments["case_insensitive"].(bool); ok {
		caseInsensitive = ci
	}

	contextLines := 0
	if cl, ok := request.Params.Arguments["context_lines"].(float64); ok {
		contextLines = int(cl)
	}

	result, err := s.adtClient.GrepObject(ctx, objectURL, pattern, caseInsensitive, contextLines)
	if err != nil {
		return wrapErr("GrepObject", err), nil
	}

	return newToolResultJSON(result), nil
}

func (s *Server) handleGrepPackage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	packageName, ok := request.Params.Arguments["package_name"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package_name is required"), nil
	}

	pattern, ok := request.Params.Arguments["pattern"].(string)
	if !ok || pattern == "" {
		return newToolResultError("pattern is required"), nil
	}

	caseInsensitive := false
	if ci, ok := request.Params.Arguments["case_insensitive"].(bool); ok {
		caseInsensitive = ci
	}

	// Parse object_types (comma-separated string to slice)
	var objectTypes []string
	if ot, ok := request.Params.Arguments["object_types"].(string); ok && ot != "" {
		objectTypes = strings.Split(ot, ",")
		// Trim whitespace from each type
		for i := range objectTypes {
			objectTypes[i] = strings.TrimSpace(objectTypes[i])
		}
	}

	maxResults := 100 // default
	if mr, ok := request.Params.Arguments["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	result, err := s.adtClient.GrepPackage(ctx, packageName, pattern, caseInsensitive, objectTypes, maxResults)
	if err != nil {
		return wrapErr("GrepPackage", err), nil
	}

	return newToolResultJSON(result), nil
}
