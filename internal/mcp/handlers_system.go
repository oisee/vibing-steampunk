// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_system.go contains handlers for system information operations.
package mcp

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- System Routing ---
// Routes for this module:
//   system: info, features, connection, components

// routeSystemAction routes system information actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeSystemAction(ctx context.Context, action, objectType, _ string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action != "system" {
		return nil, false, nil
	}

	target := objectType
	if target == "" {
		target, _ = params["type"].(string)
	}

	switch strings.ToUpper(target) {
	case "INFO":
		result, err := s.handleGetSystemInfo(ctx, newRequest(nil))
		return result, true, err

	case "FEATURES":
		result, err := s.handleGetFeatures(ctx, newRequest(nil))
		return result, true, err

	case "CONNECTION":
		result, err := s.handleGetConnectionInfo(ctx, newRequest(nil))
		return result, true, err

	case "COMPONENTS":
		result, err := s.handleGetInstalledComponents(ctx, newRequest(nil))
		return result, true, err
	}

	return nil, false, nil
}

// --- System Information Handlers ---

func (s *Server) handleGetSystemInfo(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	info, err := s.adtClient.GetSystemInfo(ctx)
	if err != nil {
		return wrapErr("GetSystemInfo", err), nil
	}
	return newToolResultJSON(info), nil
}

func (s *Server) handleGetInstalledComponents(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	components, err := s.adtClient.GetInstalledComponents(ctx)
	if err != nil {
		return wrapErr("GetInstalledComponents", err), nil
	}
	return newToolResultJSON(components), nil
}

func (s *Server) handleGetConnectionInfo(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Return current connection info for introspection
	info := map[string]any{
		"user":           s.config.Username,
		"url":            s.config.BaseURL,
		"client":         s.config.Client,
		"mode":           "unified",
		"features":       s.featureProber.FeatureSummary(ctx),
		"debugger_user":  strings.ToUpper(s.config.Username), // Debugger uses uppercase
	}
	return newToolResultJSON(info), nil
}

func (s *Server) handleGetFeatures(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Probe all features
	results := s.featureProber.ProbeAll(ctx)

	// Format output
	features := make(map[string]*adt.FeatureStatus)
	for id, status := range results {
		features[string(id)] = status
	}

	return newToolResultJSON(map[string]any{
		"features": features,
		"summary":  s.featureProber.FeatureSummary(ctx),
	}), nil
}
