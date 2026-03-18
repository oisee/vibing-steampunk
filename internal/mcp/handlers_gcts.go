// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_gcts.go contains handlers for gCTS (git-enabled Change Transport System) operations.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- gCTS Handlers ---

func (s *Server) handleGctsListRepositories(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repos, err := s.adtClient.GctsListRepositories(ctx)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GctsListRepositories failed: %v", err)), nil
	}

	if len(repos) == 0 {
		return mcp.NewToolResultText("No gCTS repositories found."), nil
	}

	jsonBytes, err := json.MarshalIndent(repos, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to format result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

func (s *Server) handleGctsGetRepository(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rid, ok := request.Params.Arguments["rid"].(string)
	if !ok || rid == "" {
		return newToolResultError("rid is required"), nil
	}

	repo, err := s.adtClient.GctsGetRepository(ctx, rid)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GctsGetRepository failed: %v", err)), nil
	}

	jsonBytes, err := json.MarshalIndent(repo, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to format result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

func (s *Server) handleGctsCreateRepository(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rid, _ := request.Params.Arguments["rid"].(string)
	name, _ := request.Params.Arguments["name"].(string)
	repoURL, _ := request.Params.Arguments["url"].(string)

	if rid == "" || name == "" || repoURL == "" {
		return newToolResultError("rid, name, and url are required"), nil
	}

	opts := adt.GctsCreateOptions{
		Rid:  rid,
		Name: name,
		URL:  repoURL,
	}

	if branch, ok := request.Params.Arguments["branch"].(string); ok && branch != "" {
		opts.Branch = branch
	}
	if pkg, ok := request.Params.Arguments["package"].(string); ok && pkg != "" {
		opts.Package = pkg
	}
	if role, ok := request.Params.Arguments["role"].(string); ok && role != "" {
		opts.Role = role
	}

	repo, err := s.adtClient.GctsCreateRepository(ctx, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GctsCreateRepository failed: %v", err)), nil
	}

	jsonBytes, err := json.MarshalIndent(repo, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to format result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

func (s *Server) handleGctsDeleteRepository(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rid, ok := request.Params.Arguments["rid"].(string)
	if !ok || rid == "" {
		return newToolResultError("rid is required"), nil
	}

	err := s.adtClient.GctsDeleteRepository(ctx, rid)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GctsDeleteRepository failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Repository %s deleted successfully.", rid)), nil
}

func (s *Server) handleGctsCloneRepository(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rid, ok := request.Params.Arguments["rid"].(string)
	if !ok || rid == "" {
		return newToolResultError("rid is required"), nil
	}

	err := s.adtClient.GctsCloneRepository(ctx, rid)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GctsCloneRepository failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Repository %s cloned successfully.", rid)), nil
}

func (s *Server) handleGctsPull(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rid, ok := request.Params.Arguments["rid"].(string)
	if !ok || rid == "" {
		return newToolResultError("rid is required"), nil
	}

	commitID, _ := request.Params.Arguments["commit_id"].(string)

	result, err := s.adtClient.GctsPull(ctx, rid, commitID)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GctsPull failed: %v", err)), nil
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to format result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

func (s *Server) handleGctsCommit(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rid, ok := request.Params.Arguments["rid"].(string)
	if !ok || rid == "" {
		return newToolResultError("rid is required"), nil
	}

	message, ok := request.Params.Arguments["message"].(string)
	if !ok || message == "" {
		return newToolResultError("message is required"), nil
	}

	opts := adt.GctsCommitOptions{
		Message: message,
	}

	// Parse objects array if provided
	if objectsRaw, ok := request.Params.Arguments["objects"]; ok && objectsRaw != nil {
		if objectsJSON, err := json.Marshal(objectsRaw); err == nil {
			var objects []adt.GctsCommitObject
			if err := json.Unmarshal(objectsJSON, &objects); err == nil {
				opts.Objects = objects
			}
		}
	}

	result, err := s.adtClient.GctsCommit(ctx, rid, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GctsCommit failed: %v", err)), nil
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to format result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

func (s *Server) handleGctsListBranches(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rid, ok := request.Params.Arguments["rid"].(string)
	if !ok || rid == "" {
		return newToolResultError("rid is required"), nil
	}

	branches, err := s.adtClient.GctsListBranches(ctx, rid)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GctsListBranches failed: %v", err)), nil
	}

	if len(branches) == 0 {
		return mcp.NewToolResultText("No branches found."), nil
	}

	jsonBytes, err := json.MarshalIndent(branches, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to format result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

func (s *Server) handleGctsSwitchBranch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rid, ok := request.Params.Arguments["rid"].(string)
	if !ok || rid == "" {
		return newToolResultError("rid is required"), nil
	}

	branch, ok := request.Params.Arguments["branch"].(string)
	if !ok || branch == "" {
		return newToolResultError("branch is required"), nil
	}

	err := s.adtClient.GctsSwitchBranch(ctx, rid, branch)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GctsSwitchBranch failed: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Switched to branch '%s' in repository %s.", branch, rid)), nil
}

func (s *Server) handleGctsGetHistory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	rid, ok := request.Params.Arguments["rid"].(string)
	if !ok || rid == "" {
		return newToolResultError("rid is required"), nil
	}

	history, err := s.adtClient.GctsGetHistory(ctx, rid)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GctsGetHistory failed: %v", err)), nil
	}

	if len(history) == 0 {
		return mcp.NewToolResultText("No commit history found."), nil
	}

	jsonBytes, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to format result: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}
