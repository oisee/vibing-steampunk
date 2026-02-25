// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_servicebinding.go contains handlers for RAP service binding publish/unpublish.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- Service Binding Publish/Unpublish Handlers ---

func (s *Server) handlePublishServiceBinding(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	serviceName, ok := request.GetArguments()["service_name"].(string)
	if !ok || serviceName == "" {
		return newToolResultError("service_name is required"), nil
	}

	serviceVersion := "0001"
	if sv, ok := request.GetArguments()["service_version"].(string); ok && sv != "" {
		serviceVersion = sv
	}

	result, err := s.adtClient.PublishServiceBinding(ctx, serviceName, serviceVersion)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to publish service binding: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleUnpublishServiceBinding(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	serviceName, ok := request.GetArguments()["service_name"].(string)
	if !ok || serviceName == "" {
		return newToolResultError("service_name is required"), nil
	}

	serviceVersion := "0001"
	if sv, ok := request.GetArguments()["service_version"].(string); ok && sv != "" {
		serviceVersion = sv
	}

	result, err := s.adtClient.UnpublishServiceBinding(ctx, serviceName, serviceVersion)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Failed to unpublish service binding: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}
