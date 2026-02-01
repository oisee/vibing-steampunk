// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_servicebinding.go contains handlers for RAP service binding publish/unpublish.
package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- Service Binding Routing ---
// Routes for this module:
//   deploy: type=publish_service <name>, type=unpublish_service <name>

// routeServiceBindingAction routes service binding publish/unpublish actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeServiceBindingAction(ctx context.Context, action, _, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "deploy" {
		return nil, false, nil
	}

	deployType, _ := params["type"].(string)
	switch deployType {
	case "publish_service":
		serviceName, _ := params["service_name"].(string)
		if serviceName == "" {
			serviceName = objectName
		}
		if serviceName == "" {
			return newToolResultError("service_name is required"), true, nil
		}
		args := map[string]any{"service_name": serviceName}
		if sv, ok := params["service_version"].(string); ok {
			args["service_version"] = sv
		}
		result, err := s.handlePublishServiceBinding(ctx, newRequest(args))
		return result, true, err

	case "unpublish_service":
		serviceName, _ := params["service_name"].(string)
		if serviceName == "" {
			serviceName = objectName
		}
		if serviceName == "" {
			return newToolResultError("service_name is required"), true, nil
		}
		args := map[string]any{"service_name": serviceName}
		if sv, ok := params["service_version"].(string); ok {
			args["service_version"] = sv
		}
		result, err := s.handleUnpublishServiceBinding(ctx, newRequest(args))
		return result, true, err
	}

	return nil, false, nil
}

// --- Service Binding Publish/Unpublish Handlers ---

func (s *Server) handlePublishServiceBinding(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	serviceName, ok := request.Params.Arguments["service_name"].(string)
	if !ok || serviceName == "" {
		return newToolResultError("service_name is required"), nil
	}

	serviceVersion := "0001"
	if sv, ok := request.Params.Arguments["service_version"].(string); ok && sv != "" {
		serviceVersion = sv
	}

	result, err := s.adtClient.PublishServiceBinding(ctx, serviceName, serviceVersion)
	if err != nil {
		return wrapErr("PublishServiceBinding", err), nil
	}

	return newToolResultJSON(result), nil
}

func (s *Server) handleUnpublishServiceBinding(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	serviceName, ok := request.Params.Arguments["service_name"].(string)
	if !ok || serviceName == "" {
		return newToolResultError("service_name is required"), nil
	}

	serviceVersion := "0001"
	if sv, ok := request.Params.Arguments["service_version"].(string); ok && sv != "" {
		serviceVersion = sv
	}

	result, err := s.adtClient.UnpublishServiceBinding(ctx, serviceName, serviceVersion)
	if err != nil {
		return wrapErr("UnpublishServiceBinding", err), nil
	}

	return newToolResultJSON(result), nil
}
