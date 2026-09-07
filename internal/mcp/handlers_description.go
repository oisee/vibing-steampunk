package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleSetDescription changes an object's short text — the title SE38
// shows, TRDIRT for a program — without touching its source:
// SAP(action="edit", target="PROG ZDEMO", params={"type": "set_description", "description": "..."}).
func (s *Server) handleSetDescription(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	objectType, name := descriptionTargetFrom(args)
	if name == "" {
		return newToolResultError("name is required (or a target such as \"PROG ZDEMO\")"), nil
	}
	description := getStringParam(args, "description")
	if description == "" {
		res, err := s.adtClient.GetDescription(ctx, objectType, name, getStringParam(args, "parent"))
		if err != nil {
			return newToolResultError(err.Error()), nil
		}
		return newToolResultJSON(res), nil
	}
	res, err := s.adtClient.SetDescription(ctx, objectType, name, getStringParam(args, "parent"), description, getStringParam(args, "transport"))
	if err != nil {
		if res != nil {
			return newToolResultJSON(map[string]any{"error": err.Error(), "result": res}), nil
		}
		return newToolResultError(fmt.Sprintf("SetDescription failed: %v", err)), nil
	}
	return newToolResultJSON(res), nil
}

func descriptionTargetFrom(args map[string]any) (string, string) {
	if p := getStringParam(args, "program_name"); p != "" {
		return "PROG", p
	}
	if c := getStringParam(args, "class_name"); c != "" {
		return "CLAS", c
	}
	name := getStringParam(args, "object_name")
	if name == "" {
		name = getStringParam(args, "name")
	}
	return getStringParam(args, "object_type"), name
}

// withDescription is the tail of a create, write or deploy that was given a
// description: the metadata is written after the source, and the outcome
// rides along in the result.
func (s *Server) withDescription(ctx context.Context, result *mcp.CallToolResult, objectType, name, parent, description, transport string) *mcp.CallToolResult {
	if description == "" || result == nil || result.IsError || len(result.Content) == 0 {
		return result
	}
	res, err := s.adtClient.SetDescription(ctx, objectType, name, parent, description, transport)
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return result
	}
	var m map[string]any
	if json.Unmarshal([]byte(tc.Text), &m) != nil || m == nil {
		m = map[string]any{"result": tc.Text}
	}
	m["description"] = res
	if err != nil {
		m["descriptionError"] = err.Error()
	}
	out, _ := json.MarshalIndent(m, "", "  ")
	return mcp.NewToolResultText(string(out))
}
