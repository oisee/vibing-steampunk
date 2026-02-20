// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_revisions.go contains handlers for object version history operations.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- Version History (Revision) Handlers ---

func (s *Server) handleGetRevisions(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectType, _ := request.Params.Arguments["type"].(string)
	name, _ := request.Params.Arguments["name"].(string)

	if objectType == "" || name == "" {
		return newToolResultError("type and name are required"), nil
	}

	opts := &adt.GetSourceOptions{}
	if include, ok := request.Params.Arguments["include"].(string); ok && include != "" {
		opts.Include = include
	}
	if parent, ok := request.Params.Arguments["parent"].(string); ok && parent != "" {
		opts.Parent = parent
	}

	revisions, err := s.adtClient.GetRevisions(ctx, objectType, name, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetRevisions failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(revisions, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleGetRevisionSource(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	versionURI, ok := request.Params.Arguments["version_uri"].(string)
	if !ok || versionURI == "" {
		return newToolResultError("version_uri is required (from GetRevisions output)"), nil
	}

	source, err := s.adtClient.GetRevisionSource(ctx, versionURI)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetRevisionSource failed: %v", err)), nil
	}

	return mcp.NewToolResultText(source), nil
}

func (s *Server) handleCompareVersions(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectType, _ := request.Params.Arguments["type"].(string)
	name, _ := request.Params.Arguments["name"].(string)
	version1, _ := request.Params.Arguments["version1_uri"].(string)
	version2, _ := request.Params.Arguments["version2_uri"].(string)

	if objectType == "" || name == "" || version1 == "" {
		return newToolResultError("type, name, and version1_uri are required"), nil
	}
	if version2 == "" {
		version2 = "current"
	}

	opts := &adt.GetSourceOptions{}
	if include, ok := request.Params.Arguments["include"].(string); ok && include != "" {
		opts.Include = include
	}
	if parent, ok := request.Params.Arguments["parent"].(string); ok && parent != "" {
		opts.Parent = parent
	}

	diff, err := s.adtClient.CompareVersions(ctx, objectType, name, version1, version2, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("CompareVersions failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(diff, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}
