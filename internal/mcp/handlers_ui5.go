// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_ui5.go contains handlers for UI5/Fiori BSP management.
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- UI5 Routing ---
// Routes for this module:
//   search: UI5 <query> (list apps)
//   read:   UI5 <app_name>, UI5_FILE <app_name> (with file_path in params)
//   edit:   UI5_FILE <app_name> (with file_path and content in params)
//   create: UI5 <app_name> (with package in params)
//   delete: UI5 <app_name>, UI5_FILE <app_name> (with file_path in params)

// routeUI5Action routes UI5/Fiori BSP management actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeUI5Action(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	switch action {
	case "search":
		if objectType == "UI5" {
			// UI5ListApps: search UI5 <query>
			query := objectName
			args := map[string]any{"query": query}
			if maxResults, ok := params["max_results"].(float64); ok {
				args["max_results"] = maxResults
			}
			result, err := s.handleUI5ListApps(ctx, newRequest(args))
			return result, true, err
		}

	case "read":
		switch objectType {
		case "UI5":
			// UI5GetApp: read UI5 <app_name>
			if objectName == "" {
				return newToolResultError("app_name is required"), true, nil
			}
			result, err := s.handleUI5GetApp(ctx, newRequest(map[string]any{"app_name": objectName}))
			return result, true, err

		case "UI5_FILE":
			// UI5GetFileContent: read UI5_FILE <app_name> with file_path in params
			if objectName == "" {
				return newToolResultError("app_name is required"), true, nil
			}
			filePath, _ := params["file_path"].(string)
			if filePath == "" {
				return newToolResultError("file_path is required in params"), true, nil
			}
			result, err := s.handleUI5GetFileContent(ctx, newRequest(map[string]any{
				"app_name":  objectName,
				"file_path": filePath,
			}))
			return result, true, err
		}

	case "edit":
		if objectType == "UI5_FILE" {
			// UI5UploadFile: edit UI5_FILE <app_name> with file_path and content in params
			if objectName == "" {
				return newToolResultError("app_name is required"), true, nil
			}
			filePath, _ := params["file_path"].(string)
			content, _ := params["content"].(string)
			if filePath == "" {
				return newToolResultError("file_path is required in params"), true, nil
			}
			args := map[string]any{
				"app_name":  objectName,
				"file_path": filePath,
				"content":   content,
			}
			if contentType, ok := params["content_type"].(string); ok {
				args["content_type"] = contentType
			}
			result, err := s.handleUI5UploadFile(ctx, newRequest(args))
			return result, true, err
		}

	case "create":
		if objectType == "UI5" {
			// UI5CreateApp: create UI5 <app_name> with package in params
			if objectName == "" {
				return newToolResultError("app_name is required"), true, nil
			}
			pkg, _ := params["package"].(string)
			if pkg == "" {
				return newToolResultError("package is required in params"), true, nil
			}
			args := map[string]any{
				"app_name": objectName,
				"package":  pkg,
			}
			if desc, ok := params["description"].(string); ok {
				args["description"] = desc
			}
			if transport, ok := params["transport"].(string); ok {
				args["transport"] = transport
			}
			result, err := s.handleUI5CreateApp(ctx, newRequest(args))
			return result, true, err
		}

	case "delete":
		switch objectType {
		case "UI5":
			// UI5DeleteApp: delete UI5 <app_name>
			if objectName == "" {
				return newToolResultError("app_name is required"), true, nil
			}
			args := map[string]any{"app_name": objectName}
			if transport, ok := params["transport"].(string); ok {
				args["transport"] = transport
			}
			result, err := s.handleUI5DeleteApp(ctx, newRequest(args))
			return result, true, err

		case "UI5_FILE":
			// UI5DeleteFile: delete UI5_FILE <app_name> with file_path in params
			if objectName == "" {
				return newToolResultError("app_name is required"), true, nil
			}
			filePath, _ := params["file_path"].(string)
			if filePath == "" {
				return newToolResultError("file_path is required in params"), true, nil
			}
			result, err := s.handleUI5DeleteFile(ctx, newRequest(map[string]any{
				"app_name":  objectName,
				"file_path": filePath,
			}))
			return result, true, err
		}
	}

	return nil, false, nil
}

// --- UI5/Fiori BSP Management Handlers ---

func (s *Server) handleUI5ListApps(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, _ := request.Params.Arguments["query"].(string)

	maxResults := 100
	if mr, ok := request.Params.Arguments["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	apps, err := s.adtClient.UI5ListApps(ctx, query, maxResults)
	if err != nil {
		return wrapErr("UI5ListApps", err), nil
	}

	if len(apps) == 0 {
		return mcp.NewToolResultText("No UI5 applications found"), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d UI5 applications:\n\n", len(apps))

	for _, app := range apps {
		fmt.Fprintf(&sb, "- %s", app.Name)
		if app.Description != "" {
			fmt.Fprintf(&sb, " (%s)", app.Description)
		}
		if app.Package != "" {
			fmt.Fprintf(&sb, " [%s]", app.Package)
		}
		sb.WriteString("\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleUI5GetApp(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	appName, ok := request.Params.Arguments["app_name"].(string)
	if !ok || appName == "" {
		return newToolResultError("app_name is required"), nil
	}

	details, err := s.adtClient.UI5GetApp(ctx, appName)
	if err != nil {
		return wrapErr("UI5GetApp", err), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "UI5 Application: %s\n", details.Name)
	if details.Description != "" {
		fmt.Fprintf(&sb, "Description: %s\n", details.Description)
	}
	if details.Package != "" {
		fmt.Fprintf(&sb, "Package: %s\n", details.Package)
	}

	if len(details.Files) > 0 {
		fmt.Fprintf(&sb, "\nFiles (%d):\n", len(details.Files))
		for _, f := range details.Files {
			if f.Type == "folder" {
				fmt.Fprintf(&sb, "  [DIR]  %s\n", f.Path)
			} else {
				fmt.Fprintf(&sb, "  [FILE] %s", f.Path)
				if f.Size > 0 {
					fmt.Fprintf(&sb, " (%d bytes)", f.Size)
				}
				sb.WriteString("\n")
			}
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleUI5GetFileContent(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	appName, ok := request.Params.Arguments["app_name"].(string)
	if !ok || appName == "" {
		return newToolResultError("app_name is required"), nil
	}

	filePath, ok := request.Params.Arguments["file_path"].(string)
	if !ok || filePath == "" {
		return newToolResultError("file_path is required"), nil
	}

	content, err := s.adtClient.UI5GetFileContent(ctx, appName, filePath)
	if err != nil {
		return wrapErr("UI5GetFileContent", err), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (s *Server) handleUI5UploadFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	appName, ok := request.Params.Arguments["app_name"].(string)
	if !ok || appName == "" {
		return newToolResultError("app_name is required"), nil
	}

	filePath, ok := request.Params.Arguments["file_path"].(string)
	if !ok || filePath == "" {
		return newToolResultError("file_path is required"), nil
	}

	content, ok := request.Params.Arguments["content"].(string)
	if !ok {
		return newToolResultError("content is required"), nil
	}

	contentType, _ := request.Params.Arguments["content_type"].(string)

	err := s.adtClient.UI5UploadFile(ctx, appName, filePath, []byte(content), contentType)
	if err != nil {
		return wrapErr("UI5UploadFile", err), nil
	}

	return mcp.NewToolResultText("Successfully uploaded " + filePath + " to " + appName), nil
}

func (s *Server) handleUI5DeleteFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	appName, ok := request.Params.Arguments["app_name"].(string)
	if !ok || appName == "" {
		return newToolResultError("app_name is required"), nil
	}

	filePath, ok := request.Params.Arguments["file_path"].(string)
	if !ok || filePath == "" {
		return newToolResultError("file_path is required"), nil
	}

	err := s.adtClient.UI5DeleteFile(ctx, appName, filePath)
	if err != nil {
		return wrapErr("UI5DeleteFile", err), nil
	}

	return mcp.NewToolResultText("Successfully deleted " + filePath + " from " + appName), nil
}

func (s *Server) handleUI5CreateApp(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	appName, ok := request.Params.Arguments["app_name"].(string)
	if !ok || appName == "" {
		return newToolResultError("app_name is required"), nil
	}

	description, _ := request.Params.Arguments["description"].(string)

	packageName, ok := request.Params.Arguments["package"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package is required"), nil
	}

	transport, _ := request.Params.Arguments["transport"].(string)

	err := s.adtClient.UI5CreateApp(ctx, appName, description, packageName, transport)
	if err != nil {
		return wrapErr("UI5CreateApp", err), nil
	}

	return mcp.NewToolResultText("Successfully created UI5 application " + appName + " in package " + packageName), nil
}

func (s *Server) handleUI5DeleteApp(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	appName, ok := request.Params.Arguments["app_name"].(string)
	if !ok || appName == "" {
		return newToolResultError("app_name is required"), nil
	}

	transport, _ := request.Params.Arguments["transport"].(string)

	err := s.adtClient.UI5DeleteApp(ctx, appName, transport)
	if err != nil {
		return wrapErr("UI5DeleteApp", err), nil
	}

	return mcp.NewToolResultText("Successfully deleted UI5 application " + appName), nil
}
