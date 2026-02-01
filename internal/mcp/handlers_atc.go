// Package mcp provides the MCP server implementation for ABAP ADT tools.
// handlers_atc.go contains handlers for ATC (ABAP Test Cockpit) operations.
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// --- ATC Routing ---
// Routes for this module:
//   system: type=atc_customizing
//   test: type=atc, target=CLAS ZCL_TEST (or object_url in params)

// routeATCAction routes ATC actions.
// Returns (result, true) if handled, (nil, false) if not handled by this module.
func (s *Server) routeATCAction(ctx context.Context, action, objectType, objectName string, params map[string]any) (*mcp.CallToolResult, bool, error) {
	switch action {
	case "system":
		systemType, _ := params["type"].(string)
		if systemType == "atc_customizing" {
			result, err := s.handleGetATCCustomizing(ctx, newRequest(nil))
			return result, true, err
		}

	case "test":
		testType, _ := params["type"].(string)
		if testType == "atc" {
			// Try object_url from params first, then build from target
			objectURL, _ := params["object_url"].(string)
			if objectURL == "" {
				objectURL = buildObjectURL(objectType, objectName)
			}
			if objectURL == "" {
				return newToolResultError("target (e.g., 'CLAS ZCL_TEST') or object_url in params is required for ATC check"), true, nil
			}
			args := map[string]any{"object_url": objectURL}
			if variant, ok := params["variant"].(string); ok {
				args["variant"] = variant
			}
			if maxResults, ok := params["max_results"].(float64); ok {
				args["max_results"] = maxResults
			}
			result, err := s.handleRunATCCheck(ctx, newRequest(args))
			return result, true, err
		}
	}

	return nil, false, nil
}

// buildObjectURL constructs ADT object URL from type and name.
// Returns empty string if type is not supported.
func buildObjectURL(objectType, objectName string) string {
	if objectType == "" || objectName == "" {
		return ""
	}
	name := strings.ToLower(objectName)
	switch objectType {
	case "CLAS":
		return fmt.Sprintf("/sap/bc/adt/oo/classes/%s", name)
	case "PROG":
		return fmt.Sprintf("/sap/bc/adt/programs/programs/%s", name)
	case "INTF":
		return fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", name)
	case "FUGR":
		return fmt.Sprintf("/sap/bc/adt/functions/groups/%s", name)
	case "DDLS":
		return fmt.Sprintf("/sap/bc/adt/ddic/ddl/sources/%s", name)
	case "BDEF":
		return fmt.Sprintf("/sap/bc/adt/bo/behaviordefinitions/%s", name)
	case "SRVD":
		return fmt.Sprintf("/sap/bc/adt/ddic/srvd/sources/%s", name)
	default:
		return ""
	}
}

// --- ATC Handlers ---

func (s *Server) handleRunATCCheck(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objectURL, ok := request.Params.Arguments["object_url"].(string)
	if !ok || objectURL == "" {
		return newToolResultError("object_url is required"), nil
	}

	variant := ""
	if v, ok := request.Params.Arguments["variant"].(string); ok {
		variant = v
	}

	maxResults := 100
	if mr, ok := request.Params.Arguments["max_results"].(float64); ok && mr > 0 {
		maxResults = int(mr)
	}

	result, err := s.adtClient.RunATCCheck(ctx, objectURL, variant, maxResults)
	if err != nil {
		return wrapErr("RunATCCheck", err), nil
	}

	// Format output with summary
	type summary struct {
		TotalObjects  int `json:"totalObjects"`
		TotalFindings int `json:"totalFindings"`
		Errors        int `json:"errors"`
		Warnings      int `json:"warnings"`
		Infos         int `json:"infos"`
	}
	type output struct {
		Summary  summary          `json:"summary"`
		Worklist *adt.ATCWorklist `json:"worklist"`
	}

	sum := summary{TotalObjects: len(result.Objects)}
	for _, obj := range result.Objects {
		sum.TotalFindings += len(obj.Findings)
		for _, f := range obj.Findings {
			switch f.Priority {
			case 1:
				sum.Errors++
			case 2:
				sum.Warnings++
			default:
				sum.Infos++
			}
		}
	}

	out := output{Summary: sum, Worklist: result}
	return newToolResultJSON(out), nil
}

func (s *Server) handleGetATCCustomizing(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := s.adtClient.GetATCCustomizing(ctx)
	if err != nil {
		return wrapErr("GetATCCustomizing", err), nil
	}

	return newToolResultJSON(result), nil
}
