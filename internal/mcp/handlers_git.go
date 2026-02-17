// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_git.go contains handlers for Git/abapGit operations via ZADT_VSP.
package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Git/abapGit Handlers ---

func (s *Server) handleGitTypes(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if errResult := s.ensureWSConnected(ctx, "GitTypes"); errResult != nil {
		return errResult, nil
	}

	types, err := s.amdpWSClient.GitTypes(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GitTypes failed: %v", err)), nil
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
			return newToolResultError(fmt.Sprintf("Invalid objects JSON: %v", err)), nil
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
		return newToolResultError(fmt.Sprintf("GitExport failed: %v", err)), nil
	}

	// Determine output directory (default: current directory)
	outputDir := "."
	if dir, ok := request.Params.Arguments["output_dir"].(string); ok && dir != "" {
		outputDir = dir
	}

	// Generate filename with timestamp
	var zipName string
	if len(params.Packages) > 0 {
		// Use first package name (sanitize $ for filename)
		pkgName := strings.ReplaceAll(params.Packages[0], "$", "")
		zipName = fmt.Sprintf("%s_%s.zip", pkgName, time.Now().Format("20060102_150405"))
	} else {
		zipName = fmt.Sprintf("abapgit_export_%s.zip", time.Now().Format("20060102_150405"))
	}
	zipPath := filepath.Join(outputDir, zipName)

	// Decode base64 and save ZIP
	zipData, err := base64.StdEncoding.DecodeString(result.ZipBase64)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to decode ZIP: %v", err)), nil
	}

	if err := os.WriteFile(zipPath, zipData, 0644); err != nil {
		return newToolResultError(fmt.Sprintf("Failed to write ZIP file: %v", err)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Git Export Successful\n\n")
	fmt.Fprintf(&sb, "Objects: %d\n", result.ObjectCount)
	fmt.Fprintf(&sb, "Files: %d\n", result.FileCount)
	fmt.Fprintf(&sb, "ZIP: %s (%d bytes)\n\n", zipPath, len(zipData))

	sb.WriteString("Files in archive:\n")
	for _, f := range result.Files {
		fmt.Fprintf(&sb, "  %s (%d bytes)\n", f.Path, f.Size)
	}

	return mcp.NewToolResultText(sb.String()), nil
}
