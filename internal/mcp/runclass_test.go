package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestRunClassTool_RequiresClassName(t *testing.T) {
	s := NewServer(&Config{BaseURL: "https://example.invalid", Username: "tester", Password: "unused", Client: "000", Language: "EN", Mode: "expert"})
	req := mcp.CallToolRequest{}
	req.Params.Name = "RunClass"
	req.Params.Arguments = map[string]any{"class_name": "  "}

	res, err := s.handleRunClass(context.Background(), req)
	if err != nil {
		t.Fatalf("handleRunClass: %v", err)
	}
	if !res.IsError {
		t.Fatal("an empty class_name must be a tool error")
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "class_name is required") {
		t.Errorf("error text = %q", text)
	}
}
