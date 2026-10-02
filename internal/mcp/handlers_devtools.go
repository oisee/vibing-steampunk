// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_devtools.go contains handlers for development tools (syntax check, activation, unit tests).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// routeDevToolsAction routes "test" (unit tests), "analyze" (syntax check), "edit" (activate), and "analyze" (execute_abap, check_abap).
func (s *Server) routeDevToolsAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	if action == "test" {
		analysisType := getStringParam(params, "type")
		// target="ATC" asks for an ATC run, not a unit-test run, even though
		// both carry an object_url; routeATCAction takes it.
		if !isATCTarget(objectType) && (analysisType == "" || analysisType == "unit") {
			// Unit tests
			objectURL := getStringParam(params, "object_url")
			if objectURL == "" {
				return nil, false, nil
			}
			args := map[string]any{"object_url": objectURL}
			if v, ok := getBoolParam(params, "include_dangerous"); ok {
				args["include_dangerous"] = v
			}
			if v, ok := getBoolParam(params, "include_long"); ok {
				args["include_long"] = v
			}
			if v, ok := params["timeout"]; ok {
				args["timeout"] = v
			}
			if v, ok := getBoolParam(params, "only_failures"); ok {
				args["only_failures"] = v
			}
			return s.callHandler(ctx, s.handleRunUnitTests, args)
		}
	}

	if action == "analyze" {
		analysisType := getStringParam(params, "type")
		switch analysisType {
		case "syntax_check":
			return s.callHandler(ctx, s.handleSyntaxCheck, params)
		case "execute_abap":
			return s.callHandler(ctx, s.handleExecuteABAP, params)
		case "check_abap":
			return s.callHandler(ctx, s.handleCheckABAP, params)
		}
	}

	if action == "edit" {
		switch objectType {
		case "ACTIVATE":
			return s.callHandler(ctx, s.handleActivate, params)
		case "ACTIVATE_MULTI":
			return s.callHandler(ctx, s.handleActivateMultiple, params)
		case "ACTIVATE_PACKAGE":
			return s.callHandler(ctx, s.handleActivatePackage, params)
		}
	}

	return nil, false, nil
}

// isATCTarget reports whether a test target names an ATC run, which
// routeATCAction answers.
func isATCTarget(objectType string) bool {
	return objectType == "ATC" || objectType == "ATC_CUSTOMIZING"
}

// --- Development Tool Handlers ---

func (s *Server) handleSyntaxCheck(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.GetArguments()["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	content, ok := request.GetArguments()["content"].(string)
	if !ok || content == "" {
		return newToolResultError("content is required"), nil
	}

	results, err := s.adtClient.SyntaxCheck(ctx, objectURL, content)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Syntax check failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(results, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

// handleCheckABAP type-checks a snippet without running it: SAP's syntax
// check of the snippet wrapped as execute_abap wraps it. It creates and
// deletes a temporary program in $TMP, so --read-only refuses it.
func (s *Server) handleCheckABAP(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	code, _ := request.GetArguments()["code"].(string)
	if code == "" {
		return newToolResultError("code is required"), nil
	}

	result, err := s.adtClient.CheckABAP(ctx, code)
	if err != nil {
		return newToolResultError(fmt.Sprintf("check_abap failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	if !result.CleanedUp {
		// The check itself is in the payload, but a program left in $TMP is
		// a failure of the call: it is named up front so it gets deleted.
		return newToolResultError(fmt.Sprintf("check_abap left the temporary program %s in $TMP: %s\n\n%s",
			result.ProgramName, strings.Join(result.Warnings, "; "), output)), nil
	}
	return mcp.NewToolResultText(string(output)), nil
}

// handleActivate and the other activations are long calls: a cold activation
// of a large object runs past the client's 60s per-request timeout, and the
// call's budget (params.timeout or --call-timeout) is what bounds it instead.
func (s *Server) handleActivate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.longCall(ctx, request, "Activate", s.activate)
}

// activate is handleActivate without the call budget (see longCall).
func (s *Server) activate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.GetArguments()["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	objectName, ok := request.GetArguments()["object_name"].(string)
	if !ok || objectName == "" {
		return newToolResultError("object_name is required"), nil
	}

	result, err := s.adtClient.Activate(ctx, objectURL, objectName)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Activation failed: %v", err)), nil
	}
	if err := adt.ActivationResultError(result); err != nil {
		// Same reasoning as the WriteSource boundary: the activation messages
		// are the diagnosis, and a verdict without them is not actionable.
		payload, _ := json.MarshalIndent(result, "", "  ")
		return newToolResultError(fmt.Sprintf("%v\n\n%s", err, payload)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

// handleActivateMultiple activates multiple objects in a single ADT request.
// params.objects: array of {"url": "...", "name": "..."} pairs, or
//
//	array of strings in "TYPE NAME" format (e.g. "INCL ZREP_F01").
func (s *Server) handleActivateMultiple(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.longCall(ctx, request, "ActivateMultiple", s.activateMultiple)
}

// activateMultiple is handleActivateMultiple without the call budget (see longCall).
func (s *Server) activateMultiple(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	raw, ok := request.GetArguments()["objects"]
	if !ok {
		return newToolResultError("objects is required (array of {url, name} or [\"TYPE NAME\", ...])"), nil
	}

	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return newToolResultError("objects must be a non-empty array"), nil
	}

	refs := make([]adt.ObjectRef, 0, len(items))
	for i, item := range items {
		switch v := item.(type) {
		case map[string]any:
			u, _ := v["url"].(string)
			n, _ := v["name"].(string)
			if u == "" || n == "" {
				return newToolResultError(fmt.Sprintf("objects[%d]: both 'url' and 'name' are required", i)), nil
			}
			refs = append(refs, adt.ObjectRef{URI: u, Name: n})
		case string:
			// "TYPE NAME" shorthand → resolve to ADT URL
			u, n, err := s.adtClient.ResolveObjectRef(v)
			if err != nil {
				return newToolResultError(fmt.Sprintf("objects[%d]: cannot resolve %q: %v", i, v, err)), nil
			}
			refs = append(refs, adt.ObjectRef{URI: u, Name: n})
		default:
			return newToolResultError(fmt.Sprintf("objects[%d]: unsupported type %T", i, item)), nil
		}
	}

	result, err := s.adtClient.ActivateMultiple(ctx, refs)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Activation failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleActivatePackage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.longCall(ctx, request, "ActivatePackage", s.activatePackage)
}

// activatePackage is handleActivatePackage without the call budget (see longCall).
func (s *Server) activatePackage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	packageName := ""
	if pkg, ok := request.GetArguments()["package"].(string); ok {
		packageName = pkg
	}

	maxObjects := 100
	if max, ok := request.GetArguments()["max_objects"].(float64); ok && max > 0 {
		maxObjects = int(max)
	}

	result, err := s.adtClient.ActivatePackage(ctx, packageName, maxObjects)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Batch activation failed: %v", err)), nil
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(output)), nil
}

func (s *Server) handleRunUnitTests(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.longCall(ctx, request, "ABAP Unit run", s.runUnitTests)
}

// runUnitTests is handleRunUnitTests without the call budget (see longCall).
func (s *Server) runUnitTests(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.GetArguments()["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	// Build flags from optional parameters
	flags := adt.DefaultUnitTestFlags()

	if includeDangerous, ok := request.GetArguments()["include_dangerous"].(bool); ok && includeDangerous {
		flags.Dangerous = true
	}

	if includeLong, ok := request.GetArguments()["include_long"].(bool); ok && includeLong {
		flags.Long = true
	}

	onlyFailures, _ := request.GetArguments()["only_failures"].(bool)

	result, err := s.adtClient.RunUnitTests(ctx, objectURL, &flags)
	if err != nil {
		return newToolResultError(fmt.Sprintf("Unit test run failed: %v", err)), nil
	}

	// ok and counts on top of the classes this tool always answered; with
	// only_failures, the failed methods alone, in the lean shape.
	output, _ := adt.IndentJSON(adt.NewUnitTestReport(result, onlyFailures))
	return mcp.NewToolResultText(string(output)), nil
}
