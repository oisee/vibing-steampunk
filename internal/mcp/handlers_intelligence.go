// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_intelligence.go contains handlers for Intelligence Layer tools
// (AnalyzeSQLPerformance, GetImpactAnalysis).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- AnalyzeSQLPerformance Handler ---

func (s *Server) handleAnalyzeSQLPerformance(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sqlQuery, ok := request.Params.Arguments["sql_query"].(string)
	if !ok || sqlQuery == "" {
		return newToolResultError("sql_query is required"), nil
	}

	// Bridge Server-level feature detection to Client method parameter
	hanaAvailable := s.featureProber.IsAvailable(ctx, adt.FeatureHANA)

	result, err := s.adtClient.AnalyzeSQLPerformance(ctx, sqlQuery, hanaAvailable)
	if err != nil {
		return newToolResultError(fmt.Sprintf("AnalyzeSQLPerformance failed: %v", err)), nil
	}

	output, err2 := json.MarshalIndent(result, "", "  ")
	if err2 != nil {
		return newToolResultError(fmt.Sprintf("failed to serialize result: %v", err2)), nil
	}
	return mcp.NewToolResultText(string(output)), nil
}

// --- GetImpactAnalysis Handler ---

func (s *Server) handleGetImpactAnalysis(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURI, ok := request.Params.Arguments["object_uri"].(string)
	if !ok || objectURI == "" {
		return newToolResultError("object_uri is required"), nil
	}

	objectName, _ := request.Params.Arguments["object_name"].(string)

	opts := adt.ImpactAnalysisOptions{
		StaticRefs: true, // always on by default
	}

	if v, ok := request.Params.Arguments["transitive"].(bool); ok {
		opts.Transitive = v
	}
	if v, ok := request.Params.Arguments["max_depth"].(float64); ok {
		opts.MaxDepth = int(v)
	}
	if v, ok := request.Params.Arguments["dynamic_patterns"].(bool); ok {
		opts.DynamicPatterns = v
	}
	if v, ok := request.Params.Arguments["extension_points"].(bool); ok {
		opts.ExtensionPoints = v
	}
	if v, ok := request.Params.Arguments["max_results"].(float64); ok {
		opts.MaxResults = int(v)
	}
	if v, ok := request.Params.Arguments["scope_packages"].([]interface{}); ok {
		for _, pkg := range v {
			if s, ok := pkg.(string); ok {
				opts.ScopePackages = append(opts.ScopePackages, s)
			}
		}
	}

	result, err := s.adtClient.GetImpactAnalysis(ctx, objectURI, objectName, opts)
	if err != nil {
		return newToolResultError(fmt.Sprintf("GetImpactAnalysis failed: %v", err)), nil
	}

	output, err2 := json.MarshalIndent(result, "", "  ")
	if err2 != nil {
		return newToolResultError(fmt.Sprintf("failed to serialize result: %v", err2)), nil
	}
	return mcp.NewToolResultText(string(output)), nil
}
