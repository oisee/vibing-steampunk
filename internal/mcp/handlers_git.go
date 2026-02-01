// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_git.go contains handlers for Git/abapGit operations via ZADT_VSP.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Git Routing ---
// Routes for this module:
//   system: type=git_types
//   deploy: export

// routeGitAction routes Git/abapGit actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeGitAction(ctx context.Context, action, objectType, _ string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	switch action {
	case "system":
		systemType, _ := params["type"].(string)
		if systemType == "git_types" {
			result, err := s.handleGitTypes(ctx, newRequest(nil))
			return result, true, err
		}

	case "deploy":
		if objectType == "export" {
			args := map[string]any{}
			if packages, ok := params["packages"].(string); ok && packages != "" {
				args["packages"] = packages
			}
			if objects, ok := params["objects"].(string); ok && objects != "" {
				args["objects"] = objects
			}
			if inclSub, ok := params["include_subpackages"].(bool); ok {
				args["include_subpackages"] = inclSub
			}
			result, err := s.handleGitExport(ctx, newRequest(args))
			return result, true, err
		}
	}

	return nil, false, nil
}

// --- Git/abapGit Handlers ---

func (s *Server) handleGitTypes(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if errResult := s.ensureWSConnected(ctx, "GitTypes"); errResult != nil {
		return errResult, nil
	}

	types, err := s.amdpWSClient.GitTypes(ctx)
	if err != nil {
		return wrapErr("GitTypes", err), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Supported abapGit Object Types: %d\n\n", len(types))
	for i, t := range types {
		sb.WriteString(t)
		if i < len(types)-1 {
			if (i+1)%10 == 0 {
				sb.WriteString("\n")
			} else {
				sb.WriteString(", ")
			}
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (s *Server) handleGitExport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if errResult := s.ensureWSConnected(ctx, "GitExport"); errResult != nil {
		return errResult, nil
	}

	params := adt.GitExportParams{}

	// Parse packages
	if pkgStr, ok := request.Params.Arguments["packages"].(string); ok && pkgStr != "" {
		params.Packages = strings.Split(pkgStr, ",")
		for i, p := range params.Packages {
			params.Packages[i] = strings.TrimSpace(p)
		}
	}

	// Parse objects
	if objsStr, ok := request.Params.Arguments["objects"].(string); ok && objsStr != "" {
		var objs []adt.GitObjectRef
		if err := json.Unmarshal([]byte(objsStr), &objs); err != nil {
			return wrapErr("GitExport/ParseObjects", err), nil
		}
		params.Objects = objs
	}

	// Include subpackages
	if inclSub, ok := request.Params.Arguments["include_subpackages"].(bool); ok {
		params.IncludeSubpackages = inclSub
	} else {
		params.IncludeSubpackages = true // default
	}

	if len(params.Packages) == 0 && len(params.Objects) == 0 {
		return newToolResultError("Either packages or objects parameter is required"), nil
	}

	result, err := s.amdpWSClient.GitExport(ctx, params)
	if err != nil {
		return wrapErr("GitExport", err), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Git Export Successful\n\n")
	fmt.Fprintf(&sb, "Objects: %d\n", result.ObjectCount)
	fmt.Fprintf(&sb, "Files: %d\n\n", result.FileCount)

	sb.WriteString("Files:\n")
	for _, f := range result.Files {
		fmt.Fprintf(&sb, "  %s (%d bytes)\n", f.Path, f.Size)
	}

	fmt.Fprintf(&sb, "\nZIP Base64 length: %d chars\n", len(result.ZipBase64))
	sb.WriteString("Use base64 decode to extract the ZIP file.\n")

	return mcp.NewToolResultText(sb.String()), nil
}
