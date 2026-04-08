// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_ui5.go contains handlers for UI5/Fiori BSP management.
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// routeUI5Action routes UI5/Fiori BSP operations.
func (s *Server) routeUI5Action(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	ui5Type := getStringParam(params, "type")

	// Check for explicit ui5 type in params
	if ui5Type != "" {
		switch ui5Type {
		case "ui5_list_apps":
			return s.callHandler(ctx, s.handleUI5ListApps, params)
		case "ui5_get_app":
			return s.callHandler(ctx, s.handleUI5GetApp, params)
		case "ui5_get_file":
			return s.callHandler(ctx, s.handleUI5GetFileContent, params)
		case "ui5_upload_file":
			return s.callHandler(ctx, s.handleUI5UploadFile, params)
		case "ui5_delete_file":
			return s.callHandler(ctx, s.handleUI5DeleteFile, params)
		case "ui5_create_app":
			return s.callHandler(ctx, s.handleUI5CreateApp, params)
		case "ui5_delete_app":
			return s.callHandler(ctx, s.handleUI5DeleteApp, params)
		}
	}

	// Route by target type
	switch {
	case action == "read" && objectType == "UI5_LIST":
		return s.callHandler(ctx, s.handleUI5ListApps, params)
	case action == "read" && objectType == "UI5_APP":
		return s.callHandler(ctx, s.handleUI5GetApp, map[string]any{"app_name": objectName})
	case action == "read" && objectType == "UI5_FILE":
		return s.callHandler(ctx, s.handleUI5GetFileContent, params)
	case action == "edit" && objectType == "UI5_UPLOAD":
		return s.callHandler(ctx, s.handleUI5UploadFile, params)
	case action == "delete" && objectType == "UI5_FILE":
		return s.callHandler(ctx, s.handleUI5DeleteFile, params)
	case action == "create" && objectType == "UI5_APP":
		return s.callHandler(ctx, s.handleUI5CreateApp, params)
	case action == "delete" && objectType == "UI5_APP":
		return s.callHandler(ctx, s.handleUI5DeleteApp, params)
	}

	return nil, false, nil
}

// --- UI5/Fiori BSP Management Handlers ---

func (s *Server) handleUI5ListApps(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, _ := request.GetArguments()["query"].(string)

	maxResults := 100
	if mr, ok := request.GetArguments()["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	apps, err := s.adtClient.UI5ListApps(ctx, query, maxResults)
	if err != nil {
		return newToolResultError(fmt.Sprintf("UI5ListApps failed: %v", err)), nil
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
	appName, ok := request.GetArguments()["app_name"].(string)
	if !ok || appName == "" {
		return newToolResultError("app_name is required"), nil
	}

	details, err := s.adtClient.UI5GetApp(ctx, appName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("UI5GetApp failed: %v", err)), nil
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
	appName, ok := request.GetArguments()["app_name"].(string)
	if !ok || appName == "" {
		return newToolResultError("app_name is required"), nil
	}

	filePath, ok := request.GetArguments()["file_path"].(string)
	if !ok || filePath == "" {
		return newToolResultError("file_path is required"), nil
	}

	content, err := s.adtClient.UI5GetFileContent(ctx, appName, filePath)
	if err != nil {
		return newToolResultError(fmt.Sprintf("UI5GetFileContent failed: %v", err)), nil
	}

	return mcp.NewToolResultText(string(content)), nil
}

func (s *Server) handleUI5UploadFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	appName, ok := request.GetArguments()["app_name"].(string)
	if !ok || appName == "" {
		return newToolResultError("app_name is required"), nil
	}

	filePath, ok := request.GetArguments()["file_path"].(string)
	if !ok || filePath == "" {
		return newToolResultError("file_path is required"), nil
	}

	content, ok := request.GetArguments()["content"].(string)
	if !ok {
		return newToolResultError("content is required"), nil
	}

	contentType, _ := request.GetArguments()["content_type"].(string)

	err := s.adtClient.UI5UploadFile(ctx, appName, filePath, []byte(content), contentType)
	if err != nil {
		return newToolResultError(fmt.Sprintf("UI5UploadFile failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully uploaded %s to %s", filePath, appName)), nil
}

func (s *Server) handleUI5DeleteFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	appName, ok := request.GetArguments()["app_name"].(string)
	if !ok || appName == "" {
		return newToolResultError("app_name is required"), nil
	}

	filePath, ok := request.GetArguments()["file_path"].(string)
	if !ok || filePath == "" {
		return newToolResultError("file_path is required"), nil
	}

	err := s.adtClient.UI5DeleteFile(ctx, appName, filePath)
	if err != nil {
		return newToolResultError(fmt.Sprintf("UI5DeleteFile failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully deleted %s from %s", filePath, appName)), nil
}

func (s *Server) handleUI5CreateApp(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	appName, ok := request.GetArguments()["app_name"].(string)
	if !ok || appName == "" {
		return newToolResultError("app_name is required"), nil
	}

	description, _ := request.GetArguments()["description"].(string)

	packageName, ok := request.GetArguments()["package"].(string)
	if !ok || packageName == "" {
		return newToolResultError("package is required"), nil
	}

	transport, _ := request.GetArguments()["transport"].(string)

	err := s.adtClient.UI5CreateApp(ctx, appName, description, packageName, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("UI5CreateApp failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully created UI5 application %s in package %s", appName, packageName)), nil
}

func (s *Server) handleUI5DeleteApp(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	appName, ok := request.GetArguments()["app_name"].(string)
	if !ok || appName == "" {
		return newToolResultError("app_name is required"), nil
	}

	transport, _ := request.GetArguments()["transport"].(string)

	err := s.adtClient.UI5DeleteApp(ctx, appName, transport)
	if err != nil {
		return newToolResultError(fmt.Sprintf("UI5DeleteApp failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Successfully deleted UI5 application %s", appName)), nil
}
